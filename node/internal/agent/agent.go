package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	pb "x-prozy/proto/nodecontrol/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Config — конфигурация node agent.
type Config struct {
	PanelAddr       string // host:port gRPC-сервера панели
	ClusterToken    string // CLUSTER_TOKEN
	ReconnectSecret string // сохранённый reconnect_secret (при рестарте)
	NodeName        string // явное имя ноды (если пусто — os.Hostname)
	NodeIP          string // публичный IP ноды (если пусто — автодетект через ifconfig.me)
	SecretFile      string // путь к файлу для persist reconnect_secret
}

// StatusCollector — функция, которая возвращает текущие метрики.
type StatusCollector func() *pb.StatusReport

// Agent — node agent, поддерживает подключение к панели.
type Agent struct {
	cfg    Config
	log    *slog.Logger
	status StatusCollector

	// Полученные при handshake данные.
	mu              sync.RWMutex
	nodeID          string
	reconnectSecret string

	heartbeatInterval time.Duration
	statusInterval    time.Duration
}

// New создаёт новый agent.
func New(cfg Config, log *slog.Logger, status StatusCollector) *Agent {
	// Загружаем reconnect_secret из файла, если не задан через ENV.
	if cfg.ReconnectSecret == "" && cfg.SecretFile != "" {
		if data, err := os.ReadFile(cfg.SecretFile); err == nil {
			cfg.ReconnectSecret = string(data)
			log.Info("loaded reconnect secret from file", "path", cfg.SecretFile)
		}
	}

	return &Agent{
		cfg:               cfg,
		log:               log,
		status:            status,
		heartbeatInterval: 10 * time.Second,
		statusInterval:    30 * time.Second,
	}
}

// Run запускает agent с reconnect loop. Блокирует до отмены контекста.
func (a *Agent) Run(ctx context.Context) error {
	for {
		a.log.Info("connecting to panel", "addr", a.cfg.PanelAddr)

		err := a.connectAndServe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		a.log.Warn("disconnected from panel", "error", err)

		// Backoff перед реконнектом.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	conn, err := grpc.NewClient(
		a.cfg.PanelAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	client := pb.NewNodeControlClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// Отправляем Handshake.
	hostname, _ := os.Hostname()
	if a.cfg.NodeName != "" {
		hostname = a.cfg.NodeName
	}
	publicIP := a.cfg.NodeIP
	if publicIP == "" {
		publicIP = detectPublicIP()
		if publicIP != "" {
			a.log.Info("detected public IP", "ip", publicIP)
		} else {
			a.log.Warn("could not detect public IP — set NODE_IP env")
		}
	}

	hs := &pb.Handshake{
		ClusterToken: a.cfg.ClusterToken,
		Hostname:     hostname,
		Os:           readOSName(),
		Arch:         runtime.GOARCH,
		Version:      "0.1.0",
		PublicIp:     publicIP,
	}

	a.mu.RLock()
	secret := a.reconnectSecret
	a.mu.RUnlock()
	if secret == "" {
		secret = a.cfg.ReconnectSecret
	}
	hs.ReconnectSecret = secret

	if err := stream.Send(&pb.NodeMessage{
		Payload: &pb.NodeMessage_Handshake{Handshake: hs},
	}); err != nil {
		return fmt.Errorf("send handshake: %w", err)
	}

	// Ждём HandshakeAck.
	firstMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv handshake ack: %w", err)
	}

	ack := firstMsg.GetHandshakeAck()
	if ack == nil {
		return fmt.Errorf("expected HandshakeAck, got something else")
	}
	if !ack.Ok {
		return fmt.Errorf("handshake rejected: %s", ack.Error)
	}

	a.mu.Lock()
	a.nodeID = ack.NodeId
	a.reconnectSecret = ack.ReconnectSecret
	if ack.HeartbeatIntervalSec > 0 {
		a.heartbeatInterval = time.Duration(ack.HeartbeatIntervalSec) * time.Second
	}
	if ack.StatusIntervalSec > 0 {
		a.statusInterval = time.Duration(ack.StatusIntervalSec) * time.Second
	}
	a.mu.Unlock()

	// Сохраняем reconnect_secret в файл для переживания рестартов.
	a.persistSecret(ack.ReconnectSecret)

	a.log.Info("connected to panel",
		"node_id", ack.NodeId,
		"heartbeat_interval", a.heartbeatInterval,
		"status_interval", a.statusInterval,
	)

	// Запускаем горутины: heartbeat, status, recv.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 3)

	// Heartbeat sender.
	go func() {
		errCh <- a.heartbeatLoop(streamCtx, stream)
	}()

	// Status sender.
	go func() {
		errCh <- a.statusLoop(streamCtx, stream)
	}()

	// Receiver (panel → node).
	go func() {
		errCh <- a.recvLoop(streamCtx, stream)
	}()

	// Ждём первую ошибку — отключаемся.
	select {
	case err := <-errCh:
		cancel()
		return err
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

func (a *Agent) heartbeatLoop(ctx context.Context, stream pb.NodeControl_ConnectClient) error {
	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := stream.Send(&pb.NodeMessage{
				Payload: &pb.NodeMessage_Heartbeat{
					Heartbeat: &pb.Heartbeat{
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}); err != nil {
				return fmt.Errorf("heartbeat send: %w", err)
			}
		}
	}
}

func (a *Agent) statusLoop(ctx context.Context, stream pb.NodeControl_ConnectClient) error {
	ticker := time.NewTicker(a.statusInterval)
	defer ticker.Stop()

	// Первый report сразу.
	if err := a.sendStatus(stream); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.sendStatus(stream); err != nil {
				return err
			}
		}
	}
}

func (a *Agent) sendStatus(stream pb.NodeControl_ConnectClient) error {
	if a.status == nil {
		return nil
	}
	report := a.status()
	if report == nil {
		return nil
	}
	return stream.Send(&pb.NodeMessage{
		Payload: &pb.NodeMessage_StatusReport{StatusReport: report},
	})
}

func (a *Agent) recvLoop(ctx context.Context, stream pb.NodeControl_ConnectClient) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("panel closed stream")
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch p := msg.Payload.(type) {
		case *pb.PanelMessage_Ping:
			_ = p // Пока просто логируем.
			a.log.Debug("received ping from panel")

		case *pb.PanelMessage_ConfigPush:
			a.log.Info("received config push", "version", p.ConfigPush.Version)
			// TODO: применить xray config

		case *pb.PanelMessage_Disconnect:
			a.log.Warn("panel requested disconnect", "reason", p.Disconnect.Reason)
			return fmt.Errorf("disconnected by panel: %s", p.Disconnect.Reason)

		default:
			a.log.Debug("unknown message from panel")
		}
	}
}

// NodeID возвращает текущий ID ноды (после handshake).
func (a *Agent) NodeID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.nodeID
}

// ReconnectSecret возвращает текущий reconnect secret.
func (a *Agent) ReconnectSecret() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.reconnectSecret
}

// persistSecret сохраняет reconnect_secret в файл (если путь задан).
func (a *Agent) persistSecret(secret string) {
	if a.cfg.SecretFile == "" {
		return
	}
	if err := os.WriteFile(a.cfg.SecretFile, []byte(secret), 0600); err != nil {
		a.log.Warn("failed to persist reconnect secret", "path", a.cfg.SecretFile, "error", err)
	} else {
		a.log.Debug("reconnect secret persisted", "path", a.cfg.SecretFile)
	}
}

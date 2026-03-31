package node

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	pb "x-prozy/proto/nodecontrol/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
)

const (
	defaultHeartbeatInterval = 10               // seconds
	defaultStatusInterval    = 30               // seconds
	heartbeatTimeout         = 45 * time.Second // если нет heartbeat за это время — offline
)

// GRPCServer — gRPC-сервер для обработки подключений нод.
type GRPCServer struct {
	pb.UnimplementedNodeControlServer

	svc          *Service
	clusterToken string
	log          *slog.Logger

	// Активные стримы: node_id -> sendFunc
	mu      sync.RWMutex
	streams map[string]*connectedNode
}

// connectedNode — метаданные активного подключения.
type connectedNode struct {
	nodeID string
	send   func(*pb.PanelMessage) error
	cancel func() // закрыть стрим
}

// NewGRPCServer создаёт gRPC сервер для нод.
func NewGRPCServer(svc *Service, clusterToken string, log *slog.Logger) *GRPCServer {
	return &GRPCServer{
		svc:          svc,
		clusterToken: clusterToken,
		log:          log.With("component", "grpc"),
		streams:      make(map[string]*connectedNode),
	}
}

// ListenAndServe запускает gRPC-сервер на указанном адресе.
// Если tlsConf != nil — используется TLS (auto-generated certs).
func (s *GRPCServer) ListenAndServe(addr string, tlsConf *tls.Config) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	if tlsConf != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConf)))
		s.log.Info("gRPC server listening", "addr", addr, "tls", true)
	} else {
		s.log.Warn("gRPC server listening WITHOUT TLS", "addr", addr)
	}

	srv := grpc.NewServer(opts...)
	pb.RegisterNodeControlServer(srv, s)

	return srv.Serve(lis)
}

// verifyAuth проверяет авторизацию ноды по HMAC-SHA256.
// Fallback: plaintext cluster_token (deprecated, для обратной совместимости).
func (s *GRPCServer) verifyAuth(hs *pb.Handshake) bool {
	if len(hs.AuthHmac) > 0 && len(hs.AuthNonce) > 0 {
		mac := hmac.New(sha256.New, []byte(s.clusterToken))
		mac.Write(hs.AuthNonce)
		return hmac.Equal(hs.AuthHmac, mac.Sum(nil))
	}
	// Legacy: plaintext token.
	if hs.ClusterToken != "" {
		s.log.Warn("node uses plaintext cluster_token (deprecated), upgrade node to HMAC")
		return hs.ClusterToken == s.clusterToken
	}
	return false
}

// Connect — bidi stream RPC. Нода шлёт Handshake, потом heartbeat/status.
// Панель шлёт HandshakeAck, ping, config_push.
func (s *GRPCServer) Connect(stream pb.NodeControl_ConnectServer) error {
	ctx := stream.Context()

	// Получаем remote addr для логов.
	remoteAddr := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		remoteAddr = p.Addr.String()
	}

	// Ждём первое сообщение: Handshake.
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	hs := firstMsg.GetHandshake()
	if hs == nil {
		_ = stream.Send(&pb.PanelMessage{
			Payload: &pb.PanelMessage_HandshakeAck{
				HandshakeAck: &pb.HandshakeAck{Ok: false, Error: "first message must be Handshake"},
			},
		})
		return nil
	}

	// Авторизация: HMAC-SHA256 (preferred) или plaintext cluster_token (legacy).
	if !s.verifyAuth(hs) {
		s.log.Warn("handshake: auth failed", "remote_addr", remoteAddr)
		_ = stream.Send(&pb.PanelMessage{
			Payload: &pb.PanelMessage_HandshakeAck{
				HandshakeAck: &pb.HandshakeAck{Ok: false, Error: "auth failed"},
			},
		})
		return nil
	}

	var nodeID, reconnectSecret string

	if hs.ReconnectSecret != "" {
		// Реконнект: нода уже была зарегистрирована.
		nodeID, reconnectSecret, err = s.svc.Reconnect(
			hs.ReconnectSecret,
			hs.Hostname, hs.Os, hs.Arch, hs.Version,
			hs.PublicIp, remoteAddr,
		)
	} else {
		// Новая нода.
		nodeID, reconnectSecret, err = s.svc.Register(
			hs.Hostname, hs.Os, hs.Arch, hs.Version,
			hs.PublicIp, remoteAddr,
		)
	}

	if err != nil {
		s.log.Error("handshake failed", "error", err, "remote_addr", remoteAddr)
		_ = stream.Send(&pb.PanelMessage{
			Payload: &pb.PanelMessage_HandshakeAck{
				HandshakeAck: &pb.HandshakeAck{Ok: false, Error: err.Error()},
			},
		})
		return nil
	}

	// Отправляем HandshakeAck.
	if err := stream.Send(&pb.PanelMessage{
		Payload: &pb.PanelMessage_HandshakeAck{
			HandshakeAck: &pb.HandshakeAck{
				Ok:                   true,
				NodeId:               nodeID,
				ReconnectSecret:      reconnectSecret,
				HeartbeatIntervalSec: defaultHeartbeatInterval,
				StatusIntervalSec:    defaultStatusInterval,
			},
		},
	}); err != nil {
		return err
	}

	s.log.Info("node connected", "node_id", nodeID, "hostname", hs.Hostname, "remote_addr", remoteAddr)

	// Регистрируем стрим.
	cn := &connectedNode{
		nodeID: nodeID,
		send: func(msg *pb.PanelMessage) error {
			return stream.Send(msg)
		},
	}
	s.mu.Lock()
	// Если нода уже подключена (старый стрим), перезаписываем.
	s.streams[nodeID] = cn
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		// Удаляем только если это наш стрим (мог быть заменён новым).
		if cur, ok := s.streams[nodeID]; ok && cur == cn {
			delete(s.streams, nodeID)
		}
		s.mu.Unlock()
		s.svc.MarkOffline(nodeID)
	}()

	// Читаем сообщения от ноды.
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				s.log.Info("node disconnected (EOF)", "node_id", nodeID)
			} else {
				s.log.Warn("node stream error", "node_id", nodeID, "error", err)
			}
			return nil
		}

		switch p := msg.Payload.(type) {
		case *pb.NodeMessage_Heartbeat:
			_ = p // timestamp для логов если надо
			s.svc.MarkOnline(nodeID)

		case *pb.NodeMessage_StatusReport:
			sr := p.StatusReport
			s.svc.MarkOnline(nodeID)
			s.svc.UpdateSnapshot(&NodeSnapshot{
				NodeID:      nodeID,
				CPUPercent:  sr.CpuPercent,
				CPUCores:    sr.CpuCores,
				CPUModel:    sr.CpuModel,
				MemTotal:    sr.MemTotal,
				MemUsed:     sr.MemUsed,
				MemPercent:  sr.MemPercent,
				SwapTotal:   sr.SwapTotal,
				SwapUsed:    sr.SwapUsed,
				SwapPercent: sr.SwapPercent,
				DiskTotal:   sr.DiskTotal,
				DiskUsed:    sr.DiskUsed,
				DiskPercent: sr.DiskPercent,
				NetUp:       sr.NetUp,
				NetDown:     sr.NetDown,
				Load1:       sr.Load1,
				Load5:       sr.Load5,
				Load15:      sr.Load15,
				TCPCount:    sr.TcpCount,
				UDPCount:    sr.UdpCount,
				Uptime:      sr.Uptime,
				Timestamp:   sr.Timestamp,
				// Xray metrics
				XrayRunning:     sr.XrayRunning,
				XrayUptime:      sr.XrayUptime,
				XrayGoroutines:  sr.XrayGoroutines,
				XrayMemAlloc:    sr.XrayMemAlloc,
				XrayTrafficUp:   sr.XrayTrafficUp,
				XrayTrafficDown: sr.XrayTrafficDown,
			})

		default:
			s.log.Debug("unknown message from node", "node_id", nodeID)
		}
	}
}

// SendToNode отправляет сообщение конкретной ноде (если подключена).
func (s *GRPCServer) SendToNode(nodeID string, msg *pb.PanelMessage) error {
	s.mu.RLock()
	cn, ok := s.streams[nodeID]
	s.mu.RUnlock()

	if !ok {
		return ErrNodeNotConnected
	}
	return cn.send(msg)
}

// DisconnectNode отправляет Disconnect и удаляет стрим.
func (s *GRPCServer) DisconnectNode(nodeID, reason string) error {
	return s.SendToNode(nodeID, &pb.PanelMessage{
		Payload: &pb.PanelMessage_Disconnect{
			Disconnect: &pb.Disconnect{Reason: reason},
		},
	})
}

// ConnectedCount возвращает количество подключённых нод.
func (s *GRPCServer) ConnectedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.streams)
}

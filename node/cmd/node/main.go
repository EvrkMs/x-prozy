package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"x-prozy/node/internal/agent"
	"x-prozy/node/internal/status"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	panelAddr := envOrDefault("PANEL_GRPC_ADDR", "127.0.0.1:9090")
	clusterToken := os.Getenv("CLUSTER_TOKEN")
	reconnectSecret := os.Getenv("RECONNECT_SECRET")                        // опционально, при рестарте
	nodeName := os.Getenv("NODE_NAME")                                      // явное имя ноды (если пусто — os.Hostname)
	nodeIP := os.Getenv("NODE_IP")                                          // публичный IP (если пусто — автодетект)
	secretFile := envOrDefault("SECRET_FILE", "/app/data/reconnect.secret") // persist secret
	caFingerprint := os.Getenv("CA_FINGERPRINT")                            // hex SHA256 CA панели

	if clusterToken == "" {
		log.Error("CLUSTER_TOKEN is required")
		os.Exit(1)
	}

	// Запускаем фоновый сборщик метрик.
	getStatus := status.StartCollector(5 * time.Second)

	cfg := agent.Config{
		PanelAddr:       panelAddr,
		ClusterToken:    clusterToken,
		ReconnectSecret: reconnectSecret,
		NodeName:        nodeName,
		NodeIP:          nodeIP,
		SecretFile:      secretFile,
		CAFingerprint:   caFingerprint,
	}

	a := agent.New(cfg, log, getStatus)

	// Graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	log.Info("node agent starting", "panel_addr", panelAddr)
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		log.Error("agent stopped", "error", err)
		os.Exit(1)
	}
	log.Info("node agent stopped")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

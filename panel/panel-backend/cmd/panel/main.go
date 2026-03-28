package main

import (
	"net/http"
	"os"

	"x-prozy/panel-backend/internal/config"
	"x-prozy/panel-backend/internal/logger"
	"x-prozy/panel-backend/internal/web"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.Log.Format, cfg.Log.Level)

	app, err := web.NewApp(cfg, log)
	if err != nil {
		log.Error("failed to create app", "error", err)
		os.Exit(1)
	}

	// Запуск gRPC-сервера для нод (в фоне).
	app.StartGRPC()

	addr := app.ListenAddr()
	log.Info("panel starting", "addr", addr)
	if err := http.ListenAndServe(addr, app.Routes()); err != nil {
		log.Error("listen failed", "error", err)
		os.Exit(1)
	}
}

package config

import (
	"os"
)

// Config содержит только инфраструктурные параметры (ENV).
// Настройки сессии, порта и пути хранятся в БД (internal/settings).
type Config struct {
	Database DatabaseConfig
	Server   ServerConfig
	GRPC     GRPCConfig
	Log      LogConfig
}

type DatabaseConfig struct {
	DSN string
}

// ServerConfig содержит адрес и порт привязки (из ENV).
type ServerConfig struct {
	Addr string
	Port string
}

// GRPCConfig — параметры gRPC-сервера для подключения нод.
type GRPCConfig struct {
	Addr         string
	Port         string
	ClusterToken string
}

type LogConfig struct {
	Format string // "json" | "text"
	Level  string // "debug" | "info" | "warn" | "error"
}

// Load загружает конфигурацию из переменных окружения.
func Load() *Config {
	return &Config{
		Database: DatabaseConfig{
			DSN: envOrDefault("DB_PATH", "./data/panel.db"),
		},
		Server: ServerConfig{
			Addr: envOrDefault("PANEL_ADDR", "0.0.0.0"),
			Port: envOrDefault("PANEL_PORT", "8080"),
		},
		GRPC: GRPCConfig{
			Addr:         envOrDefault("GRPC_ADDR", "0.0.0.0"),
			Port:         envOrDefault("GRPC_PORT", "9090"),
			ClusterToken: envOrDefault("CLUSTER_TOKEN", ""),
		},
		Log: LogConfig{
			Format: envOrDefault("LOG_FORMAT", "text"),
			Level:  envOrDefault("LOG_LEVEL", "info"),
		},
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

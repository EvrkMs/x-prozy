package config

import (
	"os"
	"strconv"
)

// Config содержит только инфраструктурные параметры (ENV).
// Настройки сессии, порта и пути хранятся в БД (internal/settings).
type Config struct {
	Database DatabaseConfig
	Server   ServerConfig
	GRPC     GRPCConfig
	Log      LogConfig
	Metrics  MetricsConfig
}

// MetricsConfig — параметры встроенного хранилища метрик.
type MetricsConfig struct {
	RetentionHours int // сколько часов хранить историю (default: 24)
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
		Metrics: MetricsConfig{
			RetentionHours: envOrDefaultInt("METRICS_RETENTION_HOURS", 24),
		},
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return fallback
}

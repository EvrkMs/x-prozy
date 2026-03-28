package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// New создаёт структурированный slog.Logger.
// format: "json" | "text" (default: "text")
// level: "debug" | "info" | "warn" | "error" (default: "info")
func New(format, level string) *slog.Logger {
	lvl := parseLevel(level)

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// WithRequestID добавляет request_id в контекст.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID достаёт request_id из контекста.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// FromContext возвращает логгер с request_id если он есть в контексте.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		return base.With("request_id", id)
	}
	return base
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

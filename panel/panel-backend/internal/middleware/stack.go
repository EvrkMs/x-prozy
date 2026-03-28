package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"x-prozy/panel-backend/internal/logger"
)

// Chain применяет middleware в порядке вызова (первый — самый внешний).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestID генерирует уникальный ID для каждого запроса и кладёт в контекст.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := logger.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SkipPaths — пути, которые не логируются (healthcheck и т.д.).
var SkipPaths = map[string]bool{
	"/healthz": true,
}

// Logger логирует каждый HTTP запрос, кроме SkipPaths.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			if SkipPaths[r.URL.Path] {
				return
			}

			duration := time.Since(start)
			l := logger.FromContext(r.Context(), log)

			l.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration", duration.String(),
				"bytes", sw.bytes,
			)
		})
	}
}

// Recover перехватывает паники и возвращает 500.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := string(debug.Stack())
					l := logger.FromContext(r.Context(), log)
					l.Error("panic recovered",
						"error", err,
						"stack", stack,
						"method", r.Method,
						"path", r.URL.Path,
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// --- statusWriter для перехвата HTTP статуса и размера ответа -----

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(b)
	sw.bytes += n
	return n, err
}

func generateID() string {
	buf := make([]byte, 8)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

package middleware

import (
	"context"
	"net/http"
	"strings"

	"x-prozy/panel-backend/internal/auth"
)

type contextKey string

const userContextKey contextKey = "user"

// RequireAuth проверяет сессию через интерфейс auth.SessionValidator.
// Middleware знает только про интерфейс — не зависит от деталей реализации.
func RequireAuth(validator auth.SessionValidator, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				redirectOrReject(w, r)
				return
			}

			user, err := validator.ValidateToken(r.Context(), cookie.Value)
			if err != nil {
				redirectOrReject(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext достаёт пользователя из контекста.
func UserFromContext(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userContextKey).(*auth.User)
	return u
}

func redirectOrReject(w http.ResponseWriter, r *http.Request) {
	// API + WebSocket → 401 JSON, остальное → redirect на логин.
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}


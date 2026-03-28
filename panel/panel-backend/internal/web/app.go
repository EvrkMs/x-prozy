package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"x-prozy/panel-backend/internal/auth"
	"x-prozy/panel-backend/internal/certs"
	"x-prozy/panel-backend/internal/config"
	"x-prozy/panel-backend/internal/middleware"
	"x-prozy/panel-backend/internal/node"
	"x-prozy/panel-backend/internal/render"
	"x-prozy/panel-backend/internal/settings"
)

// App — ядро веб-приложения.
type App struct {
	render   *render.Engine
	auth     *auth.Service
	settings *settings.Service
	nodes    *node.Service
	grpc     *node.GRPCServer
	certs    *certs.Manager
	cfg      *config.Config
	log      *slog.Logger
}

const sessionCookieName = "prozy_session"

// NewApp инициализирует приложение: auth, settings, template engine.
func NewApp(cfg *config.Config, log *slog.Logger) (*App, error) {
	// Auth
	repo, err := auth.NewRepository(cfg.Database.DSN)
	if err != nil {
		return nil, err
	}

	// Settings (использует тот же *gorm.DB)
	settingsRepo, err := settings.NewRepository(repo.DB())
	if err != nil {
		return nil, err
	}
	settingsSvc := settings.NewService(settingsRepo)

	// Auth service: session duration берём из DB
	authService, err := auth.NewService(repo, settingsSvc.SessionDuration())
	if err != nil {
		return nil, err
	}

	// Node service & gRPC server
	nodeRepo, err := node.NewRepository(repo.DB())
	if err != nil {
		return nil, err
	}
	nodeSvc, err := node.NewService(nodeRepo, log)
	if err != nil {
		return nil, err
	}
	grpcServer := node.NewGRPCServer(nodeSvc, cfg.GRPC.ClusterToken, log)

	// TLS certificates for gRPC
	tlsDir := filepath.Join(filepath.Dir(cfg.Database.DSN), "tls")
	certsMgr, err := certs.NewManager(tlsDir, log)
	if err != nil {
		return nil, err
	}

	// Template engine
	engine, err := render.NewEngine(render.EngineConfig{
		Dir: filepath.Join("web", "templates"),
		Dev: render.IsDev(),
		Log: log,
	})
	if err != nil {
		return nil, err
	}

	return &App{
		render:   engine,
		auth:     authService,
		settings: settingsSvc,
		nodes:    nodeSvc,
		grpc:     grpcServer,
		certs:    certsMgr,
		cfg:      cfg,
		log:      log,
	}, nil
}

// Routes собирает все маршруты + middleware стек.
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	// Static files (без секретного пути — ресурсы должны грузиться всегда)
	fs := http.FileServer(http.Dir(filepath.Join("web", "static")))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Health (без секретного пути)
	mux.HandleFunc("GET /healthz", a.handleHealthz)

	// Public
	mux.HandleFunc("GET /login", a.handleLoginPage)
	mux.HandleFunc("POST /login", a.handleLoginSubmit)
	mux.HandleFunc("POST /logout", a.handleLogout)

	// Protected pages
	protected := middleware.RequireAuth(a.auth, sessionCookieName)
	mux.Handle("/", protected(http.HandlerFunc(a.handleDashboard)))

	// Protected API
	mux.Handle("POST /api/settings/session", protected(http.HandlerFunc(a.apiUpdateSession)))
	mux.Handle("POST /api/settings/username", protected(http.HandlerFunc(a.apiUpdateUsername)))
	mux.Handle("POST /api/settings/password", protected(http.HandlerFunc(a.apiUpdatePassword)))
	mux.Handle("POST /api/settings/credentials", protected(http.HandlerFunc(a.apiUpdateCredentials)))
	mux.Handle("POST /api/settings/secret-path", protected(http.HandlerFunc(a.apiUpdateSecretPath)))

	// Node API
	mux.Handle("GET /api/nodes", protected(http.HandlerFunc(a.apiListNodes)))
	mux.Handle("GET /api/nodes/{id}", protected(http.HandlerFunc(a.apiGetNode)))
	mux.Handle("DELETE /api/nodes/{id}", protected(http.HandlerFunc(a.apiDeleteNode)))

	// Cluster info API
	mux.Handle("GET /api/cluster-info", protected(http.HandlerFunc(a.apiClusterInfo)))

	return middleware.Chain(
		a.secretPathHandler(mux),
		middleware.Recover(a.log),
		middleware.RequestID,
		middleware.Logger(a.log),
	)
}

// secretPathHandler — если secret path задан, требует его как URI-префикс.
// /static/ и /healthz всегда доступны без префикса.
func (a *App) secretPathHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := a.settings.SecretPath()
		if secret == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Пути без защиты
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		// Требуем секретный префикс
		if !strings.HasPrefix(r.URL.Path, secret) {
			http.NotFound(w, r)
			return
		}

		// Стрипаем префикс и передаём дальше
		stripped := strings.TrimPrefix(r.URL.Path, secret)
		if stripped == "" {
			stripped = "/"
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = stripped
		if r.URL.RawPath != "" {
			r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, secret)
		}
		next.ServeHTTP(w, r2)
	})
}

// basePath возвращает текущий секретный URI-префикс (или пустую строку).
func (a *App) basePath() string {
	return a.settings.SecretPath()
}

// ListenAddr возвращает адрес для http.ListenAndServe (addr:port), из ENV.
func (a *App) ListenAddr() string {
	return a.cfg.Server.Addr + ":" + a.cfg.Server.Port
}

// isHTTPS определяет, что соединение защищено TLS.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// setSessionCookie устанавливает cookie сессии с актуальными параметрами.
func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode, // всегда Strict: панель закрыта
		MaxAge:   maxAge,
	})
}

// --- handlers ---------------------------------------------------------------

func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (a *App) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if _, err := a.auth.ValidateToken(r.Context(), cookie.Value); err == nil {
			render.Redirect(w, r, a.basePath()+"/")
			return
		}
	}

	a.render.Render(w, "login_page", LoginPageData{
		Title:    "Prozy — Login",
		BasePath: a.basePath(),
	})
}

func (a *App) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid form submission")
		return
	}

	username := strings.TrimSpace(r.FormValue("operator"))
	password := r.FormValue("password")

	token, err := a.auth.Login(r.Context(), username, password)
	if err != nil {
		a.render.RenderWithStatus(w, http.StatusUnauthorized, "login_page", LoginPageData{
			Title:    "Prozy — Login",
			Error:    "Invalid username or password",
			Username: username,
			BasePath: a.basePath(),
		})
		return
	}

	maxAge := int(a.settings.SessionDuration().Seconds())
	a.setSessionCookie(w, r, token, maxAge)
	render.Redirect(w, r, a.basePath()+"/")
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		render.Redirect(w, r, a.basePath()+"/login")
		return
	}

	a.render.Render(w, "dashboard_page", DashboardPageData{
		Title:    "Prozy — Dashboard",
		Username: user.Username,
		Settings: a.settingsData(),
		BasePath: a.basePath(),
	})
}

// settingsData собирает текущие настройки для отображения.
func (a *App) settingsData() SettingsData {
	return SettingsData{
		// DB-backed (редактируемые через панель)
		SessionDuration: a.settings.SessionDuration().String(),
		SecretPath:      a.settings.SecretPath(),
		// ENV-only (только для просмотра)
		PanelAddr: a.cfg.Server.Addr,
		PanelPort: a.cfg.Server.Port,
		DBPath:    a.cfg.Database.DSN,
		LogFormat: a.cfg.Log.Format,
		LogLevel:  a.cfg.Log.Level,
	}
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.auth.Logout(r.Context(), cookie.Value)
	}
	a.setSessionCookie(w, r, "", -1)
	render.Redirect(w, r, a.basePath()+"/login")
}

// --- Settings API -----------------------------------------------------------

func (a *App) apiUpdateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Duration string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	req.Duration = strings.TrimSpace(req.Duration)
	if req.Duration == "" {
		render.JSONError(w, http.StatusBadRequest, "duration is required")
		return
	}

	dur, err := time.ParseDuration(req.Duration)
	if err != nil || dur <= 0 {
		render.JSONError(w, http.StatusBadRequest, "invalid duration (примеры: 24h, 168h, 720h)")
		return
	}

	if err := a.settings.SetSessionDuration(dur); err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось сохранить")
		return
	}
	a.auth.SetSessionDuration(dur)

	a.log.Info("session duration updated", "duration", req.Duration)
	render.JSONOk(w, map[string]string{"message": "Длительность сессии обновлена: " + dur.String()})
}

func (a *App) apiUpdateUsername(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		render.JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		NewUsername string `json:"new_username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	req.NewUsername = strings.TrimSpace(req.NewUsername)
	if len(req.NewUsername) < 3 {
		render.JSONError(w, http.StatusBadRequest, "логин должен быть минимум 3 символа")
		return
	}

	if err := a.auth.ChangeUsername(r.Context(), user.ID, req.NewUsername); err != nil {
		a.log.Error("change username", "error", err)
		render.JSONError(w, http.StatusInternalServerError, "не удалось сменить логин")
		return
	}

	a.log.Info("username changed", "from", user.Username, "to", req.NewUsername)
	render.JSONOk(w, map[string]string{"message": "Логин изменён на " + req.NewUsername})
}

func (a *App) apiUpdatePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		render.JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		OldPassword     string `json:"old_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		render.JSONError(w, http.StatusBadRequest, "пароли не совпадают")
		return
	}
	if len(req.NewPassword) < 4 {
		render.JSONError(w, http.StatusBadRequest, "пароль слишком короткий (мин. 4 символа)")
		return
	}

	if err := a.auth.ChangePassword(r.Context(), user.ID, req.OldPassword, req.NewPassword); err != nil {
		if err == auth.ErrInvalidCredentials {
			render.JSONError(w, http.StatusForbidden, "неверный текущий пароль")
			return
		}
		a.log.Error("change password", "error", err)
		render.JSONError(w, http.StatusInternalServerError, "не удалось сменить пароль")
		return
	}

	a.log.Info("password changed", "user", user.Username)
	render.JSONOk(w, map[string]string{"message": "Пароль изменён. Все сессии сброшены."})
}

// apiUpdateCredentials — объединённая смена логина и/или пароля.
// Всегда требует current_password для верификации (если меняется пароль).
func (a *App) apiUpdateCredentials(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		render.JSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewUsername     string `json:"new_username"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	newUsername := strings.TrimSpace(req.NewUsername)
	newPassword := req.NewPassword
	changingPassword := newPassword != ""
	changingUsername := newUsername != "" && newUsername != user.Username

	if !changingUsername && !changingPassword {
		render.JSONError(w, http.StatusBadRequest, "нет изменений для сохранения")
		return
	}

	// Смена пароля требует верификации текущего пароля
	if changingPassword {
		if req.CurrentPassword == "" {
			render.JSONError(w, http.StatusBadRequest, "введите текущий пароль")
			return
		}
		if newPassword != req.ConfirmPassword {
			render.JSONError(w, http.StatusBadRequest, "пароли не совпадают")
			return
		}
		if len(newPassword) < 4 {
			render.JSONError(w, http.StatusBadRequest, "пароль слишком короткий (мин. 4 символа)")
			return
		}
		if err := a.auth.ChangePassword(r.Context(), user.ID, req.CurrentPassword, newPassword); err != nil {
			if err == auth.ErrInvalidCredentials {
				render.JSONError(w, http.StatusForbidden, "неверный текущий пароль")
				return
			}
			a.log.Error("change password", "error", err)
			render.JSONError(w, http.StatusInternalServerError, "не удалось сменить пароль")
			return
		}
		a.log.Info("password changed", "user", user.Username)
	}

	// Смена логина (можно без текущего пароля — сессия уже авторизована)
	if changingUsername {
		if len(newUsername) < 3 {
			render.JSONError(w, http.StatusBadRequest, "логин должен быть минимум 3 символа")
			return
		}
		if err := a.auth.ChangeUsername(r.Context(), user.ID, newUsername); err != nil {
			a.log.Error("change username", "error", err)
			render.JSONError(w, http.StatusInternalServerError, "не удалось сменить логин")
			return
		}
		a.log.Info("username changed", "from", user.Username, "to", newUsername)
	}

	msg := "Данные обновлены"
	redirectToLogin := changingPassword
	if redirectToLogin {
		msg = "Пароль изменён. Все сессии сброшены. Перенаправление на вход..."
	}

	res := map[string]any{"message": msg}
	if redirectToLogin {
		res["redirect_login"] = true
	}
	render.JSONOk(w, res)
}

func (a *App) apiUpdateSecretPath(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := a.settings.SetSecretPath(req.Path); err != nil {
		render.JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	newPath := a.settings.SecretPath()
	a.log.Info("secret path updated", "path", newPath)
	redirect := newPath + "/"
	render.JSONOk(w, map[string]any{
		"message":  "Секретный путь обновлён. Перенаправление...",
		"redirect": redirect,
	})
}

// --- gRPC -------------------------------------------------------------------

// StartGRPC запускает gRPC-сервер в фоне. Вызывать из main.go.
func (a *App) StartGRPC() {
	addr := a.cfg.GRPC.Addr + ":" + a.cfg.GRPC.Port

	if a.cfg.GRPC.ClusterToken == "" {
		a.log.Warn("CLUSTER_TOKEN not set — nodes will NOT be able to connect")
		return
	}

	tlsConf := a.certs.ServerTLSConfig()

	go func() {
		if err := a.grpc.ListenAndServe(addr, tlsConf); err != nil {
			a.log.Error("gRPC server failed", "error", err)
		}
	}()
}

// apiClusterInfo возвращает CA fingerprint и токен-статус.
func (a *App) apiClusterInfo(w http.ResponseWriter, r *http.Request) {
	render.JSONOk(w, map[string]any{
		"ca_fingerprint":    a.certs.Fingerprint(),
		"cluster_token_set": a.cfg.GRPC.ClusterToken != "",
	})
}

// --- Node API ---------------------------------------------------------------

func (a *App) apiListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.nodes.List()
	if err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось получить список нод")
		return
	}
	render.JSONOk(w, nodes)
}

func (a *App) apiGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		render.JSONError(w, http.StatusBadRequest, "missing node id")
		return
	}

	info, err := a.nodes.Get(id)
	if err != nil {
		render.JSONError(w, http.StatusNotFound, "нода не найдена")
		return
	}
	render.JSONOk(w, info)
}

func (a *App) apiDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		render.JSONError(w, http.StatusBadRequest, "missing node id")
		return
	}

	// Отключаем ноду если подключена.
	_ = a.grpc.DisconnectNode(id, "deleted by admin")

	if err := a.nodes.Delete(id); err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось удалить ноду")
		return
	}

	a.log.Info("node deleted", "node_id", id)
	render.JSONOk(w, map[string]string{"message": "Нода удалена"})
}

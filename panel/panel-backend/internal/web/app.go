package web

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"x-prozy/panel-backend/internal/auth"
	"x-prozy/panel-backend/internal/certs"
	"x-prozy/panel-backend/internal/config"
	"x-prozy/panel-backend/internal/inbound"
	"x-prozy/panel-backend/internal/metrics"
	"x-prozy/panel-backend/internal/middleware"
	"x-prozy/panel-backend/internal/node"
	"x-prozy/panel-backend/internal/render"
	"x-prozy/panel-backend/internal/settings"
	"x-prozy/panel-backend/internal/ws"

	pb "x-prozy/proto/nodecontrol/v1"
)

// App — ядро веб-приложения.
type App struct {
	render    *render.Engine
	auth      *auth.Service
	settings  *settings.Service
	nodes     *node.Service
	inbounds  *inbound.Service
	grpc      *node.GRPCServer
	certs     *certs.Manager
	wsHub     *ws.Hub
	prom      *metrics.Exporter
	store     *metrics.Store // встроенное хранилище метрик (замена Prometheus)
	cfg       *config.Config
	log       *slog.Logger
	cacheBust string
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

	// Inbound service (подключения Xray)
	ibRepo, err := inbound.NewRepository(repo.DB())
	if err != nil {
		return nil, fmt.Errorf("inbound repo: %w", err)
	}
	ibSvc := inbound.NewService(ibRepo, log)

	// WebSocket hub
	wsHub := ws.NewHub(log)

	// Prometheus exporter (для /metrics endpoint — совместимость с внешним мониторингом)
	prom := metrics.NewExporter()

	// Встроенное хранилище метрик (замена внешнего Prometheus)
	// Приоритет: DB-настройка → ENV → default(24).
	retentionHours := settingsSvc.MetricsRetentionHours()
	store, err := metrics.NewStore(repo.DB(), retentionHours, log)
	if err != nil {
		return nil, fmt.Errorf("metrics store: %w", err)
	}
	store.StartCleanupLoop(context.Background())

	// При любом изменении нод — броадкастим + обновляем Prometheus gauge + пишем в store.
	nodeSvc.OnChange = func() {
		list, err := nodeSvc.List()
		if err != nil {
			log.Warn("ws: failed to list nodes for broadcast", "error", err)
			return
		}
		wsHub.Broadcast("nodes", list)

		// Prometheus: обновляем метрики.
		var online, offline int
		for _, info := range list {
			if info.Node.Status == "online" {
				online++
			} else {
				offline++
			}
			if info.Snapshot != nil {
				snap := metrics.Snapshot{
					CPUPercent:  info.Snapshot.CPUPercent,
					CPUCores:    info.Snapshot.CPUCores,
					MemTotal:    info.Snapshot.MemTotal,
					MemUsed:     info.Snapshot.MemUsed,
					MemPercent:  info.Snapshot.MemPercent,
					SwapTotal:   info.Snapshot.SwapTotal,
					SwapUsed:    info.Snapshot.SwapUsed,
					DiskTotal:   info.Snapshot.DiskTotal,
					DiskUsed:    info.Snapshot.DiskUsed,
					DiskPercent: info.Snapshot.DiskPercent,
					NetUp:       info.Snapshot.NetUp,
					NetDown:     info.Snapshot.NetDown,
					Load1:       info.Snapshot.Load1,
					Load5:       info.Snapshot.Load5,
					Load15:      info.Snapshot.Load15,
					TCPCount:    info.Snapshot.TCPCount,
					UDPCount:    info.Snapshot.UDPCount,
					Uptime:      info.Snapshot.Uptime,
					// Xray
					XrayRunning:     info.Snapshot.XrayRunning,
					XrayUptime:      info.Snapshot.XrayUptime,
					XrayGoroutines:  info.Snapshot.XrayGoroutines,
					XrayMemAlloc:    info.Snapshot.XrayMemAlloc,
					XrayTrafficUp:   info.Snapshot.XrayTrafficUp,
					XrayTrafficDown: info.Snapshot.XrayTrafficDown,
				}
				prom.SetNodeMetrics(info.Node.ID, info.Node.Hostname, snap)
				store.Record(info.Node.ID, snap)
			}
		}
		prom.SetNodeCounts(online, offline)
		prom.SetWSClients(wsHub.ClientCount())
	}

	// При изменении inbounds — пушим конфиг на все подключённые ноды.
	ibSvc.OnChange = func() {
		nodes, err := nodeSvc.List()
		if err != nil {
			log.Warn("inbound: failed to list nodes", "error", err)
			return
		}
		for _, info := range nodes {
			if info.Node.Status != node.StatusOnline {
				continue
			}
			configJSON, err := ibSvc.BuildXrayConfig(info.Node.ID)
			if err != nil {
				log.Warn("inbound: failed to build config", "node_id", info.Node.ID, "error", err)
				continue
			}
			pb := newConfigPush(configJSON)
			if err := grpcServer.SendToNode(info.Node.ID, pb); err != nil {
				log.Warn("inbound: push failed", "node_id", info.Node.ID, "error", err)
			}
		}
	}

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
		render:    engine,
		auth:      authService,
		settings:  settingsSvc,
		nodes:     nodeSvc,
		inbounds:  ibSvc,
		grpc:      grpcServer,
		certs:     certsMgr,
		wsHub:     wsHub,
		prom:      prom,
		store:     store,
		cfg:       cfg,
		log:       log,
		cacheBust: fmt.Sprintf("%d", time.Now().Unix()),
	}, nil
}

// Routes собирает все маршруты + middleware стек.
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	// Static files (без секретного пути — ресурсы должны грузиться всегда)
	fs := http.FileServer(http.Dir(filepath.Join("web", "static")))
	mux.Handle("/static/", http.StripPrefix("/static/", noCacheHandler(fs)))

	// Health + Metrics (без auth — Prometheus должен скрейпить)
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.Handle("GET /metrics", promhttp.Handler())

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
	mux.Handle("POST /api/settings/metrics-retention", protected(http.HandlerFunc(a.apiUpdateMetricsRetention)))

	// Node API
	mux.Handle("GET /api/nodes", protected(http.HandlerFunc(a.apiListNodes)))
	mux.Handle("GET /api/nodes/{id}", protected(http.HandlerFunc(a.apiGetNode)))
	mux.Handle("DELETE /api/nodes/{id}", protected(http.HandlerFunc(a.apiDeleteNode)))

	// Cluster info API
	mux.Handle("GET /api/cluster-info", protected(http.HandlerFunc(a.apiClusterInfo)))

	// Metrics history API (встроенное хранилище, замена Prometheus)
	mux.Handle("GET /api/metrics/{id}/history", protected(http.HandlerFunc(a.apiMetricsHistory)))

	// Inbound API (подключения Xray)
	mux.Handle("GET /api/inbounds", protected(http.HandlerFunc(a.apiListInbounds)))
	mux.Handle("POST /api/inbounds", protected(http.HandlerFunc(a.apiCreateInbound)))
	mux.Handle("GET /api/inbounds/{id}", protected(http.HandlerFunc(a.apiGetInbound)))
	mux.Handle("PUT /api/inbounds/{id}", protected(http.HandlerFunc(a.apiUpdateInbound)))
	mux.Handle("DELETE /api/inbounds/{id}", protected(http.HandlerFunc(a.apiDeleteInbound)))
	mux.Handle("POST /api/inbounds/{id}/toggle", protected(http.HandlerFunc(a.apiToggleInbound)))
	mux.Handle("POST /api/inbounds/push", protected(http.HandlerFunc(a.apiPushInbounds)))

	// Utils
	mux.Handle("GET /api/utils/x25519", protected(http.HandlerFunc(a.apiGenX25519)))

	// WebSocket (auth via cookie — проверяем до upgrade)
	mux.Handle("GET /ws", protected(http.HandlerFunc(a.handleWS)))

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
		if r.URL.Path == "/healthz" || r.URL.Path == "/metrics" || strings.HasPrefix(r.URL.Path, "/static/") {
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
		Title:     "Prozy — Dashboard",
		Username:  user.Username,
		Settings:  a.settingsData(),
		BasePath:  a.basePath(),
		CacheBust: a.cacheBust,
	})
}

// settingsData собирает текущие настройки для отображения.
func (a *App) settingsData() SettingsData {
	return SettingsData{
		// DB-backed (редактируемые через панель)
		SessionDuration:  a.settings.SessionDuration().String(),
		SecretPath:       a.settings.SecretPath(),
		MetricsRetention: a.settings.MetricsRetentionHours(),
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

func (a *App) apiUpdateMetricsRetention(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hours int `json:"hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := a.settings.SetMetricsRetentionHours(req.Hours); err != nil {
		render.JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	a.store.SetRetention(req.Hours)
	a.log.Info("metrics retention updated", "hours", req.Hours)
	render.JSONOk(w, map[string]any{"message": "Время хранения обновлено"})
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

	// Удаляем историю метрик ноды.
	a.store.DeleteNode(id)

	if err := a.nodes.Delete(id); err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось удалить ноду")
		return
	}

	a.log.Info("node deleted", "node_id", id)
	render.JSONOk(w, map[string]string{"message": "Нода удалена"})
}

// --- WebSocket --------------------------------------------------------------

// handleWS апгрейдит HTTP → WebSocket. Auth уже проверена middleware.
func (a *App) handleWS(w http.ResponseWriter, r *http.Request) {
	a.wsHub.Accept(w, r)
}

// noCacheHandler wraps a handler with no-cache headers for development.
func noCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// --- Metrics History API ----------------------------------------------------

// apiMetricsHistory — GET /api/metrics/{id}/history?range=1h&limit=500
// Поддерживаемые range: 30m, 1h, 6h, 12h, 24h, 7d (default: 1h).
func (a *App) apiMetricsHistory(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		render.JSONError(w, http.StatusBadRequest, "missing node id")
		return
	}

	// Проверяем что нода существует.
	if _, err := a.nodes.Get(nodeID); err != nil {
		render.JSONError(w, http.StatusNotFound, "нода не найдена")
		return
	}

	dur := parseRange(r.URL.Query().Get("range"))
	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parsePositiveInt(l); err == nil && v > 0 && v <= 5000 {
			limit = v
		}
	}

	now := time.Now()
	from := now.Add(-dur)

	samples, err := a.store.Query(nodeID, from, now, limit)
	if err != nil {
		render.JSONError(w, http.StatusInternalServerError, "ошибка запроса метрик")
		return
	}

	render.JSONOk(w, map[string]any{
		"node_id": nodeID,
		"range":   dur.String(),
		"count":   len(samples),
		"samples": samples,
	})
}

// parseRange парсит строку range в Duration. По умолчанию 1h.
func parseRange(s string) time.Duration {
	switch s {
	case "30m":
		return 30 * time.Minute
	case "1h", "":
		return 1 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
		return 1 * time.Hour
	}
}

// parsePositiveInt парсит строку в int > 0.
func parsePositiveInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// --- Inbound API (подключения Xray) ----------------------------------------

func (a *App) apiListInbounds(w http.ResponseWriter, _ *http.Request) {
	list, err := a.inbounds.List()
	if err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось получить список подключений")
		return
	}
	render.JSONOk(w, list)
}

func (a *App) apiGetInbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r.PathValue("id"))
	if err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ib, err := a.inbounds.Get(id)
	if err != nil {
		render.JSONError(w, http.StatusNotFound, "подключение не найдено")
		return
	}
	render.JSONOk(w, ib)
}

func (a *App) apiCreateInbound(w http.ResponseWriter, r *http.Request) {
	var ib inbound.Inbound
	if err := json.NewDecoder(r.Body).Decode(&ib); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := a.inbounds.Create(&ib); err != nil {
		render.JSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	render.JSONOk(w, ib)
}

func (a *App) apiUpdateInbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r.PathValue("id"))
	if err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := a.inbounds.Get(id)
	if err != nil {
		render.JSONError(w, http.StatusNotFound, "подключение не найдено")
		return
	}

	var ib inbound.Inbound
	if err := json.NewDecoder(r.Body).Decode(&ib); err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ib.ID = existing.ID
	ib.CreatedAt = existing.CreatedAt

	if err := a.inbounds.Update(&ib); err != nil {
		render.JSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	render.JSONOk(w, ib)
}

func (a *App) apiDeleteInbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r.PathValue("id"))
	if err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := a.inbounds.Delete(id); err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось удалить подключение")
		return
	}
	render.JSONOk(w, map[string]string{"message": "Подключение удалено"})
}

func (a *App) apiToggleInbound(w http.ResponseWriter, r *http.Request) {
	id, err := parseUintParam(r.PathValue("id"))
	if err != nil {
		render.JSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	ib, err := a.inbounds.Toggle(id)
	if err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось переключить подключение")
		return
	}
	render.JSONOk(w, ib)
}

// apiPushInbounds — принудительный пуш конфигов на все онлайн ноды.
func (a *App) apiPushInbounds(w http.ResponseWriter, _ *http.Request) {
	nodes, err := a.nodes.List()
	if err != nil {
		render.JSONError(w, http.StatusInternalServerError, "не удалось получить список нод")
		return
	}

	pushed := 0
	for _, info := range nodes {
		if info.Node.Status != node.StatusOnline {
			continue
		}
		configJSON, err := a.inbounds.BuildXrayConfig(info.Node.ID)
		if err != nil {
			a.log.Warn("push: build config failed", "node_id", info.Node.ID, "error", err)
			continue
		}
		pb := newConfigPush(configJSON)
		if err := a.grpc.SendToNode(info.Node.ID, pb); err != nil {
			a.log.Warn("push: send failed", "node_id", info.Node.ID, "error", err)
			continue
		}
		pushed++
	}

	render.JSONOk(w, map[string]any{
		"message": fmt.Sprintf("Конфиг отправлен на %d нод", pushed),
		"pushed":  pushed,
	})
}

// newConfigPush создаёт PanelMessage с ConfigPush.
func newConfigPush(configJSON string) *pb.PanelMessage {
	return &pb.PanelMessage{
		Payload: &pb.PanelMessage_ConfigPush{
			ConfigPush: &pb.ConfigPush{
				ConfigJson: configJSON,
			},
		},
	}
}

// apiGenX25519 генерирует пару x25519 ключей для Reality.
func (a *App) apiGenX25519(w http.ResponseWriter, _ *http.Request) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		render.JSONError(w, http.StatusInternalServerError, "ошибка генерации ключей")
		return
	}
	pub := priv.PublicKey()
	render.JSONOk(w, map[string]string{
		"private_key": base64.RawStdEncoding.EncodeToString(priv.Bytes()),
		"public_key":  base64.RawStdEncoding.EncodeToString(pub.Bytes()),
	})
}

func parseUintParam(s string) (uint, error) {
	var n uint
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

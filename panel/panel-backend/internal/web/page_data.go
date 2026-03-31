package web

// --- Page data types --------------------------------------------------------

// LoginPageData — данные для страницы логина.
type LoginPageData struct {
	Title    string
	Error    string
	Username string
	BasePath string // секретный URI-префикс (или "")
}

// DashboardPageData — данные для дашборда.
type DashboardPageData struct {
	Title      string
	Username   string
	Settings   SettingsData
	BasePath   string // секретный URI-префикс (или "")
	CacheBust  string // timestamp для cache-bust static-ресурсов
}

// SettingsData — текущие настройки панели для отображения.
type SettingsData struct {
	// DB-backed (редактируемые через панель)
	SessionDuration    string // "168h", "24h", ...
	SecretPath         string // "/mysecret" или ""
	MetricsRetention   int    // часы хранения метрик (24, 168, ...)

	// ENV-only (только для просмотра, не редактируются)
	PanelAddr string // PANEL_ADDR
	PanelPort string // PANEL_PORT
	DBPath    string // DB_PATH
	LogFormat string // LOG_FORMAT
	LogLevel  string // LOG_LEVEL
}

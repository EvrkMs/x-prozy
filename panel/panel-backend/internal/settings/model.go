package settings

import "time"

// Setting — одна запись настройки в БД (ключ → значение).
type Setting struct {
	Key       string `gorm:"primaryKey;size:64"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
}

// Ключи настроек, хранящихся в БД.
const (
	KeySessionDuration = "session_duration"
	KeySecretPath      = "panel_secret_path"
)

// values по умолчанию, если запись в БД отсутствует.
var defaults = map[string]string{
	KeySessionDuration: "168h",
	KeySecretPath:      "",
}

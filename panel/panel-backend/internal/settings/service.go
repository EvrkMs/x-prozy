package settings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Service предоставляет типизированный доступ к настройкам из БД.
type Service struct {
	repo *Repository
}

// NewService создаёт сервис настроек.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// --- Session Duration --------------------------------------------------------

// SessionDuration возвращает длительность сессии из БД (или дефолт).
func (s *Service) SessionDuration() time.Duration {
	if v, ok := s.repo.Get(KeySessionDuration); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	d, _ := time.ParseDuration(defaults[KeySessionDuration])
	return d
}

// SetSessionDuration сохраняет длительность сессии в БД.
func (s *Service) SetSessionDuration(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("settings: duration must be positive")
	}
	return s.repo.Set(KeySessionDuration, d.String())
}

// --- Secret Path -------------------------------------------------------------

// SecretPath возвращает URL-префикс для защиты панели (пустая строка = нет защиты).
func (s *Service) SecretPath() string {
	if v, ok := s.repo.Get(KeySecretPath); ok {
		return v
	}
	return ""
}

// SetSecretPath нормализует и сохраняет секретный путь в БД.
func (s *Service) SetSecretPath(path string) error {
	path = strings.TrimSpace(path)
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		path = strings.TrimRight(path, "/")
	}
	return s.repo.Set(KeySecretPath, path)
}

// --- Metrics Retention -------------------------------------------------------

// MetricsRetentionHours возвращает время хранения метрик в часах.
func (s *Service) MetricsRetentionHours() int {
	if v, ok := s.repo.Get(KeyMetricsRetention); ok {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			return h
		}
	}
	h, _ := strconv.Atoi(defaults[KeyMetricsRetention])
	return h
}

// SetMetricsRetentionHours сохраняет время хранения метрик в часах.
func (s *Service) SetMetricsRetentionHours(hours int) error {
	if hours < 1 {
		return fmt.Errorf("settings: retention must be >= 1 hour")
	}
	if hours > 8760 {
		return fmt.Errorf("settings: retention must be <= 8760 hours (365 days)")
	}
	return s.repo.Set(KeyMetricsRetention, strconv.Itoa(hours))
}

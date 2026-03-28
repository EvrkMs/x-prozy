package settings

import (
	"fmt"
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

package inbound

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// Service — бизнес-логика для подключений (inbounds).
type Service struct {
	repo *Repository
	log  *slog.Logger

	// OnChange вызывается при любом изменении (create, update, delete, toggle).
	// Используется для пуша конфигов на ноды.
	OnChange func()
}

// NewService создаёт сервис для управления inbounds.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// List возвращает все inbounds.
func (s *Service) List() ([]Inbound, error) {
	return s.repo.List()
}

// Get возвращает один inbound.
func (s *Service) Get(id uint) (*Inbound, error) {
	return s.repo.GetByID(id)
}

// Create создаёт новый inbound.
func (s *Service) Create(ib *Inbound) error {
	if err := ib.Validate(); err != nil {
		return err
	}

	// Проверяем уникальность тега.
	exists, err := s.repo.TagExists(ib.Tag, 0)
	if err != nil {
		return fmt.Errorf("check tag: %w", err)
	}
	if exists {
		return fmt.Errorf("tag '%s' already exists", ib.Tag)
	}

	if err := s.repo.Create(ib); err != nil {
		return fmt.Errorf("create: %w", err)
	}

	s.log.Info("inbound created", "id", ib.ID, "remark", ib.Remark, "protocol", ib.Protocol, "port", ib.Port)
	s.notify()
	return nil
}

// Update обновляет inbound.
func (s *Service) Update(ib *Inbound) error {
	if err := ib.Validate(); err != nil {
		return err
	}

	exists, err := s.repo.TagExists(ib.Tag, ib.ID)
	if err != nil {
		return fmt.Errorf("check tag: %w", err)
	}
	if exists {
		return fmt.Errorf("tag '%s' already exists", ib.Tag)
	}

	if err := s.repo.Update(ib); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	s.log.Info("inbound updated", "id", ib.ID, "remark", ib.Remark)
	s.notify()
	return nil
}

// Delete удаляет inbound.
func (s *Service) Delete(id uint) error {
	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	s.log.Info("inbound deleted", "id", id)
	s.notify()
	return nil
}

// Toggle включает/выключает inbound.
func (s *Service) Toggle(id uint) (*Inbound, error) {
	ib, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	ib.Enable = !ib.Enable
	if err := s.repo.Update(ib); err != nil {
		return nil, err
	}

	status := "disabled"
	if ib.Enable {
		status = "enabled"
	}
	s.log.Info("inbound toggled", "id", id, "status", status)
	s.notify()
	return ib, nil
}

// BuildXrayConfig собирает полный Xray JSON-конфиг для ноды.
// Включает все enabled inbounds, привязанные к данной ноде + stats API.
func (s *Service) BuildXrayConfig(nodeID string) (string, error) {
	inbounds, err := s.repo.ListForNode(nodeID)
	if err != nil {
		return "", fmt.Errorf("list inbounds for node: %w", err)
	}

	xrayInbounds := make([]any, 0, len(inbounds)+1)

	// Stats API inbound (всегда)
	xrayInbounds = append(xrayInbounds, map[string]any{
		"listen":   "127.0.0.1",
		"port":     10085,
		"protocol": "dokodemo-door",
		"settings": map[string]any{"address": "127.0.0.1"},
		"tag":      "api",
	})

	for _, ib := range inbounds {
		xrayInbounds = append(xrayInbounds, ib.ToXrayInbound())
	}

	// Routing: api → direct
	routingRules := []any{
		map[string]any{
			"inboundTag":  []string{"api"},
			"outboundTag": "api",
			"type":        "field",
		},
	}

	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"StatsService"},
		},
		"stats": map[string]any{},
		"policy": map[string]any{
			"system": map[string]any{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"inbounds":  xrayInbounds,
		"outbounds": []any{
			map[string]any{
				"protocol": "freedom",
				"tag":      "direct",
			},
			map[string]any{
				"protocol": "blackhole",
				"tag":      "blocked",
			},
		},
		"routing": map[string]any{
			"rules": routingRules,
		},
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Service) notify() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

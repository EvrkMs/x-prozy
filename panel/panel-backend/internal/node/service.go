package node

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service — бизнес-логика для нод.
type Service struct {
	repo *Repository
	log  *slog.Logger

	// In-memory state: подключённые ноды и их метрики.
	mu        sync.RWMutex
	snapshots map[string]*NodeSnapshot // node_id -> latest snapshot

	// OnChange вызывается при любом изменении (snapshot, online, offline, delete).
	// Callback не должен блокировать.
	OnChange func()
}

// NewService создаёт сервис для управления нодами.
func NewService(repo *Repository, log *slog.Logger) (*Service, error) {
	// При старте панели: все ноды = offline (они переподключатся).
	if err := repo.SetAllOffline(); err != nil {
		return nil, fmt.Errorf("node: reset status: %w", err)
	}

	return &Service{
		repo:      repo,
		log:       log,
		snapshots: make(map[string]*NodeSnapshot),
	}, nil
}

// Register — первый handshake: создаёт новую ноду в БД.
// Возвращает (node_id, reconnect_secret, err).
func (s *Service) Register(hostname, os, arch, version, publicIP, remoteAddr string) (string, string, error) {
	nodeID := uuid.New().String()
	secret := generateSecret(32)
	now := time.Now().UTC()

	n := &Node{
		ID:              nodeID,
		Hostname:        hostname,
		Status:          StatusOnline,
		ReconnectSecret: secret,
		Version:         version,
		OS:              os,
		Arch:            arch,
		PublicIP:        publicIP,
		RemoteAddr:      remoteAddr,
		ConnectedAt:     &now,
		LastSeenAt:      &now,
	}

	if err := s.repo.Create(n); err != nil {
		return "", "", fmt.Errorf("node: register: %w", err)
	}

	s.log.Info("node registered",
		"node_id", nodeID,
		"hostname", hostname,
		"remote_addr", remoteAddr,
	)
	return nodeID, secret, nil
}

// Reconnect — повторное подключение ноды по reconnect_secret.
// Возвращает (node_id, new_reconnect_secret, err).
func (s *Service) Reconnect(reconnectSecret, hostname, os, arch, version, publicIP, remoteAddr string) (string, string, error) {
	n, err := s.repo.GetByReconnectSecret(reconnectSecret)
	if err != nil {
		return "", "", fmt.Errorf("node: reconnect: invalid secret")
	}

	now := time.Now().UTC()
	newSecret := generateSecret(32)

	n.Status = StatusOnline
	n.ReconnectSecret = newSecret
	n.Hostname = hostname
	n.OS = os
	n.Arch = arch
	n.Version = version
	n.PublicIP = publicIP
	n.RemoteAddr = remoteAddr
	n.ConnectedAt = &now
	n.LastSeenAt = &now

	if err := s.repo.Update(n); err != nil {
		return "", "", fmt.Errorf("node: reconnect update: %w", err)
	}

	s.log.Info("node reconnected",
		"node_id", n.ID,
		"hostname", hostname,
		"remote_addr", remoteAddr,
	)
	return n.ID, newSecret, nil
}

// MarkOnline обновляет last_seen для ноды (heartbeat).
func (s *Service) MarkOnline(nodeID string) {
	now := time.Now().UTC()
	s.repo.db.Model(&Node{}).Where("id = ?", nodeID).
		Updates(map[string]any{
			"status":       StatusOnline,
			"last_seen_at": now,
		})
}

// MarkOffline помечает ноду как offline.
func (s *Service) MarkOffline(nodeID string) {
	s.repo.SetStatus(nodeID, StatusOffline)
	s.mu.Lock()
	delete(s.snapshots, nodeID)
	s.mu.Unlock()

	s.log.Info("node offline", "node_id", nodeID)
	s.notify()
}

// UpdateSnapshot сохраняет последние метрики ноды.
func (s *Service) UpdateSnapshot(snap *NodeSnapshot) {
	s.mu.Lock()
	s.snapshots[snap.NodeID] = snap
	s.mu.Unlock()
	s.notify()
}

// GetSnapshot возвращает последний snapshot для ноды.
func (s *Service) GetSnapshot(nodeID string) *NodeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshots[nodeID]
}

// List возвращает все ноды с метриками.
func (s *Service) List() ([]NodeInfo, error) {
	nodes, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		info := NodeInfo{Node: n}
		if snap, ok := s.snapshots[n.ID]; ok {
			info.Snapshot = snap
		}
		result = append(result, info)
	}
	return result, nil
}

// Get возвращает одну ноду с метриками.
func (s *Service) Get(nodeID string) (*NodeInfo, error) {
	n, err := s.repo.GetByID(nodeID)
	if err != nil {
		return nil, err
	}
	info := &NodeInfo{Node: *n}

	s.mu.RLock()
	if snap, ok := s.snapshots[nodeID]; ok {
		info.Snapshot = snap
	}
	s.mu.RUnlock()

	return info, nil
}

// Delete удаляет ноду.
func (s *Service) Delete(nodeID string) error {
	s.mu.Lock()
	delete(s.snapshots, nodeID)
	s.mu.Unlock()

	err := s.repo.Delete(nodeID)
	if err == nil {
		s.notify()
	}
	return err
}

// notify вызывает OnChange callback (если задан).
func (s *Service) notify() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// generateSecret генерирует криптобезопасный hex-токен.
func generateSecret(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

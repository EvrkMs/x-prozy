package node

import (
	"fmt"

	"gorm.io/gorm"
)

// Repository — CRUD для таблицы nodes.
type Repository struct {
	db *gorm.DB
}

// NewRepository создаёт репозиторий и выполняет миграцию.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if err := db.AutoMigrate(&Node{}); err != nil {
		return nil, fmt.Errorf("node: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// Create сохраняет новую ноду.
func (r *Repository) Create(n *Node) error {
	return r.db.Create(n).Error
}

// Update обновляет существующую ноду.
func (r *Repository) Update(n *Node) error {
	return r.db.Save(n).Error
}

// GetByID находит ноду по ID.
func (r *Repository) GetByID(id string) (*Node, error) {
	var n Node
	if err := r.db.First(&n, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// GetByReconnectSecret находит ноду по reconnect_secret (для реконнекта).
func (r *Repository) GetByReconnectSecret(secret string) (*Node, error) {
	var n Node
	if err := r.db.Where("reconnect_secret = ?", secret).First(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// List возвращает все ноды (включая offline).
func (r *Repository) List() ([]Node, error) {
	var nodes []Node
	if err := r.db.Order("created_at DESC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

// Delete мягко удаляет ноду.
func (r *Repository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&Node{}).Error
}

// SetStatus обновляет статус и last_seen_at.
func (r *Repository) SetStatus(id string, status NodeStatus) error {
	return r.db.Model(&Node{}).Where("id = ?", id).
		Update("status", status).Error
}

// SetAllOffline сбрасывает все ноды в offline (при старте панели).
func (r *Repository) SetAllOffline() error {
	return r.db.Model(&Node{}).Where("status = ?", StatusOnline).
		Update("status", StatusOffline).Error
}

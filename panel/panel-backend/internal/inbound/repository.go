package inbound

import (
	"fmt"

	"gorm.io/gorm"
)

// Repository — CRUD для таблицы inbounds.
type Repository struct {
	db *gorm.DB
}

// NewRepository создаёт репозиторий и выполняет миграцию.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if err := db.AutoMigrate(&Inbound{}); err != nil {
		return nil, fmt.Errorf("inbound: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// Create сохраняет новый inbound.
func (r *Repository) Create(i *Inbound) error {
	return r.db.Create(i).Error
}

// Update обновляет существующий inbound.
func (r *Repository) Update(i *Inbound) error {
	return r.db.Save(i).Error
}

// GetByID находит inbound по ID.
func (r *Repository) GetByID(id uint) (*Inbound, error) {
	var i Inbound
	if err := r.db.First(&i, id).Error; err != nil {
		return nil, err
	}
	return &i, nil
}

// List возвращает все inbounds.
func (r *Repository) List() ([]Inbound, error) {
	var list []Inbound
	if err := r.db.Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListEnabled возвращает включённые inbounds.
func (r *Repository) ListEnabled() ([]Inbound, error) {
	var list []Inbound
	if err := r.db.Where("enable = ?", true).Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListForNode возвращает включённые inbounds, привязанные к конкретной ноде.
func (r *Repository) ListForNode(nodeID string) ([]Inbound, error) {
	all, err := r.ListEnabled()
	if err != nil {
		return nil, err
	}
	var result []Inbound
	for _, ib := range all {
		ids := ib.GetNodeIDs()
		if len(ids) == 0 {
			// Пустой массив = деплоить на все ноды.
			result = append(result, ib)
			continue
		}
		for _, id := range ids {
			if id == nodeID {
				result = append(result, ib)
				break
			}
		}
	}
	return result, nil
}

// Delete мягко удаляет inbound.
func (r *Repository) Delete(id uint) error {
	return r.db.Delete(&Inbound{}, id).Error
}

// TagExists проверяет, существует ли inbound с данным тегом (исключая excludeID).
func (r *Repository) TagExists(tag string, excludeID uint) (bool, error) {
	var count int64
	q := r.db.Model(&Inbound{}).Where("tag = ?", tag)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

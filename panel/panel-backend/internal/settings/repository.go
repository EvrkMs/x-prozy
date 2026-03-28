package settings

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository взаимодействует с таблицей settings в БД.
type Repository struct {
	db *gorm.DB
}

// NewRepository создаёт репозиторий и запускает миграцию.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if err := db.AutoMigrate(&Setting{}); err != nil {
		return nil, fmt.Errorf("settings: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// Get возвращает значение по ключу.
func (r *Repository) Get(key string) (string, bool) {
	var s Setting
	if err := r.db.First(&s, "key = ?", key).Error; err != nil {
		return "", false
	}
	return s.Value, true
}

// Set сохраняет (upsert) значение по ключу.
func (r *Repository) Set(key, value string) error {
	s := Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return r.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&s).Error
}

// All возвращает все настройки из БД.
func (r *Repository) All() (map[string]string, error) {
	var rows []Setting
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, row := range rows {
		m[row.Key] = row.Value
	}
	return m, nil
}

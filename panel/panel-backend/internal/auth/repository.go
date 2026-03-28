package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Repository инкапсулирует все операции с БД для auth.
type Repository struct {
	db *gorm.DB
}

// NewRepository создаёт подключение к SQLite и запускает миграции.
func NewRepository(dsn string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
		return nil, fmt.Errorf("auth: create db dir: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("auth: open db: %w", err)
	}

	if err := db.AutoMigrate(&User{}, &Session{}); err != nil {
		return nil, fmt.Errorf("auth: migrate: %w", err)
	}

	return &Repository{db: db}, nil
}

// DB возвращает gorm.DB для использования другими репозиториями (settings и т.д.).
func (r *Repository) DB() *gorm.DB { return r.db }

// --- User -----------------------------------------------------------------

// GetUser возвращает единственного пользователя по username.
func (r *Repository) GetUser(username string) (*User, error) {
	var u User
	if err := r.db.Where("username = ?", username).First(&u).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: get user: %w", err)
	}
	return &u, nil
}

// GetUserByID возвращает пользователя по ID.
func (r *Repository) GetUserByID(id uint) (*User, error) {
	var u User
	if err := r.db.First(&u, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: get user by id: %w", err)
	}
	return &u, nil
}

// UserExists проверяет, есть ли хотя бы один пользователь.
func (r *Repository) UserExists() (bool, error) {
	var count int64
	if err := r.db.Model(&User{}).Count(&count).Error; err != nil {
		return false, fmt.Errorf("auth: count users: %w", err)
	}
	return count > 0, nil
}

// CreateUser создаёт пользователя.
func (r *Repository) CreateUser(u *User) error {
	if err := r.db.Create(u).Error; err != nil {
		return fmt.Errorf("auth: create user: %w", err)
	}
	return nil
}

// UpdateUser обновляет пользователя.
func (r *Repository) UpdateUser(u *User) error {
	if err := r.db.Save(u).Error; err != nil {
		return fmt.Errorf("auth: update user: %w", err)
	}
	return nil
}

// --- Session --------------------------------------------------------------

// CreateSession сохраняет новую сессию.
func (r *Repository) CreateSession(s *Session) error {
	if err := r.db.Create(s).Error; err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

// GetSession возвращает сессию с пользователем по токену.
func (r *Repository) GetSession(token string) (*Session, error) {
	var s Session
	err := r.db.Preload("User").Where("token = ?", token).First(&s).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("auth: get session: %w", err)
	}
	return &s, nil
}

// DeleteSession удаляет сессию по токену.
func (r *Repository) DeleteSession(token string) error {
	if err := r.db.Where("token = ?", token).Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// DeleteSessionsByUser удаляет все сессии пользователя.
func (r *Repository) DeleteSessionsByUser(userID uint) error {
	if err := r.db.Where("user_id = ?", userID).Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("auth: delete sessions: %w", err)
	}
	return nil
}

// DeleteExpiredSessions чистит протухшие сессии.
func (r *Repository) DeleteExpiredSessions() error {
	if err := r.db.Where("expires_at < ?", time.Now().UTC()).Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("auth: delete expired sessions: %w", err)
	}
	return nil
}

// Close закрывает соединение с БД.
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

package auth

import (
	"time"

	"gorm.io/gorm"
)

// User — единственный аккаунт панели.
type User struct {
	ID           uint           `gorm:"primaryKey"`
	Username     string         `gorm:"uniqueIndex;size:64;not null"`
	PasswordHash string         `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// Session — активная сессия пользователя.
type Session struct {
	ID        uint      `gorm:"primaryKey"`
	Token     string    `gorm:"uniqueIndex;size:64;not null"`
	UserID    uint      `gorm:"not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time

	User *User `gorm:"constraint:OnDelete:CASCADE"`
}

// IsExpired проверяет, истекла ли сессия.
func (s *Session) IsExpired() bool {
	return time.Now().UTC().After(s.ExpiresAt)
}

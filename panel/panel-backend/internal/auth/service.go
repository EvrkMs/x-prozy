package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SessionValidator — интерфейс для middleware.
// Middleware знает только про этот интерфейс, а не про весь Service.
type SessionValidator interface {
	ValidateToken(ctx context.Context, token string) (*User, error)
}

// Service — аутентификация и управление сессиями.
type Service struct {
	repo            *Repository
	sessionDuration time.Duration
	bcryptCost      int
}

// NewService создаёт auth service и сидирует дефолтного пользователя.
func NewService(repo *Repository, sessionDuration time.Duration) (*Service, error) {
	if sessionDuration == 0 {
		sessionDuration = 7 * 24 * time.Hour
	}

	svc := &Service{
		repo:            repo,
		sessionDuration: sessionDuration,
		bcryptCost:      bcrypt.DefaultCost,
	}

	if err := svc.seedDefaultUser(context.Background()); err != nil {
		return nil, err
	}

	return svc, nil
}

// Login проверяет креденшлы и возвращает сессионный токен.
func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.repo.GetUser(username)
	if err != nil {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}

	session := &Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(s.sessionDuration),
	}

	if err := s.repo.CreateSession(session); err != nil {
		return "", err
	}

	return token, nil
}

// ValidateToken проверяет токен и возвращает пользователя.
// Реализует SessionValidator для middleware.
func (s *Service) ValidateToken(ctx context.Context, token string) (*User, error) {
	session, err := s.repo.GetSession(token)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	if session.IsExpired() {
		_ = s.repo.DeleteSession(token)
		return nil, ErrSessionExpired
	}

	if session.User == nil {
		return nil, ErrUserNotFound
	}

	return session.User, nil
}

// Logout удаляет сессию.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.repo.DeleteSession(token)
}

// ChangePassword меняет пароль и инвалидирует все сессии.
func (s *Service) ChangePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}

	user.PasswordHash = string(hash)
	if err := s.repo.UpdateUser(user); err != nil {
		return err
	}

	return s.repo.DeleteSessionsByUser(userID)
}

// ChangeUsername меняет логин оператора.
func (s *Service) ChangeUsername(ctx context.Context, userID uint, newUsername string) error {
	user, err := s.repo.GetUserByID(userID)
	if err != nil {
		return err
	}
	user.Username = newUsername
	return s.repo.UpdateUser(user)
}

// SetSessionDuration обновляет длительность сессии в рантайме.
func (s *Service) SetSessionDuration(d time.Duration) {
	s.sessionDuration = d
}

// CleanupSessions удаляет протухшие сессии (вызывать по cron/ticker).
func (s *Service) CleanupSessions() error {
	return s.repo.DeleteExpiredSessions()
}

// seedDefaultUser создаёт admin:admin если в БД нет ни одного пользователя.
func (s *Service) seedDefaultUser(ctx context.Context) error {
	exists, err := s.repo.UserExists()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash default password: %w", err)
	}

	return s.repo.CreateUser(&User{
		Username:     "admin",
		PasswordHash: string(hash),
	})
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

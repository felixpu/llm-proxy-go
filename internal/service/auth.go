package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const (
	// SessionExpireHours is the default session expiration time.
	SessionExpireHours = 24
)

// CurrentUser represents the authenticated user context.
type CurrentUser struct {
	UserID       int64   `json:"user_id"`
	Username     string  `json:"username"`
	Role         string  `json:"role"`
	APIKeyPrefix *string `json:"api_key_prefix,omitempty"`
	APIKeyID     *int64  `json:"api_key_id,omitempty"`
}

// AuthService handles authentication: API key validation and session management.
type AuthService struct {
	keyRepo     repository.APIKeyRepository
	userRepo    repository.UserRepository
	sessionRepo *repository.SessionRepository
	logger      *zap.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	keyRepo repository.APIKeyRepository,
	userRepo repository.UserRepository,
	sessionRepo *repository.SessionRepository,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		keyRepo:     keyRepo,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		logger:      logger,
	}
}

// --- API Key Authentication ---

// ValidateAPIKey validates an API key and returns the associated user.
func (s *AuthService) ValidateAPIKey(ctx context.Context, rawKey string) (*CurrentUser, error) {
	keyHash := HashAPIKey(rawKey)

	apiKey, err := s.keyRepo.FindByKeyHash(ctx, keyHash)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	if !apiKey.IsActive {
		return nil, fmt.Errorf("API key is inactive")
	}

	user, err := s.userRepo.FindByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found for API key")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is inactive")
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.keyRepo.UpdateLastUsed(ctx, apiKey.ID); err != nil {
			s.logger.Debug("failed to update API key last used", zap.Error(err))
		}
	}()

	prefix := apiKey.KeyPrefix
	return &CurrentUser{
		UserID:       user.ID,
		Username:     user.Username,
		Role:         string(user.Role),
		APIKeyPrefix: &prefix,
		APIKeyID:     &apiKey.ID,
	}, nil
}

// --- Session Authentication ---

// AuthenticateUser verifies username/password and returns the user.
func (s *AuthService) AuthenticateUser(ctx context.Context, username, password string) (*models.User, error) {
	user, err := s.userRepo.FindByUsernameWithHash(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is inactive")
	}

	if !VerifyPassword(password, user.PasswordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

// CreateSession creates a new session for the user.
func (s *AuthService) CreateSession(ctx context.Context, userID int64, ipAddress, userAgent string) (*repository.Session, error) {
	token, err := repository.GenerateSessionToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(time.Duration(SessionExpireHours) * time.Hour)

	id, err := s.sessionRepo.CreateSession(ctx, userID, token, expiresAt, ipAddress, userAgent)
	if err != nil {
		return nil, err
	}

	return &repository.Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}, nil
}

// ValidateSession validates a session token and returns the user.
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*CurrentUser, error) {
	session, username, role, err := s.sessionRepo.FindValidSession(ctx, token)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("invalid or expired session")
	}

	return &CurrentUser{
		UserID:   session.UserID,
		Username: username,
		Role:     role,
	}, nil
}

// DeleteSession removes a session by token.
func (s *AuthService) DeleteSession(ctx context.Context, token string) error {
	return s.sessionRepo.DeleteByToken(ctx, token)
}

// DeleteUserSessions removes all sessions for a user.
func (s *AuthService) DeleteUserSessions(ctx context.Context, userID int64) (int64, error) {
	return s.sessionRepo.DeleteByUserID(ctx, userID)
}

// CleanupExpiredSessions removes expired sessions.
func (s *AuthService) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessionRepo.CleanupExpired(ctx)
}

// CreateDefaultAdmin creates the default admin user if none exists.
func (s *AuthService) CreateDefaultAdmin(ctx context.Context, username, password string) error {
	existing, err := s.userRepo.FindByUsername(ctx, username)
	if err == nil && existing != nil {
		return nil // Already exists
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = s.userRepo.Insert(ctx, &models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to create default admin: %w", err)
	}

	s.logger.Info("default admin created", zap.String("username", username))
	return nil
}

// --- Password Utilities (methods for handler access) ---

// HashPassword hashes a password (method wrapper for handler use).
func (s *AuthService) HashPassword(password string) (string, error) {
	return HashPassword(password)
}

// VerifyPassword verifies a password against a stored hash (method wrapper for handler use).
func (s *AuthService) VerifyPassword(password, storedHash string) bool {
	return VerifyPassword(password, storedHash)
}

// --- Password Utilities ---

// HashPassword hashes a password using bcrypt.
// For passwords > 72 bytes, pre-hashes with SHA-256.
func HashPassword(password string) (string, error) {
	input := []byte(password)
	if len(input) > 72 {
		h := sha256.Sum256(input)
		input = []byte(hex.EncodeToString(h[:]))
	}
	hash, err := bcrypt.GenerateFromPassword(input, bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against a stored hash.
// Supports bcrypt (new) and SHA-256 salt$hash (legacy) formats.
func VerifyPassword(password, storedHash string) bool {
	// Bcrypt format: $2a$, $2b$, $2y$
	if len(storedHash) > 3 && storedHash[0] == '$' && storedHash[1] == '2' {
		input := []byte(password)
		if len(input) > 72 {
			h := sha256.Sum256(input)
			input = []byte(hex.EncodeToString(h[:]))
		}
		return bcrypt.CompareHashAndPassword([]byte(storedHash), input) == nil
	}

	// Legacy SHA-256 format: salt$hash
	parts := splitOnce(storedHash, '$')
	if len(parts) != 2 {
		return false
	}
	salt, expectedHash := parts[0], parts[1]
	h := sha256.Sum256([]byte(salt + password))
	actualHash := hex.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(actualHash)) == 1
}

// HashAPIKey computes SHA-256 hex digest of an API key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

// GenerateAPIKey generates a new API key with full key, hash, and prefix.
// Returns: (fullKey, keyHash, keyPrefix)
func GenerateAPIKey() (string, string, string) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based key if random fails
		b = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}

	fullKey := fmt.Sprintf("sk-proxy-%s", hex.EncodeToString(b))
	keyHash := HashAPIKey(fullKey)
	keyPrefix := fullKey[:20] // "sk-proxy-" (9) + 11 chars

	return fullKey, keyHash, keyPrefix
}

// splitOnce splits a string on the first occurrence of sep.
func splitOnce(s string, sep byte) []string {
	for i := range len(s) {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

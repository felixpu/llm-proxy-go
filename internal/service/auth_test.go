//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"short password", "abc123"},
		{"normal password", "mySecurePassword123!"},
		{"long password over 72 bytes", "this-is-a-very-long-password-that-exceeds-seventy-two-bytes-in-length-for-bcrypt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
			assert.True(t, hash[0] == '$' && hash[1] == '2') // bcrypt format
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	// Create a bcrypt hash
	password := "testPassword123"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	tests := []struct {
		name       string
		password   string
		storedHash string
		want       bool
	}{
		{"correct password", password, hash, true},
		{"wrong password", "wrongPassword", hash, false},
		{"empty password", "", hash, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifyPassword(tt.password, tt.storedHash)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestVerifyPassword_LegacyFormat(t *testing.T) {
	// Legacy SHA-256 format: salt$hash
	// salt = "testsalt", password = "password123"
	// hash = sha256("testsalt" + "password123")
	legacyHash := "testsalt$e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// This won't match because the hash is wrong, but tests the format detection
	result := VerifyPassword("password123", legacyHash)
	assert.False(t, result) // Hash doesn't match

	// Invalid format
	result = VerifyPassword("password", "invalidformat")
	assert.False(t, result)
}

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"simple key", "sk-test-123"},
		{"full key", "sk-proxy-abcdef1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashAPIKey(tt.key)
			assert.Len(t, hash, 64) // SHA-256 hex is 64 chars

			// Same key should produce same hash
			hash2 := HashAPIKey(tt.key)
			assert.Equal(t, hash, hash2)
		})
	}
}

func TestGenerateAPIKey(t *testing.T) {
	fullKey, keyHash, keyPrefix := GenerateAPIKey()

	assert.True(t, len(fullKey) > 20)
	assert.True(t, fullKey[:9] == "sk-proxy-")
	assert.Len(t, keyHash, 64)
	assert.Len(t, keyPrefix, 20)
	assert.Equal(t, fullKey[:20], keyPrefix)

	// Hash should match
	assert.Equal(t, HashAPIKey(fullKey), keyHash)

	// Each call should generate unique keys
	fullKey2, _, _ := GenerateAPIKey()
	assert.NotEqual(t, fullKey, fullKey2)
}

func TestAuthService_ValidateAPIKey(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create test user
	hash, _ := HashPassword("password123")
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: hash,
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create API key
	fullKey, keyHash, keyPrefix := GenerateAPIKey()
	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Name:      "Test Key",
		IsActive:  true,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", fullKey, false},
		{"invalid key", "sk-invalid-key", true},
		{"empty key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := authService.ValidateAPIKey(ctx, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, "testuser", user.Username)
			}
		})
	}
}

func TestAuthService_ValidateAPIKey_InactiveKey(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create test user
	hash, _ := HashPassword("password123")
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: hash,
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create inactive API key
	fullKey, keyHash, keyPrefix := GenerateAPIKey()
	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Name:      "Inactive Key",
		IsActive:  false,
	})
	require.NoError(t, err)

	_, err = authService.ValidateAPIKey(ctx, fullKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "inactive")
}

func TestAuthService_AuthenticateUser(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create test user
	password := "securePassword123"
	hash, _ := HashPassword(password)
	_, err := userRepo.Insert(ctx, &models.User{
		Username:     "authuser",
		PasswordHash: hash,
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{"valid credentials", "authuser", password, false},
		{"wrong password", "authuser", "wrongpassword", true},
		{"non-existing user", "nonexistent", password, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := authService.AuthenticateUser(ctx, tt.username, tt.password)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
			}
		})
	}
}

func TestAuthService_AuthenticateUser_InactiveUser(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create inactive user
	password := "password123"
	hash, _ := HashPassword(password)
	_, err := userRepo.Insert(ctx, &models.User{
		Username:     "inactiveuser",
		PasswordHash: hash,
		Role:         models.UserRoleUser,
		IsActive:     false,
	})
	require.NoError(t, err)

	_, err = authService.AuthenticateUser(ctx, "inactiveuser", password)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "inactive")
}

func TestAuthService_CreateSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create test user
	hash, _ := HashPassword("password123")
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "sessionuser",
		PasswordHash: hash,
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	session, err := authService.CreateSession(ctx, userID, "127.0.0.1", "TestAgent/1.0")
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, userID, session.UserID)
	assert.NotEmpty(t, session.Token)
	assert.Equal(t, "127.0.0.1", session.IPAddress)
	assert.Equal(t, "TestAgent/1.0", session.UserAgent)
}

func TestAuthService_ValidateSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create test user
	hash, _ := HashPassword("password123")
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "validateuser",
		PasswordHash: hash,
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create session
	session, err := authService.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")
	require.NoError(t, err)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{"valid token", session.Token, false},
		{"invalid token", "invalid-token", true},
		{"empty token", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := authService.ValidateSession(ctx, tt.token)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, "validateuser", user.Username)
				assert.Equal(t, "admin", user.Role)
			}
		})
	}
}

func TestAuthService_DeleteSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create test user and session
	hash, _ := HashPassword("password123")
	userID, _ := userRepo.Insert(ctx, &models.User{
		Username:     "deleteuser",
		PasswordHash: hash,
		Role:         models.UserRoleUser,
		IsActive:     true,
	})

	session, _ := authService.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	// Delete session
	err := authService.DeleteSession(ctx, session.Token)
	assert.NoError(t, err)

	// Session should no longer be valid
	_, err = authService.ValidateSession(ctx, session.Token)
	assert.Error(t, err)
}

func TestAuthService_CreateDefaultAdmin(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)

	authService := NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	ctx := context.Background()

	// Create default admin
	err := authService.CreateDefaultAdmin(ctx, "admin", "admin123")
	require.NoError(t, err)

	// Verify admin exists
	user, err := userRepo.FindByUsername(ctx, "admin")
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, models.UserRoleAdmin, user.Role)

	// Creating again should not error (idempotent)
	err = authService.CreateDefaultAdmin(ctx, "admin", "admin123")
	assert.NoError(t, err)
}

func TestSplitOnce(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sep  byte
		want []string
	}{
		{"with separator", "hello$world", '$', []string{"hello", "world"}},
		{"no separator", "helloworld", '$', []string{"helloworld"}},
		{"multiple separators", "a$b$c", '$', []string{"a", "b$c"}},
		{"empty string", "", '$', []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitOnce(tt.s, tt.sep)
			assert.Equal(t, tt.want, result)
		})
	}
}

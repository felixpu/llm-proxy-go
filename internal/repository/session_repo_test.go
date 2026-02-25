//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestSessionRepository_CreateSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name      string
		userID    int64
		token     string
		expiresAt time.Time
		ipAddress string
		userAgent string
		wantErr   bool
	}{
		{
			name:      "valid session",
			userID:    1,
			token:     "test_token_123",
			expiresAt: time.Now().Add(24 * time.Hour),
			ipAddress: "127.0.0.1",
			userAgent: "Mozilla/5.0",
			wantErr:   false,
		},
		{
			name:      "session without ip/ua",
			userID:    2,
			token:     "test_token_456",
			expiresAt: time.Now().Add(1 * time.Hour),
			ipAddress: "",
			userAgent: "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.CreateSession(ctx, tt.userID, tt.token, tt.expiresAt, tt.ipAddress, tt.userAgent)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))
			}
		})
	}
}

func TestSessionRepository_CreateSession_DuplicateToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	token := "duplicate_token"
	expiresAt := time.Now().Add(24 * time.Hour)

	// First session
	_, err := repo.CreateSession(ctx, 1, token, expiresAt, "", "")
	require.NoError(t, err)

	// Duplicate token should fail
	_, err = repo.CreateSession(ctx, 2, token, expiresAt, "", "")
	assert.Error(t, err)
}

func TestSessionRepository_FindValidSession(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	// Create a valid session
	validToken := "valid_session_token"
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := repo.CreateSession(ctx, 1, validToken, expiresAt, "127.0.0.1", "TestAgent")
	require.NoError(t, err)

	// Create an expired session
	expiredToken := "expired_session_token"
	expiredAt := time.Now().Add(-1 * time.Hour)
	_, err = repo.CreateSession(ctx, 1, expiredToken, expiredAt, "", "")
	require.NoError(t, err)

	tests := []struct {
		name         string
		token        string
		wantSession  bool
		wantUsername string
		wantRole     string
	}{
		{"valid session", validToken, true, "admin", "admin"},
		{"expired session", expiredToken, false, "", ""},
		{"non-existing token", "nonexistent", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, username, role, err := repo.FindValidSession(ctx, tt.token)
			require.NoError(t, err)

			if tt.wantSession {
				assert.NotNil(t, session)
				assert.Equal(t, tt.token, session.Token)
				assert.Equal(t, tt.wantUsername, username)
				assert.Equal(t, tt.wantRole, role)
			} else {
				assert.Nil(t, session)
			}
		})
	}
}

func TestSessionRepository_FindValidSession_InactiveUser(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	// Create session for inactive user (user ID 3)
	token := "inactive_user_session"
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := repo.CreateSession(ctx, 3, token, expiresAt, "", "")
	require.NoError(t, err)

	// Should not find session for inactive user
	session, _, _, err := repo.FindValidSession(ctx, token)
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestSessionRepository_DeleteByToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	// Create session
	token := "to_delete_token"
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := repo.CreateSession(ctx, 1, token, expiresAt, "", "")
	require.NoError(t, err)

	// Delete session
	err = repo.DeleteByToken(ctx, token)
	require.NoError(t, err)

	// Verify deletion
	session, _, _, err := repo.FindValidSession(ctx, token)
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestSessionRepository_DeleteByUserID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	// Create multiple sessions for user
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := repo.CreateSession(ctx, 1, "user1_session1", expiresAt, "", "")
	require.NoError(t, err)
	_, err = repo.CreateSession(ctx, 1, "user1_session2", expiresAt, "", "")
	require.NoError(t, err)
	_, err = repo.CreateSession(ctx, 2, "user2_session1", expiresAt, "", "")
	require.NoError(t, err)

	// Delete all sessions for user 1
	deleted, err := repo.DeleteByUserID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify user 1 sessions are deleted
	session, _, _, err := repo.FindValidSession(ctx, "user1_session1")
	require.NoError(t, err)
	assert.Nil(t, session)

	// Verify user 2 session still exists
	session, _, _, err = repo.FindValidSession(ctx, "user2_session1")
	require.NoError(t, err)
	assert.NotNil(t, session)
}

func TestSessionRepository_CleanupExpired(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewSessionRepository(db, zap.NewNop())
	ctx := context.Background()

	// Create expired sessions
	expiredAt := time.Now().Add(-1 * time.Hour)
	_, err := repo.CreateSession(ctx, 1, "expired1", expiredAt, "", "")
	require.NoError(t, err)
	_, err = repo.CreateSession(ctx, 1, "expired2", expiredAt, "", "")
	require.NoError(t, err)

	// Create valid session
	validAt := time.Now().Add(24 * time.Hour)
	_, err = repo.CreateSession(ctx, 1, "valid1", validAt, "", "")
	require.NoError(t, err)

	// Cleanup expired
	deleted, err := repo.CleanupExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify valid session still exists
	session, _, _, err := repo.FindValidSession(ctx, "valid1")
	require.NoError(t, err)
	assert.NotNil(t, session)
}

func TestGenerateSessionToken(t *testing.T) {
	token1, err := GenerateSessionToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token1)

	token2, err := GenerateSessionToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token2)

	// Tokens should be unique
	assert.NotEqual(t, token1, token2)

	// Token should be base64 encoded (43 chars for 32 bytes)
	assert.GreaterOrEqual(t, len(token1), 40)
}

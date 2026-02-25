//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestAPIKeyRepository_FindByKeyHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		keyHash string
		wantNil bool
		wantErr bool
	}{
		{"existing key", "hash_admin_key_1", false, false},
		{"non-existing key", "nonexistent_hash", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := repo.FindByKeyHash(ctx, tt.keyHash)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, key)
				assert.Equal(t, tt.keyHash, key.KeyHash)
			}
		})
	}
}

func TestAPIKeyRepository_FindByID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		wantNil bool
		wantErr bool
	}{
		{"existing key", 1, false, false},
		{"non-existing key", 999, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := repo.FindByID(ctx, tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, key)
				assert.Equal(t, tt.id, key.ID)
			}
		})
	}
}

func TestAPIKeyRepository_FindByUserID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		userID    int64
		wantCount int
	}{
		{"admin keys", 1, 1},
		{"user keys", 2, 2},
		{"no keys", 3, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := repo.FindByUserID(ctx, tt.userID)
			require.NoError(t, err)
			assert.Len(t, keys, tt.wantCount)
			for _, k := range keys {
				assert.Equal(t, tt.userID, k.UserID)
			}
		})
	}
}

func TestAPIKeyRepository_FindAll(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	keys, err := repo.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, keys, 3) // All 3 keys
}

func TestAPIKeyRepository_Insert(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		key     *models.APIKey
		wantErr bool
	}{
		{
			name: "valid key",
			key: &models.APIKey{
				UserID:    1,
				KeyHash:   "new_hash_123",
				KeyFull:   "sk-new-full-key",
				KeyPrefix: "sk-new",
				Name:      "New Key",
				IsActive:  true,
			},
			wantErr: false,
		},
		{
			name: "key with expiration",
			key: &models.APIKey{
				UserID:    2,
				KeyHash:   "expiring_hash",
				KeyFull:   "sk-expiring-key",
				KeyPrefix: "sk-exp",
				Name:      "Expiring Key",
				IsActive:  true,
				ExpiresAt: func() *time.Time { t := time.Now().Add(24 * time.Hour); return &t }(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.Insert(ctx, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				// Verify insertion
				found, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, tt.key.Name, found.Name)
				assert.Equal(t, tt.key.KeyPrefix, found.KeyPrefix)
				assert.Equal(t, tt.key.IsActive, found.IsActive)
			}
		})
	}
}

func TestAPIKeyRepository_Insert_DuplicateHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	key := &models.APIKey{
		UserID:    1,
		KeyHash:   "hash_admin_key_1", // Already exists
		KeyFull:   "sk-duplicate",
		KeyPrefix: "sk-dup",
		Name:      "Duplicate Key",
		IsActive:  true,
	}

	_, err := repo.Insert(ctx, key)
	assert.Error(t, err) // Should fail due to UNIQUE constraint
}

func TestAPIKeyRepository_UpdateLastUsed(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	// Get key before update
	before, err := repo.FindByID(ctx, 1)
	require.NoError(t, err)
	assert.Nil(t, before.LastUsedAt)

	// Update last used
	err = repo.UpdateLastUsed(ctx, 1)
	require.NoError(t, err)

	// Verify update
	after, err := repo.FindByID(ctx, 1)
	require.NoError(t, err)
	assert.NotNil(t, after.LastUsedAt)
}

func TestAPIKeyRepository_Revoke(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	tests := []struct {
		name   string
		id     int64
		userID *int64
	}{
		{"revoke without user check", 1, nil},
		{"revoke with user check", 2, func() *int64 { id := int64(2); return &id }()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Revoke(ctx, tt.id, tt.userID)
			require.NoError(t, err)

			// Verify revocation
			key, err := repo.FindByID(ctx, tt.id)
			require.NoError(t, err)
			assert.False(t, key.IsActive)
		})
	}
}

func TestAPIKeyRepository_Delete(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	tests := []struct {
		name   string
		id     int64
		userID *int64
	}{
		{"delete without user check", 3, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Delete(ctx, tt.id, tt.userID)
			require.NoError(t, err)

			// Verify deletion
			_, err = repo.FindByID(ctx, tt.id)
			assert.Error(t, err)
		})
	}
}

func TestAPIKeyRepository_CleanupExpired(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	// Insert an expired key
	expired := time.Now().Add(-24 * time.Hour)
	expiredKey := &models.APIKey{
		UserID:    1,
		KeyHash:   "expired_hash",
		KeyFull:   "sk-expired",
		KeyPrefix: "sk-exp",
		Name:      "Expired Key",
		IsActive:  true,
		ExpiresAt: &expired,
	}
	_, err := repo.Insert(ctx, expiredKey)
	require.NoError(t, err)

	// Count before cleanup
	before, err := repo.FindAll(ctx)
	require.NoError(t, err)
	beforeCount := len(before)

	// Cleanup expired
	deleted, err := repo.CleanupExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Count after cleanup
	after, err := repo.FindAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, beforeCount-1, len(after))
}

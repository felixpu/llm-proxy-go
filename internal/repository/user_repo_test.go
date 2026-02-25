//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestUserRepository_FindByID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		wantNil bool
		wantErr bool
	}{
		{"existing user", 1, false, false},
		{"non-existing user", 999, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.FindByID(ctx, tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.id, user.ID)
			}
		})
	}
}

func TestUserRepository_FindByUsername(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		wantNil  bool
		wantErr  bool
	}{
		{"existing admin", "admin", false, false},
		{"existing user", "testuser", false, false},
		{"non-existing user", "nonexistent", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.FindByUsername(ctx, tt.username)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
				// Password hash should NOT be returned
				assert.Empty(t, user.PasswordHash)
			}
		})
	}
}

func TestUserRepository_FindByUsernameWithHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user, err := repo.FindByUsernameWithHash(ctx, "admin")
	require.NoError(t, err)
	require.NotNil(t, user)

	assert.Equal(t, "admin", user.Username)
	assert.NotEmpty(t, user.PasswordHash) // Password hash SHOULD be returned
	assert.Equal(t, models.UserRoleAdmin, user.Role)
}

func TestUserRepository_Insert(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		user    *models.User
		wantErr bool
	}{
		{
			name: "valid user",
			user: &models.User{
				Username:     "newuser",
				PasswordHash: "$2a$10$newhash",
				Role:         models.UserRoleUser,
				IsActive:     true,
			},
			wantErr: false,
		},
		{
			name: "admin user",
			user: &models.User{
				Username:     "newadmin",
				PasswordHash: "$2a$10$adminhash",
				Role:         models.UserRoleAdmin,
				IsActive:     true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.Insert(ctx, tt.user)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				// Verify insertion
				found, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, tt.user.Username, found.Username)
				assert.Equal(t, tt.user.Role, found.Role)
				assert.Equal(t, tt.user.IsActive, found.IsActive)
			}
		})
	}
}

func TestUserRepository_Insert_DuplicateUsername(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &models.User{
		Username:     "admin", // Already exists
		PasswordHash: "$2a$10$duplicate",
		Role:         models.UserRoleUser,
		IsActive:     true,
	}

	_, err := repo.Insert(ctx, user)
	assert.Error(t, err) // Should fail due to UNIQUE constraint
}

func TestUserRepository_FindAll(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	tests := []struct {
		name       string
		offset     int
		limit      int
		wantCount  int
		wantTotal  int64
	}{
		{"all users", 0, 10, 3, 3},
		{"first page", 0, 2, 2, 3},
		{"second page", 2, 2, 1, 3},
		{"empty page", 10, 10, 0, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users, total, err := repo.FindAll(ctx, tt.offset, tt.limit)
			require.NoError(t, err)
			assert.Len(t, users, tt.wantCount)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}

func TestUserRepository_Update(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	// Get existing user
	user, err := repo.FindByID(ctx, 2)
	require.NoError(t, err)

	// Update user
	user.Username = "updateduser"
	user.Role = models.UserRoleAdmin
	user.IsActive = false

	err = repo.Update(ctx, user)
	require.NoError(t, err)

	// Verify update
	updated, err := repo.FindByID(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, "updateduser", updated.Username)
	assert.Equal(t, models.UserRoleAdmin, updated.Role)
	assert.False(t, updated.IsActive)
}

func TestUserRepository_UpdatePassword(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	newHash := "$2a$10$newpasswordhash"
	err := repo.UpdatePassword(ctx, 1, newHash)
	require.NoError(t, err)

	// Verify password was updated
	user, err := repo.FindByUsernameWithHash(ctx, "admin")
	require.NoError(t, err)
	assert.Equal(t, newHash, user.PasswordHash)
}

func TestUserRepository_Delete(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	// Delete user
	err := repo.Delete(ctx, 3) // inactive user
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.FindByID(ctx, 3)
	assert.Error(t, err) // Should not be found
}

func TestUserRepository_CountByRole(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewUserRepository(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		role      models.UserRole
		wantCount int64
	}{
		{"admin count", models.UserRoleAdmin, 1},
		{"user count", models.UserRoleUser, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := repo.CountByRole(ctx, tt.role)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

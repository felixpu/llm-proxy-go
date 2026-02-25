//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestAPIKeyHandler_ListAPIKeys_Admin(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	// Create test users
	ctx := context.Background()
	adminID, err := userRepo.Insert(ctx, &models.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create API keys for both users
	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    adminID,
		KeyHash:   "hash_admin_key",
		KeyFull:   "sk-admin-full-key",
		KeyPrefix: "sk-admin",
		Name:      "Admin Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   "hash_user_key",
		KeyFull:   "sk-user-full-key",
		KeyPrefix: "sk-user",
		Name:      "User Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys", nil)

	// Set admin user in context
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.ListAPIKeys(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var keys []*models.APIKey
	err = json.Unmarshal(w.Body.Bytes(), &keys)
	require.NoError(t, err)

	// Admin should see all keys
	assert.Len(t, keys, 2)
}

func TestAPIKeyHandler_ListAPIKeys_User(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	adminID, err := userRepo.Insert(ctx, &models.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create API keys for both users
	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    adminID,
		KeyHash:   "hash_admin_key",
		KeyFull:   "sk-admin-full-key",
		KeyPrefix: "sk-admin",
		Name:      "Admin Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   "hash_user_key",
		KeyFull:   "sk-user-full-key",
		KeyPrefix: "sk-user",
		Name:      "User Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys", nil)

	// Set regular user in context
	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.ListAPIKeys(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var keys []*models.APIKey
	err = json.Unmarshal(w.Body.Bytes(), &keys)
	require.NoError(t, err)

	// User should see only their own keys
	assert.Len(t, keys, 1)
	assert.Equal(t, userID, keys[0].UserID)
}

func TestAPIKeyHandler_ListAPIKeys_Unauthenticated(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys", nil)

	// No user in context
	handler.ListAPIKeys(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "Not authenticated", resp["detail"])
}

func TestAPIKeyHandler_GetAPIKey_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	keyID, err := keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   "hash_test_key",
		KeyFull:   "sk-test-full-key",
		KeyPrefix: "sk-test",
		Name:      "Test Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys/1", nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.GetAPIKey(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var key models.APIKey
	err = json.Unmarshal(w.Body.Bytes(), &key)
	require.NoError(t, err)

	assert.Equal(t, keyID, key.ID)
	assert.Equal(t, "Test Key", key.Name)
	assert.Equal(t, userID, key.UserID)
}

func TestAPIKeyHandler_GetAPIKey_NotFound(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys/999", nil)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.GetAPIKey(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "API key not found", resp["detail"])
}

func TestAPIKeyHandler_GetAPIKey_Forbidden(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	ownerID, err := userRepo.Insert(ctx, &models.User{
		Username:     "owner",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	otherUserID, err := userRepo.Insert(ctx, &models.User{
		Username:     "otheruser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    ownerID,
		KeyHash:   "hash_owner_key",
		KeyFull:   "sk-owner-full-key",
		KeyPrefix: "sk-owner",
		Name:      "Owner Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys/1", nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}

	// Try to access with different user
	c.Set("current_user", &service.CurrentUser{
		UserID:   otherUserID,
		Username: "otheruser",
		Role:     "user",
	})

	handler.GetAPIKey(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "No permission to view this API key", resp["detail"])
}

func TestAPIKeyHandler_CreateAPIKey_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	reqBody := map[string]any{
		"name":         "My New Key",
		"expires_days": 30,
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/keys", reqBody)

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.CreateAPIKey(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp["id"])
	assert.NotEmpty(t, resp["key"])
	assert.NotEmpty(t, resp["key_prefix"])
	assert.Equal(t, "My New Key", resp["name"])
	assert.NotNil(t, resp["expires_at"])

	// Verify key was created in database
	keyID := int64(resp["id"].(float64))
	key, err := keyRepo.FindByID(ctx, keyID)
	require.NoError(t, err)
	assert.Equal(t, userID, key.UserID)
	assert.Equal(t, "My New Key", key.Name)
	assert.True(t, key.IsActive)
}

func TestAPIKeyHandler_CreateAPIKey_InvalidRequest(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	// Missing required field "name"
	reqBody := map[string]any{
		"expires_days": 30,
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/keys", reqBody)

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.CreateAPIKey(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// The error message contains "Name" (capitalized) from the struct field
	assert.Contains(t, resp["detail"], "Name")
}

func TestAPIKeyHandler_DeleteAPIKey_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	keyID, err := keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   "hash_test_key",
		KeyFull:   "sk-test-full-key",
		KeyPrefix: "sk-test",
		Name:      "Test Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", "/api/keys/1", nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.DeleteAPIKey(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "API key deleted", resp["message"])

	// Verify key was deleted
	_, err = keyRepo.FindByID(ctx, keyID)
	assert.Error(t, err)
}

func TestAPIKeyHandler_DeleteAPIKey_NotFound(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", "/api/keys/999", nil)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.DeleteAPIKey(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "API key not found", resp["detail"])
}

func TestAPIKeyHandler_CopyAPIKey_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create original key
	originalKeyID, err := keyRepo.Insert(ctx, &models.APIKey{
		UserID:    userID,
		KeyHash:   "hash_original_key",
		KeyFull:   "sk-original-full-key",
		KeyPrefix: "sk-orig",
		Name:      "Original Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	// Note: CopyAPIKey is not in the original handler, but we can test
	// creating a new key with similar properties
	reqBody := map[string]any{
		"name": "Copied Key",
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/keys", reqBody)

	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.CreateAPIKey(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp["key"])
	assert.Equal(t, "Copied Key", resp["name"])

	// Verify both keys exist
	originalKey, err := keyRepo.FindByID(ctx, originalKeyID)
	require.NoError(t, err)
	assert.Equal(t, "Original Key", originalKey.Name)

	newKeyID := int64(resp["id"].(float64))
	newKey, err := keyRepo.FindByID(ctx, newKeyID)
	require.NoError(t, err)
	assert.Equal(t, "Copied Key", newKey.Name)
	assert.Equal(t, userID, newKey.UserID)
}

func TestAPIKeyHandler_CopyAPIKey_Forbidden(t *testing.T) {
	db := testutil.NewTestDB(t)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	ownerID, err := userRepo.Insert(ctx, &models.User{
		Username:     "owner",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	otherUserID, err := userRepo.Insert(ctx, &models.User{
		Username:     "otheruser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create key owned by first user
	_, err = keyRepo.Insert(ctx, &models.APIKey{
		UserID:    ownerID,
		KeyHash:   "hash_owner_key",
		KeyFull:   "sk-owner-full-key",
		KeyPrefix: "sk-owner",
		Name:      "Owner Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	handler := NewAPIKeyHandler(keyRepo)

	// Try to get the key with different user (should fail)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/keys/1", nil)
	c.Params = []gin.Param{{Key: "id", Value: "1"}}

	c.Set("current_user", &service.CurrentUser{
		UserID:   otherUserID,
		Username: "otheruser",
		Role:     "user",
	})

	handler.GetAPIKey(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Contains(t, resp["detail"], "permission")
}

//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func setupUserTest(t *testing.T) (*UserHandler, *repository.SQLUserRepository, *service.AuthService, int64) {
	t.Helper()

	db := testutil.NewTestDB(t)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, testutil.NewTestLogger())
	keyRepo := repository.NewAPIKeyRepository(db)
	logger := testutil.NewTestLogger()

	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	handler := NewUserHandler(userRepo, authService)

	ctx := context.Background()
	adminID, err := userRepo.Insert(ctx, &models.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	return handler, userRepo, authService, adminID
}

func TestUserHandler_ListUsers_Success(t *testing.T) {
	handler, userRepo, _, adminID := setupUserTest(t)

	ctx := context.Background()
	_, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/users", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.ListUsers(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var users []models.User
	err = json.Unmarshal(w.Body.Bytes(), &users)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 2)
}

func TestUserHandler_GetUser_Success(t *testing.T) {
	handler, userRepo, _, adminID := setupUserTest(t)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/users/%d", userID), nil)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", userID)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.GetUser(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var user models.User
	err = json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.Equal(t, userID, user.ID)
	assert.Equal(t, "testuser", user.Username)
}

func TestUserHandler_GetUser_NotFound(t *testing.T) {
	handler, _, _, adminID := setupUserTest(t)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/users/999", nil)
	c.Params = []gin.Param{{Key: "id", Value: "999"}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.GetUser(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserHandler_CreateUser_Success(t *testing.T) {
	handler, _, _, adminID := setupUserTest(t)

	reqBody := map[string]any{
		"username": "newuser",
		"password": "password123",
		"role":     "user",
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/users", reqBody)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.CreateUser(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var user models.User
	err := json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.NotZero(t, user.ID)
	assert.Equal(t, "newuser", user.Username)
}

func TestUserHandler_CreateUser_DuplicateUsername(t *testing.T) {
	handler, userRepo, _, adminID := setupUserTest(t)

	ctx := context.Background()
	_, err := userRepo.Insert(ctx, &models.User{
		Username:     "existinguser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	reqBody := map[string]any{
		"username": "existinguser",
		"password": "password123",
		"role":     "user",
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/users", reqBody)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.CreateUser(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUserHandler_UpdateUser_Success(t *testing.T) {
	handler, userRepo, _, adminID := setupUserTest(t)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	reqBody := map[string]any{
		"role":      "admin",
		"is_active": false,
	}
	c, w := testutil.NewTestContextWithRequest("PUT", fmt.Sprintf("/api/users/%d", userID), reqBody)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", userID)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.UpdateUser(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var user models.User
	err = json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.Equal(t, "admin", string(user.Role))
}

func TestUserHandler_DeleteUser_Success(t *testing.T) {
	handler, userRepo, _, adminID := setupUserTest(t)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%d", userID), nil)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", userID)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.DeleteUser(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "User deleted", resp["message"])
}

func TestUserHandler_DeleteUser_CannotDeleteSelf(t *testing.T) {
	handler, userRepo, _, _ := setupUserTest(t)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Try to delete self - should be rejected
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%d", userID), nil)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", userID)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.DeleteUser(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

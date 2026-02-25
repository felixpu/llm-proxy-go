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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestAuthHandler_Login_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	// Create repositories
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)

	// Create auth service
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	// Create test user with known password
	password := "password123"
	passwordHash, err := service.HashPassword(password)
	require.NoError(t, err)

	_, err = userRepo.Insert(context.Background(), &models.User{
		Username:     "admin",
		PasswordHash: passwordHash,
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewAuthHandler(authService, logger)

	reqBody := map[string]string{
		"username": "admin",
		"password": password,
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, true, resp["success"])
	assert.NotEmpty(t, resp["token"])
	assert.NotNil(t, resp["user"])

	user := resp["user"].(map[string]any)
	assert.Equal(t, "admin", user["username"])
	assert.Equal(t, "admin", user["role"])
	assert.Equal(t, true, user["is_active"])
}

func TestAuthHandler_Login_InvalidRequest(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	handler := NewAuthHandler(authService, logger)

	// Missing password
	reqBody := map[string]string{
		"username": "admin",
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, false, resp["success"])
	assert.Contains(t, resp["message"], "Invalid request")
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	// Create test user with known password
	password := "correctpassword"
	passwordHash, err := service.HashPassword(password)
	require.NoError(t, err)

	_, err = userRepo.Insert(context.Background(), &models.User{
		Username:     "admin",
		PasswordHash: passwordHash,
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewAuthHandler(authService, logger)

	reqBody := map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	}
	c, w := testutil.NewTestContextWithRequest("POST", "/api/auth/login", reqBody)

	handler.Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, false, resp["success"])
	assert.Equal(t, "用户名或密码错误", resp["message"])
}

func TestAuthHandler_Logout_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	// Create test user
	userID, err := userRepo.Insert(context.Background(), &models.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create session
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	_, err = sessionRepo.CreateSession(context.Background(), userID, "test-token", expiresAt, "127.0.0.1", "test-agent")
	require.NoError(t, err)

	handler := NewAuthHandler(authService, logger)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/auth/logout", nil)
	c.Request.AddCookie(&http.Cookie{
		Name:  "session_token",
		Value: "test-token",
	})

	handler.Logout(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, true, resp["success"])
	assert.Equal(t, "已登出", resp["message"])
}

func TestAuthHandler_Logout_NoToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	handler := NewAuthHandler(authService, logger)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/auth/logout", nil)

	handler.Logout(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, true, resp["success"])
	assert.Equal(t, "已登出", resp["message"])
}

func TestAuthHandler_GetMe_Authenticated(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	handler := NewAuthHandler(authService, logger)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/auth/me", nil)

	// Set current user in context
	c.Set("current_user", &service.CurrentUser{
		UserID:   1,
		Username: "admin",
		Role:     "admin",
	})

	handler.GetMe(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp service.CurrentUser
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(1), resp.UserID)
	assert.Equal(t, "admin", resp.Username)
	assert.Equal(t, "admin", resp.Role)
}

func TestAuthHandler_GetMe_Unauthenticated(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	handler := NewAuthHandler(authService, logger)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/auth/me", nil)

	handler.GetMe(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "error", resp["type"])
	errorObj := resp["error"].(map[string]any)
	assert.Equal(t, "authentication_error", errorObj["type"])
	assert.Equal(t, "Not authenticated", errorObj["message"])
}

func TestAuthHandler_Refresh_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	// Create test user
	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	// Create old session
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	_, err = sessionRepo.CreateSession(ctx, userID, "old-token", expiresAt, "127.0.0.1", "test-agent")
	require.NoError(t, err)

	handler := NewAuthHandler(authService, logger)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/auth/refresh", nil)
	c.Request.AddCookie(&http.Cookie{
		Name:  "session_token",
		Value: "old-token",
	})

	// Set current user in context
	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "admin",
		Role:     "admin",
	})

	handler.Refresh(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, true, resp["success"])
	assert.NotEmpty(t, resp["token"])
	assert.NotEmpty(t, resp["expires_at"])
}

func TestAuthHandler_Refresh_Unauthenticated(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := testutil.NewTestLogger()

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db, logger)
	keyRepo := repository.NewAPIKeyRepository(db)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)

	handler := NewAuthHandler(authService, logger)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/auth/refresh", nil)

	handler.Refresh(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, false, resp["success"])
	assert.Equal(t, "Not authenticated", resp["message"])
}

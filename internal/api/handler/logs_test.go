//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestLogsHandler_GetRequestLogs_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())
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

	// Insert test logs
	_, err = logRepo.Insert(ctx, &models.RequestLogEntry{
		RequestID:    "req_test_1",
		UserID:       userID,
		ModelName:    "claude-sonnet-4",
		EndpointName: "anthropic-primary",
		TaskType:     "default",
		InputTokens:  100,
		OutputTokens: 50,
		LatencyMs:    150.5,
		Cost:         0.001,
		Success:      true,
		Stream:       false,
	})
	require.NoError(t, err)

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/logs?limit=100&offset=0", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.GetRequestLogs(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["logs"])
	assert.NotNil(t, resp["total"])

	// Verify actual data returned (not just non-nil)
	total := int(resp["total"].(float64))
	assert.Equal(t, 1, total, "should return the inserted log entry")

	logs := resp["logs"].([]any)
	assert.Len(t, logs, 1, "logs array should contain 1 entry")
}

func TestLogsHandler_GetRequestLogs_Forbidden(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/logs", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.GetRequestLogs(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestLogsHandler_GetRequestLogs_Unauthenticated(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/logs", nil)

	handler.GetRequestLogs(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestLogsHandler_DeleteRequestLogs_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())
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

	// Insert test logs
	_, err = logRepo.Insert(ctx, &models.RequestLogEntry{
		RequestID:    "req_test_1",
		UserID:       userID,
		ModelName:    "claude-sonnet-4",
		EndpointName: "anthropic-primary",
		TaskType:     "default",
		InputTokens:  100,
		OutputTokens: 50,
		LatencyMs:    150.5,
		Cost:         0.001,
		Success:      true,
		Stream:       false,
	})
	require.NoError(t, err)

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", "/api/logs", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.DeleteRequestLogs(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["deleted"])
}

func TestLogsHandler_DeleteRequestLogs_Forbidden(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", "/api/logs", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.DeleteRequestLogs(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestLogsHandler_GetLogStats_Success(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())
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

	// Insert test logs
	_, err = logRepo.Insert(ctx, &models.RequestLogEntry{
		RequestID:    "req_test_1",
		UserID:       userID,
		ModelName:    "claude-sonnet-4",
		EndpointName: "anthropic-primary",
		TaskType:     "default",
		InputTokens:  100,
		OutputTokens: 50,
		LatencyMs:    150.5,
		Cost:         0.001,
		Success:      true,
		Stream:       false,
	})
	require.NoError(t, err)

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/logs/stats", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     "admin",
	})

	handler.GetLogStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp)

	// Verify statistics reflect the inserted log
	if totalReqs, ok := resp["total_requests"]; ok {
		assert.Equal(t, float64(1), totalReqs.(float64), "stats should reflect 1 request")
	}
}

func TestLogsHandler_GetLogStats_Forbidden(t *testing.T) {
	db := testutil.NewTestDB(t)
	logRepo := repository.NewRequestLogRepositoryImpl(db, testutil.NewTestLogger())
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	userID, err := userRepo.Insert(ctx, &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleUser,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewLogsHandler(logRepo, testutil.NewTestLogger())

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/logs/stats", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   userID,
		Username: "testuser",
		Role:     "user",
	})

	handler.GetLogStats(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}


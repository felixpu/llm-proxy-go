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

func TestLogsHandler_GetLogStats_CacheMetrics(t *testing.T) {
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

	_, err = logRepo.Insert(ctx, &models.RequestLogEntry{
		RequestID:    "req_stats_cache_1",
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
	_, err = logRepo.Insert(ctx, &models.RequestLogEntry{
		RequestID:            "req_stats_cache_2",
		UserID:               userID,
		ModelName:            "claude-sonnet-4",
		EndpointName:         "anthropic-primary",
		TaskType:             "default",
		InputTokens:          120,
		OutputTokens:         30,
		LatencyMs:            100.0,
		Cost:                 0.001,
		Success:              true,
		Stream:               false,
		CacheReadInputTokens: 200,
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
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, float64(2), resp["total_requests"])
	assert.Equal(t, float64(50), resp["cache_hit_rate"])
	assert.Equal(t, float64(200), resp["total_cache_read_tokens"])
}

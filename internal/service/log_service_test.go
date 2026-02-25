//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestNewLogService(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	ls := NewLogService(db, logger)
	require.NotNil(t, ls)
	assert.NotNil(t, ls.repo)
	assert.NotNil(t, ls.logChan)
	assert.Equal(t, 100, ls.batchSize)
	assert.Equal(t, 5*time.Second, ls.flushInterval)

	ls.Stop()
}

func TestLogService_LogRequest_NoBlock(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	ls := NewLogService(db, logger)

	entry := &models.RequestLogEntry{
		UserID:       1,
		ModelName:    "claude-3-sonnet",
		EndpointName: "anthropic/claude-3-sonnet",
		InputTokens:  100,
		OutputTokens: 50,
		Cost:         0.001,
		LatencyMs:    1500.0,
		Success:      true,
		TaskType:     "default",
	}

	// LogRequest should not block or panic
	ls.LogRequest(entry)

	ls.Stop()
}

// seedLogUser ensures a test user exists for foreign key constraints
func seedLogUser(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `INSERT OR IGNORE INTO users (id, username, password_hash, role, is_active, created_at, updated_at)
		VALUES (1, 'testuser', 'hash', 'user', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
}

// seedLogs inserts log entries synchronously via repo for deterministic testing
func seedLogs(t *testing.T, db *sql.DB, repo *repository.RequestLogRepositoryImpl, count int) {
	t.Helper()
	seedLogUser(t, db)
	ctx := context.Background()

	for i := range count {
		entry := &models.RequestLogEntry{
			RequestID:    fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), i),
			UserID:       1,
			ModelName:    "claude-3-sonnet",
			EndpointName: "anthropic/claude-3-sonnet",
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         0.001,
			LatencyMs:    1500.0,
			Success:      true,
			TaskType:     "default",
		}
		_, err := repo.Insert(ctx, entry)
		require.NoError(t, err)
	}
}

func TestLogService_ListLogs(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	repo := repository.NewRequestLogRepositoryImpl(db, logger)
	ctx := context.Background()

	// Insert entries synchronously
	seedLogs(t, db, repo, 5)

	ls := NewLogService(db, logger)
	defer ls.Stop()

	logs, total, err := ls.ListLogs(ctx, &LogFilters{Limit: 10, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, logs, 5)
}

func TestLogService_ListLogs_WithFilters(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	repo := repository.NewRequestLogRepositoryImpl(db, logger)
	ctx := context.Background()

	// Insert entries with different models
	seedLogUser(t, db)
	modelNames := []string{"claude-3-haiku", "claude-3-sonnet", "claude-3-opus"}
	for i, modelName := range modelNames {
		_, err := repo.Insert(ctx, &models.RequestLogEntry{
			RequestID:    fmt.Sprintf("req-filter-%d", i),
			UserID:       1,
			ModelName:    modelName,
			EndpointName: "anthropic/" + modelName,
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         0.001,
			LatencyMs:    1500.0,
			Success:      true,
			TaskType:     "default",
		})
		require.NoError(t, err)
	}

	ls := NewLogService(db, logger)
	defer ls.Stop()

	// Filter by model
	modelFilter := "claude-3-sonnet"
	logs, total, err := ls.ListLogs(ctx, &LogFilters{
		Limit:     10,
		ModelName: &modelFilter,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, "claude-3-sonnet", logs[0].ModelName)
}

func TestLogService_GetStatistics(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	repo := repository.NewRequestLogRepositoryImpl(db, logger)
	ctx := context.Background()

	// Insert entries: 2 success, 1 failure
	seedLogUser(t, db)
	for i := range 3 {
		_, err := repo.Insert(ctx, &models.RequestLogEntry{
			RequestID:    fmt.Sprintf("req-stats-%d", i),
			UserID:       1,
			ModelName:    "claude-3-sonnet",
			EndpointName: "anthropic/claude-3-sonnet",
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         0.001,
			LatencyMs:    1500.0,
			Success:      i < 2,
			TaskType:     "default",
		})
		require.NoError(t, err)
	}

	ls := NewLogService(db, logger)
	defer ls.Stop()

	stats, err := ls.GetStatistics(ctx, &LogFilters{})
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(3), stats.TotalRequests)
}

func TestLogService_CountLogs(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	repo := repository.NewRequestLogRepositoryImpl(db, logger)
	ctx := context.Background()

	seedLogs(t, db, repo, 3)

	ls := NewLogService(db, logger)
	defer ls.Stop()

	count, err := ls.CountLogs(ctx, &LogFilters{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestLogService_DeleteLogs(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	repo := repository.NewRequestLogRepositoryImpl(db, logger)
	ctx := context.Background()

	seedLogs(t, db, repo, 3)

	ls := NewLogService(db, logger)
	defer ls.Stop()

	deleted, err := ls.DeleteLogs(ctx, &LogFilters{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	// Verify deletion
	count, err := ls.CountLogs(ctx, &LogFilters{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name         string
		model        *models.Model
		inputTokens  int
		outputTokens int
		wantCost     float64
	}{
		{
			name: "basic calculation",
			model: &models.Model{
				CostPerMtokInput:  3.0,
				CostPerMtokOutput: 15.0,
				BillingMultiplier: 1.0,
			},
			inputTokens:  1000,
			outputTokens: 500,
			wantCost:     0.0105,
		},
		{
			name: "with billing multiplier",
			model: &models.Model{
				CostPerMtokInput:  3.0,
				CostPerMtokOutput: 15.0,
				BillingMultiplier: 2.0,
			},
			inputTokens:  1000,
			outputTokens: 500,
			wantCost:     0.018,
		},
		{
			name: "zero tokens",
			model: &models.Model{
				CostPerMtokInput:  3.0,
				CostPerMtokOutput: 15.0,
				BillingMultiplier: 1.0,
			},
			inputTokens:  0,
			outputTokens: 0,
			wantCost:     0,
		},
		{
			name: "large token count",
			model: &models.Model{
				CostPerMtokInput:  3.0,
				CostPerMtokOutput: 15.0,
				BillingMultiplier: 1.0,
			},
			inputTokens:  100000,
			outputTokens: 50000,
			wantCost:     1.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := CalculateCost(tt.model, tt.inputTokens, tt.outputTokens)
			assert.InDelta(t, tt.wantCost, cost, 0.000001)
		})
	}
}

func TestLogRoundTo(t *testing.T) {
	tests := []struct {
		name   string
		val    float64
		places int
		want   float64
	}{
		{"round to 2 places", 1.2345, 2, 1.23},
		{"round to 4 places", 1.23456789, 4, 1.2346},
		{"round to 6 places", 0.0000001234, 6, 0},
		{"round up", 1.235, 2, 1.24},
		{"zero", 0, 2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logRoundTo(tt.val, tt.places)
			assert.InDelta(t, tt.want, result, 0.0000001)
		})
	}
}

func TestLogFilters(t *testing.T) {
	userID := int64(1)
	modelName := "claude-3-sonnet"
	success := true
	now := time.Now()

	filters := &LogFilters{
		Limit:        10,
		Offset:       5,
		UserID:       &userID,
		ModelName:    &modelName,
		EndpointName: nil,
		StartTime:    &now,
		EndTime:      nil,
		Success:      &success,
	}

	assert.Equal(t, 10, filters.Limit)
	assert.Equal(t, 5, filters.Offset)
	assert.Equal(t, int64(1), *filters.UserID)
	assert.Equal(t, "claude-3-sonnet", *filters.ModelName)
	assert.Nil(t, filters.EndpointName)
	assert.True(t, *filters.Success)
}

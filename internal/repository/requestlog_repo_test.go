//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestRequestLogRepository_Insert(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	entry := testutil.SampleRequestLogEntry(1)
	id, err := repo.Insert(ctx, entry)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestRequestLogRepository_Insert_Multiple(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	entries := []*models.RequestLogEntry{
		{RequestID: "req_1", UserID: 1, ModelName: "claude-sonnet-4", EndpointName: "ep1", TaskType: "default", InputTokens: 100, OutputTokens: 50, LatencyMs: 100, Cost: 0.001, Success: true},
		{RequestID: "req_2", UserID: 1, ModelName: "claude-3-haiku", EndpointName: "ep1", TaskType: "simple", InputTokens: 50, OutputTokens: 20, LatencyMs: 50, Cost: 0.0005, Success: true},
		{RequestID: "req_3", UserID: 2, ModelName: "claude-sonnet-4", EndpointName: "ep2", TaskType: "default", InputTokens: 200, OutputTokens: 100, LatencyMs: 200, Cost: 0.002, Success: false},
	}

	for _, e := range entries {
		id, err := repo.Insert(ctx, e)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))
	}

	// Verify count
	count, err := repo.Count(ctx, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestRequestLogRepository_List(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	// Seed logs
	seedRequestLogs(t, db, repo)

	tests := []struct {
		name      string
		limit     int
		offset    int
		userID    *int64
		modelName *string
		wantCount int
		wantTotal int64
	}{
		{"all logs", 10, 0, nil, nil, 3, 3},
		{"paginated first", 2, 0, nil, nil, 2, 3},
		{"paginated second", 2, 2, nil, nil, 1, 3},
		{"filter by user", 10, 0, ptrInt64(1), nil, 2, 2},
		{"filter by model", 10, 0, nil, ptrStr("claude-3-haiku"), 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := repo.List(ctx, tt.limit, tt.offset, tt.userID, tt.modelName, nil, nil, nil, nil)
			require.NoError(t, err)
			assert.Len(t, logs, tt.wantCount)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}

func TestRequestLogRepository_GetStatistics(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	seedRequestLogs(t, db, repo)

	stats, err := repo.GetStatistics(ctx, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, stats)

	assert.Equal(t, int64(3), stats.TotalRequests)
	assert.Greater(t, stats.TotalCost, 0.0)
	assert.Greater(t, stats.AvgLatency, 0.0)
	assert.NotEmpty(t, stats.ByModel)
	assert.NotEmpty(t, stats.ByEndpoint)
}

func TestRequestLogRepository_Count(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	seedRequestLogs(t, db, repo)

	tests := []struct {
		name      string
		modelName *string
		wantCount int64
	}{
		{"all logs", nil, 3},
		{"by model", ptrStr("claude-sonnet-4"), 2},
		{"non-existing model", ptrStr("nonexistent"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := repo.Count(ctx, tt.modelName, nil, nil, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

func TestRequestLogRepository_Delete(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	seedRequestLogs(t, db, repo)

	// Delete by model name
	modelName := "claude-3-haiku"
	deleted, err := repo.Delete(ctx, &modelName, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify remaining
	count, err := repo.Count(ctx, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestRequestLogRepository_Delete_All(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRequestLogRepositoryImpl(db, zap.NewNop())
	ctx := context.Background()

	seedRequestLogs(t, db, repo)

	deleted, err := repo.Delete(ctx, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	count, err := repo.Count(ctx, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func seedRequestLogs(t *testing.T, db *sql.DB, repo *RequestLogRepositoryImpl) {
	t.Helper()
	ctx := context.Background()

	entries := []*models.RequestLogEntry{
		{RequestID: "req_1", UserID: 1, ModelName: "claude-sonnet-4", EndpointName: "ep1", TaskType: "default", InputTokens: 100, OutputTokens: 50, LatencyMs: 100, Cost: 0.001, Success: true},
		{RequestID: "req_2", UserID: 1, ModelName: "claude-3-haiku", EndpointName: "ep1", TaskType: "simple", InputTokens: 50, OutputTokens: 20, LatencyMs: 50, Cost: 0.0005, Success: true},
		{RequestID: "req_3", UserID: 2, ModelName: "claude-sonnet-4", EndpointName: "ep2", TaskType: "default", InputTokens: 200, OutputTokens: 100, LatencyMs: 200, Cost: 0.002, Success: false},
	}

	for _, e := range entries {
		_, err := repo.Insert(ctx, e)
		require.NoError(t, err)
	}
}

// Helper functions
func ptrInt64(v int64) *int64 { return &v }
func ptrStr(v string) *string { return &v }

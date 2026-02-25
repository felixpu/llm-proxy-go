//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestWorkerRegistryRepository_Register(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	id, err := repo.Register(ctx, "worker_1", 12345)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	// Verify registration
	workers, err := repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	assert.Len(t, workers, 1)
	assert.Equal(t, "worker_1", workers[0].WorkerID)
	assert.Equal(t, 12345, workers[0].PID)
	assert.False(t, workers[0].IsPrimary)
}

func TestWorkerRegistryRepository_Register_Multiple(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	_, err := repo.Register(ctx, "worker_1", 1001)
	require.NoError(t, err)
	_, err = repo.Register(ctx, "worker_2", 1002)
	require.NoError(t, err)
	_, err = repo.Register(ctx, "worker_3", 1003)
	require.NoError(t, err)

	workers, err := repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	assert.Len(t, workers, 3)
}

func TestWorkerRegistryRepository_TryBecomePrimary(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	// Register workers
	_, err := repo.Register(ctx, "worker_1", 1001)
	require.NoError(t, err)
	_, err = repo.Register(ctx, "worker_2", 1002)
	require.NoError(t, err)

	// First worker becomes primary
	became, err := repo.TryBecomePrimary(ctx, "worker_1")
	require.NoError(t, err)
	assert.True(t, became)

	// Second worker cannot become primary
	became, err = repo.TryBecomePrimary(ctx, "worker_2")
	require.NoError(t, err)
	assert.False(t, became)

	// First worker is still primary
	isPrimary, err := repo.IsPrimary(ctx, "worker_1")
	require.NoError(t, err)
	assert.True(t, isPrimary)
}

func TestWorkerRegistryRepository_TryBecomePrimary_AlreadyPrimary(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	_, err := repo.Register(ctx, "worker_1", 1001)
	require.NoError(t, err)

	// Become primary
	became, err := repo.TryBecomePrimary(ctx, "worker_1")
	require.NoError(t, err)
	assert.True(t, became)

	// Try again - should return true (already primary)
	became, err = repo.TryBecomePrimary(ctx, "worker_1")
	require.NoError(t, err)
	assert.True(t, became)
}

func TestWorkerRegistryRepository_IsPrimary(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	// Non-existing worker
	isPrimary, err := repo.IsPrimary(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, isPrimary)

	// Register and check
	_, err = repo.Register(ctx, "worker_1", 1001)
	require.NoError(t, err)

	isPrimary, err = repo.IsPrimary(ctx, "worker_1")
	require.NoError(t, err)
	assert.False(t, isPrimary)

	// Become primary
	_, err = repo.TryBecomePrimary(ctx, "worker_1")
	require.NoError(t, err)

	isPrimary, err = repo.IsPrimary(ctx, "worker_1")
	require.NoError(t, err)
	assert.True(t, isPrimary)
}

func TestWorkerRegistryRepository_UpdateHeartbeat(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	_, err := repo.Register(ctx, "worker_1", 1001)
	require.NoError(t, err)

	// Get initial heartbeat
	workers, err := repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	initialHeartbeat := workers[0].LastHeartbeat

	// Update heartbeat
	err = repo.UpdateHeartbeat(ctx, "worker_1")
	require.NoError(t, err)

	// Verify heartbeat updated
	workers, err = repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	assert.True(t, workers[0].LastHeartbeat.After(initialHeartbeat) || workers[0].LastHeartbeat.Equal(initialHeartbeat))
}

func TestWorkerRegistryRepository_Unregister(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	_, err := repo.Register(ctx, "worker_1", 1001)
	require.NoError(t, err)
	_, err = repo.Register(ctx, "worker_2", 1002)
	require.NoError(t, err)

	// Unregister worker_1
	err = repo.Unregister(ctx, "worker_1")
	require.NoError(t, err)

	// Verify
	workers, err := repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	assert.Len(t, workers, 1)
	assert.Equal(t, "worker_2", workers[0].WorkerID)
}

func TestWorkerRegistryRepository_Unregister_NonExisting(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	// Should not error
	err := repo.Unregister(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestWorkerRegistryRepository_GetAllWorkers(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewWorkerRegistryRepository(db, zap.NewNop())
	ctx := context.Background()

	// Empty
	workers, err := repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	assert.Empty(t, workers)

	// Add workers
	_, err = repo.Register(ctx, "worker_a", 1001)
	require.NoError(t, err)
	_, err = repo.Register(ctx, "worker_b", 1002)
	require.NoError(t, err)

	workers, err = repo.GetAllWorkers(ctx)
	require.NoError(t, err)
	assert.Len(t, workers, 2)
}

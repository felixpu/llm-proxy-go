//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestNewWorkerCoordinator(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	wc := NewWorkerCoordinator(db, logger)
	require.NotNil(t, wc)
	assert.NotEmpty(t, wc.WorkerID())
	assert.Greater(t, wc.PID(), 0)
	assert.False(t, wc.IsPrimary())
}

func TestWorkerCoordinator_Register(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)

	err := wc.Register(ctx)
	require.NoError(t, err)

	// After registration, should have attempted primary election
	// First worker should become primary
	assert.True(t, wc.IsPrimary())
}

func TestWorkerCoordinator_Register_MultiplePrimaryElection(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc1 := NewWorkerCoordinator(db, logger)
	wc2 := NewWorkerCoordinator(db, logger)

	// Register first worker
	err := wc1.Register(ctx)
	require.NoError(t, err)
	assert.True(t, wc1.IsPrimary())

	// Register second worker
	err = wc2.Register(ctx)
	require.NoError(t, err)
	assert.False(t, wc2.IsPrimary()) // Should not become primary
}

func TestWorkerCoordinator_WorkerID(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	wc1 := NewWorkerCoordinator(db, logger)
	wc2 := NewWorkerCoordinator(db, logger)

	// Each coordinator should have a unique ID
	assert.NotEqual(t, wc1.WorkerID(), wc2.WorkerID())
}

func TestWorkerCoordinator_PID(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	wc := NewWorkerCoordinator(db, logger)

	// PID should be the current process ID
	assert.Greater(t, wc.PID(), 0)
}

func TestWorkerCoordinator_SharedState(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)
	err := wc.Register(ctx)
	require.NoError(t, err)

	// Set shared state
	err = wc.SetSharedState(ctx, "test_key", map[string]string{"hello": "world"})
	require.NoError(t, err)

	// Get shared state
	var result map[string]string
	err = wc.GetSharedState(ctx, "test_key", &result)
	require.NoError(t, err)
	assert.Equal(t, "world", result["hello"])
}

func TestWorkerCoordinator_SharedState_NonExisting(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)
	err := wc.Register(ctx)
	require.NoError(t, err)

	// Get non-existing key should not error
	var result map[string]string
	err = wc.GetSharedState(ctx, "nonexistent", &result)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestWorkerCoordinator_SharedState_Overwrite(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)
	err := wc.Register(ctx)
	require.NoError(t, err)

	// Set initial value
	err = wc.SetSharedState(ctx, "counter", 1)
	require.NoError(t, err)

	// Overwrite
	err = wc.SetSharedState(ctx, "counter", 42)
	require.NoError(t, err)

	// Verify overwritten value
	var result int
	err = wc.GetSharedState(ctx, "counter", &result)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestWorkerCoordinator_Unregister(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)
	err := wc.Register(ctx)
	require.NoError(t, err)

	// Unregister
	err = wc.Unregister(ctx)
	assert.NoError(t, err)
}

func TestWorkerCoordinator_Stop(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)
	err := wc.Register(ctx)
	require.NoError(t, err)

	// Start the coordinator
	wc.Start(ctx)

	// Stop should not hang
	wc.Stop()
}

func TestWorkerCoordinator_Stop_NotRunning(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	wc := NewWorkerCoordinator(db, logger)

	// Stop without starting should not panic
	wc.Stop()
}

func TestWorkerCoordinator_StartTwice(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	wc := NewWorkerCoordinator(db, logger)
	err := wc.Register(ctx)
	require.NoError(t, err)

	// Start twice should be idempotent
	wc.Start(ctx)
	wc.Start(ctx) // Should return immediately

	wc.Stop()
}

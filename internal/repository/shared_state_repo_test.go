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

func TestSharedStateRepository_SetState(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewSharedStateRepository(db, zap.NewNop())
	ctx := context.Background()

	err := repo.SetState(ctx, "test_key", "test_value", "worker_1")
	require.NoError(t, err)

	// Verify
	state, err := repo.GetState(ctx, "test_key")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "test_key", state.Key)
	assert.Equal(t, "test_value", state.Value)
	assert.Equal(t, "worker_1", state.UpdatedBy)
}

func TestSharedStateRepository_SetState_Upsert(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewSharedStateRepository(db, zap.NewNop())
	ctx := context.Background()

	// Insert
	err := repo.SetState(ctx, "key1", "value1", "worker_1")
	require.NoError(t, err)

	// Update (upsert)
	err = repo.SetState(ctx, "key1", "value2", "worker_2")
	require.NoError(t, err)

	// Verify updated
	state, err := repo.GetState(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value2", state.Value)
	assert.Equal(t, "worker_2", state.UpdatedBy)
}

func TestSharedStateRepository_GetState(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewSharedStateRepository(db, zap.NewNop())
	ctx := context.Background()

	// Non-existing key
	state, err := repo.GetState(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, state)

	// Insert and get
	err = repo.SetState(ctx, "existing", "value", "worker")
	require.NoError(t, err)

	state, err = repo.GetState(ctx, "existing")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "existing", state.Key)
	assert.Equal(t, "value", state.Value)
}

func TestSharedStateRepository_DeleteState(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewSharedStateRepository(db, zap.NewNop())
	ctx := context.Background()

	// Insert
	err := repo.SetState(ctx, "to_delete", "value", "worker")
	require.NoError(t, err)

	// Delete
	err = repo.DeleteState(ctx, "to_delete")
	require.NoError(t, err)

	// Verify deleted
	state, err := repo.GetState(ctx, "to_delete")
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestSharedStateRepository_DeleteState_NonExisting(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewSharedStateRepository(db, zap.NewNop())
	ctx := context.Background()

	// Delete non-existing should not error
	err := repo.DeleteState(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestSharedStateRepository_GetAllStates(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewSharedStateRepository(db, zap.NewNop())
	ctx := context.Background()

	// Empty
	states, err := repo.GetAllStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, states)

	// Insert multiple
	err = repo.SetState(ctx, "key_a", "value_a", "worker")
	require.NoError(t, err)
	err = repo.SetState(ctx, "key_b", "value_b", "worker")
	require.NoError(t, err)
	err = repo.SetState(ctx, "key_c", "value_c", "worker")
	require.NoError(t, err)

	// Get all
	states, err = repo.GetAllStates(ctx)
	require.NoError(t, err)
	assert.Len(t, states, 3)

	// Should be ordered by key
	assert.Equal(t, "key_a", states[0].Key)
	assert.Equal(t, "key_b", states[1].Key)
	assert.Equal(t, "key_c", states[2].Key)
}

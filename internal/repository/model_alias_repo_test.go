//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestModelAliasRepository_CRUD(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelAliasRepository(db)
	ctx := context.Background()

	id, err := repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 2,
		Enabled:       true,
	})
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	alias, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", alias.AliasName)
	assert.Equal(t, int64(2), alias.TargetModelID)
	assert.True(t, alias.Enabled)

	byName, err := repo.FindByAliasName(ctx, "Claude-Sonnet-4-6")
	require.NoError(t, err)
	require.NotNil(t, byName)
	assert.Equal(t, id, byName.ID)

	newTarget := int64(3)
	enabled := false
	err = repo.UpdatePatch(ctx, id, ModelAliasPatch{
		TargetModelID: &newTarget,
		Enabled:       &enabled,
	})
	require.NoError(t, err)

	updated, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, int64(3), updated.TargetModelID)
	assert.False(t, updated.Enabled)

	byName, err = repo.FindByAliasName(ctx, "claude-sonnet-4-6")
	require.NoError(t, err)
	assert.Nil(t, byName)

	err = repo.Delete(ctx, id)
	require.NoError(t, err)
	_, err = repo.FindByID(ctx, id)
	assert.Error(t, err)
}

func TestModelAliasRepository_ReadsUseReadDB(t *testing.T) {
	db, readDB := testutil.NewFileBackedTestDBPair(t)
	testutil.SeedTestData(t, db)
	repo := NewModelAliasRepository(db, readDB)
	ctx := context.Background()

	id, err := repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 2,
		Enabled:       true,
	})
	require.NoError(t, err)

	require.NoError(t, db.Close())

	alias, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, alias)
	assert.Equal(t, "claude-sonnet-4-6", alias.AliasName)
}

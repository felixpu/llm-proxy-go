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

	byNameList, err := repo.FindByAliasName(ctx, "Claude-Sonnet-4-6")
	require.NoError(t, err)
	require.NotNil(t, byNameList)
	require.Len(t, byNameList, 1)
	assert.Equal(t, id, byNameList[0].ID)

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

	byNameList, err = repo.FindByAliasName(ctx, "claude-sonnet-4-6")
	require.NoError(t, err)
	assert.Nil(t, byNameList)

	err = repo.Delete(ctx, id)
	require.NoError(t, err)
	_, err = repo.FindByID(ctx, id)
	assert.Error(t, err)
}

func TestModelAliasRepository_FindByAliasName_MultipleTargets(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelAliasRepository(db)
	ctx := context.Background()

	_, err := repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 2,
		Enabled:       true,
	})
	require.NoError(t, err)
	_, err = repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 3,
		Enabled:       true,
	})
	require.NoError(t, err)

	aliases, err := repo.FindByAliasName(ctx, "claude-sonnet-4-6")
	require.NoError(t, err)
	require.Len(t, aliases, 2)
	assert.Equal(t, int64(2), aliases[0].TargetModelID)
	assert.Equal(t, int64(3), aliases[1].TargetModelID)
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

func TestModelAliasRepository_ProviderIDRoundTrip(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelAliasRepository(db)
	ctx := context.Background()

	providerID := int64(2)
	id, err := repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 2,
		ProviderID:    &providerID,
		Enabled:       true,
	})
	require.NoError(t, err)

	alias, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, alias.ProviderID)
	assert.Equal(t, int64(2), *alias.ProviderID)

	err = repo.UpdatePatch(ctx, id, ModelAliasPatch{
		ProviderIDSet: true,
		ProviderID:    nil,
	})
	require.NoError(t, err)

	updated, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, updated.ProviderID)
}

func TestModelAliasRepository_InsertDuplicateMapping_Fails(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelAliasRepository(db)
	ctx := context.Background()

	providerID := int64(1)
	_, err := repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 2,
		ProviderID:    &providerID,
		Enabled:       true,
	})
	require.NoError(t, err)

	_, err = repo.Insert(ctx, &models.ModelAlias{
		AliasName:     "claude-sonnet-4-6",
		TargetModelID: 2,
		ProviderID:    &providerID,
		Enabled:       true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE constraint failed")
}

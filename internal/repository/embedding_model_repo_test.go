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

func TestEmbeddingModelRepository_ListModels(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedEmbeddingModels(t, db)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name        string
		enabledOnly bool
		wantCount   int
	}{
		{"all models", false, 3},
		{"enabled only", true, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modelList, err := repo.ListModels(ctx, tt.enabledOnly)
			require.NoError(t, err)
			assert.Len(t, modelList, tt.wantCount)
		})
	}
}

func TestEmbeddingModelRepository_GetModelByName(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedEmbeddingModels(t, db)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name      string
		modelName string
		wantNil   bool
	}{
		{"existing model", "paraphrase-multilingual-MiniLM-L12-v2", false},
		{"non-existing model", "nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := repo.GetModelByName(ctx, tt.modelName)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, model)
			} else {
				assert.NotNil(t, model)
				assert.Equal(t, tt.modelName, model.Name)
			}
		})
	}
}

func TestEmbeddingModelRepository_AddModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	model := &models.EmbeddingModel{
		Name:               "test-embedding-model",
		Dimension:          512,
		Description:        "Test model",
		FastembedSupported: true,
		FastembedName:      "test/model",
		IsBuiltin:          false,
		Enabled:            true,
		SortOrder:          10,
	}

	id, err := repo.AddModel(ctx, model)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	// Verify
	found, err := repo.GetModelByName(ctx, "test-embedding-model")
	require.NoError(t, err)
	assert.Equal(t, model.Name, found.Name)
	assert.Equal(t, model.Dimension, found.Dimension)
	assert.Equal(t, model.FastembedSupported, found.FastembedSupported)
}

func TestEmbeddingModelRepository_UpdateModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedEmbeddingModels(t, db)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		updates map[string]any
		verify  func(t *testing.T, m *models.EmbeddingModel)
	}{
		{
			name: "update description",
			id:   1,
			updates: map[string]any{
				"description": "Updated description",
			},
			verify: func(t *testing.T, m *models.EmbeddingModel) {
				assert.Equal(t, "Updated description", m.Description)
			},
		},
		{
			name: "update enabled",
			id:   1,
			updates: map[string]any{
				"enabled": false,
			},
			verify: func(t *testing.T, m *models.EmbeddingModel) {
				assert.False(t, m.Enabled)
			},
		},
		{
			name: "update sort order",
			id:   2,
			updates: map[string]any{
				"sort_order": 100,
			},
			verify: func(t *testing.T, m *models.EmbeddingModel) {
				assert.Equal(t, 100, m.SortOrder)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpdateModel(ctx, tt.id, tt.updates)
			require.NoError(t, err)

			// Get model by listing and finding by ID
			modelList, err := repo.ListModels(ctx, false)
			require.NoError(t, err)
			var found *models.EmbeddingModel
			for _, m := range modelList {
				if m.ID == tt.id {
					found = m
					break
				}
			}
			require.NotNil(t, found)
			tt.verify(t, found)
		})
	}
}

func TestEmbeddingModelRepository_DeleteModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedEmbeddingModels(t, db)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	// Cannot delete builtin model
	err := repo.DeleteModel(ctx, 1) // builtin
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete builtin")

	// Can delete non-builtin model
	err = repo.DeleteModel(ctx, 3) // non-builtin
	require.NoError(t, err)

	// Verify deletion
	modelList, err := repo.ListModels(ctx, false)
	require.NoError(t, err)
	assert.Len(t, modelList, 2)
}

func TestEmbeddingModelRepository_DeleteModel_NonExisting(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	err := repo.DeleteModel(ctx, 999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEmbeddingModelRepository_GetEnabledModelNames(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedEmbeddingModels(t, db)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	names, err := repo.GetEnabledModelNames(ctx)
	require.NoError(t, err)
	assert.Len(t, names, 2)
}

func TestEmbeddingModelRepository_GetFastembedMapping(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedEmbeddingModels(t, db)
	repo := NewEmbeddingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	mapping, err := repo.GetFastembedMapping(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, mapping)

	// Check specific mapping
	fastembedName, ok := mapping["paraphrase-multilingual-MiniLM-L12-v2"]
	assert.True(t, ok)
	assert.Equal(t, "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2", fastembedName)
}

func seedEmbeddingModels(t *testing.T, db *sql.DB) {
	t.Helper()

	queries := []string{
		`INSERT INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, enabled, sort_order, created_at, updated_at)
		 VALUES ('paraphrase-multilingual-MiniLM-L12-v2', 384, 'Multilingual model', 1, 'sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2', 1, 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, enabled, sort_order, created_at, updated_at)
		 VALUES ('all-MiniLM-L6-v2', 384, 'English model', 1, 'sentence-transformers/all-MiniLM-L6-v2', 1, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, enabled, sort_order, created_at, updated_at)
		 VALUES ('custom-model', 512, 'Custom model', 0, NULL, 0, 0, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
}

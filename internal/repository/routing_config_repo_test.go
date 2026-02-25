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
	"go.uber.org/zap"
)

func TestRoutingConfigRepository_GetConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	config, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify default values
	assert.False(t, config.Enabled)
	assert.True(t, config.CacheEnabled)
	assert.Equal(t, 300, config.CacheTTLSeconds)
	assert.Equal(t, 604800, config.CacheTTLL3Seconds)
	assert.True(t, config.SemanticCacheEnabled)
	assert.Equal(t, 0.82, config.SimilarityThreshold)
	assert.Equal(t, "paraphrase-multilingual-MiniLM-L12-v2", config.LocalEmbeddingModel)
}

func TestRoutingConfigRepository_GetConfig_NoRow(t *testing.T) {
	db := testutil.NewTestDB(t) // No defaults inserted
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	config, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Should return default config
	assert.False(t, config.Enabled)
	assert.True(t, config.CacheEnabled)
}

func TestRoutingConfigRepository_UpdateConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		updates map[string]any
		verify  func(t *testing.T, cfg *models.RoutingConfig)
	}{
		{
			name: "enable routing",
			updates: map[string]any{
				"enabled": true,
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				assert.True(t, cfg.Enabled)
			},
		},
		{
			name: "update cache settings",
			updates: map[string]any{
				"cache_enabled":     false,
				"cache_ttl_seconds": 600,
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				assert.False(t, cfg.CacheEnabled)
				assert.Equal(t, 600, cfg.CacheTTLSeconds)
			},
		},
		{
			name: "update semantic cache",
			updates: map[string]any{
				"semantic_cache_enabled": false,
				"similarity_threshold":   0.9,
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				assert.False(t, cfg.SemanticCacheEnabled)
				assert.Equal(t, 0.9, cfg.SimilarityThreshold)
			},
		},
		{
			name: "update model IDs",
			updates: map[string]any{
				"primary_model_id":  int64(1),
				"fallback_model_id": int64(2),
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				require.NotNil(t, cfg.PrimaryModelID)
				assert.Equal(t, int64(1), *cfg.PrimaryModelID)
				require.NotNil(t, cfg.FallbackModelID)
				assert.Equal(t, int64(2), *cfg.FallbackModelID)
			},
		},
		{
			name: "update timeout and retry",
			updates: map[string]any{
				"timeout_seconds": 60,
				"retry_count":     3,
				"max_tokens":      2048,
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				assert.Equal(t, 60, cfg.TimeoutSeconds)
				assert.Equal(t, 3, cfg.RetryCount)
				assert.Equal(t, 2048, cfg.MaxTokens)
			},
		},
		{
			name: "update local embedding model",
			updates: map[string]any{
				"local_embedding_model": "all-MiniLM-L6-v2",
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				assert.Equal(t, "all-MiniLM-L6-v2", cfg.LocalEmbeddingModel)
			},
		},
		{
			name: "enable force smart routing",
			updates: map[string]any{
				"force_smart_routing": true,
			},
			verify: func(t *testing.T, cfg *models.RoutingConfig) {
				assert.True(t, cfg.ForceSmartRouting)
			},
		},
		{
			name:    "empty updates",
			updates: map[string]any{},
			verify:  func(t *testing.T, cfg *models.RoutingConfig) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpdateConfig(ctx, tt.updates)
			require.NoError(t, err)

			if len(tt.updates) > 0 {
				config, err := repo.GetConfig(ctx)
				require.NoError(t, err)
				tt.verify(t, config)
			}
		})
	}
}

func TestRoutingConfigRepository_UpdateConfig_ClearModelID(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	// First set a model ID
	err := repo.UpdateConfig(ctx, map[string]any{
		"primary_model_id": int64(1),
	})
	require.NoError(t, err)

	config, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config.PrimaryModelID)

	// Clear model ID by setting to 0 or negative
	err = repo.UpdateConfig(ctx, map[string]any{
		"primary_model_id": int64(0),
	})
	require.NoError(t, err)

	config, err = repo.GetConfig(ctx)
	require.NoError(t, err)
	assert.Nil(t, config.PrimaryModelID)
}

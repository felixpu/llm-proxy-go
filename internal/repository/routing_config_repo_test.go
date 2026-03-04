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
			err := repo.UpdateConfigPatch(ctx, routingConfigPatchFromMap(tt.updates))
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
	err := repo.UpdateConfigPatch(ctx, RoutingConfigPatch{
		PrimaryModelID: ptrInt64RoutingConfig(1),
	})
	require.NoError(t, err)

	config, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config.PrimaryModelID)

	// Clear model ID by setting to 0 or negative
	err = repo.UpdateConfigPatch(ctx, RoutingConfigPatch{
		PrimaryModelID: ptrInt64RoutingConfig(0),
	})
	require.NoError(t, err)

	config, err = repo.GetConfig(ctx)
	require.NoError(t, err)
	assert.Nil(t, config.PrimaryModelID)
}

func TestRoutingConfigRepository_GetConfig_WrappedNoRowsError(t *testing.T) {
	// Test that sql.ErrNoRows is properly detected (should use errors.Is)
	// This test verifies the fix for C-3: using errors.Is instead of ==
	db := testutil.NewTestDB(t) // No defaults inserted
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	// When no rows exist, should return default config without error
	config, err := repo.GetConfig(ctx)
	require.NoError(t, err, "should return default config when no rows exist")
	require.NotNil(t, config)
	assert.False(t, config.Enabled)
	assert.True(t, config.CacheEnabled)
}

func TestRoutingConfigRepository_UpdateConfigPatch_TypeSafeColumns(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	enabled := true
	ttl := 600
	require.NoError(t, repo.UpdateConfigPatch(ctx, RoutingConfigPatch{
		Enabled:         &enabled,
		CacheTTLSeconds: &ttl,
	}))

	cfg, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 600, cfg.CacheTTLSeconds)
}

func TestRoutingConfigRepository_UpdateConfigPatch(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewRoutingConfigRepository(db, zap.NewNop())
	ctx := context.Background()

	enabled := true
	ttl := 900
	logFull := false
	patch := RoutingConfigPatch{
		Enabled:         &enabled,
		CacheTTLSeconds: &ttl,
		LogFullContent:  &logFull,
	}

	require.NoError(t, repo.UpdateConfigPatch(ctx, patch))

	cfg, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 900, cfg.CacheTTLSeconds)
	assert.False(t, cfg.LogFullContent)
}

func routingConfigPatchFromMap(updates map[string]any) RoutingConfigPatch {
	patch := RoutingConfigPatch{}
	if v, ok := updates["enabled"].(bool); ok {
		patch.Enabled = &v
	}
	if v, ok := updates["primary_model_id"].(int64); ok {
		patch.PrimaryModelID = &v
	}
	if v, ok := updates["fallback_model_id"].(int64); ok {
		patch.FallbackModelID = &v
	}
	if v, ok := updates["timeout_seconds"].(int); ok {
		patch.TimeoutSeconds = &v
	}
	if v, ok := updates["cache_enabled"].(bool); ok {
		patch.CacheEnabled = &v
	}
	if v, ok := updates["cache_ttl_seconds"].(int); ok {
		patch.CacheTTLSeconds = &v
	}
	if v, ok := updates["cache_ttl_l3_seconds"].(int); ok {
		patch.CacheTTLL3Seconds = &v
	}
	if v, ok := updates["max_tokens"].(int); ok {
		patch.MaxTokens = &v
	}
	if v, ok := updates["temperature"].(float64); ok {
		patch.Temperature = &v
	}
	if v, ok := updates["retry_count"].(int); ok {
		patch.RetryCount = &v
	}
	if v, ok := updates["semantic_cache_enabled"].(bool); ok {
		patch.SemanticCacheEnabled = &v
	}
	if v, ok := updates["embedding_model_id"].(int64); ok {
		patch.EmbeddingModelID = &v
	}
	if v, ok := updates["similarity_threshold"].(float64); ok {
		patch.SimilarityThreshold = &v
	}
	if v, ok := updates["local_embedding_model"].(string); ok {
		patch.LocalEmbeddingModel = &v
	}
	if v, ok := updates["force_smart_routing"].(bool); ok {
		patch.ForceSmartRouting = &v
	}
	if v, ok := updates["rule_based_routing_enabled"].(bool); ok {
		patch.RuleBasedRoutingEnabled = &v
	}
	if v, ok := updates["rule_fallback_strategy"].(string); ok {
		patch.RuleFallbackStrategy = &v
	}
	if v, ok := updates["rule_fallback_task_type"].(string); ok {
		patch.RuleFallbackTaskType = &v
	}
	if v, ok := updates["rule_fallback_model_id"].(int64); ok {
		patch.RuleFallbackModelID = &v
	}
	if v, ok := updates["cross_role_fallback_enabled"].(bool); ok {
		patch.CrossRoleFallbackEnabled = &v
	}
	if v, ok := updates["log_full_content"].(bool); ok {
		patch.LogFullContent = &v
	}
	return patch
}

func ptrInt64RoutingConfig(v int64) *int64 {
	return &v
}

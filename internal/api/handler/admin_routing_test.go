//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

type stubRoutingModelStore struct{}

func (stubRoutingModelStore) ListModels(context.Context, *int64) ([]*models.RoutingModel, error) {
	return nil, nil
}

func (stubRoutingModelStore) GetModel(context.Context, int64) (*models.RoutingModel, error) {
	return nil, nil
}

func (stubRoutingModelStore) AddModel(context.Context, *models.RoutingModel) (int64, error) {
	return 0, nil
}

func (stubRoutingModelStore) UpdateModelPatch(context.Context, int64, repository.RoutingModelPatch) error {
	return nil
}

func (stubRoutingModelStore) DeleteModel(context.Context, int64) error {
	return nil
}

type captureRoutingConfigStore struct {
	cfg       *models.RoutingConfig
	lastPatch repository.RoutingConfigPatch
}

func (s *captureRoutingConfigStore) GetConfig(context.Context) (*models.RoutingConfig, error) {
	return s.cfg, nil
}

func (s *captureRoutingConfigStore) UpdateConfigPatch(_ context.Context, patch repository.RoutingConfigPatch) error {
	s.lastPatch = patch
	return nil
}

func TestRoutingHandler_GetLLMRoutingConfig_OmitsDeprecatedSemanticFields(t *testing.T) {
	primaryModelID := int64(1)
	fallbackModelID := int64(2)

	repo := &captureRoutingConfigStore{
		cfg: &models.RoutingConfig{
			Enabled:                  true,
			PrimaryModelID:           &primaryModelID,
			FallbackModelID:          &fallbackModelID,
			TimeoutSeconds:           5,
			CacheEnabled:             true,
			CacheTTLSeconds:          300,
			CacheTTLL3Seconds:        604800,
			MaxTokens:                256,
			Temperature:              0,
			RetryCount:               2,
			ForceSmartRouting:        true,
			ShadowRoutingEnabled:     true,
			ShadowSampleRate:         0.5,
			ShadowMaxQPS:             30,
			RuleBasedRoutingEnabled:  true,
			RuleFallbackStrategy:     models.FallbackLLM,
			RuleFallbackTaskType:     "default",
			CrossRoleFallbackEnabled: true,
			LogFullContent:           true,
		},
	}
	handler := NewRoutingHandler(stubRoutingModelStore{}, repo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest(http.MethodGet, "/api/config/routing", nil)

	handler.GetLLMRoutingConfig(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, true, resp["enabled"])
	assert.EqualValues(t, 300, resp["cache_ttl_seconds"])
	assert.Equal(t, true, resp["force_smart_routing"])
	assert.Equal(t, true, resp["shadow_routing_enabled"])
	assert.Equal(t, 0.5, resp["shadow_sample_rate"])
	assert.EqualValues(t, 30, resp["shadow_max_qps"])
	assert.Equal(t, true, resp["rule_based_routing_enabled"])
	assert.Equal(t, true, resp["cross_role_fallback_enabled"])
	assert.Equal(t, true, resp["log_full_content"])

	_, hasL3TTL := resp["cache_ttl_l3_seconds"]
	_, hasSemanticEnabled := resp["semantic_cache_enabled"]
	_, hasEmbeddingModelID := resp["embedding_model_id"]
	_, hasSimilarityThreshold := resp["similarity_threshold"]
	_, hasLocalEmbeddingModel := resp["local_embedding_model"]

	assert.False(t, hasL3TTL)
	assert.False(t, hasSemanticEnabled)
	assert.False(t, hasEmbeddingModelID)
	assert.False(t, hasSimilarityThreshold)
	assert.False(t, hasLocalEmbeddingModel)
}

func TestRoutingHandler_UpdateLLMRoutingConfig_IgnoresDeprecatedSemanticFields(t *testing.T) {
	repo := &captureRoutingConfigStore{}
	handler := NewRoutingHandler(stubRoutingModelStore{}, repo)

	body := `{
		"enabled": true,
		"cache_ttl_seconds": 123,
		"shadow_routing_enabled": true,
		"shadow_sample_rate": 0.25,
		"shadow_max_qps": 15,
		"semantic_cache_enabled": false,
		"embedding_model_id": 9,
		"similarity_threshold": 0.9,
		"local_embedding_model": "all-MiniLM-L6-v2",
		"cache_ttl_l3_seconds": 86400
	}`

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/config/routing", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateLLMRoutingConfig(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.lastPatch.Enabled)
	require.NotNil(t, repo.lastPatch.CacheTTLSeconds)
	require.NotNil(t, repo.lastPatch.ShadowRoutingEnabled)
	require.NotNil(t, repo.lastPatch.ShadowSampleRate)
	require.NotNil(t, repo.lastPatch.ShadowMaxQPS)
	assert.True(t, *repo.lastPatch.Enabled)
	assert.Equal(t, 123, *repo.lastPatch.CacheTTLSeconds)
	assert.True(t, *repo.lastPatch.ShadowRoutingEnabled)
	assert.Equal(t, 0.25, *repo.lastPatch.ShadowSampleRate)
	assert.Equal(t, 15, *repo.lastPatch.ShadowMaxQPS)
	assert.Nil(t, repo.lastPatch.CacheTTLL3Seconds)
	assert.Nil(t, repo.lastPatch.SemanticCacheEnabled)
	assert.Nil(t, repo.lastPatch.EmbeddingModelID)
	assert.Nil(t, repo.lastPatch.SimilarityThreshold)
	assert.Nil(t, repo.lastPatch.LocalEmbeddingModel)
}

func TestRoutingHandler_CreateRoutingModel_InvalidatesAPIDetectCache(t *testing.T) {
	assertRoutingModelMutationInvalidatesDetectCache(t, func(h *RoutingHandler) {
		c, w := testutil.NewTestContext()
		body := `{"provider_id": 1, "model_name": "router-model-a"}`
		c.Request = httptest.NewRequest(http.MethodPost, "/api/routing/models", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.CreateRoutingModel(c)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRoutingHandler_UpdateRoutingModel_InvalidatesAPIDetectCache(t *testing.T) {
	assertRoutingModelMutationInvalidatesDetectCache(t, func(h *RoutingHandler) {
		c, w := testutil.NewTestContext()
		c.Params = append(c.Params, gin.Param{Key: "model_id", Value: "1"})
		c.Request = httptest.NewRequest(http.MethodPatch, "/api/routing/models/1", bytes.NewBufferString(`{"priority": 10}`))
		c.Request.Header.Set("Content-Type", "application/json")

		h.UpdateRoutingModel(c)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRoutingHandler_DeleteRoutingModel_InvalidatesAPIDetectCache(t *testing.T) {
	assertRoutingModelMutationInvalidatesDetectCache(t, func(h *RoutingHandler) {
		c, w := testutil.NewTestContext()
		c.Params = append(c.Params, gin.Param{Key: "model_id", Value: "1"})
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/routing/models/1", nil)

		h.DeleteRoutingModel(c)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func assertRoutingModelMutationInvalidatesDetectCache(t *testing.T, mutate func(h *RoutingHandler)) {
	t.Helper()

	service.InvalidateAPIDetectionCache()

	var detectCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		detectCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	const (
		apiKey    = "test-key"
		modelName = "router-model-a"
	)

	_, err := service.DetectAPITypeForModel(context.Background(), upstream.URL, apiKey, modelName)
	require.NoError(t, err)
	require.Equal(t, 1, detectCalls)

	// Ensure baseline cache hit before mutation.
	_, err = service.DetectAPITypeForModel(context.Background(), upstream.URL, apiKey, modelName)
	require.NoError(t, err)
	require.Equal(t, 1, detectCalls)

	handler := NewRoutingHandler(stubRoutingModelStore{}, &captureRoutingConfigStore{})
	mutate(handler)

	_, err = service.DetectAPITypeForModel(context.Background(), upstream.URL, apiKey, modelName)
	require.NoError(t, err)
	require.Equal(t, 2, detectCalls)
}

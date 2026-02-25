//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestNewEmbeddingService(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)
	require.NotNil(t, es)
	assert.NotNil(t, es.client)
}

func TestEmbeddingService_GetEmbedding_Disabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	// Default config has semantic_cache_enabled = true, but let's ensure
	// the service handles the case where it's disabled
	// First, disable semantic cache
	err := configRepo.UpdateConfig(ctx, map[string]any{
		"semantic_cache_enabled": false,
	})
	require.NoError(t, err)

	embedding, err := es.GetEmbedding(ctx, "test text")
	assert.NoError(t, err)
	assert.Nil(t, embedding)
}

func TestEmbeddingService_GetEmbedding_NoModelConfigured(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	// Enable semantic cache but don't configure embedding model
	err := configRepo.UpdateConfig(ctx, map[string]any{
		"semantic_cache_enabled": true,
	})
	require.NoError(t, err)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	embedding, err := es.GetEmbedding(ctx, "test text")
	assert.NoError(t, err)
	assert.Nil(t, embedding) // No model configured, returns nil
}

func TestEmbeddingService_CallEmbeddingAPI_Success(t *testing.T) {
	// Create mock embedding API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		resp := embeddingAPIResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
			}{
				{Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	embedding, err := es.CallEmbeddingAPI(ctx, server.URL, "test-key", "test-model", "hello world")
	require.NoError(t, err)
	assert.Len(t, embedding, 5)
	assert.Equal(t, 0.1, embedding[0])
}

func TestEmbeddingService_CallEmbeddingAPI_ServerError(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	embedding, err := es.CallEmbeddingAPI(ctx, server.URL, "test-key", "test-model", "hello world")
	assert.Error(t, err)
	assert.Nil(t, embedding)
}

func TestEmbeddingService_CallEmbeddingAPI_EmptyResponse(t *testing.T) {
	// Create mock server that returns empty data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingAPIResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
			}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	embedding, err := es.CallEmbeddingAPI(ctx, server.URL, "test-key", "test-model", "hello world")
	assert.Error(t, err)
	assert.Nil(t, embedding)
	assert.Contains(t, err.Error(), "failed")
}

func TestEmbeddingService_CallEmbeddingAPI_InvalidJSON(t *testing.T) {
	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	embedding, err := es.CallEmbeddingAPI(ctx, server.URL, "test-key", "test-model", "hello world")
	assert.Error(t, err)
	assert.Nil(t, embedding)
}

func TestEmbeddingService_CallEmbeddingAPI_Unreachable(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	ctx := context.Background()

	configRepo := repository.NewRoutingConfigRepository(db, logger)
	modelRepo := repository.NewEmbeddingModelRepository(db, logger)

	es := NewEmbeddingService(configRepo, modelRepo, logger)

	// Use an unreachable URL
	embedding, err := es.CallEmbeddingAPI(ctx, "http://127.0.0.1:1", "test-key", "test-model", "hello world")
	assert.Error(t, err)
	assert.Nil(t, embedding)
}

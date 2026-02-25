//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestCacheHandler_GetStats_Success(t *testing.T) {
	logger := testutil.NewTestLogger()
	db := testutil.NewTestDB(t)
	defer db.Close()

	routingCache := service.NewRoutingCache(1000, logger)
	embeddingCacheRepo := repository.NewEmbeddingCacheRepository(db, logger)
	handler := NewCacheHandler(routingCache, embeddingCacheRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/cache/stats", nil)

	handler.GetStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["summary"])
	assert.NotNil(t, resp["by_layer"])
}

func TestCacheHandler_GetEntries_Success(t *testing.T) {
	logger := testutil.NewTestLogger()
	db := testutil.NewTestDB(t)
	defer db.Close()

	routingCache := service.NewRoutingCache(1000, logger)
	embeddingCacheRepo := repository.NewEmbeddingCacheRepository(db, logger)
	handler := NewCacheHandler(routingCache, embeddingCacheRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/cache/entries", nil)

	handler.GetEntries(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["entries"])
	assert.NotNil(t, resp["total"])
}

func TestCacheHandler_Clear_Success(t *testing.T) {
	logger := testutil.NewTestLogger()
	db := testutil.NewTestDB(t)
	defer db.Close()

	routingCache := service.NewRoutingCache(1000, logger)
	embeddingCacheRepo := repository.NewEmbeddingCacheRepository(db, logger)
	handler := NewCacheHandler(routingCache, embeddingCacheRepo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/cache/clear", nil)

	handler.Clear(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "Cache cleared successfully", resp["message"])
}

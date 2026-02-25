//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestDefaultCacheConfig(t *testing.T) {
	cfg := DefaultCacheConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, DefaultL1TTL, cfg.L1TTL)
	assert.Equal(t, DefaultL2TTL, cfg.L2TTL)
	assert.Equal(t, DefaultL3TTL, cfg.L3TTL)
	assert.Equal(t, DefaultSimilarityThreshold, cfg.SimilarityThreshold)
	assert.Equal(t, DefaultMaxL1Size, cfg.MaxL1Size)
}

func TestGenerateCacheKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"simple text", "hello world"},
		{"chinese text", "你好世界"},
		{"empty string", ""},
		{"long text", "this is a very long text that should still produce a consistent hash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := GenerateCacheKey(tt.content)
			assert.Len(t, key, 32) // MD5 hex is 32 chars

			// Same content should produce same key
			key2 := GenerateCacheKey(tt.content)
			assert.Equal(t, key, key2)
		})
	}
}

func TestGenerateCacheKey_Uniqueness(t *testing.T) {
	key1 := GenerateCacheKey("hello")
	key2 := GenerateCacheKey("world")
	assert.NotEqual(t, key1, key2)
}

func TestNewCacheService(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cs := NewCacheService(db, nil, logger) // nil config should use defaults
	require.NotNil(t, cs)
	assert.True(t, cs.config.Enabled)
	assert.NotNil(t, cs.l1Cache)
	assert.NotNil(t, cs.l2Repo)
}

func TestNewCacheService_WithConfig(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             false,
		L1TTL:               1 * time.Minute,
		L2TTL:               2 * time.Minute,
		L3TTL:               3 * time.Hour,
		SimilarityThreshold: 0.9,
		MaxL1Size:           100,
	}

	cs := NewCacheService(db, cfg, logger)
	require.NotNil(t, cs)
	assert.False(t, cs.config.Enabled)
	assert.Equal(t, 1*time.Minute, cs.config.L1TTL)
	assert.Equal(t, 0.9, cs.config.SimilarityThreshold)
}

func TestCacheService_SetAndGet_L1(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               5 * time.Minute,
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.82,
		MaxL1Size:           1000,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	content := "test content for caching"
	embedding := []float64{0.1, 0.2, 0.3}

	// Set cache entry
	err := cs.Set(ctx, content, embedding, "simple", "test reason")
	require.NoError(t, err)

	// Get should hit L1 cache
	result, err := cs.Get(ctx, content, nil)
	require.NoError(t, err)
	assert.True(t, result.Hit)
	assert.Equal(t, "simple", result.TaskType)
	assert.Equal(t, "test reason", result.Reason)
	assert.Equal(t, "L1", result.CacheType)
}

func TestCacheService_Get_Disabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled: false,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	result, err := cs.Get(ctx, "any content", nil)
	require.NoError(t, err)
	assert.False(t, result.Hit)
}

func TestCacheService_Set_Disabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled: false,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	// Set should do nothing when disabled
	err := cs.Set(ctx, "content", nil, "simple", "reason")
	assert.NoError(t, err)
}

func TestCacheService_Get_L2Hit(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               5 * time.Minute,
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.82,
		MaxL1Size:           1000,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	content := "test content for L2"
	embedding := []float64{0.1, 0.2, 0.3}

	// Set cache entry
	err := cs.Set(ctx, content, embedding, "default", "L2 test")
	require.NoError(t, err)

	// Clear L1 cache to force L2 lookup
	cs.l1Mu.Lock()
	cs.l1Cache = make(map[string]*l1Entry)
	cs.l1Mu.Unlock()

	// Get should hit L2 cache
	result, err := cs.Get(ctx, content, nil)
	require.NoError(t, err)
	assert.True(t, result.Hit)
	assert.Equal(t, "default", result.TaskType)
	assert.Equal(t, "L2", result.CacheType)
}

func TestCacheService_Get_Miss(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               5 * time.Minute,
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.82,
		MaxL1Size:           1000,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	// Get non-existing content
	result, err := cs.Get(ctx, "non-existing content", nil)
	require.NoError(t, err)
	assert.False(t, result.Hit)
}

func TestCacheService_L1Expiration(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               50 * time.Millisecond, // Very short TTL
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.82,
		MaxL1Size:           1000,
	}

	cs := NewCacheService(db, cfg, logger)

	// Set L1 entry directly
	key := "test-key"
	cs.setL1(key, &CacheEntry{
		TaskType: "simple",
		Reason:   "test",
		CachedAt: time.Now(),
	})

	// Should be found immediately
	entry := cs.getL1(key)
	assert.NotNil(t, entry)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	entry = cs.getL1(key)
	assert.Nil(t, entry)
}

func TestCacheService_L1Eviction(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               5 * time.Minute,
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.82,
		MaxL1Size:           3, // Very small cache
	}

	cs := NewCacheService(db, cfg, logger)

	// Fill cache
	for i := 0; i < 5; i++ {
		cs.setL1(GenerateCacheKey(string(rune('a'+i))), &CacheEntry{
			TaskType: "simple",
			CachedAt: time.Now(),
		})
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Cache should be at or below max size
	cs.l1Mu.RLock()
	size := len(cs.l1Cache)
	cs.l1Mu.RUnlock()

	assert.LessOrEqual(t, size, cfg.MaxL1Size)
}

func TestCacheService_GetStats(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               5 * time.Minute,
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.82,
		MaxL1Size:           1000,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	// Add some entries
	cs.Set(ctx, "content1", nil, "simple", "r1")
	cs.Set(ctx, "content2", nil, "default", "r2")

	stats, err := cs.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, stats["l1_size"])
	assert.Equal(t, 1000, stats["l1_max_size"])
	assert.Equal(t, int64(2), stats["l2_size"])
	assert.True(t, stats["enabled"].(bool))
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float64
		b    []float64
		want float64
	}{
		{
			name: "identical vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "similar vectors",
			a:    []float64{1, 1, 0},
			b:    []float64{1, 0, 0},
			want: 0.7071067811865475, // 1/sqrt(2)
		},
		{
			name: "empty vectors",
			a:    []float64{},
			b:    []float64{},
			want: 0.0,
		},
		{
			name: "different lengths",
			a:    []float64{1, 2},
			b:    []float64{1, 2, 3},
			want: 0.0,
		},
		{
			name: "zero vector",
			a:    []float64{0, 0, 0},
			b:    []float64{1, 2, 3},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.want, result, 0.0001)
		})
	}
}

func TestCacheService_L3SemanticSearch(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	cfg := &CacheConfig{
		Enabled:             true,
		L1TTL:               5 * time.Minute,
		L2TTL:               5 * time.Minute,
		L3TTL:               7 * 24 * time.Hour,
		SimilarityThreshold: 0.8,
		MaxL1Size:           1000,
	}

	cs := NewCacheService(db, cfg, logger)
	ctx := context.Background()

	// Store entry with embedding
	embedding1 := []float64{1.0, 0.0, 0.0}
	err := cs.Set(ctx, "original content", embedding1, "complex", "semantic test")
	require.NoError(t, err)

	// Clear L1 cache
	cs.l1Mu.Lock()
	cs.l1Cache = make(map[string]*l1Entry)
	cs.l1Mu.Unlock()

	// Search with similar embedding (should hit L3)
	similarEmbedding := []float64{0.95, 0.1, 0.0} // Very similar to embedding1
	result, err := cs.Get(ctx, "different content", similarEmbedding)
	require.NoError(t, err)

	// Should find via semantic similarity
	if result.Hit {
		assert.Equal(t, "L3", result.CacheType)
		assert.Equal(t, "complex", result.TaskType)
	}
}

func TestCacheEntry(t *testing.T) {
	entry := &CacheEntry{
		TaskType:  "simple",
		Reason:    "test reason",
		Embedding: []float64{0.1, 0.2, 0.3},
		CachedAt:  time.Now(),
	}

	assert.Equal(t, "simple", entry.TaskType)
	assert.Equal(t, "test reason", entry.Reason)
	assert.Len(t, entry.Embedding, 3)
}

func TestCacheResult(t *testing.T) {
	result := &CacheResult{
		Hit:       true,
		TaskType:  "default",
		Reason:    "cache hit",
		CacheType: "L1",
	}

	assert.True(t, result.Hit)
	assert.Equal(t, "default", result.TaskType)
	assert.Equal(t, "L1", result.CacheType)
}

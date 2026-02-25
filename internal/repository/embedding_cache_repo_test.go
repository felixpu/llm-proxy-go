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

func TestEmbeddingCacheRepository_SaveCache(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	embedding := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	err := repo.SaveCache(ctx, "hash_123", "test content preview", embedding, "default", "test reason")
	require.NoError(t, err)

	// Verify
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestEmbeddingCacheRepository_SaveCache_Upsert(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	embedding1 := []float64{0.1, 0.2, 0.3}
	err := repo.SaveCache(ctx, "hash_123", "preview1", embedding1, "simple", "reason1")
	require.NoError(t, err)

	// Upsert with same hash
	embedding2 := []float64{0.4, 0.5, 0.6}
	err = repo.SaveCache(ctx, "hash_123", "preview2", embedding2, "complex", "reason2")
	require.NoError(t, err)

	// Should still be 1 entry
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify updated values
	entry, err := repo.GetExactMatch(ctx, "hash_123", 3600)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "complex", entry.TaskType)
	assert.Equal(t, "reason2", entry.Reason)
}

func TestEmbeddingCacheRepository_GetExactMatch(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	embedding := []float64{0.1, 0.2, 0.3}
	err := repo.SaveCache(ctx, "hash_exact", "preview", embedding, "default", "reason")
	require.NoError(t, err)

	tests := []struct {
		name       string
		hash       string
		ttlSeconds int
		wantNil    bool
	}{
		{"existing hash", "hash_exact", 3600, false},
		{"non-existing hash", "nonexistent", 3600, true},
		{"zero TTL", "hash_exact", 0, true},
		{"negative TTL", "hash_exact", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := repo.GetExactMatch(ctx, tt.hash, tt.ttlSeconds)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, entry)
			} else {
				assert.NotNil(t, entry)
				assert.Equal(t, tt.hash, entry.ContentHash)
				assert.Equal(t, embedding, entry.Embedding)
			}
		})
	}
}

func TestEmbeddingCacheRepository_FindAllEmbeddings(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	// Insert multiple entries
	err := repo.SaveCache(ctx, "hash_1", "preview1", []float64{0.1, 0.2}, "simple", "r1")
	require.NoError(t, err)
	err = repo.SaveCache(ctx, "hash_2", "preview2", []float64{0.3, 0.4}, "default", "r2")
	require.NoError(t, err)
	err = repo.SaveCache(ctx, "hash_3", "preview3", []float64{0.5, 0.6}, "complex", "r3")
	require.NoError(t, err)

	tests := []struct {
		name       string
		ttlSeconds int
		wantCount  int
	}{
		{"with TTL", 3600, 3},
		{"zero TTL", 0, 0},
		{"negative TTL", -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := repo.FindAllEmbeddings(ctx, tt.ttlSeconds)
			require.NoError(t, err)
			assert.Len(t, entries, tt.wantCount)
		})
	}
}

func TestEmbeddingCacheRepository_UpdateHitCount(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	err := repo.SaveCache(ctx, "hash_hit", "preview", []float64{0.1}, "default", "reason")
	require.NoError(t, err)

	// Get entry to find ID
	entry, err := repo.GetExactMatch(ctx, "hash_hit", 3600)
	require.NoError(t, err)
	assert.Equal(t, 0, entry.HitCount)

	// Update hit count
	err = repo.UpdateHitCount(ctx, entry.ID)
	require.NoError(t, err)

	// Verify
	entry, err = repo.GetExactMatch(ctx, "hash_hit", 3600)
	require.NoError(t, err)
	assert.Equal(t, 1, entry.HitCount)
	assert.NotNil(t, entry.LastHitAt)
}

func TestEmbeddingCacheRepository_UpdateHitCountByHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	err := repo.SaveCache(ctx, "hash_by_hash", "preview", []float64{0.1}, "default", "reason")
	require.NoError(t, err)

	// Update by hash
	err = repo.UpdateHitCountByHash(ctx, "hash_by_hash")
	require.NoError(t, err)
	err = repo.UpdateHitCountByHash(ctx, "hash_by_hash")
	require.NoError(t, err)

	// Verify
	entry, err := repo.GetExactMatch(ctx, "hash_by_hash", 3600)
	require.NoError(t, err)
	assert.Equal(t, 2, entry.HitCount)
}

func TestEmbeddingCacheRepository_DeleteAll(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	// Insert entries
	err := repo.SaveCache(ctx, "hash_1", "p1", []float64{0.1}, "simple", "r1")
	require.NoError(t, err)
	err = repo.SaveCache(ctx, "hash_2", "p2", []float64{0.2}, "default", "r2")
	require.NoError(t, err)

	// Delete all
	deleted, err := repo.DeleteAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestEmbeddingCacheRepository_Count(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	// Empty
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Add entries
	err = repo.SaveCache(ctx, "h1", "p1", []float64{0.1}, "s", "r")
	require.NoError(t, err)
	err = repo.SaveCache(ctx, "h2", "p2", []float64{0.2}, "d", "r")
	require.NoError(t, err)

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestEmbeddingCacheRepository_GetTopEntries(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	// Insert entries with different hit counts
	err := repo.SaveCache(ctx, "h1", "p1", []float64{0.1}, "s", "r")
	require.NoError(t, err)
	err = repo.SaveCache(ctx, "h2", "p2", []float64{0.2}, "d", "r")
	require.NoError(t, err)
	err = repo.SaveCache(ctx, "h3", "p3", []float64{0.3}, "c", "r")
	require.NoError(t, err)

	// Update hit counts
	err = repo.UpdateHitCountByHash(ctx, "h2")
	require.NoError(t, err)
	err = repo.UpdateHitCountByHash(ctx, "h2")
	require.NoError(t, err)
	err = repo.UpdateHitCountByHash(ctx, "h3")
	require.NoError(t, err)

	// Get top by hit count
	entries, err := repo.GetTopEntries(ctx, "hit_count", 2)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "h2", entries[0].ContentHash) // Most hits
}

func TestEmbeddingCacheRepository_GetTopEntries_InvalidSortField(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewEmbeddingCacheRepository(db, zap.NewNop())
	ctx := context.Background()

	err := repo.SaveCache(ctx, "h1", "p1", []float64{0.1}, "s", "r")
	require.NoError(t, err)

	// Invalid sort field should default to hit_count
	entries, err := repo.GetTopEntries(ctx, "invalid_field", 10)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

type stubRoutingEmbeddingCache struct {
	entry *repository.EmbeddingCacheEntry
}

func (s *stubRoutingEmbeddingCache) GetExactMatch(context.Context, string, int) (*repository.EmbeddingCacheEntry, error) {
	return s.entry, nil
}

func (s *stubRoutingEmbeddingCache) UpdateHitCountByHash(context.Context, string) error {
	return nil
}

func (s *stubRoutingEmbeddingCache) SaveCache(context.Context, string, string, []float64, string, string) error {
	return nil
}

func TestHybridRoutingDecisionCache_L1PreservesReason(t *testing.T) {
	cache := NewRoutingCache(16, zap.NewNop())
	decisionCache := newHybridRoutingDecisionCache(cache, nil, zap.NewNop(), nil)

	err := decisionCache.Store(t.Context(), "k1", "preview", models.ModelRoleDefault, "llm: inferred task type default")
	require.NoError(t, err)

	taskType, decision, hit := decisionCache.Lookup(t.Context(), "k1", 300)
	require.True(t, hit)
	require.NotNil(t, decision)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.Equal(t, "llm: inferred task type default", decision.Reason)
	assert.True(t, decision.FromCache)
	assert.Equal(t, "L1", decision.CacheType)
}

func TestHybridRoutingDecisionCache_L2HitBackfillsL1Reason(t *testing.T) {
	cache := NewRoutingCache(16, zap.NewNop())
	decisionCache := newHybridRoutingDecisionCache(cache, &stubRoutingEmbeddingCache{
		entry: &repository.EmbeddingCacheEntry{
			TaskType: "complex",
			Reason:   "matched historical routing decision",
		},
	}, zap.NewNop(), nil)

	taskType, decision, hit := decisionCache.Lookup(t.Context(), "k2", 300)
	require.True(t, hit)
	require.NotNil(t, decision)
	assert.Equal(t, models.ModelRoleComplex, taskType)
	assert.Equal(t, "matched historical routing decision", decision.Reason)
	assert.Equal(t, "L2", decision.CacheType)

	taskType, decision, hit = decisionCache.Lookup(t.Context(), "k2", 300)
	require.True(t, hit)
	require.NotNil(t, decision)
	assert.Equal(t, models.ModelRoleComplex, taskType)
	assert.Equal(t, "matched historical routing decision", decision.Reason)
	assert.Equal(t, "L1", decision.CacheType)
}

func TestHybridRoutingDecisionCache_FallsBackToGenericReasonWhenMissing(t *testing.T) {
	cache := NewRoutingCache(16, zap.NewNop())
	decisionCache := newHybridRoutingDecisionCache(cache, &stubRoutingEmbeddingCache{
		entry: &repository.EmbeddingCacheEntry{
			TaskType: "default",
		},
	}, zap.NewNop(), nil)

	taskType, decision, hit := decisionCache.Lookup(t.Context(), "k3", 300)
	require.True(t, hit)
	require.NotNil(t, decision)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.Equal(t, "cache: reused task type default", decision.Reason)
}

package service

import (
	"context"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

type routingDecisionCache interface {
	Lookup(ctx context.Context, cacheKey string, ttlSeconds int) (models.ModelRole, *models.RoutingDecision, bool)
	Store(ctx context.Context, cacheKey string, contentPreview string, taskType models.ModelRole, reason string) error
}

type hybridRoutingDecisionCache struct {
	l1        *RoutingCache
	l2        routingEmbeddingCache
	logger    *zap.Logger
	asyncPool *AsyncWorkerPool
}

func newHybridRoutingDecisionCache(l1 *RoutingCache, l2 routingEmbeddingCache, logger *zap.Logger, asyncPool *AsyncWorkerPool) routingDecisionCache {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &hybridRoutingDecisionCache{
		l1:        l1,
		l2:        l2,
		logger:    logger,
		asyncPool: asyncPool,
	}
}

func (c *hybridRoutingDecisionCache) Lookup(ctx context.Context, cacheKey string, ttlSeconds int) (models.ModelRole, *models.RoutingDecision, bool) {
	if c.l1 != nil {
		if taskType, hit := c.l1.Get(cacheKey, ttlSeconds); hit {
			return taskType, &models.RoutingDecision{
				TaskType:  taskType,
				FromCache: true,
				CacheType: "L1",
			}, true
		}
	}

	if c.l2 == nil {
		return models.ModelRoleDefault, nil, false
	}

	entry, err := c.l2.GetExactMatch(ctx, cacheKey, ttlSeconds)
	if err != nil {
		c.logger.Warn("L2 cache lookup failed", zap.Error(err))
		return models.ModelRoleDefault, nil, false
	}
	if entry == nil {
		return models.ModelRoleDefault, nil, false
	}

	taskType := parseModelRole(entry.TaskType)
	if c.l1 != nil {
		c.l1.Set(cacheKey, taskType)
	}

	if ok := c.asyncPool.Submit(func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), DefaultAsyncRepoTimeout)
		defer cancel()
		if err := c.l2.UpdateHitCountByHash(updateCtx, cacheKey); err != nil {
			c.logger.Warn("failed to update cache hit count", zap.Error(err))
		}
	}); !ok {
		c.logger.Warn("dropped routing cache hit count update",
			zap.String("cache_key", cacheKey))
	}

	return taskType, &models.RoutingDecision{
		TaskType:  taskType,
		Reason:    entry.Reason,
		FromCache: true,
		CacheType: "L2",
	}, true
}

func (c *hybridRoutingDecisionCache) Store(ctx context.Context, cacheKey string, contentPreview string, taskType models.ModelRole, reason string) error {
	if c.l1 != nil {
		c.l1.Set(cacheKey, taskType)
	}
	if c.l2 == nil {
		return nil
	}

	if len(contentPreview) > DefaultContentPreviewMaxChars {
		contentPreview = contentPreview[:DefaultContentPreviewMaxChars]
	}
	return c.l2.SaveCache(ctx, cacheKey, contentPreview, nil, string(taskType), reason)
}

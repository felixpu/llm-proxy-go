package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// CacheHandler handles cache monitoring API endpoints.
type CacheHandler struct {
	routingCache       *service.RoutingCache
	embeddingCacheRepo *repository.EmbeddingCacheRepository
}

// NewCacheHandler creates a new CacheHandler.
func NewCacheHandler(rc *service.RoutingCache, ecr *repository.EmbeddingCacheRepository) *CacheHandler {
	return &CacheHandler{
		routingCache:       rc,
		embeddingCacheRepo: ecr,
	}
}

// GetStats returns cache statistics overview.
func (h *CacheHandler) GetStats(c *gin.Context) {
	l1Size := 0
	if h.routingCache != nil {
		l1Size = h.routingCache.Size()
	}

	var l2Size int64
	var l2Stats map[string]interface{}
	if h.embeddingCacheRepo != nil {
		count, err := h.embeddingCacheRepo.Count(c.Request.Context())
		if err == nil {
			l2Size = count
		}
		stats, err := h.embeddingCacheRepo.GetStats(c.Request.Context())
		if err == nil {
			l2Stats = stats
		}
	}

	// Calculate hit rate for L2 cache
	var l2HitRate float64
	var l2Hits int64
	if l2Stats != nil {
		if totalHits, ok := l2Stats["total_hits"].(int64); ok {
			l2Hits = totalHits
			if l2Size > 0 {
				l2HitRate = float64(l2Hits) / float64(l2Size)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": gin.H{
			"total_requests":         l2Hits,
			"overall_hit_rate":       l2HitRate,
			"cache_enabled":          h.routingCache != nil,
			"semantic_cache_enabled": h.embeddingCacheRepo != nil,
		},
		"by_layer": gin.H{
			"l1": gin.H{"size": l1Size, "max_size": 10000, "hit_rate": 0.0, "hits": 0, "misses": 0},
			"l2": gin.H{"size": l2Size, "max_size": 0, "hit_rate": l2HitRate, "hits": l2Hits, "misses": 0},
			"l3": gin.H{"size": l2Size, "max_size": 0, "hit_rate": 0.0, "hits": 0, "misses": 0},
		},
		"llm": gin.H{
			"total": 0, "errors": 0, "avg_latency_ms": 0.0,
		},
		"cache_efficiency": gin.H{
			"memory_usage_mb":  0.0,
			"eviction_count":   0,
			"avg_entry_size_b": 0,
		},
	})
}

// GetTimeseries returns cache timeseries data.
func (h *CacheHandler) GetTimeseries(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"data_points": []any{},
	})
}

// GetEntries returns top cache entries.
func (h *CacheHandler) GetEntries(c *gin.Context) {
	if h.embeddingCacheRepo == nil {
		c.JSON(http.StatusOK, gin.H{"total": 0, "entries": []any{}})
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	sortBy := c.DefaultQuery("sort_by", "hit_count")

	entries, err := h.embeddingCacheRepo.GetTopEntries(c.Request.Context(), sortBy, limit)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		result = append(result, gin.H{
			"id":              e.ID,
			"content_hash":    e.ContentHash,
			"content_preview": e.ContentPreview,
			"task_type":       e.TaskType,
			"reason":          e.Reason,
			"hit_count":       e.HitCount,
			"created_at":      e.CreatedAt,
			"last_hit_at":     e.LastHitAt,
		})
	}

	total, _ := h.embeddingCacheRepo.Count(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"total": total, "entries": result})
}

// Clear clears the cache.
func (h *CacheHandler) Clear(c *gin.Context) {
	if h.routingCache != nil {
		h.routingCache.Clear()
	}
	if h.embeddingCacheRepo != nil {
		_, _ = h.embeddingCacheRepo.DeleteAll(c.Request.Context())
	}
	c.JSON(http.StatusOK, gin.H{"message": "Cache cleared successfully"})
}

// ResetStats resets cache statistics.
func (h *CacheHandler) ResetStats(c *gin.Context) {
	if h.routingCache != nil {
		h.routingCache.Clear()
	}
	c.JSON(http.StatusOK, gin.H{"message": "Cache statistics reset successfully"})
}

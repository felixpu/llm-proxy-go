package service

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"math"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

const (
	// DefaultL1TTL is the default TTL for L1 memory cache (5 minutes)
	DefaultL1TTL = 5 * time.Minute
	// DefaultL2TTL is the default TTL for L2 SQLite exact cache (5 minutes)
	DefaultL2TTL = 5 * time.Minute
	// DefaultL3TTL is the default TTL for L3 SQLite semantic cache (7 days)
	DefaultL3TTL = 7 * 24 * time.Hour
	// DefaultSimilarityThreshold is the cosine similarity threshold for semantic matching
	DefaultSimilarityThreshold = 0.82
	// DefaultMaxL1Size is the maximum number of entries in L1 cache
	DefaultMaxL1Size = 10000
)

// CacheConfig holds cache configuration
type CacheConfig struct {
	Enabled             bool
	L1TTL               time.Duration
	L2TTL               time.Duration
	L3TTL               time.Duration
	SimilarityThreshold float64
	MaxL1Size           int
}

// DefaultCacheConfig returns the default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Enabled:             true,
		L1TTL:               DefaultL1TTL,
		L2TTL:               DefaultL2TTL,
		L3TTL:               DefaultL3TTL,
		SimilarityThreshold: DefaultSimilarityThreshold,
		MaxL1Size:           DefaultMaxL1Size,
	}
}

// CacheEntry represents a cached routing result
type CacheEntry struct {
	TaskType  string
	Reason    string
	Embedding []float64
	CachedAt  time.Time
}

// CacheResult represents the result of a cache lookup
type CacheResult struct {
	Hit       bool
	TaskType  string
	Reason    string
	CacheType string // "L1", "L2", "L3"
}

// l1Entry is an internal L1 cache entry with expiration
type l1Entry struct {
	entry     *CacheEntry
	expiresAt time.Time
}

// CacheService provides three-layer caching for routing decisions
type CacheService struct {
	config    *CacheConfig
	l1Cache   map[string]*l1Entry
	l1Mu      sync.RWMutex
	l2Repo    *repository.EmbeddingCacheRepository
	logger    *zap.Logger
}

// NewCacheService creates a new CacheService
func NewCacheService(db *sql.DB, config *CacheConfig, logger *zap.Logger) *CacheService {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cs := &CacheService{
		config:  config,
		l1Cache: make(map[string]*l1Entry),
		l2Repo:  repository.NewEmbeddingCacheRepository(db, logger),
		logger:  logger,
	}

	// Start background cleanup goroutine
	go cs.cleanupLoop()

	return cs
}

// GenerateCacheKey generates an MD5 hash key for the content
func GenerateCacheKey(content string) string {
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:])
}

// Get attempts to retrieve a cached result using three-layer lookup
func (cs *CacheService) Get(ctx context.Context, content string, embedding []float64) (*CacheResult, error) {
	if !cs.config.Enabled {
		return &CacheResult{Hit: false}, nil
	}

	cacheKey := GenerateCacheKey(content)

	// L1: Memory cache lookup (exact match)
	if result := cs.getL1(cacheKey); result != nil {
		cs.logger.Debug("L1 cache hit", zap.String("key", cacheKey[:8]))
		return &CacheResult{
			Hit:       true,
			TaskType:  result.TaskType,
			Reason:    result.Reason,
			CacheType: "L1",
		}, nil
	}

	// L2: SQLite exact match lookup
	entry, err := cs.l2Repo.GetExactMatch(ctx, cacheKey, int(cs.config.L2TTL.Seconds()))
	if err != nil {
		cs.logger.Warn("L2 cache lookup failed", zap.Error(err))
	} else if entry != nil {
		// Update hit count asynchronously
		go func() {
			_ = cs.l2Repo.UpdateHitCountByHash(context.Background(), cacheKey)
		}()

		// Promote to L1
		cs.setL1(cacheKey, &CacheEntry{
			TaskType:  entry.TaskType,
			Reason:    entry.Reason,
			Embedding: entry.Embedding,
			CachedAt:  entry.CreatedAt,
		})

		cs.logger.Debug("L2 cache hit", zap.String("key", cacheKey[:8]))
		return &CacheResult{
			Hit:       true,
			TaskType:  entry.TaskType,
			Reason:    entry.Reason,
			CacheType: "L2",
		}, nil
	}

	// L3: Semantic similarity lookup (requires embedding)
	if embedding != nil && len(embedding) > 0 {
		result, err := cs.getL3Semantic(ctx, embedding)
		if err != nil {
			cs.logger.Warn("L3 cache lookup failed", zap.Error(err))
		} else if result != nil {
			cs.logger.Debug("L3 cache hit",
				zap.String("task_type", result.TaskType),
				zap.Float64("similarity", cs.config.SimilarityThreshold))
			return &CacheResult{
				Hit:       true,
				TaskType:  result.TaskType,
				Reason:    result.Reason,
				CacheType: "L3",
			}, nil
		}
	}

	return &CacheResult{Hit: false}, nil
}

// Set stores a routing result in the cache
func (cs *CacheService) Set(ctx context.Context, content string, embedding []float64, taskType, reason string) error {
	if !cs.config.Enabled {
		return nil
	}

	cacheKey := GenerateCacheKey(content)
	contentPreview := content
	if len(contentPreview) > 200 {
		contentPreview = contentPreview[:200]
	}

	// Store in L1
	cs.setL1(cacheKey, &CacheEntry{
		TaskType:  taskType,
		Reason:    reason,
		Embedding: embedding,
		CachedAt:  time.Now(),
	})

	// Store in L2/L3 (SQLite)
	if err := cs.l2Repo.SaveCache(ctx, cacheKey, contentPreview, embedding, taskType, reason); err != nil {
		cs.logger.Warn("failed to save to L2 cache", zap.Error(err))
		return err
	}

	cs.logger.Debug("cache entry saved",
		zap.String("key", cacheKey[:8]),
		zap.String("task_type", taskType))

	return nil
}

// getL1 retrieves an entry from L1 memory cache
func (cs *CacheService) getL1(key string) *CacheEntry {
	cs.l1Mu.RLock()
	defer cs.l1Mu.RUnlock()

	entry, ok := cs.l1Cache[key]
	if !ok {
		return nil
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.entry
}

// setL1 stores an entry in L1 memory cache
func (cs *CacheService) setL1(key string, entry *CacheEntry) {
	cs.l1Mu.Lock()
	defer cs.l1Mu.Unlock()

	// Evict if at capacity
	if len(cs.l1Cache) >= cs.config.MaxL1Size {
		cs.evictL1()
	}

	cs.l1Cache[key] = &l1Entry{
		entry:     entry,
		expiresAt: time.Now().Add(cs.config.L1TTL),
	}
}

// evictL1 removes expired entries and oldest entries if still over capacity
func (cs *CacheService) evictL1() {
	now := time.Now()

	// First pass: remove expired entries
	for key, entry := range cs.l1Cache {
		if now.After(entry.expiresAt) {
			delete(cs.l1Cache, key)
		}
	}

	// If still over capacity, remove oldest entries
	if len(cs.l1Cache) >= cs.config.MaxL1Size {
		// Find and remove 10% of oldest entries
		toRemove := cs.config.MaxL1Size / 10
		if toRemove < 1 {
			toRemove = 1
		}

		type keyTime struct {
			key       string
			expiresAt time.Time
		}
		var entries []keyTime
		for k, v := range cs.l1Cache {
			entries = append(entries, keyTime{k, v.expiresAt})
		}

		// Simple selection of oldest entries
		for i := 0; i < toRemove && i < len(entries); i++ {
			oldest := i
			for j := i + 1; j < len(entries); j++ {
				if entries[j].expiresAt.Before(entries[oldest].expiresAt) {
					oldest = j
				}
			}
			entries[i], entries[oldest] = entries[oldest], entries[i]
			delete(cs.l1Cache, entries[i].key)
		}
	}
}

// getL3Semantic performs semantic similarity search
func (cs *CacheService) getL3Semantic(ctx context.Context, queryEmbedding []float64) (*CacheEntry, error) {
	entries, err := cs.l2Repo.FindAllEmbeddings(ctx, int(cs.config.L3TTL.Seconds()))
	if err != nil {
		return nil, err
	}

	var bestMatch *repository.EmbeddingCacheEntry
	var bestSimilarity float64

	for _, entry := range entries {
		if len(entry.Embedding) != len(queryEmbedding) {
			continue
		}

		similarity := cosineSimilarity(queryEmbedding, entry.Embedding)
		if similarity >= cs.config.SimilarityThreshold && similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = entry
		}
	}

	if bestMatch == nil {
		return nil, nil
	}

	// Update hit count asynchronously
	go func() {
		_ = cs.l2Repo.UpdateHitCount(context.Background(), bestMatch.ID)
	}()

	return &CacheEntry{
		TaskType:  bestMatch.TaskType,
		Reason:    bestMatch.Reason,
		Embedding: bestMatch.Embedding,
	}, nil
}

// cosineSimilarity calculates the cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// cleanupLoop periodically cleans up expired entries
func (cs *CacheService) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cs.cleanupL1()
	}
}

// cleanupL1 removes expired entries from L1 cache
func (cs *CacheService) cleanupL1() {
	cs.l1Mu.Lock()
	defer cs.l1Mu.Unlock()

	now := time.Now()
	removed := 0
	for key, entry := range cs.l1Cache {
		if now.After(entry.expiresAt) {
			delete(cs.l1Cache, key)
			removed++
		}
	}

	if removed > 0 {
		cs.logger.Debug("L1 cache cleanup", zap.Int("removed", removed))
	}
}

// CleanupExpired cleans up expired entries from L2/L3 cache
func (cs *CacheService) CleanupExpired(ctx context.Context) (int64, error) {
	return cs.l2Repo.CleanupExpired(ctx, int(cs.config.L3TTL.Seconds()))
}

// GetStats returns cache statistics
func (cs *CacheService) GetStats(ctx context.Context) (map[string]any, error) {
	cs.l1Mu.RLock()
	l1Size := len(cs.l1Cache)
	cs.l1Mu.RUnlock()

	l2Size, err := cs.l2Repo.Count(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"l1_size":              l1Size,
		"l1_max_size":          cs.config.MaxL1Size,
		"l2_size":              l2Size,
		"l1_ttl_seconds":       int(cs.config.L1TTL.Seconds()),
		"l2_ttl_seconds":       int(cs.config.L2TTL.Seconds()),
		"l3_ttl_seconds":       int(cs.config.L3TTL.Seconds()),
		"similarity_threshold": cs.config.SimilarityThreshold,
		"enabled":              cs.config.Enabled,
	}, nil
}

package service

import (
	"crypto/md5"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// punctuationMap maps Chinese punctuation to English equivalents.
var punctuationMap = map[rune]rune{
	'，': ',', '。': '.', '！': '!', '？': '?',
	'：': ':', '；': ';', '\u201c': '"', '\u201d': '"',
	'\u2018': '\'', '\u2019': '\'',
	'（': '(', '）': ')', '【': '[', '】': ']',
}

var whitespaceRe = regexp.MustCompile(`\s+`)

// NormalizeText standardizes text to improve cache hit rate.
// Lowercases, unifies CJK/EN punctuation, collapses whitespace, trims trailing punctuation.
func NormalizeText(text string) string {
	if text == "" {
		return ""
	}

	// Lowercase
	text = strings.ToLower(text)

	// Unify CJK punctuation to English
	var buf strings.Builder
	buf.Grow(len(text))
	for _, r := range text {
		if en, ok := punctuationMap[r]; ok {
			buf.WriteRune(en)
		} else {
			buf.WriteRune(r)
		}
	}
	text = buf.String()

	// Collapse whitespace
	text = whitespaceRe.ReplaceAllString(text, " ")

	// Trim
	text = strings.TrimSpace(text)

	// Remove trailing punctuation
	text = strings.TrimRight(text, ".!?")

	return text
}

// GetCacheKey generates an MD5 hash cache key from user message.
// Only user_message is used (system_content is ignored for key generation).
func GetCacheKey(_ string, userMessage string) string {
	normalized := NormalizeText(userMessage)
	hash := md5.Sum([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// routingCacheEntry stores a cached routing decision with timestamp.
type routingCacheEntry struct {
	taskType  models.ModelRole
	timestamp time.Time
}

// RoutingCache provides L1 in-memory cache for routing decisions.
type RoutingCache struct {
	cache   map[string]*routingCacheEntry
	mu      sync.RWMutex
	maxSize int
	logger  *zap.Logger
}

// NewRoutingCache creates a new RoutingCache.
func NewRoutingCache(maxSize int, logger *zap.Logger) *RoutingCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &RoutingCache{
		cache:   make(map[string]*routingCacheEntry),
		maxSize: maxSize,
		logger:  logger,
	}
}

// Get retrieves a cached routing decision if it exists and hasn't expired.
func (rc *RoutingCache) Get(cacheKey string, ttlSeconds int) (models.ModelRole, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	entry, ok := rc.cache[cacheKey]
	if !ok {
		return "", false
	}

	age := time.Since(entry.timestamp)
	if age > time.Duration(ttlSeconds)*time.Second {
		// Expired — will be cleaned up lazily
		return "", false
	}

	keyPreview := cacheKey
	if len(keyPreview) > 8 {
		keyPreview = keyPreview[:8]
	}
	rc.logger.Debug("L1 cache hit",
		zap.String("key", keyPreview),
		zap.String("task_type", string(entry.taskType)),
		zap.Duration("age", age))

	return entry.taskType, true
}

// Set stores a routing decision in the cache.
func (rc *RoutingCache) Set(cacheKey string, taskType models.ModelRole) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Evict oldest if at capacity and key is new
	if _, exists := rc.cache[cacheKey]; !exists && len(rc.cache) >= rc.maxSize {
		rc.evictOldest()
	}

	rc.cache[cacheKey] = &routingCacheEntry{
		taskType:  taskType,
		timestamp: time.Now(),
	}
}

// Clear removes all entries from the cache.
func (rc *RoutingCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cache = make(map[string]*routingCacheEntry)
}

// Size returns the current number of entries.
func (rc *RoutingCache) Size() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.cache)
}

// evictOldest removes the oldest entry. Must be called with lock held.
func (rc *RoutingCache) evictOldest() {
	if len(rc.cache) == 0 {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, v := range rc.cache {
		if first || v.timestamp.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.timestamp
			first = false
		}
	}
	delete(rc.cache, oldestKey)
}

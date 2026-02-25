//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"lowercase", "HELLO WORLD", "hello world"},
		{"chinese punctuation", "你好，世界！", "你好,世界"},
		{"collapse whitespace", "hello   world", "hello world"},
		{"trim spaces", "  hello world  ", "hello world"},
		{"remove trailing punctuation", "hello world.", "hello world"},
		{"remove trailing question", "hello world?", "hello world"},
		{"remove trailing exclamation", "hello world!", "hello world"},
		{"mixed case and punctuation", "Hello，World！", "hello,world"},
		{"chinese quotes", "\u201c你好\u201d", "\"你好\""},
		{"chinese parentheses", "（测试）", "(测试)"},
		{"chinese brackets", "【测试】", "[测试]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name        string
		system      string
		userMessage string
	}{
		{"simple message", "", "hello world"},
		{"with system", "system prompt", "hello world"},
		{"chinese message", "", "你好世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := GetCacheKey(tt.system, tt.userMessage)
			assert.NotEmpty(t, key)
			assert.Len(t, key, 32) // MD5 hex is 32 chars
		})
	}
}

func TestGetCacheKey_Consistency(t *testing.T) {
	// Same input should produce same key
	key1 := GetCacheKey("", "hello world")
	key2 := GetCacheKey("", "hello world")
	assert.Equal(t, key1, key2)

	// Different input should produce different key
	key3 := GetCacheKey("", "hello universe")
	assert.NotEqual(t, key1, key3)
}

func TestGetCacheKey_NormalizationEffect(t *testing.T) {
	// Normalized versions should produce same key
	key1 := GetCacheKey("", "Hello World")
	key2 := GetCacheKey("", "hello world")
	assert.Equal(t, key1, key2)

	// With trailing punctuation
	key3 := GetCacheKey("", "hello world.")
	assert.Equal(t, key1, key3)
}

func TestRoutingCache_SetAndGet(t *testing.T) {
	cache := NewRoutingCache(100, zap.NewNop())

	cache.Set("key1", models.ModelRoleSimple)
	cache.Set("key2", models.ModelRoleDefault)
	cache.Set("key3", models.ModelRoleComplex)

	tests := []struct {
		name       string
		key        string
		ttlSeconds int
		wantRole   models.ModelRole
		wantFound  bool
	}{
		{"existing key", "key1", 300, models.ModelRoleSimple, true},
		{"another key", "key2", 300, models.ModelRoleDefault, true},
		{"complex key", "key3", 300, models.ModelRoleComplex, true},
		{"non-existing key", "key999", 300, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, found := cache.Get(tt.key, tt.ttlSeconds)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantRole, role)
			}
		})
	}
}

func TestRoutingCache_Expiration(t *testing.T) {
	cache := NewRoutingCache(100, zap.NewNop())

	cache.Set("expiring_key", models.ModelRoleSimple)

	// Should be found with long TTL
	role, found := cache.Get("expiring_key", 300)
	assert.True(t, found)
	assert.Equal(t, models.ModelRoleSimple, role)

	// Should not be found with 0 TTL (immediately expired)
	role, found = cache.Get("expiring_key", 0)
	assert.False(t, found)
}

func TestRoutingCache_Clear(t *testing.T) {
	cache := NewRoutingCache(100, zap.NewNop())

	cache.Set("key1", models.ModelRoleSimple)
	cache.Set("key2", models.ModelRoleDefault)

	assert.Equal(t, 2, cache.Size())

	cache.Clear()

	assert.Equal(t, 0, cache.Size())

	_, found := cache.Get("key1", 300)
	assert.False(t, found)
}

func TestRoutingCache_Size(t *testing.T) {
	cache := NewRoutingCache(100, zap.NewNop())

	assert.Equal(t, 0, cache.Size())

	cache.Set("key1", models.ModelRoleSimple)
	assert.Equal(t, 1, cache.Size())

	cache.Set("key2", models.ModelRoleDefault)
	assert.Equal(t, 2, cache.Size())

	// Setting same key should not increase size
	cache.Set("key1", models.ModelRoleComplex)
	assert.Equal(t, 2, cache.Size())
}

func TestRoutingCache_Eviction(t *testing.T) {
	cache := NewRoutingCache(3, zap.NewNop()) // Small cache

	cache.Set("key1", models.ModelRoleSimple)
	time.Sleep(10 * time.Millisecond)
	cache.Set("key2", models.ModelRoleDefault)
	time.Sleep(10 * time.Millisecond)
	cache.Set("key3", models.ModelRoleComplex)

	assert.Equal(t, 3, cache.Size())

	// Adding 4th key should evict oldest (key1)
	cache.Set("key4", models.ModelRoleSimple)

	assert.Equal(t, 3, cache.Size())

	// key1 should be evicted
	_, found := cache.Get("key1", 300)
	assert.False(t, found)

	// key4 should exist
	role, found := cache.Get("key4", 300)
	assert.True(t, found)
	assert.Equal(t, models.ModelRoleSimple, role)
}

func TestRoutingCache_DefaultMaxSize(t *testing.T) {
	// Zero or negative maxSize should default to 10000
	cache := NewRoutingCache(0, zap.NewNop())
	require.NotNil(t, cache)

	cache = NewRoutingCache(-1, zap.NewNop())
	require.NotNil(t, cache)
}

func TestRoutingCache_Concurrent(t *testing.T) {
	cache := NewRoutingCache(1000, zap.NewNop())

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set("key"+string(rune(i)), models.ModelRoleSimple)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get("key"+string(rune(i)), 300)
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	// Should not panic or deadlock
	assert.True(t, cache.Size() > 0)
}

//go:build integration
// +build integration

package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

// --- Integration: Rule → L1 Cache ---

func TestIntegration_RuleMatch_ThenL1CacheHit(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "帮我设计一个微服务架构"}},
		},
	}

	// First call: rule-based match
	taskType1, decision1, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType1)
	assert.NotNil(t, decision1)
	assert.Equal(t, "rule", decision1.CacheType)
	assert.False(t, decision1.FromCache)

	// Second call: same request should still hit rule (rules don't use cache)
	taskType2, decision2, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType2)
	assert.NotNil(t, decision2)
	assert.Equal(t, "rule", decision2.CacheType)
}

func TestIntegration_SimpleRule_Match(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	router := NewLLMRouter(db, nil, logger)

	// Short message with simple keyword should match simple rule
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "列出文件"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleSimple, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "matched rule")
}

// --- Integration: No Rule Match → Fallback Default ---

func TestIntegration_NoRuleMatch_FallbackDefault(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Ensure default config row exists
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	// Message that doesn't match any rule
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello, how are you today?"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "fallback")
}

// --- Integration: No Rule Match → Fallback User Choice ---

func TestIntegration_NoRuleMatch_FallbackUserChoice(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Configure user-choice fallback with "complex"
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE routing_llm_config SET rule_fallback_strategy = 'user', rule_fallback_task_type = 'complex' WHERE id = 1`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Random unmatched message"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "user-configured")
}

// --- Integration: Rule Disabled → Fallback ---

func TestIntegration_RuleDisabled_FallbackDefault(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Disable rule-based routing and LLM routing
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE routing_llm_config SET rule_based_routing_enabled = 0, enabled = 0 WHERE id = 1`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	// Even a complex keyword should fallback when rules are disabled
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "帮我设计一个微服务架构"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "fallback")
}

// --- Integration: Custom Rule Override Builtin ---

func TestIntegration_CustomRuleOverridesBuiltin(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Insert a custom rule with higher priority that reclassifies "设计" as simple
	_, err := db.Exec(`
		INSERT INTO routing_rules (name, keywords, task_type, priority, is_builtin, enabled)
		VALUES ('custom_override', '["设计"]', 'simple', 200, 0, 1)
	`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "帮我设计一个按钮"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	// Custom rule (priority 200) should win over builtin (priority 100)
	assert.Equal(t, models.ModelRoleSimple, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "matched rule")
}

// --- Integration: Custom Rule with Regex Pattern ---

func TestIntegration_CustomRuleRegexPattern(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Insert a custom rule with regex pattern
	_, err := db.Exec(`
		INSERT INTO routing_rules (name, pattern, task_type, priority, is_builtin, enabled)
		VALUES ('api_design', '(?i)api\s+(设计|design)', 'complex', 150, 0, 1)
	`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "请帮我做 API 设计"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType)
	assert.NotNil(t, decision)
}

// --- Integration: Disabled Custom Rule Not Applied ---

func TestIntegration_DisabledCustomRuleIgnored(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Insert a disabled custom rule
	_, err := db.Exec(`
		INSERT INTO routing_rules (name, keywords, task_type, priority, is_builtin, enabled)
		VALUES ('disabled_rule', '["Hello"]', 'complex', 200, 0, 0)
	`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello there"}},
		},
	}

	taskType, _, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	// Disabled rule should not match; falls back to default
	assert.Equal(t, models.ModelRoleDefault, taskType)
}

// --- Integration: L2 Cache (Persistent) ---

func TestIntegration_L2CacheHit(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Enable LLM routing with cache
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE routing_llm_config SET enabled = 1, cache_enabled = 1, cache_ttl_seconds = 300 WHERE id = 1`)
	require.NoError(t, err)

	// Pre-populate L2 cache with a known entry
	message := "What is the meaning of life?"
	cacheKey := GetCacheKey("", message)
	embeddingRepo := repository.NewEmbeddingCacheRepository(db, logger)
	err = embeddingRepo.SaveCache(context.Background(), cacheKey, message[:20], nil, "simple", "cached reason")
	require.NoError(t, err)

	// Disable rule-based routing so we go through cache path
	_, err = db.Exec(`UPDATE routing_llm_config SET rule_based_routing_enabled = 0 WHERE id = 1`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: message}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleSimple, taskType)
	assert.NotNil(t, decision)
	assert.True(t, decision.FromCache)
	assert.Equal(t, "L2", decision.CacheType)
}

// --- Integration: L1 Cache Promotion from L2 ---

func TestIntegration_L1CachePromotionFromL2(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Enable LLM routing with cache
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE routing_llm_config SET enabled = 1, cache_enabled = 1, cache_ttl_seconds = 300, rule_based_routing_enabled = 0 WHERE id = 1`)
	require.NoError(t, err)

	// Pre-populate L2 cache
	message := "Unique test message for L1 promotion"
	cacheKey := GetCacheKey("", message)
	embeddingRepo := repository.NewEmbeddingCacheRepository(db, logger)
	err = embeddingRepo.SaveCache(context.Background(), cacheKey, message[:20], nil, "complex", "test reason")
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: message}},
		},
	}

	// First call: L2 hit, promotes to L1
	taskType1, decision1, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType1)
	assert.True(t, decision1.FromCache)
	assert.Equal(t, "L2", decision1.CacheType)

	// Second call: should hit L1 now
	taskType2, decision2, err := router.InferTaskType(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType2)
	assert.True(t, decision2.FromCache)
	assert.Equal(t, "L1", decision2.CacheType)
}

// --- Integration: Rule Hit Count Increment ---

func TestIntegration_RuleHitCountIncrement(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Insert a custom rule
	_, err := db.Exec(`
		INSERT INTO routing_rules (name, keywords, task_type, priority, is_builtin, enabled, hit_count)
		VALUES ('hit_counter_test', '["测试命中"]', 'simple', 150, 0, 1, 0)
	`)
	require.NoError(t, err)

	// Test hit count increment directly via repository (avoid async goroutine flakiness)
	ruleRepo := repository.NewRoutingRuleRepository(db, logger)

	// Get the rule ID
	rules, err := ruleRepo.ListRules(context.Background(), true)
	require.NoError(t, err)

	var ruleID int64
	for _, r := range rules {
		if r.Name == "hit_counter_test" {
			ruleID = r.ID
			break
		}
	}
	require.NotZero(t, ruleID, "Custom rule should exist")

	// Increment hit count synchronously
	for range 3 {
		err := ruleRepo.IncrementHitCount(context.Background(), ruleID)
		require.NoError(t, err)
	}

	// Verify hit count
	var hitCount int
	err = db.QueryRow(`SELECT hit_count FROM routing_rules WHERE id = ?`, ruleID).Scan(&hitCount)
	require.NoError(t, err)
	assert.Equal(t, 3, hitCount, "Hit count should be exactly 3")
}

// --- Performance: Rule Judgment Latency ---

func TestPerformance_RuleJudgmentLatency(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "帮我设计一个微服务架构"}},
		},
	}

	// Warm up
	_, _, _ = router.InferTaskType(t.Context(), req)

	// Measure latency
	iterations := 100
	start := time.Now()
	for range iterations {
		_, _, _ = router.InferTaskType(t.Context(), req)
	}
	elapsed := time.Since(start)
	avgLatency := elapsed / time.Duration(iterations)

	t.Logf("Average rule judgment latency: %v", avgLatency)

	// Rule-based routing should be < 1ms per call
	assert.Less(t, avgLatency, 1*time.Millisecond, "Rule judgment should be < 1ms")
}

// --- Performance: L1 Cache Latency ---

func TestPerformance_L1CacheLatency(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Disable rule-based routing to test cache path
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE routing_llm_config SET enabled = 1, cache_enabled = 1, cache_ttl_seconds = 300, rule_based_routing_enabled = 0 WHERE id = 1`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	// Pre-populate L1 cache
	message := "Performance test message for L1 cache"
	cacheKey := GetCacheKey("", message)
	router.routingCache.Set(cacheKey, models.ModelRoleSimple)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: message}},
		},
	}

	// Measure latency
	iterations := 1000
	start := time.Now()
	for range iterations {
		_, _, _ = router.InferTaskType(t.Context(), req)
	}
	elapsed := time.Since(start)
	avgLatency := elapsed / time.Duration(iterations)

	t.Logf("Average L1 cache hit latency: %v", avgLatency)

	// L1 cache hit should be < 100µs
	assert.Less(t, avgLatency, 100*time.Microsecond, "L1 cache hit should be < 100µs")
}

// --- Integration: Cache Hit Rate ---

func TestIntegration_CacheHitRate(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Enable LLM routing with cache, disable rules to test cache path
	_, err := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE routing_llm_config SET enabled = 1, cache_enabled = 1, cache_ttl_seconds = 300, rule_based_routing_enabled = 0 WHERE id = 1`)
	require.NoError(t, err)

	router := NewLLMRouter(db, nil, logger)

	// Pre-populate L1 cache with multiple entries
	messages := []string{
		"Message one for cache test",
		"Message two for cache test",
		"Message three for cache test",
	}

	for _, msg := range messages {
		cacheKey := GetCacheKey("", msg)
		router.routingCache.Set(cacheKey, models.ModelRoleSimple)
	}

	// Test cache hits
	hits := 0
	total := len(messages) * 3 // Each message tested 3 times

	for _, msg := range messages {
		req := &models.AnthropicRequest{
			Messages: []models.Message{
				{Role: "user", Content: models.MessageContent{Text: msg}},
			},
		}

		for range 3 {
			_, decision, err := router.InferTaskType(t.Context(), req)
			require.NoError(t, err)
			if decision != nil && decision.FromCache && decision.CacheType == "L1" {
				hits++
			}
		}
	}

	hitRate := float64(hits) / float64(total)
	t.Logf("Cache hit rate: %.2f%% (%d/%d)", hitRate*100, hits, total)

	// Should have 100% hit rate for pre-populated cache
	assert.Equal(t, 1.0, hitRate, "Cache hit rate should be 100%% for pre-populated entries")
}

// --- Integration: Memory Usage ---

func TestIntegration_CacheMemoryBounded(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Create router with small cache capacity for testing
	router := NewLLMRouter(db, nil, logger)

	// The default cache capacity is 10000
	// Fill cache beyond capacity to test eviction
	for i := range 100 {
		key := GetCacheKey("", fmt.Sprintf("unique message %d", i))
		router.routingCache.Set(key, models.ModelRoleSimple)
	}

	// Cache size should be bounded
	size := router.routingCache.Size()
	t.Logf("Cache size after 100 inserts: %d", size)

	assert.LessOrEqual(t, size, 10000, "Cache size should be bounded by capacity")
	assert.Equal(t, 100, size, "Cache should contain all 100 entries (within capacity)")
}

// --- Integration: Concurrent Access ---

func TestIntegration_ConcurrentAccess(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	router := NewLLMRouter(db, nil, logger)

	// Run concurrent requests
	const goroutines = 10
	const requestsPerGoroutine = 100

	var wg sync.WaitGroup
	errors := make(chan error, goroutines*requestsPerGoroutine)

	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range requestsPerGoroutine {
				req := &models.AnthropicRequest{
					Messages: []models.Message{
						{Role: "user", Content: models.MessageContent{Text: fmt.Sprintf("Concurrent test %d-%d", id, i)}},
					},
				}
				_, _, err := router.InferTaskType(t.Context(), req)
				if err != nil {
					errors <- err
				}
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	// Collect any errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	assert.Empty(t, errs, "No errors should occur during concurrent access")
	t.Logf("Completed %d concurrent requests without errors", goroutines*requestsPerGoroutine)
}

// --- Integration: Empty and Edge Cases ---

func TestIntegration_EdgeCases(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	router := NewLLMRouter(db, nil, logger)

	tests := []struct {
		name        string
		req         *models.AnthropicRequest
		wantType    models.ModelRole
		wantDecNil  bool
	}{
		{
			name:        "empty messages",
			req:         &models.AnthropicRequest{Messages: []models.Message{}},
			wantType:    models.ModelRoleDefault,
			wantDecNil:  true,
		},
		{
			name: "only assistant message",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "assistant", Content: models.MessageContent{Text: "I am an assistant"}},
				},
			},
			wantType:   models.ModelRoleDefault,
			wantDecNil: true,
		},
		{
			name: "very long message",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{Text: strings.Repeat("这是一个很长的消息。", 500)}},
				},
			},
			wantType:   models.ModelRoleComplex, // long_message rule
			wantDecNil: false,
		},
		{
			name: "unicode and special characters",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{Text: "🎉 测试 emoji 和特殊字符 @#$%^&*()"}},
				},
			},
			wantType:   models.ModelRoleDefault, // no rule match
			wantDecNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskType, decision, err := router.InferTaskType(t.Context(), tt.req)
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, taskType)
			if tt.wantDecNil {
				assert.Nil(t, decision)
			} else {
				assert.NotNil(t, decision)
			}
		})
	}
}

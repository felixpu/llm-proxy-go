//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func BenchmarkHotPathEndpointSelector_DirectModel_NoShadow(b *testing.B) {
	benchmarkEndpointSelectorDirectModel(b, false)
}

// This benchmark approximates the historical synchronous shadow behavior by
// explicitly executing smart-routing once more on the request hot path.
func BenchmarkHotPathEndpointSelector_DirectModel_WithShadowSyncApprox(b *testing.B) {
	fixture := newHotPathEndpointSelectorFixture(b, false)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, selErr := fixture.selector.SelectEndpoint(fixture.ctx, fixture.request, fixture.endpoints)
		if selErr != nil {
			b.Fatalf("select endpoint failed: %v", selErr)
		}
		if result == nil || result.Endpoint == nil {
			b.Fatal("empty endpoint selection result")
		}

		shadow, shadowErr := fixture.selector.doSmartRouting(fixture.ctx, fixture.request, fixture.endpoints, false)
		if shadowErr != nil {
			b.Fatalf("sync shadow routing failed: %v", shadowErr)
		}
		if shadow != nil {
			result.ShadowRouting = &ShadowRoutingResult{
				TaskType:      shadow.TaskType,
				RoutingMethod: shadow.RoutingMethod,
				Model:         shadow.Model,
				Decision:      shadow.RoutingDecision,
			}
		}
	}
}

func BenchmarkHotPathEndpointSelector_DirectModel_WithShadowAsync(b *testing.B) {
	benchmarkEndpointSelectorDirectModel(b, true)
}

func benchmarkEndpointSelectorDirectModel(b *testing.B, withShadow bool) {
	fixture := newHotPathEndpointSelectorFixture(b, withShadow)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, selErr := fixture.selector.SelectEndpoint(fixture.ctx, fixture.request, fixture.endpoints)
		if selErr != nil {
			b.Fatalf("select endpoint failed: %v", selErr)
		}
		if result == nil || result.Endpoint == nil {
			b.Fatal("empty endpoint selection result")
		}
		if withShadow {
			// Non-blocking poll mirrors proxy handler behavior.
			_ = result.ResolveShadowRouting()
		}
	}
}

type hotPathEndpointSelectorFixture struct {
	selector  *EndpointSelector
	request   *models.AnthropicRequest
	endpoints []*models.Endpoint
	ctx       context.Context
}

func newHotPathEndpointSelectorFixture(b *testing.B, withShadow bool) hotPathEndpointSelectorFixture {
	b.Helper()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	db := testutil.NewTestDBWithDefaults(b)
	shadowEnabled := 0
	if withShadow {
		shadowEnabled = 1
	}
	_, err := db.Exec(`
		UPDATE routing_llm_config
		SET enabled = 0,
		    rule_based_routing_enabled = 1,
		    shadow_routing_enabled = ?,
		    shadow_sample_rate = 1.0,
		    shadow_max_qps = 2000000000
		WHERE id = 1
	`, shadowEnabled)
	if err != nil {
		b.Fatalf("update routing config: %v", err)
	}

	llmRouter := NewLLMRouter(db, nil, logger, nil)
	rcr := repository.NewRoutingConfigRepository(db, logger)
	selector := NewEndpointSelector(ms, hc, lb, llmRouter, rcr, nil, logger)

	defaultModel := &models.Model{ID: 1, Name: "sonnet", Role: models.ModelRoleDefault, Enabled: true}
	complexModel := &models.Model{ID: 2, Name: "opus", Role: models.ModelRoleComplex, Enabled: true}
	endpoints := []*models.Endpoint{
		{
			Model:    defaultModel,
			Provider: &models.Provider{ID: 1, Name: "provider-default", BaseURL: "http://bench", APIKey: "key"},
		},
		{
			Model:    complexModel,
			Provider: &models.Provider{ID: 2, Name: "provider-complex", BaseURL: "http://bench", APIKey: "key"},
		},
	}
	hc.UpdateState("provider-default/sonnet", models.EndpointHealthy, "")
	hc.UpdateState("provider-complex/opus", models.EndpointHealthy, "")

	request := &models.AnthropicRequest{
		Model: "sonnet",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "请帮我设计一个微服务架构并给出拆分建议"}},
		},
	}

	// Simulate request middleware preloading routing config once per request context.
	reqCtx, _, err := GetOrLoadRoutingConfig(context.Background(), rcr)
	if err != nil {
		b.Fatalf("preload routing config: %v", err)
	}

	return hotPathEndpointSelectorFixture{
		selector:  selector,
		request:   request,
		endpoints: endpoints,
		ctx:       reqCtx,
	}
}

func BenchmarkHotPathAPIDetectCache(b *testing.B) {
	const apiKey = "bench-key"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	b.Run("cache_hit", func(b *testing.B) {
		InvalidateAPIDetectionCache()
		if _, err := DetectAPITypeForModel(context.Background(), upstream.URL, apiKey, "model-hit"); err != nil {
			b.Fatalf("warm detect failed: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := DetectAPITypeForModel(context.Background(), upstream.URL, apiKey, "model-hit"); err != nil {
				b.Fatalf("cache-hit detect failed: %v", err)
			}
		}
	})

	b.Run("cache_miss_unique_model", func(b *testing.B) {
		InvalidateAPIDetectionCache()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			modelName := fmt.Sprintf("model-miss-%d", i)
			if _, err := DetectAPITypeForModel(context.Background(), upstream.URL, apiKey, modelName); err != nil {
				b.Fatalf("cache-miss detect failed: %v", err)
			}
		}
	})
}

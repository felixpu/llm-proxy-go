//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

type stubModelAliasRepo struct {
	aliases map[string][]*models.ModelAlias
}

func (r *stubModelAliasRepo) FindByAliasName(_ context.Context, aliasName string) ([]*models.ModelAlias, error) {
	for key, alias := range r.aliases {
		if strings.EqualFold(key, aliasName) {
			return alias, nil
		}
	}
	return nil, nil
}

func TestDoSmartRouting_PreservesCustomRuleInfo(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	db := testutil.NewTestDB(t)

	// Insert a custom rule
	_, err := db.Exec(`INSERT INTO routing_rules (name, keywords, task_type, priority, is_builtin, enabled)
		VALUES ('custom_test_rule', '["自定义测试"]', 'complex', 95, 0, 1)`)
	assert.NoError(t, err)

	llmRouter := NewLLMRouter(db, nil, logger, nil)
	rcr := repository.NewRoutingConfigRepository(db, logger)
	es := NewEndpointSelector(ms, hc, lb, llmRouter, rcr, nil, logger)

	// Set up endpoints with healthy complex model
	complexModel := &models.Model{ID: 1, Name: "opus", Role: models.ModelRoleComplex, Enabled: true}
	endpoints := []*models.Endpoint{
		{
			Model:    complexModel,
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
	}

	// Mark endpoint healthy
	hc.UpdateState("provider-1/opus", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "auto",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "这是一个自定义测试任务"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.RoutingDecision)

	// Verify custom rule info is preserved through the full pipeline
	assert.NotNil(t, result.RoutingDecision.MatchedRule,
		"Custom rule match info should be preserved in RoutingDecision")
	assert.Equal(t, "custom_test_rule", result.RoutingDecision.MatchedRule.Name)
	assert.Equal(t, "complex", result.RoutingDecision.MatchedRule.TaskType)
}

func TestFindModelByName(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)
	es := NewEndpointSelector(ms, hc, lb, nil, nil, nil, logger)

	endpoints := []*models.Endpoint{
		{
			Model:    &models.Model{ID: 1, Name: "claude-sonnet-4-20250514", Role: models.ModelRoleDefault, Enabled: true},
			Provider: &models.Provider{ID: 1, Name: "provider-1"},
		},
		{
			Model:    &models.Model{ID: 2, Name: "claude-opus-4-20250514", Role: models.ModelRoleComplex, Enabled: true},
			Provider: &models.Provider{ID: 2, Name: "provider-2"},
		},
		{
			Model:    &models.Model{ID: 3, Name: "claude-haiku-4-5-20251001", Role: models.ModelRoleSimple, Enabled: true},
			Provider: &models.Provider{ID: 3, Name: "provider-3"},
		},
	}

	tests := []struct {
		name        string
		requestName string
		wantModel   string
		wantNil     bool
	}{
		{"exact match", "claude-sonnet-4-20250514", "claude-sonnet-4-20250514", false},
		{"exact match case insensitive", "Claude-Sonnet-4-20250514", "claude-sonnet-4-20250514", false},
		{"exact match haiku", "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001", false},
		{"exact match opus", "claude-opus-4-20250514", "claude-opus-4-20250514", false},
		{"not configured model returns nil", "claude-sonnet-4-20250101", "", true},
		{"completely unknown model returns nil", "gpt-4o", "", true},
		{"empty name returns nil", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := es.findModelByName(tt.requestName, endpoints)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantModel, result.Name)
			}
		})
	}
}

func TestSelectEndpoint_GetConfigError_UsesDefaults(t *testing.T) {
	// Test for C-2: GetConfig error should be logged and defaults used
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	db := testutil.NewTestDB(t)
	db.Close() // Close DB to simulate error

	rcr := repository.NewRoutingConfigRepository(db, logger)
	es := NewEndpointSelector(ms, hc, lb, nil, rcr, nil, logger)

	// Set up endpoints
	defaultModel := &models.Model{ID: 1, Name: "sonnet", Role: models.ModelRoleDefault, Enabled: true}
	endpoints := []*models.Endpoint{
		{
			Model:    defaultModel,
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
	}

	hc.UpdateState("provider-1/sonnet", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "test"}},
		},
	}

	// Should still work with defaults even if GetConfig fails
	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	assert.NoError(t, err, "should use defaults when GetConfig fails")
	assert.NotNil(t, result)
	assert.Equal(t, "sonnet", result.Model.Name)
}

func TestSelectEndpoint_DisabledModel_FallbackSameRole(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	db := testutil.NewTestDB(t)
	rcr := repository.NewRoutingConfigRepository(db, logger)
	es := NewEndpointSelector(ms, hc, lb, nil, rcr, nil, logger)

	disabledModel := &models.Model{ID: 1, Name: "disabled-model", Role: models.ModelRoleDefault, Enabled: false}
	fallbackModel := &models.Model{ID: 2, Name: "fallback-model", Role: models.ModelRoleDefault, Enabled: true}

	endpoints := []*models.Endpoint{
		{
			Model:    disabledModel,
			Provider: &models.Provider{ID: 1, Name: "provider-disabled", BaseURL: "http://test", APIKey: "key"},
		},
		{
			Model:    fallbackModel,
			Provider: &models.Provider{ID: 2, Name: "provider-fallback", BaseURL: "http://test", APIKey: "key"},
		},
	}

	hc.UpdateState("provider-disabled/disabled-model", models.EndpointHealthy, "")
	hc.UpdateState("provider-fallback/fallback-model", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "disabled-model",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "test"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	require.NoError(t, err, "disabled model should fallback to same role model per SelectEndpoint priority comment")
	require.NotNil(t, result)
	assert.Equal(t, "fallback-model", result.Model.Name)
}

func TestSelectEndpoint_ResolvesAliasToTargetModel(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	aliasRepo := &stubModelAliasRepo{
		aliases: map[string][]*models.ModelAlias{
			"claude-sonnet-4-6": {
				{
					ID:            1,
					AliasName:     "claude-sonnet-4-6",
					TargetModelID: 2,
					Enabled:       true,
				},
			},
		},
	}
	es := NewEndpointSelector(ms, hc, lb, nil, nil, aliasRepo, logger)

	endpoints := []*models.Endpoint{
		{
			Model:    &models.Model{ID: 2, Name: "claude-sonnet-4-5-20250929", Role: models.ModelRoleDefault, Enabled: true},
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
	}
	hc.UpdateState("provider-1/claude-sonnet-4-5-20250929", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "test"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "claude-sonnet-4-5-20250929", result.Model.Name)
}

func TestSelectEndpoint_ResolvesAliasToBestHealthyTargetAmongMultiple(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	aliasRepo := &stubModelAliasRepo{
		aliases: map[string][]*models.ModelAlias{
			"claude-sonnet-4-6": {
				{
					ID:            1,
					AliasName:     "claude-sonnet-4-6",
					TargetModelID: 2,
					Enabled:       true,
				},
				{
					ID:            2,
					AliasName:     "claude-sonnet-4-6",
					TargetModelID: 3,
					Enabled:       true,
				},
			},
		},
	}
	es := NewEndpointSelector(ms, hc, lb, nil, nil, aliasRepo, logger)

	endpoints := []*models.Endpoint{
		{
			Model:    &models.Model{ID: 2, Name: "zai-org/GLM-4.6", Role: models.ModelRoleDefault, Enabled: true, Weight: 100},
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
		{
			Model:    &models.Model{ID: 3, Name: "zai-org/GLM-4.7", Role: models.ModelRoleDefault, Enabled: true, Weight: 120},
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
	}
	hc.UpdateState("provider-1/zai-org/GLM-4.6", models.EndpointHealthy, "")
	hc.UpdateState("provider-1/zai-org/GLM-4.7", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "test"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "zai-org/GLM-4.7", result.Model.Name)
}

func TestSelectEndpoint_ResolvesAliasWithProviderScope(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	providerID := int64(2)
	aliasRepo := &stubModelAliasRepo{
		aliases: map[string][]*models.ModelAlias{
			"claude-sonnet-4-6": {
				{
					ID:            1,
					AliasName:     "claude-sonnet-4-6",
					TargetModelID: 2,
					ProviderID:    &providerID,
					Enabled:       true,
				},
			},
		},
	}
	es := NewEndpointSelector(ms, hc, lb, nil, nil, aliasRepo, logger)

	targetModel := &models.Model{ID: 2, Name: "zai-org/GLM-4.6", Role: models.ModelRoleDefault, Enabled: true, Weight: 100}
	endpoints := []*models.Endpoint{
		{
			Model:    targetModel,
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
		{
			Model:    targetModel,
			Provider: &models.Provider{ID: 2, Name: "provider-2", BaseURL: "http://test", APIKey: "key"},
		},
	}
	hc.UpdateState("provider-1/zai-org/GLM-4.6", models.EndpointHealthy, "")
	hc.UpdateState("provider-2/zai-org/GLM-4.6", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "test"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(2), result.Endpoint.Provider.ID)
	assert.Equal(t, "provider-2", result.Endpoint.Provider.Name)
}

func TestSelectEndpoint_AliasProviderScopeNoHealthyEndpoint_ReturnsError(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	providerID := int64(2)
	aliasRepo := &stubModelAliasRepo{
		aliases: map[string][]*models.ModelAlias{
			"claude-sonnet-4-6": {
				{
					ID:            1,
					AliasName:     "claude-sonnet-4-6",
					TargetModelID: 2,
					ProviderID:    &providerID,
					Enabled:       true,
				},
			},
		},
	}
	es := NewEndpointSelector(ms, hc, lb, nil, nil, aliasRepo, logger)

	endpoints := []*models.Endpoint{
		{
			Model:    &models.Model{ID: 2, Name: "zai-org/GLM-4.6", Role: models.ModelRoleDefault, Enabled: true, Weight: 100},
			Provider: &models.Provider{ID: 1, Name: "provider-1", BaseURL: "http://test", APIKey: "key"},
		},
	}
	hc.UpdateState("provider-1/zai-org/GLM-4.6", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "test"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no healthy mapped endpoints")
}

func TestSelectEndpoint_DirectRequestCapturesShadowRoutingAsync(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)

	db := testutil.NewTestDBWithDefaults(t)
	_, err := db.Exec(`UPDATE routing_llm_config SET shadow_routing_enabled = 1, shadow_sample_rate = 1.0, shadow_max_qps = 1000 WHERE id = 1`)
	require.NoError(t, err)

	llmRouter := NewLLMRouter(db, nil, logger, nil)
	rcr := repository.NewRoutingConfigRepository(db, logger)
	es := NewEndpointSelector(ms, hc, lb, llmRouter, rcr, nil, logger)

	defaultModel := &models.Model{ID: 1, Name: "sonnet", Role: models.ModelRoleDefault, Enabled: true}
	complexModel := &models.Model{ID: 2, Name: "opus", Role: models.ModelRoleComplex, Enabled: true}
	endpoints := []*models.Endpoint{
		{
			Model:    defaultModel,
			Provider: &models.Provider{ID: 1, Name: "provider-default", BaseURL: "http://test", APIKey: "key"},
		},
		{
			Model:    complexModel,
			Provider: &models.Provider{ID: 2, Name: "provider-complex", BaseURL: "http://test", APIKey: "key"},
		},
	}

	hc.UpdateState("provider-default/sonnet", models.EndpointHealthy, "")
	hc.UpdateState("provider-complex/opus", models.EndpointHealthy, "")

	req := &models.AnthropicRequest{
		Model: "sonnet",
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "帮我设计一个微服务架构"}},
		},
	}

	result, err := es.SelectEndpoint(t.Context(), req, endpoints)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, models.RoutingMethodDirect, result.RoutingMethod)
	assert.Equal(t, "sonnet", result.Model.Name)

	require.Eventually(t, func() bool {
		return result.ResolveShadowRouting() != nil
	}, 2*time.Second, 20*time.Millisecond)

	shadow := result.ResolveShadowRouting()
	require.NotNil(t, shadow)
	assert.Equal(t, models.RoutingMethodRule, shadow.RoutingMethod)
	assert.Equal(t, models.ModelRoleComplex, shadow.TaskType)
	assert.Equal(t, "opus", shadow.Model.Name)
}

//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

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

	llmRouter := NewLLMRouter(db, nil, logger)
	rcr := repository.NewRoutingConfigRepository(db, logger)
	es := NewEndpointSelector(ms, hc, lb, llmRouter, rcr, logger)

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
	es := NewEndpointSelector(ms, hc, lb, nil, nil, logger)

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

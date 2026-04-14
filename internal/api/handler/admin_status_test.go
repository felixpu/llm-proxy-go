//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

type statusTestModelRepo struct {
	models []*models.Model
}

func (r *statusTestModelRepo) FindAllEnabled(context.Context) ([]*models.Model, error) {
	return r.models, nil
}

type statusTestProviderRepo struct {
	providers map[int64][]*models.Provider
}

func (r *statusTestProviderRepo) FindByModelID(_ context.Context, modelID int64) ([]*models.Provider, error) {
	return r.providers[modelID], nil
}

func TestStatusHandler_TestRouting_UsesDerivedRoutingMethod(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()
	router := service.NewLLMRouter(db, nil, logger, nil)
	hc := service.NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := service.NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := service.NewModelSelector(hc, logger)

	modelRepo := &statusTestModelRepo{
		models: []*models.Model{
			{ID: 1, Name: "claude-opus-4", Role: models.ModelRoleComplex, Enabled: true},
		},
	}
	providerRepo := &statusTestProviderRepo{
		providers: map[int64][]*models.Provider{
			1: {
				{ID: 1, Name: "provider-1", BaseURL: "https://example.com", APIKey: "key", Enabled: true},
			},
		},
	}
	store := service.NewEndpointStore(modelRepo, providerRepo, logger)
	require.NoError(t, store.Load(context.Background()))
	hc.UpdateState("provider-1/claude-opus-4", models.EndpointHealthy, "")
	selector := service.NewEndpointSelector(ms, hc, lb, router, nil, nil, logger)

	handler := NewStatusHandler(nil, nil, nil, router, store, selector)

	body := map[string]any{
		"model": "auto",
		"messages": []map[string]any{
			{"role": "user", "content": "帮我设计一个微服务架构"},
		},
	}

	c, w := testutil.NewTestContextWithRequest(http.MethodPost, "/api/status/routing/test", body)
	handler.TestRouting(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp RoutingTestResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, string(models.ModelRoleComplex), resp.InferredTaskType)
	assert.Equal(t, models.RoutingMethodRule, resp.RoutingMethod)
	assert.Contains(t, resp.Reasoning, "matched rule")
	assert.Equal(t, "claude-opus-4", resp.SelectedModel)
}

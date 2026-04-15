//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

type stubSystemConfigRepo struct {
	lastHealthPatch repository.SystemHealthCheckConfigPatch
	updateErr       error
}

func (s *stubSystemConfigRepo) GetRoutingConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *stubSystemConfigRepo) UpdateRoutingConfigPatch(context.Context, repository.SystemRoutingConfigPatch) error {
	return nil
}

func (s *stubSystemConfigRepo) GetLoadBalanceConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *stubSystemConfigRepo) UpdateLoadBalanceConfigPatch(context.Context, repository.SystemLoadBalanceConfigPatch) error {
	return nil
}

func (s *stubSystemConfigRepo) GetHealthCheckConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *stubSystemConfigRepo) UpdateHealthCheckConfigPatch(_ context.Context, patch repository.SystemHealthCheckConfigPatch) error {
	s.lastHealthPatch = patch
	return s.updateErr
}

func (s *stubSystemConfigRepo) GetUIConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *stubSystemConfigRepo) UpdateUIConfigPatch(context.Context, repository.SystemUIConfigPatch) error {
	return nil
}

func TestConfigHandler_UpdateHealthCheckConfig_ValidateEffectiveTimeout(t *testing.T) {
	repo := &stubSystemConfigRepo{}
	hc := service.NewHealthChecker(config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}, zap.NewNop())
	h := NewConfigHandler(repo, hc)

	body := []byte(`{"interval_seconds":8}`)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest(http.MethodPut, "/api/config/health-check", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateHealthCheckConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Nil(t, repo.lastHealthPatch.IntervalSeconds)
}

func TestConfigHandler_UpdateHealthCheckConfig_AppliesCircuitBreaker(t *testing.T) {
	repo := &stubSystemConfigRepo{}
	hc := service.NewHealthChecker(config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:                 true,
			ConsecutiveFailures:     5,
			PermanentErrorThreshold: 3,
			CooldownSeconds:         60,
			HalfOpenMaxRequests:     3,
		},
	}, zap.NewNop())
	h := NewConfigHandler(repo, hc)

	body := []byte(`{
		"cb_enabled": false,
		"cb_consecutive_failures": 8,
		"cb_permanent_error_threshold": 4,
		"cb_cooldown_seconds": 90,
		"cb_half_open_max_requests": 2
	}`)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest(http.MethodPut, "/api/config/health-check", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateHealthCheckConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.lastHealthPatch.CBEnabled)
	require.NotNil(t, repo.lastHealthPatch.CBConsecutiveFailures)
	require.NotNil(t, repo.lastHealthPatch.CBPermanentErrorThreshold)
	require.NotNil(t, repo.lastHealthPatch.CBCooldownSeconds)
	require.NotNil(t, repo.lastHealthPatch.CBHalfOpenMaxRequests)
	assert.False(t, *repo.lastHealthPatch.CBEnabled)
	assert.Equal(t, 8, *repo.lastHealthPatch.CBConsecutiveFailures)
	assert.Equal(t, 4, *repo.lastHealthPatch.CBPermanentErrorThreshold)
	assert.Equal(t, 90, *repo.lastHealthPatch.CBCooldownSeconds)
	assert.Equal(t, 2, *repo.lastHealthPatch.CBHalfOpenMaxRequests)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, true, resp["applied"])

	cfg := hc.GetConfig()
	assert.False(t, cfg.CircuitBreaker.Enabled)
	assert.Equal(t, 8, cfg.CircuitBreaker.ConsecutiveFailures)
	assert.Equal(t, 4, cfg.CircuitBreaker.PermanentErrorThreshold)
	assert.Equal(t, 90, cfg.CircuitBreaker.CooldownSeconds)
	assert.Equal(t, 2, cfg.CircuitBreaker.HalfOpenMaxRequests)
}

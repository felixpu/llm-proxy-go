//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// defaultCircuitBreakerConfig returns a default circuit breaker config for testing
func defaultCircuitBreakerConfig() config.CircuitBreakerConfig {
	return config.CircuitBreakerConfig{
		Enabled:                 true,
		ConsecutiveFailures:     5,
		PermanentErrorThreshold: 3,
		CooldownSeconds:         60,
		HalfOpenMaxRequests:     3,
	}
}

// defaultHealthCheckConfig returns a default health check config for testing
func defaultHealthCheckConfig() config.HealthCheckConfig {
	return config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
		CircuitBreaker:  defaultCircuitBreakerConfig(),
	}
}

func TestNewHealthChecker(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())
	require.NotNil(t, hc)
	assert.Equal(t, cfg.Enabled, hc.cfg.Enabled)
	assert.NotNil(t, hc.client)
	assert.NotNil(t, hc.states)
}

func TestNewHealthChecker_SanitizesInvalidConfig(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 0,
		TimeoutSeconds:  -5,
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:                 true,
			ConsecutiveFailures:     0,
			PermanentErrorThreshold: 0,
			CooldownSeconds:         0,
			HalfOpenMaxRequests:     0,
		},
	}

	hc := NewHealthChecker(cfg, zap.NewNop())
	require.NotNil(t, hc)
	assert.Equal(t, 60, hc.cfg.IntervalSeconds)
	assert.Equal(t, 10, hc.cfg.TimeoutSeconds)
	assert.Equal(t, 5, hc.cfg.CircuitBreaker.ConsecutiveFailures)
	assert.Equal(t, 3, hc.cfg.CircuitBreaker.PermanentErrorThreshold)
	assert.Equal(t, 60, hc.cfg.CircuitBreaker.CooldownSeconds)
	assert.Equal(t, 3, hc.cfg.CircuitBreaker.HalfOpenMaxRequests)
	assert.Equal(t, 10*time.Second, hc.client.Timeout)
}

func TestNewHealthChecker_DisablesCircuitBreakerWhenHealthCheckDisabled(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         false,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:                 true,
			ConsecutiveFailures:     5,
			PermanentErrorThreshold: 3,
			CooldownSeconds:         60,
			HalfOpenMaxRequests:     3,
		},
	}

	hc := NewHealthChecker(cfg, zap.NewNop())
	require.NotNil(t, hc)
	assert.False(t, hc.cfg.CircuitBreaker.Enabled)
}

func TestHealthChecker_CheckAll_NoOverlappingRuns(t *testing.T) {
	var calls int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker(config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1,
		TimeoutSeconds:  2,
	}, zap.NewNop())

	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "provider1",
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
		Model: &models.Model{Name: "model1"},
	}
	endpoints := []*models.Endpoint{ep}

	done := make(chan struct{})
	go func() {
		hc.checkAll(context.Background(), endpoints)
		close(done)
	}()

	<-started // Ensure first checkAll is in-flight.

	hc.checkAll(context.Background(), endpoints) // Should be skipped while first run is active.
	close(release)
	<-done

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestHealthChecker_ApplyConfig_Runtime(t *testing.T) {
	hc := NewHealthChecker(config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 30,
		TimeoutSeconds:  5,
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:                 true,
			ConsecutiveFailures:     5,
			PermanentErrorThreshold: 3,
			CooldownSeconds:         60,
			HalfOpenMaxRequests:     3,
		},
	}, zap.NewNop())

	cfg := hc.GetConfig()
	cfg.Enabled = false
	cfg.TimeoutSeconds = 2
	hc.ApplyConfig(cfg)

	got := hc.GetConfig()
	assert.False(t, got.Enabled)
	assert.Equal(t, 2, got.TimeoutSeconds)
	assert.False(t, got.CircuitBreaker.Enabled, "circuit breaker should be disabled when health check is disabled")
	assert.Equal(t, 2*time.Second, hc.client.Timeout)
}

func TestHealthChecker_IsHealthy_Unknown(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())

	// Unknown endpoint should return true (backward compatibility - assume healthy if not tracked)
	assert.True(t, hc.IsHealthy("unknown/endpoint"))
}

func TestHealthChecker_GetState_Unknown(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	// Unknown endpoint should return nil
	state := hc.GetState("unknown/endpoint")
	assert.Nil(t, state)
}

func TestHealthChecker_IncrementDecrementConnections(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	// Create a mock endpoint
	ep := createHealthTestEndpoint("provider1", "model1")
	endpoints := []*models.Endpoint{ep}

	// Initialize states manually (simulating Start without the goroutine)
	hc.mu.Lock()
	name := "provider1/model1"
	hc.states[name] = &EndpointState{
		Name:   name,
		Status: models.EndpointHealthy,
	}
	hc.mu.Unlock()

	// Increment connections
	hc.IncrementConnections(name)
	hc.IncrementConnections(name)

	state := hc.GetState(name)
	require.NotNil(t, state)
	assert.Equal(t, 2, state.CurrentConnections)

	// Decrement connections
	hc.DecrementConnections(name)
	state = hc.GetState(name)
	assert.Equal(t, 1, state.CurrentConnections)

	// Decrement to zero
	hc.DecrementConnections(name)
	state = hc.GetState(name)
	assert.Equal(t, 0, state.CurrentConnections)

	// Decrement below zero should stay at zero
	hc.DecrementConnections(name)
	state = hc.GetState(name)
	assert.Equal(t, 0, state.CurrentConnections)

	_ = endpoints // suppress unused warning
}

func TestHealthChecker_UpdateRequestStats(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	name := "provider1/model1"
	hc.mu.Lock()
	hc.states[name] = &EndpointState{
		Name:   name,
		Status: models.EndpointHealthy,
	}
	hc.mu.Unlock()

	// Record successful requests
	hc.UpdateRequestStats(name, true, 100.0)
	hc.UpdateRequestStats(name, true, 200.0)

	state := hc.GetState(name)
	require.NotNil(t, state)
	assert.Equal(t, 2, state.TotalRequests)
	assert.Equal(t, 0, state.TotalErrors)
	assert.Equal(t, 150.0, state.AvgResponseTimeMs)

	// Record failed request
	hc.UpdateRequestStats(name, false, 50.0)

	state = hc.GetState(name)
	assert.Equal(t, 3, state.TotalRequests)
	assert.Equal(t, 1, state.TotalErrors)
}

func TestHealthChecker_GetHealthyEndpoints(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep1 := createHealthTestEndpoint("provider1", "model1")
	ep2 := createHealthTestEndpoint("provider2", "model1")
	ep3 := createHealthTestEndpoint("provider3", "model1")
	endpoints := []*models.Endpoint{ep1, ep2, ep3}

	// Initialize states
	hc.mu.Lock()
	hc.states["provider1/model1"] = &EndpointState{Name: "provider1/model1", Status: models.EndpointHealthy, CircuitState: CircuitClosed}
	hc.states["provider2/model1"] = &EndpointState{Name: "provider2/model1", Status: models.EndpointUnhealthy, CircuitState: CircuitClosed}
	hc.states["provider3/model1"] = &EndpointState{Name: "provider3/model1", Status: models.EndpointHealthy, CircuitState: CircuitClosed}
	hc.mu.Unlock()

	healthy := hc.GetHealthyEndpoints(endpoints)
	assert.Len(t, healthy, 2)
}

func TestHealthChecker_GetHealthyEndpoints_CircuitBreaker(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep1 := createHealthTestEndpoint("provider1", "model1")
	ep2 := createHealthTestEndpoint("provider2", "model1")
	ep3 := createHealthTestEndpoint("provider3", "model1")
	endpoints := []*models.Endpoint{ep1, ep2, ep3}

	now := time.Now()

	hc.mu.Lock()
	// provider1: healthy + circuit closed -> allowed
	hc.states["provider1/model1"] = &EndpointState{
		Name: "provider1/model1", Status: models.EndpointHealthy, CircuitState: CircuitClosed,
	}
	// provider2: healthy but circuit open (recently) -> blocked
	hc.states["provider2/model1"] = &EndpointState{
		Name: "provider2/model1", Status: models.EndpointHealthy, CircuitState: CircuitOpen,
		CircuitOpenedAt: &now,
	}
	// provider3: healthy + circuit closed -> allowed
	hc.states["provider3/model1"] = &EndpointState{
		Name: "provider3/model1", Status: models.EndpointHealthy, CircuitState: CircuitClosed,
	}
	hc.mu.Unlock()

	healthy := hc.GetHealthyEndpoints(endpoints)
	assert.Len(t, healthy, 2, "endpoint with open circuit should be excluded")

	// Now simulate cooldown passed -> should transition to half-open and allow
	hc.mu.Lock()
	pastCooldown := now.Add(-61 * time.Second)
	hc.states["provider2/model1"].CircuitOpenedAt = &pastCooldown
	hc.mu.Unlock()

	healthy = hc.GetHealthyEndpoints(endpoints)
	assert.Len(t, healthy, 3, "endpoint should be allowed after cooldown (half-open)")

	// Verify the state transitioned to half-open
	hc.mu.RLock()
	assert.Equal(t, CircuitHalfOpen, hc.states["provider2/model1"].CircuitState)
	hc.mu.RUnlock()
}

func TestHealthChecker_GetAllStates(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	// Initialize states
	hc.mu.Lock()
	hc.states["ep1"] = &EndpointState{Name: "ep1", Status: models.EndpointHealthy}
	hc.states["ep2"] = &EndpointState{Name: "ep2", Status: models.EndpointUnhealthy}
	hc.mu.Unlock()

	states := hc.GetAllStates()
	assert.Len(t, states, 2)
	assert.Equal(t, models.EndpointHealthy, states["ep1"].Status)
	assert.Equal(t, models.EndpointUnhealthy, states["ep2"].Status)
}

func TestHealthChecker_CheckEndpoint_Healthy(t *testing.T) {
	// Create mock server that returns 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "test-provider",
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
		Model: &models.Model{
			Name: "test-model",
		},
	}

	name := "test-provider/test-model"
	hc.mu.Lock()
	hc.states[name] = &EndpointState{Name: name, Status: models.EndpointUnknown}
	hc.mu.Unlock()

	// Check endpoint
	hc.checkEndpoint(t.Context(), ep)

	state := hc.GetState(name)
	require.NotNil(t, state)
	assert.Equal(t, models.EndpointHealthy, state.Status)
}

func TestHealthChecker_CheckEndpoint_Unhealthy_ServerError(t *testing.T) {
	// Create mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "test-provider",
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
		Model: &models.Model{
			Name: "test-model",
		},
	}

	name := "test-provider/test-model"
	hc.mu.Lock()
	hc.states[name] = &EndpointState{Name: name, Status: models.EndpointUnknown}
	hc.mu.Unlock()

	hc.checkEndpoint(t.Context(), ep)

	state := hc.GetState(name)
	require.NotNil(t, state)
	assert.Equal(t, models.EndpointUnhealthy, state.Status)
}

func TestHealthChecker_CheckEndpoint_Unhealthy_Unauthorized(t *testing.T) {
	// Create mock server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  5,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "test-provider",
			BaseURL: server.URL,
			APIKey:  "invalid-key",
		},
		Model: &models.Model{
			Name: "test-model",
		},
	}

	name := "test-provider/test-model"
	hc.mu.Lock()
	hc.states[name] = &EndpointState{Name: name, Status: models.EndpointUnknown}
	hc.mu.Unlock()

	hc.checkEndpoint(t.Context(), ep)

	state := hc.GetState(name)
	require.NotNil(t, state)
	assert.Equal(t, models.EndpointUnhealthy, state.Status)
}

func TestHealthChecker_Start_Disabled(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         false,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep := createHealthTestEndpoint("provider1", "model1")
	endpoints := []*models.Endpoint{ep}

	// Start should mark all endpoints as healthy when disabled
	hc.Start(endpoints)

	// States should exist and be healthy (so endpoints are usable)
	states := hc.GetAllStates()
	assert.Len(t, states, 1)
	assert.Equal(t, models.EndpointHealthy, states["provider1/model1"].Status)
}

func TestHealthChecker_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1, // Short interval for testing
		TimeoutSeconds:  5,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "test-provider",
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
		Model: &models.Model{
			Name: "test-model",
		},
	}
	endpoints := []*models.Endpoint{ep}

	hc.Start(endpoints)

	// Wait for initial check
	time.Sleep(100 * time.Millisecond)

	// Should have state
	state := hc.GetState("test-provider/test-model")
	require.NotNil(t, state)

	// Stop should not hang
	hc.Stop()
}

func TestHealthChecker_Stop_Idempotent(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1,
		TimeoutSeconds:  1,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	// Stop before Start should be no-op.
	assert.NotPanics(t, func() {
		hc.Stop()
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ep := &models.Endpoint{
		Provider: &models.Provider{Name: "test-provider", BaseURL: server.URL, APIKey: "test-key"},
		Model:    &models.Model{Name: "test-model"},
	}
	hc.Start([]*models.Endpoint{ep})
	time.Sleep(50 * time.Millisecond)

	// Repeated Stop should not panic or block.
	assert.NotPanics(t, func() {
		hc.Stop()
		hc.Stop()
	})
}

func TestHealthChecker_Start_Idempotent_NoLeakedLoopAfterStop(t *testing.T) {
	var checks int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&checks, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1,
		TimeoutSeconds:  1,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())
	ep := &models.Endpoint{
		Provider: &models.Provider{Name: "test-provider", BaseURL: server.URL, APIKey: "test-key"},
		Model:    &models.Model{Name: "test-model"},
	}
	endpoints := []*models.Endpoint{ep}

	// Start twice should not create two long-running loops.
	hc.Start(endpoints)
	time.Sleep(100 * time.Millisecond)
	hc.Start(endpoints)
	time.Sleep(100 * time.Millisecond)

	hc.Stop()

	// Allow in-flight check to settle, then assert no new checks keep happening.
	time.Sleep(200 * time.Millisecond)
	afterStop := atomic.LoadInt64(&checks)
	time.Sleep(1500 * time.Millisecond)
	finalChecks := atomic.LoadInt64(&checks)
	assert.Equal(t, afterStop, finalChecks, "health checker loop should be fully stopped")
}

func TestEndpointStateSnapshot(t *testing.T) {
	now := time.Now()
	state := &EndpointState{
		Name:               "test/endpoint",
		Status:             models.EndpointHealthy,
		CurrentConnections: 5,
		TotalRequests:      100,
		TotalErrors:        2,
		LastCheckTime:      &now,
		LastError:          "",
		AvgResponseTimeMs:  150.5,
	}

	snapshot := state.snapshot()

	assert.Equal(t, state.Name, snapshot.Name)
	assert.Equal(t, state.Status, snapshot.Status)
	assert.Equal(t, state.CurrentConnections, snapshot.CurrentConnections)
	assert.Equal(t, state.TotalRequests, snapshot.TotalRequests)
	assert.Equal(t, state.TotalErrors, snapshot.TotalErrors)
	assert.Equal(t, state.AvgResponseTimeMs, snapshot.AvgResponseTimeMs)
}

// Helper function to create test endpoints
func createHealthTestEndpoint(providerName, modelName string) *models.Endpoint {
	return &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    providerName,
			BaseURL: "https://api.example.com",
			APIKey:  "test-key",
			Weight:  1,
			Enabled: true,
		},
		Model: &models.Model{
			ID:      1,
			Name:    modelName,
			Role:    models.ModelRoleDefault,
			Enabled: true,
		},
		Status: models.EndpointHealthy,
	}
}

// TestClassifyHTTPError tests HTTP error classification
func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		responseBody []byte
		expected     ErrorType
	}{
		{
			name:         "400 with model error",
			statusCode:   400,
			responseBody: []byte(`{"error": "model not found"}`),
			expected:     ErrorTypePermanent,
		},
		{
			name:         "400 without model error",
			statusCode:   400,
			responseBody: []byte(`{"error": "invalid request"}`),
			expected:     ErrorTypePermanent,
		},
		{
			name:         "400 with overloaded error",
			statusCode:   400,
			responseBody: []byte(`{"error": {"message": "server is overloaded, try again later"}}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "401 unauthorized",
			statusCode:   401,
			responseBody: []byte(`{"error": "unauthorized"}`),
			expected:     ErrorTypeAuth,
		},
		{
			name:         "403 forbidden",
			statusCode:   403,
			responseBody: []byte(`{"error": "forbidden"}`),
			expected:     ErrorTypeAuth,
		},
		{
			name:         "404 not found",
			statusCode:   404,
			responseBody: []byte(`{"error": "not found"}`),
			expected:     ErrorTypePermanent,
		},
		{
			name:         "422 with model error",
			statusCode:   422,
			responseBody: []byte(`{"error": "invalid model specified"}`),
			expected:     ErrorTypePermanent,
		},
		{
			name:         "422 without model error",
			statusCode:   422,
			responseBody: []byte(`{"error": "validation failed"}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "429 rate limit",
			statusCode:   429,
			responseBody: []byte(`{"error": "rate limit exceeded"}`),
			expected:     ErrorTypeRateLimit,
		},
		{
			name:         "408 timeout",
			statusCode:   408,
			responseBody: []byte(`{"error": "timeout"}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "502 bad gateway",
			statusCode:   502,
			responseBody: []byte(`{"error": "bad gateway"}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "503 service unavailable",
			statusCode:   503,
			responseBody: []byte(`{"error": "service unavailable"}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "504 gateway timeout",
			statusCode:   504,
			responseBody: []byte(`{"error": "gateway timeout"}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "500 internal server error",
			statusCode:   500,
			responseBody: []byte(`{"error": "internal server error"}`),
			expected:     ErrorTypeTemporary,
		},
		{
			name:         "200 success",
			statusCode:   200,
			responseBody: []byte(`{"result": "ok"}`),
			expected:     ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyHTTPError(tt.statusCode, tt.responseBody)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContainsModelError tests model error detection in response body
func TestContainsModelError(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected bool
	}{
		{
			name:     "model not found",
			body:     []byte(`{"error": "model not found"}`),
			expected: true,
		},
		{
			name:     "model does not exist",
			body:     []byte(`{"error": "The model does not exist"}`),
			expected: true,
		},
		{
			name:     "invalid model",
			body:     []byte(`{"error": "invalid model specified"}`),
			expected: true,
		},
		{
			name:     "unsupported model",
			body:     []byte(`{"error": "unsupported model type"}`),
			expected: true,
		},
		{
			name:     "model is not available",
			body:     []byte(`{"error": "model is not available"}`),
			expected: true,
		},
		{
			name:     "case insensitive - MODEL NOT FOUND",
			body:     []byte(`{"error": "MODEL NOT FOUND"}`),
			expected: true,
		},
		{
			name:     "no model error",
			body:     []byte(`{"error": "invalid request"}`),
			expected: false,
		},
		{
			name:     "empty body",
			body:     []byte(``),
			expected: false,
		},
		{
			name:     "nil body",
			body:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsModelError(tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestClassifyError tests general error classification
func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: ErrorTypeUnknown,
		},
		{
			name:     "timeout error",
			err:      fmt.Errorf("request timeout"),
			expected: ErrorTypeTemporary,
		},
		{
			name:     "connection refused",
			err:      fmt.Errorf("connection refused"),
			expected: ErrorTypeTemporary,
		},
		{
			name:     "no such host",
			err:      fmt.Errorf("no such host"),
			expected: ErrorTypeTemporary,
		},
		{
			name:     "case insensitive - TIMEOUT",
			err:      fmt.Errorf("REQUEST TIMEOUT"),
			expected: ErrorTypeTemporary,
		},
		{
			name:     "unknown error",
			err:      fmt.Errorf("some other error"),
			expected: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractErrorMessage tests error message extraction
func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		responseBody []byte
		expected     string
	}{
		{
			name:         "error with message",
			err:          fmt.Errorf("connection failed"),
			responseBody: nil,
			expected:     "connection failed",
		},
		{
			name:         "response body only",
			err:          nil,
			responseBody: []byte(`{"error": "model not found"}`),
			expected:     `{"error": "model not found"}`,
		},
		{
			name:         "error takes precedence",
			err:          fmt.Errorf("network error"),
			responseBody: []byte(`{"error": "server error"}`),
			expected:     "network error",
		},
		{
			name:         "long response body truncated",
			err:          nil,
			responseBody: []byte(string(make([]byte, 600))),
			expected:     string(make([]byte, 500)) + "...",
		},
		{
			name:         "both nil",
			err:          nil,
			responseBody: nil,
			expected:     "unknown error",
		},
		{
			name:         "empty response body",
			err:          nil,
			responseBody: []byte(``),
			expected:     "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractErrorMessage(tt.err, tt.responseBody)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestShouldTransitionToOpen tests circuit breaker open transition logic
func TestShouldTransitionToOpen(t *testing.T) {
	tests := []struct {
		name           string
		state          *EndpointState
		expected       bool
		expectedReason string
	}{
		{
			name: "5 consecutive failures",
			state: &EndpointState{
				CircuitState:        CircuitClosed,
				ConsecutiveFailures: 5,
			},
			expected:       true,
			expectedReason: "consecutive failures",
		},
		{
			name: "3 consecutive permanent errors",
			state: &EndpointState{
				CircuitState:         CircuitClosed,
				ConsecutiveFailures:  2,
				ConsecutivePermanent: 3,
			},
			expected:       true,
			expectedReason: "consecutive permanent errors",
		},
		{
			name: "4 consecutive failures - not enough",
			state: &EndpointState{
				CircuitState:        CircuitClosed,
				ConsecutiveFailures: 4,
			},
			expected: false,
		},
		{
			name: "2 consecutive permanent - not enough",
			state: &EndpointState{
				CircuitState:         CircuitClosed,
				ConsecutivePermanent: 2,
			},
			expected: false,
		},
		{
			name: "already open",
			state: &EndpointState{
				CircuitState:        CircuitOpen,
				ConsecutiveFailures: 10,
			},
			expected: false,
		},
		{
			name: "half-open state",
			state: &EndpointState{
				CircuitState:        CircuitHalfOpen,
				ConsecutiveFailures: 5,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, reason := shouldTransitionToOpen(tt.state, defaultCircuitBreakerConfig())
			assert.Equal(t, tt.expected, result)
			if tt.expected && tt.expectedReason != "" {
				assert.Contains(t, reason, tt.expectedReason)
			}
		})
	}
}

// TestShouldTransitionToHalfOpen tests circuit breaker half-open transition logic
func TestShouldTransitionToHalfOpen(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		state    *EndpointState
		expected bool
	}{
		{
			name: "cooldown period passed",
			state: &EndpointState{
				CircuitState:    CircuitOpen,
				CircuitOpenedAt: timePtr(now.Add(-61 * time.Second)),
			},
			expected: true,
		},
		{
			name: "cooldown period not passed",
			state: &EndpointState{
				CircuitState:    CircuitOpen,
				CircuitOpenedAt: timePtr(now.Add(-30 * time.Second)),
			},
			expected: false,
		},
		{
			name: "no opened time",
			state: &EndpointState{
				CircuitState:    CircuitOpen,
				CircuitOpenedAt: nil,
			},
			expected: false,
		},
		{
			name: "not in open state",
			state: &EndpointState{
				CircuitState:    CircuitClosed,
				CircuitOpenedAt: timePtr(now.Add(-61 * time.Second)),
			},
			expected: false,
		},
		{
			name: "already half-open",
			state: &EndpointState{
				CircuitState:    CircuitHalfOpen,
				CircuitOpenedAt: timePtr(now.Add(-61 * time.Second)),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldTransitionToHalfOpen(tt.state, defaultCircuitBreakerConfig())
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestShouldAllowRequest tests request allowance logic based on circuit state
func TestShouldAllowRequest(t *testing.T) {
	tests := []struct {
		name     string
		state    *EndpointState
		expected bool
	}{
		{
			name: "closed state - allow",
			state: &EndpointState{
				CircuitState: CircuitClosed,
			},
			expected: true,
		},
		{
			name: "open state - deny",
			state: &EndpointState{
				CircuitState: CircuitOpen,
			},
			expected: false,
		},
		{
			name: "half-open with room for more requests",
			state: &EndpointState{
				CircuitState:      CircuitHalfOpen,
				HalfOpenSuccesses: 1,
				HalfOpenFailures:  1,
			},
			expected: true,
		},
		{
			name: "half-open at max requests",
			state: &EndpointState{
				CircuitState:      CircuitHalfOpen,
				HalfOpenSuccesses: 3,
				HalfOpenFailures:  2,
			},
			expected: false,
		},
		{
			name: "half-open all successes",
			state: &EndpointState{
				CircuitState:      CircuitHalfOpen,
				HalfOpenSuccesses: 5,
				HalfOpenFailures:  0,
			},
			expected: false,
		},
		{
			name: "half-open all failures",
			state: &EndpointState{
				CircuitState:      CircuitHalfOpen,
				HalfOpenSuccesses: 0,
				HalfOpenFailures:  5,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldAllowRequest(tt.state, defaultCircuitBreakerConfig())
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}

// TestUpdateRequestStatsWithCircuitBreaker tests the integration of UpdateRequestStats with circuit breaker logic
func TestUpdateRequestStatsWithCircuitBreaker(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())
	name := "test-provider/test-model"

	// Initialize state
	hc.mu.Lock()
	hc.states[name] = NewEndpointState(name)
	hc.mu.Unlock()

	t.Run("successful requests keep circuit closed", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			result := RequestResult{
				EndpointName: name,
				Success:      true,
				LatencyMs:    100.0,
				StatusCode:   200,
			}
			hc.UpdateRequestStatsV2(result)
		}

		hc.mu.RLock()
		state := hc.states[name]
		assert.Equal(t, CircuitClosed, state.CircuitState)
		assert.Equal(t, 0, state.ConsecutiveFailures)
		hc.mu.RUnlock()
	})

	t.Run("5 consecutive failures open circuit", func(t *testing.T) {
		// Reset state
		hc.mu.Lock()
		hc.states[name] = NewEndpointState(name)
		hc.mu.Unlock()

		// Send 5 consecutive failures
		for i := 0; i < 5; i++ {
			result := RequestResult{
				EndpointName: name,
				Success:      false,
				LatencyMs:    100.0,
				StatusCode:   500,
				Err:          fmt.Errorf("internal server error"),
			}
			hc.UpdateRequestStatsV2(result)
		}

		hc.mu.RLock()
		state := hc.states[name]
		assert.Equal(t, CircuitOpen, state.CircuitState)
		assert.Equal(t, 5, state.ConsecutiveFailures)
		assert.NotNil(t, state.CircuitOpenedAt)
		hc.mu.RUnlock()
	})

	t.Run("3 consecutive permanent errors open circuit", func(t *testing.T) {
		// Reset state
		hc.mu.Lock()
		hc.states[name] = NewEndpointState(name)
		hc.mu.Unlock()

		// Send 3 consecutive permanent errors
		for i := 0; i < 3; i++ {
			result := RequestResult{
				EndpointName: name,
				Success:      false,
				LatencyMs:    100.0,
				StatusCode:   404,
				ResponseBody: []byte(`{"error": "model not found"}`),
			}
			hc.UpdateRequestStatsV2(result)
		}

		hc.mu.RLock()
		state := hc.states[name]
		assert.Equal(t, CircuitOpen, state.CircuitState)
		assert.Equal(t, 3, state.ConsecutivePermanent)
		hc.mu.RUnlock()
	})

	t.Run("success resets consecutive failures", func(t *testing.T) {
		// Reset state
		hc.mu.Lock()
		hc.states[name] = NewEndpointState(name)
		hc.mu.Unlock()

		// Send 4 failures
		for i := 0; i < 4; i++ {
			result := RequestResult{
				EndpointName: name,
				Success:      false,
				LatencyMs:    100.0,
				StatusCode:   500,
			}
			hc.UpdateRequestStatsV2(result)
		}

		// Send 1 success
		result := RequestResult{
			EndpointName: name,
			Success:      true,
			LatencyMs:    100.0,
			StatusCode:   200,
		}
		hc.UpdateRequestStatsV2(result)

		hc.mu.RLock()
		state := hc.states[name]
		assert.Equal(t, CircuitClosed, state.CircuitState)
		assert.Equal(t, 0, state.ConsecutiveFailures)
		hc.mu.RUnlock()
	})
}

// TestUpdateRequestStatsWithCircuitBreaker_Disabled verifies that when circuit breaker
// is disabled, failures do not transition circuit state to open.
func TestUpdateRequestStatsWithCircuitBreaker_Disabled(t *testing.T) {
	cfg := defaultHealthCheckConfig()
	cfg.CircuitBreaker.Enabled = false
	cfg.CircuitBreaker.ConsecutiveFailures = 1
	cfg.CircuitBreaker.PermanentErrorThreshold = 1

	hc := NewHealthChecker(cfg, zap.NewNop())
	name := "test-provider/test-model-disabled-cb"

	hc.mu.Lock()
	state := NewEndpointState(name)
	state.Status = models.EndpointHealthy
	hc.states[name] = state
	hc.mu.Unlock()

	// Even with thresholds set to 1, disabled circuit breaker should keep circuit closed.
	hc.UpdateRequestStatsV2(RequestResult{
		EndpointName: name,
		Success:      false,
		LatencyMs:    50,
		StatusCode:   500,
	})

	hc.mu.RLock()
	circuitState := hc.states[name].CircuitState
	hc.mu.RUnlock()
	assert.Equal(t, CircuitClosed, circuitState, "disabled circuit breaker should not open")
	assert.True(t, hc.IsHealthy(name), "disabled circuit breaker should not block endpoint traffic")
}

// TestCircuitBreakerFullFlow tests the complete circuit breaker state transitions
func TestCircuitBreakerFullFlow(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())
	name := "test-provider/test-model"

	// Initialize state
	hc.mu.Lock()
	hc.states[name] = NewEndpointState(name)
	hc.mu.Unlock()

	// Step 1: Circuit starts closed
	hc.mu.RLock()
	assert.Equal(t, CircuitClosed, hc.states[name].CircuitState)
	hc.mu.RUnlock()
	assert.True(t, hc.IsHealthy(name))

	// Step 2: 5 failures -> circuit opens
	for i := 0; i < 5; i++ {
		result := RequestResult{
			EndpointName: name,
			Success:      false,
			LatencyMs:    100.0,
			StatusCode:   500,
		}
		hc.UpdateRequestStatsV2(result)
	}

	hc.mu.RLock()
	assert.Equal(t, CircuitOpen, hc.states[name].CircuitState)
	hc.mu.RUnlock()
	assert.False(t, hc.IsHealthy(name))

	// Step 3: Simulate cooldown period passing
	hc.mu.Lock()
	openedAt := time.Now().Add(-61 * time.Second)
	hc.states[name].CircuitOpenedAt = &openedAt
	hc.mu.Unlock()

	// Step 4: Check if should transition to half-open
	hc.mu.RLock()
	shouldTransition := shouldTransitionToHalfOpen(hc.states[name], hc.cfg.CircuitBreaker)
	hc.mu.RUnlock()
	assert.True(t, shouldTransition)

	// Step 5: Manually transition to half-open (in real code, this happens in IsHealthy)
	hc.mu.Lock()
	hc.states[name].CircuitState = CircuitHalfOpen
	hc.states[name].HalfOpenSuccesses = 0
	hc.states[name].HalfOpenFailures = 0
	hc.mu.Unlock()

	// Step 6: Send 5 successful test requests -> circuit closes
	for i := 0; i < 5; i++ {
		result := RequestResult{
			EndpointName: name,
			Success:      true,
			LatencyMs:    100.0,
			StatusCode:   200,
		}
		hc.UpdateRequestStatsV2(result)
	}

	hc.mu.RLock()
	assert.Equal(t, CircuitClosed, hc.states[name].CircuitState)
	// After closing, counters are reset
	assert.Equal(t, 0, hc.states[name].HalfOpenSuccesses)
	assert.Equal(t, 0, hc.states[name].HalfOpenFailures)
	hc.mu.RUnlock()
	assert.True(t, hc.IsHealthy(name))
}

// TestIsHealthyWithCircuitBreaker tests IsHealthy method with circuit breaker integration
func TestIsHealthyWithCircuitBreaker(t *testing.T) {
	cfg := defaultHealthCheckConfig()

	hc := NewHealthChecker(cfg, zap.NewNop())
	name := "test-provider/test-model"

	t.Run("closed circuit is healthy", func(t *testing.T) {
		hc.mu.Lock()
		hc.states[name] = NewEndpointState(name)
		hc.states[name].CircuitState = CircuitClosed
		hc.mu.Unlock()

		assert.True(t, hc.IsHealthy(name))
	})

	t.Run("open circuit is unhealthy", func(t *testing.T) {
		hc.mu.Lock()
		hc.states[name].CircuitState = CircuitOpen
		now := time.Now()
		hc.states[name].CircuitOpenedAt = &now
		hc.mu.Unlock()

		assert.False(t, hc.IsHealthy(name))
	})

	t.Run("half-open with available slots is healthy", func(t *testing.T) {
		hc.mu.Lock()
		hc.states[name].CircuitState = CircuitHalfOpen
		hc.states[name].HalfOpenSuccesses = 1
		hc.states[name].HalfOpenFailures = 1
		hc.mu.Unlock()

		assert.True(t, hc.IsHealthy(name))
	})

	t.Run("half-open at max requests is unhealthy", func(t *testing.T) {
		hc.mu.Lock()
		hc.states[name].CircuitState = CircuitHalfOpen
		hc.states[name].HalfOpenSuccesses = 3
		hc.states[name].HalfOpenFailures = 2
		hc.mu.Unlock()

		assert.False(t, hc.IsHealthy(name))
	})

	t.Run("open circuit transitions to half-open after cooldown", func(t *testing.T) {
		hc.mu.Lock()
		hc.states[name].CircuitState = CircuitOpen
		openedAt := time.Now().Add(-61 * time.Second)
		hc.states[name].CircuitOpenedAt = &openedAt
		hc.mu.Unlock()

		// First call should transition to half-open
		result := hc.IsHealthy(name)

		hc.mu.RLock()
		state := hc.states[name]
		hc.mu.RUnlock()

		// Should transition to half-open and allow request
		assert.Equal(t, CircuitHalfOpen, state.CircuitState)
		assert.True(t, result)
	})
}

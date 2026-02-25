//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func TestNewHealthChecker(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())
	require.NotNil(t, hc)
	assert.Equal(t, cfg.Enabled, hc.cfg.Enabled)
	assert.NotNil(t, hc.client)
	assert.NotNil(t, hc.states)
}

func TestHealthChecker_IsHealthy_Unknown(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	// Unknown endpoint should return false
	assert.False(t, hc.IsHealthy("unknown/endpoint"))
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
	cfg := config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 60,
		TimeoutSeconds:  10,
	}

	hc := NewHealthChecker(cfg, zap.NewNop())

	ep1 := createHealthTestEndpoint("provider1", "model1")
	ep2 := createHealthTestEndpoint("provider2", "model1")
	ep3 := createHealthTestEndpoint("provider3", "model1")
	endpoints := []*models.Endpoint{ep1, ep2, ep3}

	// Initialize states
	hc.mu.Lock()
	hc.states["provider1/model1"] = &EndpointState{Name: "provider1/model1", Status: models.EndpointHealthy}
	hc.states["provider2/model1"] = &EndpointState{Name: "provider2/model1", Status: models.EndpointUnhealthy}
	hc.states["provider3/model1"] = &EndpointState{Name: "provider3/model1", Status: models.EndpointHealthy}
	hc.mu.Unlock()

	healthy := hc.GetHealthyEndpoints(endpoints)
	assert.Len(t, healthy, 2)
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

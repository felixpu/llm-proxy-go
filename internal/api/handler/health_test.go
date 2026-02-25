//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

// Helper to initialize endpoint state in HealthChecker
func initializeHealthCheckerState(hc *service.HealthChecker, endpoints []*models.Endpoint) {
	hc.Start(endpoints)
}

func TestHealthHandler_Health_AllHealthy(t *testing.T) {
	cfg := config.HealthCheckConfig{Enabled: false}
	hc := service.NewHealthChecker(cfg, testutil.NewTestLogger())

	// Create test endpoints
	endpoints := []*models.Endpoint{
		{
			Provider: &models.Provider{Name: "provider1"},
			Model:    &models.Model{Name: "model1"},
		},
		{
			Provider: &models.Provider{Name: "provider2"},
			Model:    &models.Model{Name: "model2"},
		},
	}
	initializeHealthCheckerState(hc, endpoints)

	// Update states
	hc.UpdateState("provider1/model1", models.EndpointHealthy, "")
	hc.UpdateState("provider2/model2", models.EndpointHealthy, "")

	handler := NewHealthHandler(hc)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/health", nil)

	handler.Health(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "healthy", resp["status"])
	assert.Equal(t, float64(2), resp["healthy"])
	assert.Equal(t, float64(0), resp["unhealthy"])
}

func TestHealthHandler_Health_AllUnhealthy(t *testing.T) {
	cfg := config.HealthCheckConfig{Enabled: false}
	hc := service.NewHealthChecker(cfg, testutil.NewTestLogger())

	endpoints := []*models.Endpoint{
		{
			Provider: &models.Provider{Name: "provider1"},
			Model:    &models.Model{Name: "model1"},
		},
		{
			Provider: &models.Provider{Name: "provider2"},
			Model:    &models.Model{Name: "model2"},
		},
	}
	initializeHealthCheckerState(hc, endpoints)

	hc.UpdateState("provider1/model1", models.EndpointUnhealthy, "error")
	hc.UpdateState("provider2/model2", models.EndpointUnhealthy, "error")

	handler := NewHealthHandler(hc)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/health", nil)

	handler.Health(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", resp["status"])
	assert.Equal(t, float64(0), resp["healthy"])
	assert.Equal(t, float64(2), resp["unhealthy"])
}

func TestHealthHandler_Health_Degraded(t *testing.T) {
	cfg := config.HealthCheckConfig{Enabled: false}
	hc := service.NewHealthChecker(cfg, testutil.NewTestLogger())

	endpoints := []*models.Endpoint{
		{
			Provider: &models.Provider{Name: "provider1"},
			Model:    &models.Model{Name: "model1"},
		},
		{
			Provider: &models.Provider{Name: "provider2"},
			Model:    &models.Model{Name: "model2"},
		},
		{
			Provider: &models.Provider{Name: "provider3"},
			Model:    &models.Model{Name: "model3"},
		},
	}
	initializeHealthCheckerState(hc, endpoints)

	hc.UpdateState("provider1/model1", models.EndpointHealthy, "")
	hc.UpdateState("provider2/model2", models.EndpointUnhealthy, "error")
	hc.UpdateState("provider3/model3", models.EndpointUnhealthy, "error")

	handler := NewHealthHandler(hc)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/health", nil)

	handler.Health(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "degraded", resp["status"])
	assert.Equal(t, float64(1), resp["healthy"])
	assert.Equal(t, float64(2), resp["unhealthy"])
}

func TestHealthHandler_Health_Empty(t *testing.T) {
	cfg := config.HealthCheckConfig{Enabled: false}
	hc := service.NewHealthChecker(cfg, testutil.NewTestLogger())

	handler := NewHealthHandler(hc)
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/health", nil)

	handler.Health(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "healthy", resp["status"])
	assert.Equal(t, float64(0), resp["healthy"])
	assert.Equal(t, float64(0), resp["unhealthy"])
}

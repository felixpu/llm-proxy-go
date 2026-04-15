package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/internal/version"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	healthChecker *service.HealthChecker
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(hc *service.HealthChecker) *HealthHandler {
	return &HealthHandler{healthChecker: hc}
}

// Health returns the service health status.
func (h *HealthHandler) Health(c *gin.Context) {
	states := h.healthChecker.GetAllStates()

	healthy := 0
	unhealthy := 0
	unknown := 0
	circuitOpen := 0
	for _, s := range states {
		switch s.Status {
		case models.EndpointHealthy:
			healthy++
		case models.EndpointUnhealthy:
			unhealthy++
		default:
			unknown++
		}
		if s.CircuitState == service.CircuitOpen {
			circuitOpen++
		}
	}

	status := "healthy"
	if unhealthy > 0 || unknown > 0 {
		status = "degraded"
	}
	if healthy == 0 && unknown == 0 && unhealthy > 0 {
		status = "unhealthy"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       status,
		"version":      version.Short(),
		"healthy":      healthy,
		"unhealthy":    unhealthy,
		"unknown":      unknown,
		"circuit_open": circuitOpen,
		"endpoints":    states,
	})
}

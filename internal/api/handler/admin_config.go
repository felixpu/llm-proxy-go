package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/repository"
)

// RoutingUpdate represents a routing configuration update.
type RoutingUpdate struct {
	DefaultRole string `json:"default_role" binding:"required"`
}

// LoadBalanceUpdate represents a load balance configuration update.
type LoadBalanceUpdate struct {
	Strategy string `json:"strategy" binding:"required"`
}

// HealthCheckConfigUpdate represents a health check configuration update.
type HealthCheckConfigUpdate struct {
	Enabled         *bool `json:"enabled"`
	IntervalSeconds *int  `json:"interval_seconds"`
	TimeoutSeconds  *int  `json:"timeout_seconds"`
}

// UIConfigUpdate represents a UI configuration update.
type UIConfigUpdate struct {
	DashboardRefreshSeconds *int `json:"dashboard_refresh_seconds"`
	LogsRefreshSeconds      *int `json:"logs_refresh_seconds"`
}

// ConfigHandler handles system configuration API endpoints.
type ConfigHandler struct {
	repo *repository.SystemConfigRepository
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(repo *repository.SystemConfigRepository) *ConfigHandler {
	return &ConfigHandler{repo: repo}
}

// GetRoutingConfig returns the current routing configuration.
func (h *ConfigHandler) GetRoutingConfig(c *gin.Context) {
	cfg, err := h.repo.GetRoutingConfig(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateRoutingConfig updates the routing configuration.
func (h *ConfigHandler) UpdateRoutingConfig(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateRoutingConfig(c.Request.Context(), req); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Routing config updated"})
}

// GetLoadBalanceConfig returns the load balance configuration.
func (h *ConfigHandler) GetLoadBalanceConfig(c *gin.Context) {
	cfg, err := h.repo.GetLoadBalanceConfig(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateLoadBalanceConfig updates the load balance configuration.
func (h *ConfigHandler) UpdateLoadBalanceConfig(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if strategy, ok := req["strategy"].(string); ok {
		valid := map[string]bool{
			"round_robin": true, "weighted": true,
			"least_connections": true, "conversation_hash": true,
		}
		if !valid[strategy] {
			errorResponse(c, http.StatusBadRequest, "invalid strategy")
			return
		}
	}
	if err := h.repo.UpdateLoadBalanceConfig(c.Request.Context(), req); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Load balance config updated"})
}
// GetHealthCheckConfig returns the health check configuration.
func (h *ConfigHandler) GetHealthCheckConfig(c *gin.Context) {
	cfg, err := h.repo.GetHealthCheckConfig(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateHealthCheckConfig updates the health check configuration.
func (h *ConfigHandler) UpdateHealthCheckConfig(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateHealthCheckConfig(c.Request.Context(), req); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Health check config updated"})
}

// GetUIConfig returns the UI configuration.
func (h *ConfigHandler) GetUIConfig(c *gin.Context) {
	cfg, err := h.repo.GetUIConfig(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateUIConfig updates the UI configuration.
func (h *ConfigHandler) UpdateUIConfig(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateUIConfig(c.Request.Context(), req); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "UI config updated"})
}

// ReloadConfig reloads the configuration.
func ReloadConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Config reloaded"})
}

// MigrateConfig handles config migration (stub).
func MigrateConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"message": "Configuration migration not needed in Go version",
		"data": gin.H{
			"migrated": false,
			"message":  "Go version uses unified configuration, no migration required",
		},
	})
}

// ListEndpoints returns an empty endpoint list (legacy compatibility stub).
func ListEndpoints(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"type":    "success",
		"data":    []any{},
		"total":   0,
		"message": "Endpoints have been migrated to provider-model architecture",
	})
}

// CreateEndpoint is a legacy compatibility stub.
func CreateEndpoint(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "deprecated",
			"message": "Endpoint API is deprecated. Use provider-model management instead.",
		},
	})
}

// DeleteEndpoint is a legacy compatibility stub.
func DeleteEndpoint(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "deprecated",
			"message": "Endpoint API is deprecated. Use provider-model management instead.",
		},
	})
}

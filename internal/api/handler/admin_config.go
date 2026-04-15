package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// RoutingUpdate represents a routing configuration update.
type RoutingUpdate struct {
	DefaultRole *string `json:"default_role"`
}

// LoadBalanceUpdate represents a load balance configuration update.
type LoadBalanceUpdate struct {
	Strategy *string `json:"strategy"`
}

// HealthCheckConfigUpdate represents a health check configuration update.
type HealthCheckConfigUpdate struct {
	Enabled                   *bool `json:"enabled"`
	IntervalSeconds           *int  `json:"interval_seconds"`
	TimeoutSeconds            *int  `json:"timeout_seconds"`
	CBEnabled                 *bool `json:"cb_enabled"`
	CBConsecutiveFailures     *int  `json:"cb_consecutive_failures"`
	CBPermanentErrorThreshold *int  `json:"cb_permanent_error_threshold"`
	CBCooldownSeconds         *int  `json:"cb_cooldown_seconds"`
	CBHalfOpenMaxRequests     *int  `json:"cb_half_open_max_requests"`
}

// UIConfigUpdate represents a UI configuration update.
type UIConfigUpdate struct {
	DashboardRefreshSeconds *int `json:"dashboard_refresh_seconds"`
	LogsRefreshSeconds      *int `json:"logs_refresh_seconds"`
}

// ConfigHandler handles system configuration API endpoints.
type ConfigHandler struct {
	repo          systemConfigWriterReader
	healthChecker *service.HealthChecker
}

type systemConfigWriterReader interface {
	GetRoutingConfig(ctx context.Context) (map[string]any, error)
	UpdateRoutingConfigPatch(ctx context.Context, patch repository.SystemRoutingConfigPatch) error
	GetLoadBalanceConfig(ctx context.Context) (map[string]any, error)
	UpdateLoadBalanceConfigPatch(ctx context.Context, patch repository.SystemLoadBalanceConfigPatch) error
	GetHealthCheckConfig(ctx context.Context) (map[string]any, error)
	UpdateHealthCheckConfigPatch(ctx context.Context, patch repository.SystemHealthCheckConfigPatch) error
	GetUIConfig(ctx context.Context) (map[string]any, error)
	UpdateUIConfigPatch(ctx context.Context, patch repository.SystemUIConfigPatch) error
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(repo systemConfigWriterReader, healthChecker ...*service.HealthChecker) *ConfigHandler {
	var hc *service.HealthChecker
	if len(healthChecker) > 0 {
		hc = healthChecker[0]
	}
	return &ConfigHandler{
		repo:          repo,
		healthChecker: hc,
	}
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
	var req RoutingUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateRoutingConfigPatch(c.Request.Context(), repository.SystemRoutingConfigPatch{
		DefaultRole: req.DefaultRole,
	}); err != nil {
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
	var req LoadBalanceUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Strategy != nil {
		valid := map[string]bool{
			"round_robin": true, "weighted": true,
			"least_connections": true, "conversation_hash": true,
		}
		if !valid[*req.Strategy] {
			errorResponse(c, http.StatusBadRequest, "invalid strategy")
			return
		}
	}
	if err := h.repo.UpdateLoadBalanceConfigPatch(c.Request.Context(), repository.SystemLoadBalanceConfigPatch{
		Strategy: req.Strategy,
	}); err != nil {
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
	var req HealthCheckConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.IntervalSeconds != nil && *req.IntervalSeconds < 5 {
		errorResponse(c, http.StatusBadRequest, "interval_seconds must be >= 5")
		return
	}
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds <= 0 {
		errorResponse(c, http.StatusBadRequest, "timeout_seconds must be > 0")
		return
	}
	if req.CBConsecutiveFailures != nil && *req.CBConsecutiveFailures <= 0 {
		errorResponse(c, http.StatusBadRequest, "cb_consecutive_failures must be > 0")
		return
	}
	if req.CBPermanentErrorThreshold != nil && *req.CBPermanentErrorThreshold <= 0 {
		errorResponse(c, http.StatusBadRequest, "cb_permanent_error_threshold must be > 0")
		return
	}
	if req.CBCooldownSeconds != nil && *req.CBCooldownSeconds <= 0 {
		errorResponse(c, http.StatusBadRequest, "cb_cooldown_seconds must be > 0")
		return
	}
	if req.CBHalfOpenMaxRequests != nil && *req.CBHalfOpenMaxRequests <= 0 {
		errorResponse(c, http.StatusBadRequest, "cb_half_open_max_requests must be > 0")
		return
	}

	effectiveInterval := 0
	effectiveTimeout := 0
	if h.healthChecker != nil {
		runtimeCfg := h.healthChecker.GetConfig()
		effectiveInterval = runtimeCfg.IntervalSeconds
		effectiveTimeout = runtimeCfg.TimeoutSeconds
	}
	if req.IntervalSeconds != nil {
		effectiveInterval = *req.IntervalSeconds
	}
	if req.TimeoutSeconds != nil {
		effectiveTimeout = *req.TimeoutSeconds
	}
	if effectiveInterval > 0 && effectiveTimeout > 0 && effectiveTimeout >= effectiveInterval {
		errorResponse(c, http.StatusBadRequest, "timeout_seconds must be less than interval_seconds")
		return
	}

	if err := h.repo.UpdateHealthCheckConfigPatch(c.Request.Context(), repository.SystemHealthCheckConfigPatch{
		Enabled:                   req.Enabled,
		IntervalSeconds:           req.IntervalSeconds,
		TimeoutSeconds:            req.TimeoutSeconds,
		CBEnabled:                 req.CBEnabled,
		CBConsecutiveFailures:     req.CBConsecutiveFailures,
		CBPermanentErrorThreshold: req.CBPermanentErrorThreshold,
		CBCooldownSeconds:         req.CBCooldownSeconds,
		CBHalfOpenMaxRequests:     req.CBHalfOpenMaxRequests,
	}); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if h.healthChecker != nil {
		runtimeCfg := h.healthChecker.GetConfig()
		if req.Enabled != nil {
			runtimeCfg.Enabled = *req.Enabled
		}
		if req.IntervalSeconds != nil {
			runtimeCfg.IntervalSeconds = *req.IntervalSeconds
		}
		if req.TimeoutSeconds != nil {
			runtimeCfg.TimeoutSeconds = *req.TimeoutSeconds
		}
		if req.CBEnabled != nil {
			runtimeCfg.CircuitBreaker.Enabled = *req.CBEnabled
		}
		if req.CBConsecutiveFailures != nil {
			runtimeCfg.CircuitBreaker.ConsecutiveFailures = *req.CBConsecutiveFailures
		}
		if req.CBPermanentErrorThreshold != nil {
			runtimeCfg.CircuitBreaker.PermanentErrorThreshold = *req.CBPermanentErrorThreshold
		}
		if req.CBCooldownSeconds != nil {
			runtimeCfg.CircuitBreaker.CooldownSeconds = *req.CBCooldownSeconds
		}
		if req.CBHalfOpenMaxRequests != nil {
			runtimeCfg.CircuitBreaker.HalfOpenMaxRequests = *req.CBHalfOpenMaxRequests
		}
		h.healthChecker.ApplyConfig(runtimeCfg)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Health check config updated",
		"applied": h.healthChecker != nil,
	})
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
	var req UIConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateUIConfigPatch(c.Request.Context(), repository.SystemUIConfigPatch{
		DashboardRefreshSeconds: req.DashboardRefreshSeconds,
		LogsRefreshSeconds:      req.LogsRefreshSeconds,
	}); err != nil {
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

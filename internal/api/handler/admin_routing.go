package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
)

// RoutingModelCreate represents a routing model creation request.
type RoutingModelCreate struct {
	ProviderID        int64   `json:"provider_id" binding:"required"`
	ModelName         string  `json:"model_name" binding:"required"`
	Enabled           bool    `json:"enabled"`
	Priority          int     `json:"priority"`
	CostPerMtokInput  float64 `json:"cost_per_mtok_input"`
	CostPerMtokOutput float64 `json:"cost_per_mtok_output"`
	BillingMultiplier float64 `json:"billing_multiplier"`
	Description       string  `json:"description"`
}

// RoutingModelUpdate represents a routing model update request.
type RoutingModelUpdate struct {
	ProviderID        *int64   `json:"provider_id"`
	ModelName         *string  `json:"model_name"`
	Enabled           *bool    `json:"enabled"`
	Priority          *int     `json:"priority"`
	CostPerMtokInput  *float64 `json:"cost_per_mtok_input"`
	CostPerMtokOutput *float64 `json:"cost_per_mtok_output"`
	BillingMultiplier *float64 `json:"billing_multiplier"`
	Description       *string  `json:"description"`
}

// LLMRoutingConfigUpdate represents an LLM routing configuration update.
type LLMRoutingConfigUpdate struct {
	Enabled                 *bool    `json:"enabled"`
	PrimaryModelID          *int64   `json:"primary_model_id"`
	FallbackModelID         *int64   `json:"fallback_model_id"`
	TimeoutSeconds          *int     `json:"timeout_seconds"`
	CacheEnabled            *bool    `json:"cache_enabled"`
	CacheTTLSeconds         *int     `json:"cache_ttl_seconds"`
	CacheTTLL3Seconds       *int     `json:"cache_ttl_l3_seconds"`
	MaxTokens               *int     `json:"max_tokens"`
	Temperature             *float64 `json:"temperature"`
	RetryCount              *int     `json:"retry_count"`
	SemanticCacheEnabled    *bool    `json:"semantic_cache_enabled"`
	EmbeddingModelID        *int64   `json:"embedding_model_id"`
	SimilarityThreshold     *float64 `json:"similarity_threshold"`
	LocalEmbeddingModel     *string  `json:"local_embedding_model"`
	ForceSmartRouting       *bool    `json:"force_smart_routing"`
	RuleBasedRoutingEnabled *bool    `json:"rule_based_routing_enabled"`
	RuleFallbackStrategy    *string  `json:"rule_fallback_strategy"`
	RuleFallbackTaskType    *string  `json:"rule_fallback_task_type"`
}

// RoutingHandler handles routing model and LLM config API endpoints.
type RoutingHandler struct {
	modelRepo  *repository.RoutingModelRepository
	configRepo *repository.RoutingConfigRepository
}

// NewRoutingHandler creates a new RoutingHandler.
func NewRoutingHandler(modelRepo *repository.RoutingModelRepository, configRepo *repository.RoutingConfigRepository) *RoutingHandler {
	return &RoutingHandler{modelRepo: modelRepo, configRepo: configRepo}
}

// ListRoutingModels returns all routing models.
func (h *RoutingHandler) ListRoutingModels(c *gin.Context) {
	var providerID *int64
	if pidStr := c.Query("provider_id"); pidStr != "" {
		pid, err := strconv.ParseInt(pidStr, 10, 64)
		if err == nil {
			providerID = &pid
		}
	}
	list, err := h.modelRepo.ListModels(c.Request.Context(), providerID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*models.RoutingModel{}
	}
	c.JSON(http.StatusOK, gin.H{"models": list})
}

// GetRoutingModel returns a single routing model by ID.
func (h *RoutingHandler) GetRoutingModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid model_id")
		return
	}
	m, err := h.modelRepo.GetModel(c.Request.Context(), id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		errorResponse(c, http.StatusNotFound, "routing model not found")
		return
	}
	c.JSON(http.StatusOK, m)
}
// CreateRoutingModel creates a new routing model.
func (h *RoutingHandler) CreateRoutingModel(c *gin.Context) {
	var req RoutingModelCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	m := &models.RoutingModel{
		ProviderID:        req.ProviderID,
		ModelName:         req.ModelName,
		Enabled:           req.Enabled,
		Priority:          req.Priority,
		CostPerMtokInput:  req.CostPerMtokInput,
		CostPerMtokOutput: req.CostPerMtokOutput,
		BillingMultiplier: req.BillingMultiplier,
		Description:       req.Description,
	}
	id, err := h.modelRepo.AddModel(c.Request.Context(), m)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Routing model created"})
}

// UpdateRoutingModel updates an existing routing model.
func (h *RoutingHandler) UpdateRoutingModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid model_id")
		return
	}
	var req RoutingModelUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	updates := make(map[string]any)
	if req.ProviderID != nil { updates["provider_id"] = *req.ProviderID }
	if req.ModelName != nil { updates["model_name"] = *req.ModelName }
	if req.Enabled != nil { updates["enabled"] = *req.Enabled }
	if req.Priority != nil { updates["priority"] = *req.Priority }
	if req.CostPerMtokInput != nil { updates["cost_per_mtok_input"] = *req.CostPerMtokInput }
	if req.CostPerMtokOutput != nil { updates["cost_per_mtok_output"] = *req.CostPerMtokOutput }
	if req.BillingMultiplier != nil { updates["billing_multiplier"] = *req.BillingMultiplier }
	if req.Description != nil { updates["description"] = *req.Description }
	if err := h.modelRepo.UpdateModel(c.Request.Context(), id, updates); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Routing model updated"})
}

// DeleteRoutingModel deletes a routing model.
func (h *RoutingHandler) DeleteRoutingModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid model_id")
		return
	}
	if err := h.modelRepo.DeleteModel(c.Request.Context(), id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Routing model deleted"})
}

// GetLLMRoutingConfig returns the LLM routing configuration.
func (h *RoutingHandler) GetLLMRoutingConfig(c *gin.Context) {
	cfg, err := h.configRepo.GetConfig(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateLLMRoutingConfig updates the LLM routing configuration.
func (h *RoutingHandler) UpdateLLMRoutingConfig(c *gin.Context) {
	var req LLMRoutingConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	updates := make(map[string]any)
	if req.Enabled != nil { updates["enabled"] = *req.Enabled }
	if req.PrimaryModelID != nil { updates["primary_model_id"] = *req.PrimaryModelID }
	if req.FallbackModelID != nil { updates["fallback_model_id"] = *req.FallbackModelID }
	if req.TimeoutSeconds != nil { updates["timeout_seconds"] = *req.TimeoutSeconds }
	if req.CacheEnabled != nil { updates["cache_enabled"] = *req.CacheEnabled }
	if req.CacheTTLSeconds != nil { updates["cache_ttl_seconds"] = *req.CacheTTLSeconds }
	if req.CacheTTLL3Seconds != nil { updates["cache_ttl_l3_seconds"] = *req.CacheTTLL3Seconds }
	if req.MaxTokens != nil { updates["max_tokens"] = *req.MaxTokens }
	if req.Temperature != nil { updates["temperature"] = *req.Temperature }
	if req.RetryCount != nil { updates["retry_count"] = *req.RetryCount }
	if req.SemanticCacheEnabled != nil { updates["semantic_cache_enabled"] = *req.SemanticCacheEnabled }
	if req.EmbeddingModelID != nil { updates["embedding_model_id"] = *req.EmbeddingModelID }
	if req.SimilarityThreshold != nil { updates["similarity_threshold"] = *req.SimilarityThreshold }
	if req.LocalEmbeddingModel != nil { updates["local_embedding_model"] = *req.LocalEmbeddingModel }
	if req.ForceSmartRouting != nil { updates["force_smart_routing"] = *req.ForceSmartRouting }
	if req.RuleBasedRoutingEnabled != nil { updates["rule_based_routing_enabled"] = *req.RuleBasedRoutingEnabled }
	if req.RuleFallbackStrategy != nil { updates["rule_fallback_strategy"] = *req.RuleFallbackStrategy }
	if req.RuleFallbackTaskType != nil { updates["rule_fallback_task_type"] = *req.RuleFallbackTaskType }
	if err := h.configRepo.UpdateConfig(c.Request.Context(), updates); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "LLM routing config updated"})
}

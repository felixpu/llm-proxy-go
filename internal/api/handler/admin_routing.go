package handler

import (
	"context"
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
	Enabled                  *bool    `json:"enabled"`
	PrimaryModelID           *int64   `json:"primary_model_id"`
	FallbackModelID          *int64   `json:"fallback_model_id"`
	TimeoutSeconds           *int     `json:"timeout_seconds"`
	CacheEnabled             *bool    `json:"cache_enabled"`
	CacheTTLSeconds          *int     `json:"cache_ttl_seconds"`
	CacheTTLL3Seconds        *int     `json:"cache_ttl_l3_seconds"`
	MaxTokens                *int     `json:"max_tokens"`
	Temperature              *float64 `json:"temperature"`
	RetryCount               *int     `json:"retry_count"`
	SemanticCacheEnabled     *bool    `json:"semantic_cache_enabled"`
	EmbeddingModelID         *int64   `json:"embedding_model_id"`
	SimilarityThreshold      *float64 `json:"similarity_threshold"`
	LocalEmbeddingModel      *string  `json:"local_embedding_model"`
	ForceSmartRouting        *bool    `json:"force_smart_routing"`
	RuleBasedRoutingEnabled  *bool    `json:"rule_based_routing_enabled"`
	RuleFallbackStrategy     *string  `json:"rule_fallback_strategy"`
	RuleFallbackTaskType     *string  `json:"rule_fallback_task_type"`
	CrossRoleFallbackEnabled *bool    `json:"cross_role_fallback_enabled"`
}

// RoutingHandler handles routing model and LLM config API endpoints.
type RoutingHandler struct {
	modelRepo  routingModelStore
	configRepo routingConfigWriterReader
}

type routingModelStore interface {
	ListModels(ctx context.Context, providerID *int64) ([]*models.RoutingModel, error)
	GetModel(ctx context.Context, id int64) (*models.RoutingModel, error)
	AddModel(ctx context.Context, m *models.RoutingModel) (int64, error)
	UpdateModelPatch(ctx context.Context, id int64, patch repository.RoutingModelPatch) error
	DeleteModel(ctx context.Context, id int64) error
}

type routingConfigWriterReader interface {
	GetConfig(ctx context.Context) (*models.RoutingConfig, error)
	UpdateConfigPatch(ctx context.Context, patch repository.RoutingConfigPatch) error
}

// NewRoutingHandler creates a new RoutingHandler.
func NewRoutingHandler(modelRepo routingModelStore, configRepo routingConfigWriterReader) *RoutingHandler {
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
	patch := repository.RoutingModelPatch{
		ProviderID:        req.ProviderID,
		ModelName:         req.ModelName,
		Enabled:           req.Enabled,
		Priority:          req.Priority,
		CostPerMtokInput:  req.CostPerMtokInput,
		CostPerMtokOutput: req.CostPerMtokOutput,
		BillingMultiplier: req.BillingMultiplier,
		Description:       req.Description,
	}
	if err := h.modelRepo.UpdateModelPatch(c.Request.Context(), id, patch); err != nil {
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
	patch := repository.RoutingConfigPatch{
		Enabled:                  req.Enabled,
		PrimaryModelID:           req.PrimaryModelID,
		FallbackModelID:          req.FallbackModelID,
		TimeoutSeconds:           req.TimeoutSeconds,
		CacheEnabled:             req.CacheEnabled,
		CacheTTLSeconds:          req.CacheTTLSeconds,
		CacheTTLL3Seconds:        req.CacheTTLL3Seconds,
		MaxTokens:                req.MaxTokens,
		Temperature:              req.Temperature,
		RetryCount:               req.RetryCount,
		SemanticCacheEnabled:     req.SemanticCacheEnabled,
		EmbeddingModelID:         req.EmbeddingModelID,
		SimilarityThreshold:      req.SimilarityThreshold,
		LocalEmbeddingModel:      req.LocalEmbeddingModel,
		ForceSmartRouting:        req.ForceSmartRouting,
		RuleBasedRoutingEnabled:  req.RuleBasedRoutingEnabled,
		RuleFallbackStrategy:     req.RuleFallbackStrategy,
		RuleFallbackTaskType:     req.RuleFallbackTaskType,
		CrossRoleFallbackEnabled: req.CrossRoleFallbackEnabled,
	}
	if err := h.configRepo.UpdateConfigPatch(c.Request.Context(), patch); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "LLM routing config updated"})
}

package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// maskAPIKey masks sensitive API key for display
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:4] + "****...****" + apiKey[len(apiKey)-4:]
}

// ProviderCreate represents a provider creation request.
type ProviderCreate struct {
	Name          string  `json:"name" binding:"required"`
	BaseURL       string  `json:"base_url" binding:"required"`
	APIKey        string  `json:"api_key" binding:"required"`
	Weight        int     `json:"weight"`
	MaxConcurrent int     `json:"max_concurrent"`
	Enabled       bool    `json:"enabled"`
	Description   string  `json:"description"`
	ModelIDs      []int64 `json:"model_ids"`
}

// ProviderUpdate represents a provider update request.
type ProviderUpdate struct {
	Name          *string `json:"name"`
	BaseURL       *string `json:"base_url"`
	APIKey        *string `json:"api_key"`
	Weight        *int    `json:"weight"`
	MaxConcurrent *int    `json:"max_concurrent"`
	Enabled       *bool   `json:"enabled"`
	Description   *string `json:"description"`
	ModelIDs      []int64 `json:"model_ids"`
}

// DetectModelsRequest represents a model detection request.
type DetectModelsRequest struct {
	BaseURL      string  `json:"base_url" binding:"required"`
	APIKey       *string `json:"api_key"`
	ProviderID   *int64  `json:"provider_id"`
	ProviderType *string `json:"provider_type"`
}

// DetectModelsResponse represents a model detection response.
type DetectModelsResponse struct {
	Success   bool                    `json:"success"`
	Models    []service.DetectedModel `json:"models"`
	APIFormat string                  `json:"api_format"`
	Error     *string                 `json:"error,omitempty"`
}

// ProviderResponse extends Provider with model details for API responses.
type ProviderResponse struct {
	*models.Provider
	APIKey string         `json:"api_key,omitempty"`
	Models []*models.Model `json:"models"`
}

// ProviderHandler handles provider management API endpoints.
type ProviderHandler struct {
	providerRepo  *repository.SQLProviderRepository
	modelRepo     *repository.SQLModelRepository
	modelDetector *service.ModelDetector
	endpointStore *service.EndpointStore
}

// NewProviderHandler creates a new ProviderHandler.
func NewProviderHandler(providerRepo *repository.SQLProviderRepository, modelRepo *repository.SQLModelRepository, modelDetector *service.ModelDetector, endpointStore *service.EndpointStore) *ProviderHandler {
	return &ProviderHandler{providerRepo: providerRepo, modelRepo: modelRepo, modelDetector: modelDetector, endpointStore: endpointStore}
}

// ListProviders returns all providers with their models.
func (h *ProviderHandler) ListProviders(c *gin.Context) {
	providers, err := h.providerRepo.FindAll(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]ProviderResponse, 0, len(providers))
	for _, p := range providers {
		mids, _ := h.providerRepo.GetModelIDsForProvider(c.Request.Context(), p.ID)
		models := make([]*models.Model, 0, len(mids))
		for _, mid := range mids {
			m, err := h.modelRepo.FindByID(c.Request.Context(), mid)
			if err == nil && m != nil {
				models = append(models, m)
			}
		}
		result = append(result, ProviderResponse{
			Provider: p,
			Models:   models,
			APIKey:   maskAPIKey(p.APIKey),
		})
	}
	c.JSON(http.StatusOK, gin.H{"providers": result})
}
// GetProvider returns a single provider by ID.
func (h *ProviderHandler) GetProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("provider_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid provider_id")
		return
	}
	p, err := h.providerRepo.FindByID(c.Request.Context(), id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		errorResponse(c, http.StatusNotFound, "provider not found")
		return
	}
	mids, _ := h.providerRepo.GetModelIDsForProvider(c.Request.Context(), id)
	models := make([]*models.Model, 0, len(mids))
	for _, mid := range mids {
		m, err := h.modelRepo.FindByID(c.Request.Context(), mid)
		if err == nil && m != nil {
			models = append(models, m)
		}
	}
	c.JSON(http.StatusOK, ProviderResponse{
		Provider: p,
		Models:   models,
		APIKey:   maskAPIKey(p.APIKey),
	})
}

// CreateProvider creates a new provider.
func (h *ProviderHandler) CreateProvider(c *gin.Context) {
	var req ProviderCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	p := &models.Provider{
		Name:          req.Name,
		BaseURL:       req.BaseURL,
		APIKey:        req.APIKey,
		Weight:        req.Weight,
		MaxConcurrent: req.MaxConcurrent,
		Enabled:       req.Enabled,
		Description:   req.Description,
	}
	id, err := h.providerRepo.Insert(c.Request.Context(), p, req.ModelIDs)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Provider created"})
	go h.endpointStore.ReloadAndNotify(context.Background())
}

// UpdateProvider updates an existing provider.
func (h *ProviderHandler) UpdateProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("provider_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid provider_id")
		return
	}
	var req ProviderUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	updates := make(map[string]any)
	if req.Name != nil { updates["name"] = *req.Name }
	if req.BaseURL != nil { updates["base_url"] = *req.BaseURL }
	if req.APIKey != nil { updates["api_key"] = *req.APIKey }
	if req.Weight != nil { updates["weight"] = *req.Weight }
	if req.MaxConcurrent != nil { updates["max_concurrent"] = *req.MaxConcurrent }
	if req.Enabled != nil { updates["enabled"] = *req.Enabled }
	if req.Description != nil { updates["description"] = *req.Description }
	if err := h.providerRepo.Update(c.Request.Context(), id, updates, req.ModelIDs); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Provider updated"})
	go h.endpointStore.ReloadAndNotify(context.Background())
}
// DeleteProvider deletes a provider.
func (h *ProviderHandler) DeleteProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("provider_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid provider_id")
		return
	}
	if err := h.providerRepo.Delete(c.Request.Context(), id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Provider deleted"})
	go h.endpointStore.ReloadAndNotify(context.Background())
}

// GetProviderModels returns models associated with a provider.
func (h *ProviderHandler) GetProviderModels(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("provider_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid provider_id")
		return
	}
	mids, err := h.providerRepo.GetModelIDsForProvider(c.Request.Context(), id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if mids == nil { mids = []int64{} }
	// Resolve model details
	modelList := make([]*models.Model, 0, len(mids))
	for _, mid := range mids {
		m, err := h.modelRepo.FindByID(c.Request.Context(), mid)
		if err == nil && m != nil {
			modelList = append(modelList, m)
		}
	}
	c.JSON(http.StatusOK, gin.H{"provider_id": id, "models": modelList})
}

// DetectModels detects available models from a provider.
func (h *ProviderHandler) DetectModels(c *gin.Context) {
	var req DetectModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Get API key: use provided key, or fall back to database lookup
	apiKey := ""
	if req.APIKey != nil && *req.APIKey != "" {
		apiKey = *req.APIKey
	} else if req.ProviderID != nil {
		// Look up API key from database
		provider, err := h.providerRepo.FindByID(c.Request.Context(), *req.ProviderID)
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, err.Error())
			return
		}
		if provider == nil {
			errorResponse(c, http.StatusNotFound, "Provider not found")
			return
		}
		apiKey = provider.APIKey
	}

	if apiKey == "" {
		errorResponse(c, http.StatusBadRequest, "API Key is required")
		return
	}

	result := h.modelDetector.Detect(c.Request.Context(), req.BaseURL, apiKey)

	resp := DetectModelsResponse{
		Success:   result.Success,
		Models:    result.Models,
		APIFormat: result.APIFormat,
	}
	if resp.Models == nil {
		resp.Models = []service.DetectedModel{}
	}
	if result.Error != "" {
		resp.Error = &result.Error
	}
	c.JSON(http.StatusOK, resp)
}

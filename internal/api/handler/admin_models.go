package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// ModelCreate represents a model creation request.
type ModelCreate struct {
	Name              string  `json:"name" binding:"required"`
	Role              string  `json:"role" binding:"required"`
	CostPerMtokInput  float64 `json:"cost_per_mtok_input"`
	CostPerMtokOutput float64 `json:"cost_per_mtok_output"`
	BillingMultiplier float64 `json:"billing_multiplier"`
	SupportsThinking  bool    `json:"supports_thinking"`
	Enabled           bool    `json:"enabled"`
	Weight            int     `json:"weight"`
}

// ModelUpdate represents a model update request.
type ModelUpdate struct {
	Name              *string  `json:"name"`
	Role              *string  `json:"role"`
	CostPerMtokInput  *float64 `json:"cost_per_mtok_input"`
	CostPerMtokOutput *float64 `json:"cost_per_mtok_output"`
	BillingMultiplier *float64 `json:"billing_multiplier"`
	SupportsThinking  *bool    `json:"supports_thinking"`
	Enabled           *bool    `json:"enabled"`
	Weight            *int     `json:"weight"`
}

// ModelHandler handles model management API endpoints.
type ModelHandler struct {
	repo          *repository.SQLModelRepository
	endpointStore *service.EndpointStore
}

// NewModelHandler creates a new ModelHandler.
func NewModelHandler(repo *repository.SQLModelRepository, endpointStore *service.EndpointStore) *ModelHandler {
	return &ModelHandler{repo: repo, endpointStore: endpointStore}
}
// ListModels returns all models.
func (h *ModelHandler) ListModels(c *gin.Context) {
	list, err := h.repo.FindAll(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*models.Model{}
	}
	c.JSON(http.StatusOK, gin.H{"models": list})
}

// GetModel returns a single model by ID.
func (h *ModelHandler) GetModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid model_id")
		return
	}
	m, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		errorResponse(c, http.StatusNotFound, "model not found")
		return
	}
	c.JSON(http.StatusOK, m)
}

// CreateModel creates a new model.
func (h *ModelHandler) CreateModel(c *gin.Context) {
	var req ModelCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	m := &models.Model{
		Name:              req.Name,
		Role:              models.ModelRole(req.Role),
		CostPerMtokInput:  req.CostPerMtokInput,
		CostPerMtokOutput: req.CostPerMtokOutput,
		BillingMultiplier: req.BillingMultiplier,
		SupportsThinking:  req.SupportsThinking,
		Enabled:           req.Enabled,
		Weight:            req.Weight,
	}
	id, err := h.repo.Insert(c.Request.Context(), m)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Model created"})
	go h.endpointStore.ReloadAndNotify(context.Background())
}

// UpdateModel updates an existing model.
func (h *ModelHandler) UpdateModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid model_id")
		return
	}
	var req ModelUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	updates := make(map[string]any)
	if req.Name != nil { updates["name"] = *req.Name }
	if req.Role != nil { updates["role"] = *req.Role }
	if req.CostPerMtokInput != nil { updates["cost_per_mtok_input"] = *req.CostPerMtokInput }
	if req.CostPerMtokOutput != nil { updates["cost_per_mtok_output"] = *req.CostPerMtokOutput }
	if req.BillingMultiplier != nil { updates["billing_multiplier"] = *req.BillingMultiplier }
	if req.SupportsThinking != nil { updates["supports_thinking"] = *req.SupportsThinking }
	if req.Enabled != nil { updates["enabled"] = *req.Enabled }
	if req.Weight != nil { updates["weight"] = *req.Weight }
	if err := h.repo.Update(c.Request.Context(), id, updates); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Model updated"})
	go h.endpointStore.ReloadAndNotify(context.Background())
}

// DeleteModel deletes a model.
func (h *ModelHandler) DeleteModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid model_id")
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Model deleted"})
	go h.endpointStore.ReloadAndNotify(context.Background())
}

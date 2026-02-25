package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
)

// EmbeddingHandler handles embedding model management endpoints.
type EmbeddingHandler struct {
	repo *repository.EmbeddingModelRepository
}

// NewEmbeddingHandler creates a new EmbeddingHandler.
func NewEmbeddingHandler(repo *repository.EmbeddingModelRepository) *EmbeddingHandler {
	return &EmbeddingHandler{repo: repo}
}

// ListModels returns all embedding models.
// GET /api/config/embedding/models
func (h *EmbeddingHandler) ListModels(c *gin.Context) {
	enabledOnly := c.Query("enabled_only") == "true"

	modelList, err := h.repo.ListModels(c.Request.Context(), enabledOnly)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	var data any = modelList
	if modelList == nil {
		data = []any{}
	}

	c.JSON(http.StatusOK, gin.H{"models": data})
}

// CreateModel creates a new embedding model.
// POST /api/config/embedding/models
func (h *EmbeddingHandler) CreateModel(c *gin.Context) {
	var req struct {
		Name               string `json:"name" binding:"required"`
		Dimension          int    `json:"dimension" binding:"required,min=1"`
		Description        string `json:"description"`
		FastembedSupported bool   `json:"fastembed_supported"`
		FastembedName      string `json:"fastembed_name"`
		Enabled            bool   `json:"enabled"`
		SortOrder          int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := h.repo.GetModelByName(c.Request.Context(), req.Name)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		errorResponse(c, http.StatusConflict, "Embedding model with this name already exists")
		return
	}

	model := &models.EmbeddingModel{
		Name:               req.Name,
		Dimension:          req.Dimension,
		Description:        req.Description,
		FastembedSupported: req.FastembedSupported,
		FastembedName:      req.FastembedName,
		Enabled:            req.Enabled,
		SortOrder:          req.SortOrder,
	}

	id, err := h.repo.AddModel(c.Request.Context(), model)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                 id,
		"name":               req.Name,
		"dimension":          req.Dimension,
		"description":        req.Description,
		"fastembed_supported": req.FastembedSupported,
		"fastembed_name":     req.FastembedName,
		"enabled":            req.Enabled,
		"sort_order":         req.SortOrder,
	})
}

// UpdateModel updates an existing embedding model.
// PUT /api/config/embedding/models/:model_id
func (h *EmbeddingHandler) UpdateModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid model ID")
		return
	}

	var req struct {
		Name               *string `json:"name"`
		Dimension          *int    `json:"dimension"`
		Description        *string `json:"description"`
		FastembedSupported *bool   `json:"fastembed_supported"`
		FastembedName      *string `json:"fastembed_name"`
		Enabled            *bool   `json:"enabled"`
		SortOrder          *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	updates := make(map[string]any)
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Dimension != nil {
		updates["dimension"] = *req.Dimension
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.FastembedSupported != nil {
		updates["fastembed_supported"] = *req.FastembedSupported
	}
	if req.FastembedName != nil {
		updates["fastembed_name"] = *req.FastembedName
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}

	if err := h.repo.UpdateModel(c.Request.Context(), id, updates); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Embedding model updated"})
}

// DeleteModel deletes an embedding model.
// DELETE /api/config/embedding/models/:model_id
func (h *EmbeddingHandler) DeleteModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("model_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid model ID")
		return
	}

	if err := h.repo.DeleteModel(c.Request.Context(), id); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "builtin") {
			errorResponse(c, http.StatusBadRequest, errMsg)
			return
		}
		if strings.Contains(errMsg, "not found") {
			errorResponse(c, http.StatusNotFound, errMsg)
			return
		}
		errorResponse(c, http.StatusInternalServerError, errMsg)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Embedding model deleted"})
}

// ListLocalModels returns available local embedding models from the database.
// GET /api/config/embedding/local-models
func (h *EmbeddingHandler) ListLocalModels(c *gin.Context) {
	modelList, err := h.repo.ListModels(c.Request.Context(), true)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, 0, len(modelList))
	for _, m := range modelList {
		result = append(result, map[string]any{
			"name":        m.Name,
			"dimension":   m.Dimension,
			"description": m.Description,
			"downloaded":  false,
			"downloading": false,
		})
	}

	c.JSON(http.StatusOK, gin.H{"models": result})
}

// GetModelStatus returns the download status of a local model.
// GET /api/config/embedding/local-models/:model_name/status
func (h *EmbeddingHandler) GetModelStatus(c *gin.Context) {
	modelName := c.Param("model_name")
	c.JSON(http.StatusOK, gin.H{
		"model_name": modelName,
		"status":     "not_available",
		"message":    "Local embedding models not supported in Go version",
	})
}

// DownloadModel triggers a local model download (not supported).
// POST /api/config/embedding/local-models/:model_name/download
func (h *EmbeddingHandler) DownloadModel(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "not_supported",
		"message": "Local embedding model download not supported in Go version",
	})
}

// DeleteLocalModel deletes a local model file (not supported).
// DELETE /api/config/embedding/local-models/:model_name
func (h *EmbeddingHandler) DeleteLocalModel(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "not_supported",
		"message": "Local embedding model deletion not supported in Go version",
	})
}

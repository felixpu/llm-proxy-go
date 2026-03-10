package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
)

// ModelAliasCreate represents a model alias creation request.
type ModelAliasCreate struct {
	AliasName     string `json:"alias_name" binding:"required"`
	TargetModelID int64  `json:"target_model_id" binding:"required"`
	Enabled       *bool  `json:"enabled"`
}

// ModelAliasUpdate represents a model alias update request.
type ModelAliasUpdate struct {
	AliasName     *string `json:"alias_name"`
	TargetModelID *int64  `json:"target_model_id"`
	Enabled       *bool   `json:"enabled"`
}

// ModelAliasHandler handles model alias management API endpoints.
type ModelAliasHandler struct {
	repo      repository.ModelAliasRepository
	modelRepo repository.ModelRepository
}

// NewModelAliasHandler creates a new ModelAliasHandler.
func NewModelAliasHandler(repo repository.ModelAliasRepository, modelRepo repository.ModelRepository) *ModelAliasHandler {
	return &ModelAliasHandler{repo: repo, modelRepo: modelRepo}
}

// ListModelAliases returns all model aliases.
func (h *ModelAliasHandler) ListModelAliases(c *gin.Context) {
	list, err := h.repo.FindAll(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*models.ModelAlias{}
	}
	c.JSON(http.StatusOK, gin.H{"model_aliases": list})
}

// GetModelAlias returns a single model alias by ID.
func (h *ModelAliasHandler) GetModelAlias(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("alias_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid alias_id")
		return
	}
	alias, err := h.repo.FindByID(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		errorResponse(c, http.StatusNotFound, "model alias not found")
		return
	}
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, alias)
}

// CreateModelAlias creates a new model alias.
func (h *ModelAliasHandler) CreateModelAlias(c *gin.Context) {
	var req ModelAliasCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	aliasName := strings.TrimSpace(req.AliasName)
	if err := h.validateAliasRequest(c, aliasName, req.TargetModelID); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	id, err := h.repo.Insert(c.Request.Context(), &models.ModelAlias{
		AliasName:     aliasName,
		TargetModelID: req.TargetModelID,
		Enabled:       enabled,
	})
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Model alias created"})
}

// UpdateModelAlias updates an existing model alias.
func (h *ModelAliasHandler) UpdateModelAlias(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("alias_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid alias_id")
		return
	}
	var req ModelAliasUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	current, err := h.repo.FindByID(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		errorResponse(c, http.StatusNotFound, "model alias not found")
		return
	}
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	aliasName := current.AliasName
	if req.AliasName != nil {
		aliasName = strings.TrimSpace(*req.AliasName)
	}
	targetModelID := current.TargetModelID
	if req.TargetModelID != nil {
		targetModelID = *req.TargetModelID
	}
	if err := h.validateAliasRequest(c, aliasName, targetModelID); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	patch := repository.ModelAliasPatch{
		AliasName:     &aliasName,
		TargetModelID: &targetModelID,
		Enabled:       req.Enabled,
	}
	if err := h.repo.UpdatePatch(c.Request.Context(), id, patch); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Model alias updated"})
}

// DeleteModelAlias deletes a model alias.
func (h *ModelAliasHandler) DeleteModelAlias(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("alias_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid alias_id")
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Model alias deleted"})
}

func (h *ModelAliasHandler) validateAliasRequest(c *gin.Context, aliasName string, targetModelID int64) error {
	if aliasName == "" {
		return errors.New("alias_name is required")
	}
	if strings.EqualFold(aliasName, "auto") {
		return errors.New(`alias_name "auto" is reserved`)
	}
	model, err := h.modelRepo.FindByID(c.Request.Context(), targetModelID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("target model not found")
	}
	if err != nil {
		return err
	}
	if model == nil {
		return errors.New("target model not found")
	}
	return nil
}

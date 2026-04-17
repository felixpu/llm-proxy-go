package handler

import (
	"context"
	"database/sql"
	"encoding/json"
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
	ProviderID    *int64 `json:"provider_id"`
	Enabled       *bool  `json:"enabled"`
}

// ModelAliasUpdate represents a model alias update request.
type ModelAliasUpdate struct {
	AliasName     *string `json:"alias_name"`
	TargetModelID *int64  `json:"target_model_id"`
	ProviderID    *int64  `json:"provider_id"`
	ProviderIDSet bool    `json:"-"`
	Enabled       *bool   `json:"enabled"`
}

// ModelAliasHandler handles model alias management API endpoints.
type ModelAliasHandler struct {
	repo         repository.ModelAliasRepository
	modelRepo    repository.ModelRepository
	providerRepo providerByIDFinder
}

type providerByIDFinder interface {
	FindByID(ctx context.Context, id int64) (*models.Provider, error)
	GetModelIDsForProvider(ctx context.Context, providerID int64) ([]int64, error)
}

// NewModelAliasHandler creates a new ModelAliasHandler.
func NewModelAliasHandler(
	repo repository.ModelAliasRepository,
	modelRepo repository.ModelRepository,
	providerRepo providerByIDFinder,
) *ModelAliasHandler {
	return &ModelAliasHandler{
		repo:         repo,
		modelRepo:    modelRepo,
		providerRepo: providerRepo,
	}
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
	if err := h.validateAliasRequest(c, aliasName, req.TargetModelID, req.ProviderID); err != nil {
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
		ProviderID:    req.ProviderID,
		Enabled:       enabled,
	})
	if err != nil {
		if isDuplicateModelAliasMappingError(err) {
			errorResponse(c, http.StatusConflict, "duplicate model alias mapping")
			return
		}
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
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if v, ok := raw["alias_name"]; ok {
		var aliasName string
		if err := json.Unmarshal(v, &aliasName); err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid alias_name")
			return
		}
		req.AliasName = &aliasName
	}
	if v, ok := raw["target_model_id"]; ok {
		if string(v) != "null" {
			var targetModelID int64
			if err := json.Unmarshal(v, &targetModelID); err != nil {
				errorResponse(c, http.StatusBadRequest, "invalid target_model_id")
				return
			}
			req.TargetModelID = &targetModelID
		}
	}
	if v, ok := raw["provider_id"]; ok {
		req.ProviderIDSet = true
		if string(v) != "null" {
			var providerID int64
			if err := json.Unmarshal(v, &providerID); err != nil {
				errorResponse(c, http.StatusBadRequest, "invalid provider_id")
				return
			}
			req.ProviderID = &providerID
		}
	}
	if v, ok := raw["enabled"]; ok {
		var enabled bool
		if err := json.Unmarshal(v, &enabled); err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid enabled")
			return
		}
		req.Enabled = &enabled
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
	providerID := current.ProviderID
	if req.ProviderIDSet {
		providerID = req.ProviderID
	}
	if err := h.validateAliasRequest(c, aliasName, targetModelID, providerID); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	patch := repository.ModelAliasPatch{
		AliasName:     &aliasName,
		TargetModelID: &targetModelID,
		ProviderID:    providerID,
		ProviderIDSet: req.ProviderIDSet,
		Enabled:       req.Enabled,
	}
	if err := h.repo.UpdatePatch(c.Request.Context(), id, patch); err != nil {
		if isDuplicateModelAliasMappingError(err) {
			errorResponse(c, http.StatusConflict, "duplicate model alias mapping")
			return
		}
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

func (h *ModelAliasHandler) validateAliasRequest(c *gin.Context, aliasName string, targetModelID int64, providerID *int64) error {
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
	if providerID != nil && h.providerRepo != nil {
		provider, err := h.providerRepo.FindByID(c.Request.Context(), *providerID)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("provider not found")
		}
		if err != nil {
			return err
		}
		if provider == nil {
			return errors.New("provider not found")
		}

		modelIDs, err := h.providerRepo.GetModelIDsForProvider(c.Request.Context(), *providerID)
		if err != nil {
			return err
		}
		containsTargetModel := false
		for _, modelID := range modelIDs {
			if modelID == targetModelID {
				containsTargetModel = true
				break
			}
		}
		if !containsTargetModel {
			return errors.New("provider is not associated with target model")
		}
	}
	return nil
}

func isDuplicateModelAliasMappingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed")
}

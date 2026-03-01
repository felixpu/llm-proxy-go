package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// APIKeyHandler handles API key management endpoints.
type APIKeyHandler struct {
	keyRepo repository.APIKeyRepository
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(keyRepo repository.APIKeyRepository) *APIKeyHandler {
	return &APIKeyHandler{keyRepo: keyRepo}
}

// ListAPIKeys lists API keys.
// GET /api/keys
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var keys []*models.APIKey
	var err error

	// Admin can see all keys, users see only their own
	if currentUser.Role == string(models.UserRoleAdmin) {
		keys, err = h.keyRepo.FindAll(c.Request.Context())
	} else {
		keys, err = h.keyRepo.FindByUserID(c.Request.Context(), currentUser.UserID)
	}

	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to list API keys")
		return
	}

	c.JSON(http.StatusOK, keys)
}

// GetAPIKey retrieves an API key by ID.
// GET /api/keys/:id
func (h *APIKeyHandler) GetAPIKey(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid key ID")
		return
	}

	key, err := h.keyRepo.FindByID(c.Request.Context(), keyID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "API key not found")
		return
	}

	// Check permission: users can only see their own keys
	if currentUser.Role != string(models.UserRoleAdmin) && key.UserID != currentUser.UserID {
		errorResponse(c, http.StatusForbidden, "No permission to view this API key")
		return
	}

	c.JSON(http.StatusOK, key)
}

// CreateAPIKey creates a new API key.
// POST /api/keys
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req struct {
		Name        string     `json:"name" binding:"required,max=100"`
		ExpiresDays *int       `json:"expires_days"`
		ExpiresAt   *time.Time `json:"expires_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Generate API key
	fullKey, keyHash, keyPrefix := service.GenerateAPIKey()

	// Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	} else if req.ExpiresDays != nil && *req.ExpiresDays > 0 {
		exp := time.Now().UTC().AddDate(0, 0, *req.ExpiresDays)
		expiresAt = &exp
	}

	key := &models.APIKey{
		UserID:    currentUser.UserID,
		KeyHash:   keyHash,
		KeyFull:   fullKey,
		KeyPrefix: keyPrefix,
		Name:      req.Name,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}

	id, err := h.keyRepo.Insert(c.Request.Context(), key)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to create API key")
		return
	}

	key.ID = id
	c.JSON(http.StatusCreated, gin.H{
		"id":         key.ID,
		"key":        fullKey,
		"key_prefix": keyPrefix,
		"name":       key.Name,
		"expires_at": expiresAt,
	})
}

// RevokeAPIKey revokes (disables) an API key.
// POST /api/keys/:id/revoke
func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid key ID")
		return
	}

	key, err := h.keyRepo.FindByID(c.Request.Context(), keyID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "API key not found")
		return
	}

	// Check permission
	if currentUser.Role != string(models.UserRoleAdmin) && key.UserID != currentUser.UserID {
		errorResponse(c, http.StatusForbidden, "No permission to revoke this API key")
		return
	}

	var userID *int64
	if currentUser.Role != string(models.UserRoleAdmin) {
		userID = &currentUser.UserID
	}

	if err := h.keyRepo.Revoke(c.Request.Context(), keyID, userID); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to revoke API key")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked"})
}

// ToggleAPIKey toggles the is_active state of an API key.
// POST /api/keys/:id/toggle
func (h *APIKeyHandler) ToggleAPIKey(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid key ID")
		return
	}

	key, err := h.keyRepo.FindByID(c.Request.Context(), keyID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "API key not found")
		return
	}

	// Check permission
	if currentUser.Role != string(models.UserRoleAdmin) && key.UserID != currentUser.UserID {
		errorResponse(c, http.StatusForbidden, "No permission to modify this API key")
		return
	}

	var userID *int64
	if currentUser.Role != string(models.UserRoleAdmin) {
		userID = &currentUser.UserID
	}

	newActive := !key.IsActive
	if err := h.keyRepo.SetActive(c.Request.Context(), keyID, userID, newActive); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to toggle API key")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "OK", "is_active": newActive})
}

// DeleteAPIKey deletes an API key.
// DELETE /api/keys/:id
func (h *APIKeyHandler) DeleteAPIKey(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid key ID")
		return
	}

	key, err := h.keyRepo.FindByID(c.Request.Context(), keyID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "API key not found")
		return
	}

	// Check permission
	if currentUser.Role != string(models.UserRoleAdmin) && key.UserID != currentUser.UserID {
		errorResponse(c, http.StatusForbidden, "No permission to delete this API key")
		return
	}

	var userID *int64
	if currentUser.Role != string(models.UserRoleAdmin) {
		userID = &currentUser.UserID
	}

	if err := h.keyRepo.Delete(c.Request.Context(), keyID, userID); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to delete API key")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key deleted"})
}

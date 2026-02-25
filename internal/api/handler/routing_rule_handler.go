package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

// RoutingRuleCreate represents a routing rule creation request.
type RoutingRuleCreate struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Pattern     string   `json:"pattern"`
	Condition   string   `json:"condition"`
	TaskType    string   `json:"task_type" binding:"required"`
	Priority    int      `json:"priority"`
	Enabled     bool     `json:"enabled"`
}

// RoutingRuleUpdate represents a routing rule update request.
type RoutingRuleUpdate struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Keywords    *[]string `json:"keywords"`
	Pattern     *string   `json:"pattern"`
	Condition   *string   `json:"condition"`
	TaskType    *string   `json:"task_type"`
	Priority    *int      `json:"priority"`
	Enabled     *bool     `json:"enabled"`
}

// TestMessageRequest represents a rule test request.
type TestMessageRequest struct {
	Message      string `json:"message" binding:"required"`
	SystemPrompt string `json:"system_prompt"`
}

// RoutingRuleHandler handles routing rule API endpoints.
type RoutingRuleHandler struct {
	ruleRepo *repository.RoutingRuleRepo
	logger   *zap.Logger
}

// NewRoutingRuleHandler creates a new RoutingRuleHandler.
func NewRoutingRuleHandler(ruleRepo *repository.RoutingRuleRepo, logger *zap.Logger) *RoutingRuleHandler {
	return &RoutingRuleHandler{ruleRepo: ruleRepo, logger: logger}
}

// ListRules returns all routing rules, optionally filtered by enabled status.
func (h *RoutingRuleHandler) ListRules(c *gin.Context) {
	enabledOnly := c.Query("enabled_only") == "true"

	rules, err := h.ruleRepo.ListRules(c.Request.Context(), enabledOnly)
	if err != nil {
		h.logger.Error("failed to list rules", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []*models.RoutingRule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// GetRule returns a single routing rule by ID.
func (h *RoutingRuleHandler) GetRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid rule_id")
		return
	}

	rule, err := h.ruleRepo.GetRule(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("failed to get rule", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if rule == nil {
		errorResponse(c, http.StatusNotFound, "routing rule not found")
		return
	}
	c.JSON(http.StatusOK, rule)
}

// CreateRule creates a new routing rule.
func (h *RoutingRuleHandler) CreateRule(c *gin.Context) {
	var req RoutingRuleCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	rule := &models.RoutingRule{
		Name:        req.Name,
		Description: req.Description,
		Keywords:    req.Keywords,
		Pattern:     req.Pattern,
		Condition:   req.Condition,
		TaskType:    req.TaskType,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
	}

	id, err := h.ruleRepo.AddRule(c.Request.Context(), rule)
	if err != nil {
		h.logger.Error("failed to create rule", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Routing rule created"})
}

// UpdateRule updates an existing routing rule.
func (h *RoutingRuleHandler) UpdateRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid rule_id")
		return
	}

	// Check if rule exists
	existing, err := h.ruleRepo.GetRule(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("failed to get rule", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		errorResponse(c, http.StatusNotFound, "routing rule not found")
		return
	}

	var req RoutingRuleUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	updates := make(map[string]any)
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Keywords != nil {
		updates["keywords"] = *req.Keywords
	}
	if req.Pattern != nil {
		updates["pattern"] = *req.Pattern
	}
	if req.Condition != nil {
		updates["condition"] = *req.Condition
	}
	if req.TaskType != nil {
		updates["task_type"] = *req.TaskType
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.ruleRepo.UpdateRule(c.Request.Context(), id, updates); err != nil {
		h.logger.Error("failed to update rule", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Routing rule updated"})
}

// DeleteRule deletes a routing rule.
func (h *RoutingRuleHandler) DeleteRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid rule_id")
		return
	}

	// Check if rule exists and is not builtin
	existing, err := h.ruleRepo.GetRule(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("failed to get rule", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		errorResponse(c, http.StatusNotFound, "routing rule not found")
		return
	}
	if existing.IsBuiltin {
		errorResponse(c, http.StatusForbidden, "cannot delete builtin rule")
		return
	}

	if err := h.ruleRepo.DeleteRule(c.Request.Context(), id); err != nil {
		h.logger.Error("failed to delete rule", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "message": "Routing rule deleted"})
}

// TestMessage tests a message against all routing rules.
func (h *RoutingRuleHandler) TestMessage(c *gin.Context) {
	var req TestMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Load all enabled rules from DB
	rules, err := h.ruleRepo.ListRules(c.Request.Context(), true)
	if err != nil {
		h.logger.Error("failed to list rules for test", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	classifier := service.NewRoutingClassifier(rules)
	result := classifier.TestMessage(req.Message)

	resp := gin.H{
		"final_task_type": result.TaskType,
		"reason":          result.Reason,
		"all_matches":     result.Matches,
	}
	if result.Rule != nil {
		resp["matched_rule"] = gin.H{
			"id":        result.Rule.ID,
			"name":      result.Rule.Name,
			"task_type": result.Rule.TaskType,
		}
	}
	c.JSON(http.StatusOK, resp)
}

// GetStats returns routing rule statistics.
func (h *RoutingRuleHandler) GetStats(c *gin.Context) {
	stats, err := h.ruleRepo.GetStats(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get stats", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, stats)
}

// ListBuiltinRules returns only builtin routing rules.
func (h *RoutingRuleHandler) ListBuiltinRules(c *gin.Context) {
	rules, err := h.ruleRepo.ListBuiltinRules(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list builtin rules", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []*models.RoutingRule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// ListCustomRules returns only custom (non-builtin) routing rules.
func (h *RoutingRuleHandler) ListCustomRules(c *gin.Context) {
	rules, err := h.ruleRepo.ListCustomRules(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list custom rules", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []*models.RoutingRule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

const (
	// routingQueryTimeout caps the maximum execution time for routing analysis queries.
	routingQueryTimeout = 10 * time.Second
)

// RoutingAnalysisHandler handles routing analysis endpoints.
type RoutingAnalysisHandler struct {
	logRepo  repository.RequestLogRepository
	ruleRepo repository.RoutingRuleRepository
	logger   *zap.Logger
}

// NewRoutingAnalysisHandler creates a new RoutingAnalysisHandler.
func NewRoutingAnalysisHandler(
	logRepo repository.RequestLogRepository,
	ruleRepo repository.RoutingRuleRepository,
	logger *zap.Logger,
) *RoutingAnalysisHandler {
	return &RoutingAnalysisHandler{
		logRepo:  logRepo,
		ruleRepo: ruleRepo,
		logger:   logger,
	}
}

// RoutingStats represents routing statistics.
type RoutingStats struct {
	TotalRequests    int64                    `json:"total_requests"`
	ByMethod         map[string]MethodStats   `json:"by_method"`
	ByRule           []RuleStats              `json:"by_rule"`
	InaccurateCount  int64                    `json:"inaccurate_count"`
	InaccurateRate   float64                  `json:"inaccurate_rate"`
}

// MethodStats represents statistics for a routing method.
type MethodStats struct {
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// RuleStats represents statistics for a matched rule.
type RuleStats struct {
	RuleID     *int64  `json:"rule_id,omitempty"`
	RuleName   string  `json:"rule_name"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// GetRoutingStats returns routing decision statistics via SQL aggregation.
// GET /api/routing/analysis/stats?start_time=...&end_time=...
func (h *RoutingAnalysisHandler) GetRoutingStats(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), routingQueryTimeout)
	defer cancel()

	var startTime, endTime *time.Time
	if st := c.Query("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			startTime = &t
		}
	}
	if et := c.Query("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			endTime = &t
		}
	}

	agg, err := h.logRepo.GetRoutingAggregation(ctx, startTime, endTime)
	if err != nil {
		h.logger.Error("failed to get routing aggregation", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, "Failed to get routing statistics")
		return
	}

	total := agg.TotalRequests
	stats := RoutingStats{
		TotalRequests: total,
		ByMethod:      make(map[string]MethodStats),
		ByRule:        make([]RuleStats, 0),
	}

	for method, count := range agg.MethodCounts {
		pct := 0.0
		if total > 0 {
			pct = float64(count) * 100.0 / float64(total)
		}
		stats.ByMethod[method] = MethodStats{
			Count:      count,
			Percentage: roundToPlaces(pct, 2),
		}
	}

	for ruleName, count := range agg.RuleCounts {
		pct := 0.0
		if total > 0 {
			pct = float64(count) * 100.0 / float64(total)
		}
		stats.ByRule = append(stats.ByRule, RuleStats{
			RuleID:     agg.RuleIDs[ruleName],
			RuleName:   ruleName,
			Count:      count,
			Percentage: roundToPlaces(pct, 2),
		})
	}

	stats.InaccurateCount = agg.InaccurateCount
	if total > 0 {
		stats.InaccurateRate = roundToPlaces(float64(agg.InaccurateCount)*100.0/float64(total), 2)
	}

	c.JSON(http.StatusOK, stats)
}

// GetInaccurateLogs returns logs marked as inaccurate via SQL-level filtering.
// GET /api/routing/analysis/inaccurate?limit=50&offset=0
func (h *RoutingAnalysisHandler) GetInaccurateLogs(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), routingQueryTimeout)
	defer cancel()

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, total, err := h.logRepo.ListInaccurate(ctx, limit, offset)
	if err != nil {
		h.logger.Error("failed to get inaccurate logs", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, "Failed to get inaccurate logs")
		return
	}

	// Map to response format
	type inaccurateEntry struct {
		ID              int64  `json:"id"`
		CreatedAt       string `json:"created_at"`
		ModelName       string `json:"model_name"`
		TaskType        string `json:"task_type"`
		RoutingMethod   string `json:"routing_method"`
		MatchedRuleName string `json:"matched_rule_name"`
		MessagePreview  string `json:"message_preview"`
	}

	entries := make([]inaccurateEntry, 0, len(logs))
	for _, log := range logs {
		entries = append(entries, inaccurateEntry{
			ID:              log.ID,
			CreatedAt:       log.CreatedAt.Format(time.RFC3339),
			ModelName:       log.ModelName,
			TaskType:        log.TaskType,
			RoutingMethod:   log.RoutingMethod,
			MatchedRuleName: log.MatchedRuleName,
			MessagePreview:  log.MessagePreview,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   entries,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// MarkLogInaccurate marks or unmarks a log as inaccurate.
// POST /api/logs/:id/mark-inaccurate
func (h *RoutingAnalysisHandler) MarkLogInaccurate(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid log ID")
		return
	}

	var req struct {
		Inaccurate bool `json:"inaccurate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx := c.Request.Context()
	if err := h.logRepo.MarkInaccurate(ctx, id, req.Inaccurate); err != nil {
		h.logger.Error("failed to mark log inaccurate", zap.Error(err), zap.Int64("id", id))
		errorResponse(c, http.StatusInternalServerError, "Failed to update log")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Log updated",
	})
}

// GetLogDetail returns detailed information for a single log.
// GET /api/logs/:id
func (h *RoutingAnalysisHandler) GetLogDetail(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid log ID")
		return
	}

	ctx := c.Request.Context()
	log, err := h.logRepo.GetByID(ctx, id)
	if err != nil {
		h.logger.Error("failed to get log detail", zap.Error(err), zap.Int64("id", id))
		errorResponse(c, http.StatusNotFound, "Log not found")
		return
	}

	c.JSON(http.StatusOK, log)
}

// ExportRoutingData exports routing data for LLM analysis.
// GET /api/routing/analysis/export?limit=100&inaccurate_only=false
func (h *RoutingAnalysisHandler) ExportRoutingData(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	ctx := c.Request.Context()
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	inaccurateOnly := c.Query("inaccurate_only") == "true"

	logs, _, err := h.logRepo.List(ctx, limit, 0, nil, nil, nil, nil, nil, nil)
	if err != nil {
		h.logger.Error("failed to export routing data", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, "Failed to export data")
		return
	}

	type ExportEntry struct {
		ID              int64    `json:"id"`
		MessagePreview  string   `json:"message_preview"`
		RequestContent  string   `json:"request_content,omitempty"`
		TaskType        string   `json:"task_type"`
		RoutingMethod   string   `json:"routing_method"`
		RoutingReason   string   `json:"routing_reason"`
		MatchedRuleName string   `json:"matched_rule_name"`
		IsInaccurate    bool     `json:"is_inaccurate"`
	}

	var entries []ExportEntry
	for _, log := range logs {
		if inaccurateOnly && !log.IsInaccurate {
			continue
		}
		entries = append(entries, ExportEntry{
			ID:              log.ID,
			MessagePreview:  log.MessagePreview,
			RequestContent:  log.RequestContent,
			TaskType:        log.TaskType,
			RoutingMethod:   log.RoutingMethod,
			RoutingReason:   log.RoutingReason,
			MatchedRuleName: log.MatchedRuleName,
			IsInaccurate:    log.IsInaccurate,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"count":   len(entries),
	})
}

// roundToPlaces rounds a float to n decimal places.
func roundToPlaces(val float64, places int) float64 {
	multiplier := 1.0
	for range places {
		multiplier *= 10
	}
	return float64(int(val*multiplier+0.5)) / multiplier
}

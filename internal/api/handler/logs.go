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
	// logQueryTimeout caps the maximum execution time for log read queries.
	logQueryTimeout = 10 * time.Second
	// maxLogLimit caps the maximum number of log entries per page.
	maxLogLimit = 500
)

// LogsHandler handles request log endpoints.
type LogsHandler struct {
	logRepo repository.RequestLogRepository
	logger  *zap.Logger
}

// NewLogsHandler creates a new LogsHandler.
func NewLogsHandler(logRepo repository.RequestLogRepository, logger *zap.Logger) *LogsHandler {
	return &LogsHandler{logRepo: logRepo, logger: logger}
}

// optionalStringParam returns a pointer to the query parameter value if non-empty, nil otherwise.
// This fixes the bug where empty strings were passed as non-nil pointers to repository methods.
func optionalStringParam(c *gin.Context, key string) *string {
	if v := c.Query(key); v != "" {
		return &v
	}
	return nil
}

// GetRequestLogs retrieves request logs (admin only).
// GET /api/logs?limit=100&offset=0&model=...&endpoint=...&start_time=...&end_time=...&success=...
func (h *LogsHandler) GetRequestLogs(c *gin.Context) {
	// Check admin permission
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	// Parse query parameters
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit > maxLogLimit {
		limit = maxLogLimit
	}

	model := optionalStringParam(c, "model")
	endpoint := optionalStringParam(c, "endpoint")

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

	var success *bool
	if s := c.Query("success"); s != "" {
		b := s == "true"
		success = &b
	}

	// Query logs with timeout to prevent slow queries from blocking the connection pool.
	ctx, cancel := context.WithTimeout(c.Request.Context(), logQueryTimeout)
	defer cancel()

	logs, total, err := h.logRepo.List(
		ctx,
		limit, offset,
		nil, // userID
		model, endpoint,
		startTime, endTime,
		success,
	)
	if err != nil {
		h.logger.Error("failed to retrieve logs", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, "Failed to retrieve logs")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// DeleteRequestLogs deletes request logs (admin only).
// DELETE /api/logs?model=...&endpoint=...&start_time=...&end_time=...
func (h *LogsHandler) DeleteRequestLogs(c *gin.Context) {
	// Check admin permission
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	model := optionalStringParam(c, "model")
	endpoint := optionalStringParam(c, "endpoint")

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

	// Delete logs
	deleted, err := h.logRepo.Delete(
		c.Request.Context(),
		model, endpoint,
		startTime, endTime,
	)
	if err != nil {
		h.logger.Error("failed to delete logs", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, "Failed to delete logs")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"message": "Logs deleted",
	})
}

// GetLogStats retrieves log statistics (admin only).
// GET /api/logs/stats?start_time=...&end_time=...&model=...&endpoint=...&success=...
func (h *LogsHandler) GetLogStats(c *gin.Context) {
	// Check admin permission
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil || currentUser.Role != "admin" {
		errorResponse(c, http.StatusForbidden, "Admin access required")
		return
	}

	model := optionalStringParam(c, "model")
	endpoint := optionalStringParam(c, "endpoint")

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

	var success *bool
	if s := c.Query("success"); s != "" {
		b := s == "true"
		success = &b
	}

	// Get statistics with timeout to prevent slow aggregation queries from blocking the pool.
	ctx, cancel := context.WithTimeout(c.Request.Context(), logQueryTimeout)
	defer cancel()

	stats, err := h.logRepo.GetStatistics(
		ctx,
		startTime, endTime,
		nil, // userID
		model, endpoint,
		success,
	)
	if err != nil {
		h.logger.Error("failed to retrieve statistics", zap.Error(err))
		errorResponse(c, http.StatusInternalServerError, "Failed to retrieve statistics")
		return
	}

	c.JSON(http.StatusOK, stats)
}

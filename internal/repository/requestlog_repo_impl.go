package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// RequestLogRepositoryImpl implements request log data access.
type RequestLogRepositoryImpl struct {
	db     *sql.DB // write operations
	readDB *sql.DB // read operations (may be a separate read-only pool)
	logger *zap.Logger
}

// NewRequestLogRepositoryImpl creates a new RequestLogRepositoryImpl.
// If readDB is nil, db is used for both reads and writes.
func NewRequestLogRepositoryImpl(db *sql.DB, logger *zap.Logger, readDB ...*sql.DB) *RequestLogRepositoryImpl {
	r := &RequestLogRepositoryImpl{
		db:     db,
		readDB: db,
		logger: logger,
	}
	if len(readDB) > 0 && readDB[0] != nil {
		r.readDB = readDB[0]
	}
	return r
}

// buildWhere builds the WHERE clause for log queries.
// All column references are qualified with table name to avoid ambiguity in JOIN queries.
func (r *RequestLogRepositoryImpl) buildWhere(
	userID *int64,
	modelName, endpointName *string,
	startTime, endTime *time.Time,
	success *bool,
) (string, []any) {
	conditions := []string{"1=1"}
	var params []any

	if userID != nil {
		conditions = append(conditions, "request_logs.user_id = ?")
		params = append(params, *userID)
	}
	if modelName != nil {
		conditions = append(conditions, "request_logs.model_name = ?")
		params = append(params, *modelName)
	}
	if endpointName != nil {
		conditions = append(conditions, "request_logs.endpoint_name = ?")
		params = append(params, *endpointName)
	}
	if startTime != nil {
		conditions = append(conditions, "request_logs.created_at >= ?")
		params = append(params, startTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		conditions = append(conditions, "request_logs.created_at <= ?")
		params = append(params, endTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if success != nil {
		conditions = append(conditions, "request_logs.success = ?")
		params = append(params, boolToInt(*success))
	}

	return strings.Join(conditions, " AND "), params
}

// scanLog scans a row into a RequestLog.
func (r *RequestLogRepositoryImpl) scanLog(rows *sql.Rows) (*models.RequestLog, error) {
	var log models.RequestLog
	var apiKeyID sql.NullInt64
	var statusCode sql.NullInt64
	var taskType sql.NullString
	var success, stream int
	var createdAt string

	var cacheCreationInputTokens, cacheReadInputTokens int
	var messagePreview, requestContent, responseContent sql.NullString
	var routingMethod, routingReason sql.NullString
	var matchedRuleID sql.NullInt64
	var matchedRuleName sql.NullString
	var allMatchesJSON sql.NullString
	var isInaccurate int

	err := rows.Scan(
		&log.ID, &log.RequestID, &log.UserID, &log.Username,
		&apiKeyID, &log.ModelName, &log.EndpointName, &taskType,
		&log.InputTokens, &log.OutputTokens, &log.LatencyMs, &log.Cost,
		&statusCode, &success, &stream, &createdAt,
		&cacheCreationInputTokens, &cacheReadInputTokens,
		&messagePreview, &requestContent, &responseContent,
		&routingMethod, &routingReason,
		&matchedRuleID, &matchedRuleName, &allMatchesJSON,
		&isInaccurate,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan log: %w", err)
	}

	if apiKeyID.Valid {
		log.APIKeyID = &apiKeyID.Int64
	}
	if statusCode.Valid {
		sc := int(statusCode.Int64)
		log.StatusCode = &sc
	}
	if taskType.Valid {
		log.TaskType = taskType.String
	}
	log.Success = success == 1
	log.Stream = stream == 1
	log.CreatedAt = parseFlexibleTime(createdAt)
	log.CacheCreationInputTokens = cacheCreationInputTokens
	log.CacheReadInputTokens = cacheReadInputTokens

	if messagePreview.Valid {
		log.MessagePreview = messagePreview.String
	}
	if requestContent.Valid {
		log.RequestContent = requestContent.String
	}
	if responseContent.Valid {
		log.ResponseContent = responseContent.String
	}
	if routingMethod.Valid {
		log.RoutingMethod = routingMethod.String
	}
	if routingReason.Valid {
		log.RoutingReason = routingReason.String
	}
	if matchedRuleID.Valid {
		id := matchedRuleID.Int64
		log.MatchedRuleID = &id
	}
	if matchedRuleName.Valid {
		log.MatchedRuleName = matchedRuleName.String
	}
	if allMatchesJSON.Valid && allMatchesJSON.String != "" {
		var matches []*models.RuleHit
		if err := json.Unmarshal([]byte(allMatchesJSON.String), &matches); err == nil {
			log.AllMatches = matches
		}
	}
	log.IsInaccurate = isInaccurate == 1

	return &log, nil
}

// parseFlexibleTime tries multiple time formats commonly used by SQLite.
func parseFlexibleTime(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05-07:00",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

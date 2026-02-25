package repository

import (
	"context"
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
	db     *sql.DB
	logger *zap.Logger
}

// NewRequestLogRepositoryImpl creates a new RequestLogRepositoryImpl.
func NewRequestLogRepositoryImpl(db *sql.DB, logger *zap.Logger) *RequestLogRepositoryImpl {
	return &RequestLogRepositoryImpl{
		db:     db,
		logger: logger,
	}
}

// Insert inserts a new request log entry.
func (r *RequestLogRepositoryImpl) Insert(ctx context.Context, entry *models.RequestLogEntry) (int64, error) {
	allMatchesJSON, err := json.Marshal(entry.AllMatches)
	if err != nil {
		allMatchesJSON = []byte("[]")
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO request_logs (
			request_id, user_id, api_key_id, model_name, endpoint_name,
			task_type, input_tokens, output_tokens, latency_ms, cost,
			status_code, success, stream,
			message_preview, request_content, response_content,
			routing_method, routing_reason,
			matched_rule_id, matched_rule_name, all_matches,
			is_inaccurate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.RequestID, entry.UserID, entry.APIKeyID, entry.ModelName, entry.EndpointName,
		entry.TaskType, entry.InputTokens, entry.OutputTokens, entry.LatencyMs, entry.Cost,
		entry.StatusCode, boolToInt(entry.Success), boolToInt(entry.Stream),
		entry.MessagePreview, entry.RequestContent, entry.ResponseContent,
		entry.RoutingMethod, entry.RoutingReason,
		entry.MatchedRuleID, entry.MatchedRuleName, string(allMatchesJSON),
		boolToInt(entry.IsInaccurate), time.Now().UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("failed to insert request log: %w", err)
	}
	return result.LastInsertId()
}

// List retrieves request logs with filtering and pagination.
func (r *RequestLogRepositoryImpl) List(
	ctx context.Context,
	limit, offset int,
	userID *int64,
	modelName, endpointName *string,
	startTime, endTime *time.Time,
	success *bool,
) ([]*models.RequestLog, int64, error) {
	whereSQL, params := r.buildWhere(userID, modelName, endpointName, startTime, endTime, success)

	// Count total
	var total int64
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM request_logs WHERE %s`, whereSQL)
	if err := r.db.QueryRowContext(ctx, countQuery, params...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	// Query with pagination
	query := fmt.Sprintf(`
		SELECT
			request_logs.id, request_logs.request_id, request_logs.user_id,
			COALESCE(u.username, '未知用户') as username,
			request_logs.api_key_id, request_logs.model_name, request_logs.endpoint_name,
			request_logs.task_type, request_logs.input_tokens, request_logs.output_tokens,
			request_logs.latency_ms, request_logs.cost, request_logs.status_code,
			request_logs.success, request_logs.stream, request_logs.created_at,
			request_logs.message_preview, request_logs.request_content, request_logs.response_content,
			request_logs.routing_method, request_logs.routing_reason,
			request_logs.matched_rule_id, request_logs.matched_rule_name, request_logs.all_matches,
			request_logs.is_inaccurate
		FROM request_logs
		LEFT JOIN users u ON request_logs.user_id = u.id
		WHERE %s
		ORDER BY request_logs.created_at DESC
		LIMIT ? OFFSET ?
	`, whereSQL)

	params = append(params, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []*models.RequestLog = make([]*models.RequestLog, 0)
	for rows.Next() {
		log, err := r.scanLog(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}

	return logs, total, rows.Err()
}

// GetStatistics retrieves aggregated statistics.
func (r *RequestLogRepositoryImpl) GetStatistics(
	ctx context.Context,
	startTime, endTime *time.Time,
	userID *int64,
	modelName, endpointName *string,
	success *bool,
) (*LogStatistics, error) {
	whereSQL, params := r.buildWhere(userID, modelName, endpointName, startTime, endTime, success)

	// Overall statistics
	overallQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(cost), 0) as total_cost,
			COALESCE(AVG(latency_ms), 0) as avg_latency,
			CASE WHEN COUNT(*) > 0
				THEN SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) * 100.0 / COUNT(*)
				ELSE 0
			END as success_rate,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens
		FROM request_logs
		WHERE %s
	`, whereSQL)

	var stats LogStatistics
	err := r.db.QueryRowContext(ctx, overallQuery, params...).Scan(
		&stats.TotalRequests, &stats.TotalCost, &stats.AvgLatency,
		&stats.SuccessRate, &stats.TotalInputTokens, &stats.TotalOutputTokens,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get overall statistics: %w", err)
	}

	// Round values
	stats.TotalCost = roundToPlaces(stats.TotalCost, 6)
	stats.AvgLatency = roundToPlaces(stats.AvgLatency, 2)
	stats.SuccessRate = roundToPlaces(stats.SuccessRate, 2)

	// By model statistics
	modelQuery := fmt.Sprintf(`
		SELECT
			model_name,
			COUNT(*) as requests,
			COALESCE(SUM(cost), 0) as cost,
			COALESCE(AVG(latency_ms), 0) as avg_latency,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens
		FROM request_logs
		WHERE %s
		GROUP BY model_name
		ORDER BY requests DESC
	`, whereSQL)

	modelRows, err := r.db.QueryContext(ctx, modelQuery, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get model statistics: %w", err)
	}
	defer modelRows.Close()

	for modelRows.Next() {
		var ms ModelStatistics
		if err := modelRows.Scan(&ms.ModelName, &ms.Requests, &ms.Cost, &ms.AvgLatency, &ms.InputTokens, &ms.OutputTokens); err != nil {
			return nil, fmt.Errorf("failed to scan model statistics: %w", err)
		}
		ms.Cost = roundToPlaces(ms.Cost, 6)
		ms.AvgLatency = roundToPlaces(ms.AvgLatency, 2)
		stats.ByModel = append(stats.ByModel, ms)
	}

	// By endpoint statistics
	endpointQuery := fmt.Sprintf(`
		SELECT
			endpoint_name,
			COUNT(*) as requests,
			COALESCE(SUM(cost), 0) as cost,
			COALESCE(AVG(latency_ms), 0) as avg_latency,
			CASE WHEN COUNT(*) > 0
				THEN SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) * 100.0 / COUNT(*)
				ELSE 0
			END as success_rate
		FROM request_logs
		WHERE %s
		GROUP BY endpoint_name
		ORDER BY requests DESC
	`, whereSQL)

	endpointRows, err := r.db.QueryContext(ctx, endpointQuery, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint statistics: %w", err)
	}
	defer endpointRows.Close()

	for endpointRows.Next() {
		var es EndpointStatistics
		if err := endpointRows.Scan(&es.EndpointName, &es.Requests, &es.Cost, &es.AvgLatency, &es.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint statistics: %w", err)
		}
		es.Cost = roundToPlaces(es.Cost, 6)
		es.AvgLatency = roundToPlaces(es.AvgLatency, 2)
		es.SuccessRate = roundToPlaces(es.SuccessRate, 2)
		stats.ByEndpoint = append(stats.ByEndpoint, es)
	}

	return &stats, nil
}

// Count counts logs matching the filters.
func (r *RequestLogRepositoryImpl) Count(
	ctx context.Context,
	modelName, endpointName *string,
	startTime, endTime *time.Time,
) (int64, error) {
	whereSQL, params := r.buildWhere(nil, modelName, endpointName, startTime, endTime, nil)

	var count int64
	query := fmt.Sprintf(`SELECT COUNT(*) FROM request_logs WHERE %s`, whereSQL)
	if err := r.db.QueryRowContext(ctx, query, params...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return count, nil
}

// Delete deletes logs matching the filters.
func (r *RequestLogRepositoryImpl) Delete(
	ctx context.Context,
	modelName, endpointName *string,
	startTime, endTime *time.Time,
) (int64, error) {
	whereSQL, params := r.buildWhere(nil, modelName, endpointName, startTime, endTime, nil)

	query := fmt.Sprintf(`DELETE FROM request_logs WHERE %s`, whereSQL)
	result, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete logs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		r.logger.Info("deleted request logs", zap.Int64("count", rowsAffected))
	}

	return rowsAffected, nil
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
		conditions = append(conditions, "datetime(request_logs.created_at) >= datetime(?)")
		params = append(params, startTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		conditions = append(conditions, "datetime(request_logs.created_at) <= datetime(?)")
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

	// New fields
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
	log.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)

	// Populate new fields
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

// GetByID retrieves a single request log by ID.
func (r *RequestLogRepositoryImpl) GetByID(ctx context.Context, id int64) (*models.RequestLog, error) {
	query := `
		SELECT
			request_logs.id, request_logs.request_id, request_logs.user_id,
			COALESCE(u.username, '未知用户') as username,
			request_logs.api_key_id, request_logs.model_name, request_logs.endpoint_name,
			request_logs.task_type, request_logs.input_tokens, request_logs.output_tokens,
			request_logs.latency_ms, request_logs.cost, request_logs.status_code,
			request_logs.success, request_logs.stream, request_logs.created_at,
			request_logs.message_preview, request_logs.request_content, request_logs.response_content,
			request_logs.routing_method, request_logs.routing_reason,
			request_logs.matched_rule_id, request_logs.matched_rule_name, request_logs.all_matches,
			request_logs.is_inaccurate
		FROM request_logs
		LEFT JOIN users u ON request_logs.user_id = u.id
		WHERE request_logs.id = ?
	`
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query log by id: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return r.scanLog(rows)
}

// MarkInaccurate marks or unmarks a request log as inaccurate.
func (r *RequestLogRepositoryImpl) MarkInaccurate(ctx context.Context, id int64, inaccurate bool) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE request_logs SET is_inaccurate = ? WHERE id = ?`,
		boolToInt(inaccurate), id)
	if err != nil {
		return fmt.Errorf("failed to mark log inaccurate: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}


// LogStatistics contains aggregated log statistics.
type LogStatistics struct {
	TotalRequests     int64                `json:"total_requests"`
	TotalCost         float64              `json:"total_cost"`
	AvgLatency        float64              `json:"avg_latency"`
	SuccessRate       float64              `json:"success_rate"`
	TotalInputTokens  int64                `json:"total_input_tokens"`
	TotalOutputTokens int64                `json:"total_output_tokens"`
	ByModel           []ModelStatistics    `json:"by_model"`
	ByEndpoint        []EndpointStatistics `json:"by_endpoint"`
}

// ModelStatistics contains per-model statistics.
type ModelStatistics struct {
	ModelName    string  `json:"model_name"`
	Requests     int64   `json:"requests"`
	Cost         float64 `json:"cost"`
	AvgLatency   float64 `json:"avg_latency"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// EndpointStatistics contains per-endpoint statistics.
type EndpointStatistics struct {
	EndpointName string  `json:"endpoint_name"`
	Requests     int64   `json:"requests"`
	Cost         float64 `json:"cost"`
	AvgLatency   float64 `json:"avg_latency"`
	SuccessRate  float64 `json:"success_rate"`
}

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
	if err := r.readDB.QueryRowContext(ctx, countQuery, params...).Scan(&total); err != nil {
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
			'' as message_preview, '' as request_content, '' as response_content,
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
	rows, err := r.readDB.QueryContext(ctx, query, params...)
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

// GetStatistics retrieves aggregated statistics. Queries run sequentially
// to stay compatible with single-connection SQLite (e.g. in-memory test DBs).
func (r *RequestLogRepositoryImpl) GetStatistics(
	ctx context.Context,
	startTime, endTime *time.Time,
	userID *int64,
	modelName, endpointName *string,
	success *bool,
) (*LogStatistics, error) {
	whereSQL, params := r.buildWhere(userID, modelName, endpointName, startTime, endTime, success)

	var stats LogStatistics

	// 1. Overall statistics
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
	if err := r.readDB.QueryRowContext(ctx, overallQuery, params...).Scan(
		&stats.TotalRequests, &stats.TotalCost, &stats.AvgLatency,
		&stats.SuccessRate, &stats.TotalInputTokens, &stats.TotalOutputTokens,
	); err != nil {
		return nil, fmt.Errorf("failed to get overall statistics: %w", err)
	}
	stats.TotalCost = roundToPlaces(stats.TotalCost, 6)
	stats.AvgLatency = roundToPlaces(stats.AvgLatency, 2)
	stats.SuccessRate = roundToPlaces(stats.SuccessRate, 2)

	// 2. By model + by endpoint in a single UNION ALL query
	unionQuery := fmt.Sprintf(`
		SELECT 'model' AS kind, model_name AS name,
			COUNT(*) AS requests, COALESCE(SUM(cost),0) AS cost,
			COALESCE(AVG(latency_ms),0) AS avg_latency,
			COALESCE(SUM(input_tokens),0) AS input_tokens,
			COALESCE(SUM(output_tokens),0) AS output_tokens,
			0 AS success_rate
		FROM request_logs WHERE %s GROUP BY model_name
		UNION ALL
		SELECT 'endpoint' AS kind, endpoint_name AS name,
			COUNT(*) AS requests, COALESCE(SUM(cost),0) AS cost,
			COALESCE(AVG(latency_ms),0) AS avg_latency,
			0 AS input_tokens, 0 AS output_tokens,
			CASE WHEN COUNT(*)>0
				THEN SUM(CASE WHEN success=1 THEN 1 ELSE 0 END)*100.0/COUNT(*)
				ELSE 0
			END AS success_rate
		FROM request_logs WHERE %s GROUP BY endpoint_name
	`, whereSQL, whereSQL)

	// params are used twice (once per sub-query)
	unionParams := make([]any, 0, len(params)*2)
	unionParams = append(unionParams, params...)
	unionParams = append(unionParams, params...)

	rows, err := r.readDB.QueryContext(ctx, unionQuery, unionParams...)
	if err != nil {
		return nil, fmt.Errorf("failed to get grouped statistics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind, name string
		var requests, inputTokens, outputTokens int64
		var cost, avgLatency, successRate float64
		if err := rows.Scan(&kind, &name, &requests, &cost, &avgLatency, &inputTokens, &outputTokens, &successRate); err != nil {
			return nil, fmt.Errorf("failed to scan grouped statistics: %w", err)
		}
		switch kind {
		case "model":
			stats.ByModel = append(stats.ByModel, ModelStatistics{
				ModelName:    name,
				Requests:     requests,
				Cost:         roundToPlaces(cost, 6),
				AvgLatency:   roundToPlaces(avgLatency, 2),
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
			})
		case "endpoint":
			stats.ByEndpoint = append(stats.ByEndpoint, EndpointStatistics{
				EndpointName: name,
				Requests:     requests,
				Cost:         roundToPlaces(cost, 6),
				AvgLatency:   roundToPlaces(avgLatency, 2),
				SuccessRate:  roundToPlaces(successRate, 2),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate grouped statistics: %w", err)
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
	if err := r.readDB.QueryRowContext(ctx, query, params...).Scan(&count); err != nil {
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
	log.CreatedAt = parseFlexibleTime(createdAt)

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
	rows, err := r.readDB.QueryContext(ctx, query, id)
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

// RoutingAggregation holds SQL-aggregated routing statistics.
type RoutingAggregation struct {
	TotalRequests   int64
	MethodCounts    map[string]int64
	RuleCounts      map[string]int64
	RuleIDs         map[string]*int64
	InaccurateCount int64
}

// GetRoutingAggregation returns routing method/rule counts via SQL aggregation.
func (r *RequestLogRepositoryImpl) GetRoutingAggregation(ctx context.Context, startTime, endTime *time.Time) (*RoutingAggregation, error) {
	whereSQL, params := r.buildWhere(nil, nil, nil, startTime, endTime, nil)

	// Total count
	var total int64
	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM request_logs WHERE %s`, whereSQL)
	if err := r.readDB.QueryRowContext(ctx, countQ, params...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count logs for routing aggregation: %w", err)
	}

	agg := &RoutingAggregation{
		TotalRequests: total,
		MethodCounts:  make(map[string]int64),
		RuleCounts:    make(map[string]int64),
		RuleIDs:       make(map[string]*int64),
	}

	// Aggregate by routing_method
	methodQ := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(routing_method,''), 'unknown') AS method, COUNT(*) AS cnt
		FROM request_logs WHERE %s GROUP BY method
	`, whereSQL)
	methodRows, err := r.readDB.QueryContext(ctx, methodQ, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate routing methods: %w", err)
	}
	defer methodRows.Close()
	for methodRows.Next() {
		var method string
		var cnt int64
		if err := methodRows.Scan(&method, &cnt); err != nil {
			return nil, fmt.Errorf("failed to scan routing method row: %w", err)
		}
		agg.MethodCounts[method] = cnt
	}

	// Aggregate by matched_rule_name (non-empty only)
	ruleQ := fmt.Sprintf(`
		SELECT matched_rule_name, MIN(matched_rule_id) AS rule_id, COUNT(*) AS cnt
		FROM request_logs
		WHERE %s AND matched_rule_name != ''
		GROUP BY matched_rule_name
	`, whereSQL)
	ruleRows, err := r.readDB.QueryContext(ctx, ruleQ, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate routing rules: %w", err)
	}
	defer ruleRows.Close()
	for ruleRows.Next() {
		var name string
		var ruleID sql.NullInt64
		var cnt int64
		if err := ruleRows.Scan(&name, &ruleID, &cnt); err != nil {
			return nil, fmt.Errorf("failed to scan routing rule row: %w", err)
		}
		agg.RuleCounts[name] = cnt
		if ruleID.Valid {
			id := ruleID.Int64
			agg.RuleIDs[name] = &id
		}
	}

	// Inaccurate count
	inaccQ := fmt.Sprintf(`SELECT COUNT(*) FROM request_logs WHERE %s AND is_inaccurate = 1`, whereSQL)
	if err := r.readDB.QueryRowContext(ctx, inaccQ, params...).Scan(&agg.InaccurateCount); err != nil {
		return nil, fmt.Errorf("failed to count inaccurate logs: %w", err)
	}

	return agg, nil
}

// ListInaccurate returns inaccurate logs with SQL-level filtering and pagination.
func (r *RequestLogRepositoryImpl) ListInaccurate(ctx context.Context, limit, offset int) ([]*models.RequestLog, int64, error) {
	// Count
	var total int64
	if err := r.readDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM request_logs WHERE is_inaccurate = 1`,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count inaccurate logs: %w", err)
	}

	query := `
		SELECT
			request_logs.id, request_logs.request_id, request_logs.user_id,
			COALESCE(u.username, '未知用户') as username,
			request_logs.api_key_id, request_logs.model_name, request_logs.endpoint_name,
			request_logs.task_type, request_logs.input_tokens, request_logs.output_tokens,
			request_logs.latency_ms, request_logs.cost, request_logs.status_code,
			request_logs.success, request_logs.stream, request_logs.created_at,
			'' as message_preview, '' as request_content, '' as response_content,
			request_logs.routing_method, request_logs.routing_reason,
			request_logs.matched_rule_id, request_logs.matched_rule_name, request_logs.all_matches,
			request_logs.is_inaccurate
		FROM request_logs
		LEFT JOIN users u ON request_logs.user_id = u.id
		WHERE request_logs.is_inaccurate = 1
		ORDER BY request_logs.created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.readDB.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query inaccurate logs: %w", err)
	}
	defer rows.Close()

	logs := make([]*models.RequestLog, 0)
	for rows.Next() {
		log, err := r.scanLog(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}
	return logs, total, rows.Err()
}

// ListForAnalysis returns logs with request_content for routing analysis.
func (r *RequestLogRepositoryImpl) ListForAnalysis(ctx context.Context, startTime, endTime *time.Time, maxResults int) ([]*models.RequestLog, error) {
	var conditions []string
	var params []any

	conditions = append(conditions, "request_logs.routing_method != ''")
	if startTime != nil {
		conditions = append(conditions, "request_logs.created_at >= ?")
		params = append(params, startTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		conditions = append(conditions, "request_logs.created_at <= ?")
		params = append(params, endTime.UTC().Format("2006-01-02 15:04:05"))
	}

	whereSQL := strings.Join(conditions, " AND ")
	params = append(params, maxResults)

	query := fmt.Sprintf(`
		SELECT
			request_logs.id, request_logs.request_id, request_logs.user_id,
			COALESCE(u.username, '') as username,
			request_logs.api_key_id, request_logs.model_name, request_logs.endpoint_name,
			request_logs.task_type, request_logs.input_tokens, request_logs.output_tokens,
			request_logs.latency_ms, request_logs.cost, request_logs.status_code,
			request_logs.success, request_logs.stream, request_logs.created_at,
			request_logs.message_preview, request_logs.request_content, '' as response_content,
			request_logs.routing_method, request_logs.routing_reason,
			request_logs.matched_rule_id, request_logs.matched_rule_name, request_logs.all_matches,
			request_logs.is_inaccurate
		FROM request_logs
		LEFT JOIN users u ON request_logs.user_id = u.id
		WHERE %s
		ORDER BY request_logs.is_inaccurate DESC, request_logs.created_at DESC
		LIMIT ?
	`, whereSQL)

	rows, err := r.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs for analysis: %w", err)
	}
	defer rows.Close()

	logs := make([]*models.RequestLog, 0)
	for rows.Next() {
		log, err := r.scanLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// EndpointModelStats contains historical per-endpoint-model statistics.
type EndpointModelStats struct {
	TotalRequests int64   `json:"total_requests"`
	TotalErrors   int64   `json:"total_errors"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// GetEndpointModelStats returns historical stats grouped by endpoint_name/model_name.
func (r *RequestLogRepositoryImpl) GetEndpointModelStats(ctx context.Context) (map[string]*EndpointModelStats, error) {
	query := `
		SELECT endpoint_name, model_name,
			COUNT(*) AS total_requests,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) AS total_errors,
			COALESCE(AVG(latency_ms), 0) AS avg_latency
		FROM request_logs
		GROUP BY endpoint_name, model_name
	`
	rows, err := r.readDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint model stats: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*EndpointModelStats)
	for rows.Next() {
		var epName, modelName string
		var stats EndpointModelStats
		if err := rows.Scan(&epName, &modelName, &stats.TotalRequests, &stats.TotalErrors, &stats.AvgLatencyMs); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint model stats: %w", err)
		}
		key := epName + "/" + modelName
		result[key] = &stats
	}
	return result, rows.Err()
}

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// GetRoutingAggregation returns routing method/rule counts via SQL aggregation.
func (r *RequestLogRepositoryImpl) GetRoutingAggregation(ctx context.Context, startTime, endTime *time.Time) (*RoutingAggregation, error) {
	whereSQL, params := r.buildWhere(nil, nil, nil, startTime, endTime, nil)

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

	methodQ := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(routing_method,''), 'direct') AS method, COUNT(*) AS cnt
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

	inaccQ := fmt.Sprintf(`SELECT COUNT(*) FROM request_logs WHERE %s AND is_inaccurate = 1`, whereSQL)
	if err := r.readDB.QueryRowContext(ctx, inaccQ, params...).Scan(&agg.InaccurateCount); err != nil {
		return nil, fmt.Errorf("failed to count inaccurate logs: %w", err)
	}

	return agg, nil
}

// ListInaccurate returns inaccurate logs with SQL-level filtering and pagination.
func (r *RequestLogRepositoryImpl) ListInaccurate(ctx context.Context, limit, offset int) ([]*models.RequestLog, int64, error) {
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
				COALESCE(request_logs.cache_creation_input_tokens, 0),
				COALESCE(request_logs.cache_read_input_tokens, 0),
				request_logs.message_preview, '' as request_content, '' as response_content,
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

	if startTime != nil {
		conditions = append(conditions, "request_logs.created_at >= ?")
		params = append(params, startTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		conditions = append(conditions, "request_logs.created_at <= ?")
		params = append(params, endTime.UTC().Format("2006-01-02 15:04:05"))
	}

	whereSQL := "1=1"
	if len(conditions) > 0 {
		whereSQL = strings.Join(conditions, " AND ")
	}
	params = append(params, maxResults)

	query := fmt.Sprintf(`
		SELECT
			request_logs.id, request_logs.request_id, request_logs.user_id,
			COALESCE(u.username, '') as username,
			request_logs.api_key_id, request_logs.model_name, request_logs.endpoint_name,
			request_logs.task_type, request_logs.input_tokens, request_logs.output_tokens,
			request_logs.latency_ms, request_logs.cost, request_logs.status_code,
			request_logs.success, request_logs.stream, request_logs.created_at,
			COALESCE(request_logs.cache_creation_input_tokens, 0),
			COALESCE(request_logs.cache_read_input_tokens, 0),
			request_logs.message_preview, request_logs.request_content, '' as response_content,
			COALESCE(request_logs.routing_method, '') as routing_method,
			COALESCE(request_logs.routing_reason, '') as routing_reason,
			request_logs.matched_rule_id,
			COALESCE(request_logs.matched_rule_name, '') as matched_rule_name,
			COALESCE(request_logs.all_matches, '') as all_matches,
			COALESCE(request_logs.is_inaccurate, 0) as is_inaccurate
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

// CountForAnalysis returns the total count of logs matching analysis criteria.
func (r *RequestLogRepositoryImpl) CountForAnalysis(ctx context.Context, startTime, endTime *time.Time) (int, error) {
	var conditions []string
	var params []any

	if startTime != nil {
		conditions = append(conditions, "created_at >= ?")
		params = append(params, startTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		conditions = append(conditions, "created_at <= ?")
		params = append(params, endTime.UTC().Format("2006-01-02 15:04:05"))
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}
	query := "SELECT COUNT(*) FROM request_logs WHERE " + whereClause
	var count int
	err := r.readDB.QueryRowContext(ctx, query, params...).Scan(&count)
	return count, err
}

// CountInaccurateForAnalysis returns inaccurate log count in the analysis time range.
func (r *RequestLogRepositoryImpl) CountInaccurateForAnalysis(ctx context.Context, startTime, endTime *time.Time) (int, error) {
	var conditions []string
	var params []any

	conditions = append(conditions, "is_inaccurate = 1")
	if startTime != nil {
		conditions = append(conditions, "created_at >= ?")
		params = append(params, startTime.UTC().Format("2006-01-02 15:04:05"))
	}
	if endTime != nil {
		conditions = append(conditions, "created_at <= ?")
		params = append(params, endTime.UTC().Format("2006-01-02 15:04:05"))
	}

	query := "SELECT COUNT(*) FROM request_logs WHERE " + strings.Join(conditions, " AND ")
	var count int
	err := r.readDB.QueryRowContext(ctx, query, params...).Scan(&count)
	return count, err
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

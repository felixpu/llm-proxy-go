package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

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

	var total int64
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM request_logs WHERE %s`, whereSQL)
	if err := r.readDB.QueryRowContext(ctx, countQuery, params...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT
			request_logs.id, request_logs.request_id, request_logs.user_id,
			COALESCE(u.username, '未知用户') as username,
			request_logs.api_key_id, request_logs.model_name, request_logs.endpoint_name,
			request_logs.task_type, request_logs.input_tokens, request_logs.output_tokens,
			request_logs.latency_ms, request_logs.cost, request_logs.status_code,
			request_logs.success, request_logs.stream, request_logs.created_at,
			COALESCE(request_logs.cache_creation_input_tokens, 0),
			COALESCE(request_logs.cache_read_input_tokens, 0),
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
	if err := r.getOverallStats(ctx, whereSQL, params, &stats); err != nil {
		return nil, err
	}
	if err := r.getGroupedStats(ctx, whereSQL, params, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *RequestLogRepositoryImpl) getOverallStats(ctx context.Context, whereSQL string, params []any, stats *LogStatistics) error {
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
		return fmt.Errorf("failed to get overall statistics: %w", err)
	}
	stats.TotalCost = roundToPlaces(stats.TotalCost, 6)
	stats.AvgLatency = roundToPlaces(stats.AvgLatency, 2)
	stats.SuccessRate = roundToPlaces(stats.SuccessRate, 2)
	return nil
}

func (r *RequestLogRepositoryImpl) getGroupedStats(ctx context.Context, whereSQL string, params []any, stats *LogStatistics) error {
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

	unionParams := make([]any, 0, len(params)*2)
	unionParams = append(unionParams, params...)
	unionParams = append(unionParams, params...)

	rows, err := r.readDB.QueryContext(ctx, unionQuery, unionParams...)
	if err != nil {
		return fmt.Errorf("failed to get grouped statistics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind, name string
		var requests, inputTokens, outputTokens int64
		var cost, avgLatency, successRate float64
		if err := rows.Scan(&kind, &name, &requests, &cost, &avgLatency, &inputTokens, &outputTokens, &successRate); err != nil {
			return fmt.Errorf("failed to scan grouped statistics: %w", err)
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
		return fmt.Errorf("failed to iterate grouped statistics: %w", err)
	}
	return nil
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
			COALESCE(request_logs.cache_creation_input_tokens, 0),
			COALESCE(request_logs.cache_read_input_tokens, 0),
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

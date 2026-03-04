package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

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
			cache_creation_input_tokens, cache_read_input_tokens,
			message_preview, request_content, response_content,
			routing_method, routing_reason,
			matched_rule_id, matched_rule_name, all_matches,
			is_inaccurate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.RequestID, entry.UserID, entry.APIKeyID, entry.ModelName, entry.EndpointName,
		entry.TaskType, entry.InputTokens, entry.OutputTokens, entry.LatencyMs, entry.Cost,
		entry.StatusCode, boolToInt(entry.Success), boolToInt(entry.Stream),
		entry.CacheCreationInputTokens, entry.CacheReadInputTokens,
		entry.MessagePreview, entry.RequestContent, entry.ResponseContent,
		entry.RoutingMethod, entry.RoutingReason,
		entry.MatchedRuleID, entry.MatchedRuleName, string(allMatchesJSON),
		boolToInt(entry.IsInaccurate), time.Now().UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("failed to insert request log: %w", err)
	}
	return result.LastInsertId()
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

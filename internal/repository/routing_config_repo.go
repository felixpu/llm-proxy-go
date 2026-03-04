package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// RoutingConfigRepository handles LLM routing config data access (single row, id=1).
type RoutingConfigRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// RoutingConfigPatch is a typed partial update payload for routing_llm_config.
type RoutingConfigPatch struct {
	Enabled                  *bool
	PrimaryModelID           *int64
	FallbackModelID          *int64
	TimeoutSeconds           *int
	CacheEnabled             *bool
	CacheTTLSeconds          *int
	CacheTTLL3Seconds        *int
	MaxTokens                *int
	Temperature              *float64
	RetryCount               *int
	SemanticCacheEnabled     *bool
	EmbeddingModelID         *int64
	SimilarityThreshold      *float64
	LocalEmbeddingModel      *string
	ForceSmartRouting        *bool
	RuleBasedRoutingEnabled  *bool
	RuleFallbackStrategy     *string
	RuleFallbackTaskType     *string
	RuleFallbackModelID      *int64
	CrossRoleFallbackEnabled *bool
	LogFullContent           *bool
}

// NewRoutingConfigRepository creates a new RoutingConfigRepository.
func NewRoutingConfigRepository(db *sql.DB, logger *zap.Logger) *RoutingConfigRepository {
	return &RoutingConfigRepository{db: db, logger: logger}
}

// boolFields lists the boolean fields in routing_llm_config.
var routingConfigBoolFields = map[string]bool{
	"enabled":                     true,
	"cache_enabled":               true,
	"semantic_cache_enabled":      true,
	"force_smart_routing":         true,
	"rule_based_routing_enabled":  true,
	"log_full_content":            true,
	"cross_role_fallback_enabled": true,
}

// validRoutingConfigColumns is the allowlist of valid column names for UpdateConfig.
// This prevents SQL injection via column names.
var validRoutingConfigColumns = map[string]bool{
	"enabled":                     true,
	"primary_model_id":            true,
	"fallback_model_id":           true,
	"timeout_seconds":             true,
	"cache_enabled":               true,
	"cache_ttl_seconds":           true,
	"cache_ttl_l3_seconds":        true,
	"max_tokens":                  true,
	"temperature":                 true,
	"retry_count":                 true,
	"semantic_cache_enabled":      true,
	"embedding_model_id":          true,
	"similarity_threshold":        true,
	"local_embedding_model":       true,
	"force_smart_routing":         true,
	"rule_based_routing_enabled":  true,
	"rule_fallback_strategy":      true,
	"rule_fallback_task_type":     true,
	"rule_fallback_model_id":      true,
	"cross_role_fallback_enabled": true,
	"log_full_content":            true,
}

// GetConfig retrieves the LLM routing configuration.
// Returns default config if no row exists.
func (r *RoutingConfigRepository) GetConfig(ctx context.Context) (*models.RoutingConfig, error) {
	var cfg models.RoutingConfig
	var primaryModelID, fallbackModelID, embeddingModelID sql.NullInt64
	var cacheTTLL3 sql.NullInt64
	var semanticEnabled sql.NullInt64
	var similarityThreshold sql.NullFloat64
	var localEmbeddingModel sql.NullString
	var forceSmartRouting sql.NullInt64
	var enabled, cacheEnabled int

	// Rule-based routing fields
	var ruleBasedEnabled sql.NullInt64
	var ruleFallbackStrategy sql.NullString
	var ruleFallbackTaskType sql.NullString
	var ruleFallbackModelID sql.NullInt64

	// Cross-role fallback
	var crossRoleFallbackEnabled sql.NullInt64

	// Logging fields
	var logFullContent sql.NullInt64

	err := r.db.QueryRowContext(ctx, `
		SELECT enabled, primary_model_id, fallback_model_id, timeout_seconds,
			cache_enabled, cache_ttl_seconds, cache_ttl_l3_seconds, max_tokens,
			temperature, retry_count, semantic_cache_enabled, embedding_model_id,
			similarity_threshold, local_embedding_model, force_smart_routing,
			rule_based_routing_enabled, rule_fallback_strategy, rule_fallback_task_type,
			rule_fallback_model_id, cross_role_fallback_enabled, log_full_content
		FROM routing_llm_config
		WHERE id = 1
	`).Scan(
		&enabled, &primaryModelID, &fallbackModelID, &cfg.TimeoutSeconds,
		&cacheEnabled, &cfg.CacheTTLSeconds, &cacheTTLL3, &cfg.MaxTokens,
		&cfg.Temperature, &cfg.RetryCount, &semanticEnabled, &embeddingModelID,
		&similarityThreshold, &localEmbeddingModel, &forceSmartRouting,
		&ruleBasedEnabled, &ruleFallbackStrategy, &ruleFallbackTaskType,
		&ruleFallbackModelID, &crossRoleFallbackEnabled, &logFullContent,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.Debug("no routing config found, using defaults")
			return models.DefaultRoutingConfig(), nil
		}
		return nil, fmt.Errorf("failed to get routing config: %w", err)
	}

	cfg.Enabled = enabled == 1
	cfg.CacheEnabled = cacheEnabled == 1

	if primaryModelID.Valid {
		cfg.PrimaryModelID = &primaryModelID.Int64
	}
	if fallbackModelID.Valid {
		cfg.FallbackModelID = &fallbackModelID.Int64
	}
	if embeddingModelID.Valid {
		cfg.EmbeddingModelID = &embeddingModelID.Int64
	}

	// Apply defaults for nullable fields
	defaults := models.DefaultRoutingConfig()

	if cacheTTLL3.Valid {
		cfg.CacheTTLL3Seconds = int(cacheTTLL3.Int64)
	} else {
		cfg.CacheTTLL3Seconds = defaults.CacheTTLL3Seconds
	}
	if semanticEnabled.Valid {
		cfg.SemanticCacheEnabled = semanticEnabled.Int64 == 1
	} else {
		cfg.SemanticCacheEnabled = defaults.SemanticCacheEnabled
	}
	if similarityThreshold.Valid {
		cfg.SimilarityThreshold = similarityThreshold.Float64
	} else {
		cfg.SimilarityThreshold = defaults.SimilarityThreshold
	}
	if localEmbeddingModel.Valid && localEmbeddingModel.String != "" {
		cfg.LocalEmbeddingModel = localEmbeddingModel.String
	} else {
		cfg.LocalEmbeddingModel = defaults.LocalEmbeddingModel
	}
	if forceSmartRouting.Valid {
		cfg.ForceSmartRouting = forceSmartRouting.Int64 == 1
	} else {
		cfg.ForceSmartRouting = defaults.ForceSmartRouting
	}

	// Rule-based routing fields
	if ruleBasedEnabled.Valid {
		cfg.RuleBasedRoutingEnabled = ruleBasedEnabled.Int64 == 1
	} else {
		cfg.RuleBasedRoutingEnabled = defaults.RuleBasedRoutingEnabled
	}
	if ruleFallbackStrategy.Valid && ruleFallbackStrategy.String != "" {
		cfg.RuleFallbackStrategy = models.FallbackStrategy(ruleFallbackStrategy.String)
	} else {
		cfg.RuleFallbackStrategy = defaults.RuleFallbackStrategy
	}
	if ruleFallbackTaskType.Valid && ruleFallbackTaskType.String != "" {
		cfg.RuleFallbackTaskType = ruleFallbackTaskType.String
	} else {
		cfg.RuleFallbackTaskType = defaults.RuleFallbackTaskType
	}
	if ruleFallbackModelID.Valid {
		cfg.RuleFallbackModelID = &ruleFallbackModelID.Int64
	}

	// Cross-role fallback
	if crossRoleFallbackEnabled.Valid {
		cfg.CrossRoleFallbackEnabled = crossRoleFallbackEnabled.Int64 == 1
	} else {
		cfg.CrossRoleFallbackEnabled = defaults.CrossRoleFallbackEnabled
	}

	// Logging fields
	if logFullContent.Valid {
		cfg.LogFullContent = logFullContent.Int64 == 1
	} else {
		cfg.LogFullContent = defaults.LogFullContent
	}

	return &cfg, nil
}

// UpdateConfig dynamically updates routing configuration fields.
func (r *RoutingConfigRepository) updateWithMap(ctx context.Context, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(updates))
	params := make([]any, 0, len(updates))

	for field, value := range updates {
		// Validate column name against allowlist to prevent SQL injection
		if !validRoutingConfigColumns[field] {
			return fmt.Errorf("invalid column name: %q", field)
		}

		// Convert bool to int for SQLite
		if routingConfigBoolFields[field] {
			if b, ok := value.(bool); ok {
				value = boolToInt(b)
			}
		}
		// Handle nullable model IDs: 0 or negative means NULL
		if field == "primary_model_id" || field == "fallback_model_id" || field == "embedding_model_id" {
			if id, ok := value.(int64); ok && id <= 0 {
				value = nil
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", field))
		params = append(params, value)
	}

	query := fmt.Sprintf("UPDATE routing_llm_config SET %s WHERE id = 1",
		joinStrings(setClauses, ", "))

	result, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to update routing config: %w", err)
	}

	rows, _ := result.RowsAffected()
	r.logger.Debug("routing config updated", zap.Int64("rows_affected", rows))
	return nil
}

// UpdateConfigPatch updates routing config using a typed patch payload.
func (r *RoutingConfigRepository) UpdateConfigPatch(ctx context.Context, patch RoutingConfigPatch) error {
	updates := make(map[string]any)
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if patch.PrimaryModelID != nil {
		updates["primary_model_id"] = *patch.PrimaryModelID
	}
	if patch.FallbackModelID != nil {
		updates["fallback_model_id"] = *patch.FallbackModelID
	}
	if patch.TimeoutSeconds != nil {
		updates["timeout_seconds"] = *patch.TimeoutSeconds
	}
	if patch.CacheEnabled != nil {
		updates["cache_enabled"] = *patch.CacheEnabled
	}
	if patch.CacheTTLSeconds != nil {
		updates["cache_ttl_seconds"] = *patch.CacheTTLSeconds
	}
	if patch.CacheTTLL3Seconds != nil {
		updates["cache_ttl_l3_seconds"] = *patch.CacheTTLL3Seconds
	}
	if patch.MaxTokens != nil {
		updates["max_tokens"] = *patch.MaxTokens
	}
	if patch.Temperature != nil {
		updates["temperature"] = *patch.Temperature
	}
	if patch.RetryCount != nil {
		updates["retry_count"] = *patch.RetryCount
	}
	if patch.SemanticCacheEnabled != nil {
		updates["semantic_cache_enabled"] = *patch.SemanticCacheEnabled
	}
	if patch.EmbeddingModelID != nil {
		updates["embedding_model_id"] = *patch.EmbeddingModelID
	}
	if patch.SimilarityThreshold != nil {
		updates["similarity_threshold"] = *patch.SimilarityThreshold
	}
	if patch.LocalEmbeddingModel != nil {
		updates["local_embedding_model"] = *patch.LocalEmbeddingModel
	}
	if patch.ForceSmartRouting != nil {
		updates["force_smart_routing"] = *patch.ForceSmartRouting
	}
	if patch.RuleBasedRoutingEnabled != nil {
		updates["rule_based_routing_enabled"] = *patch.RuleBasedRoutingEnabled
	}
	if patch.RuleFallbackStrategy != nil {
		updates["rule_fallback_strategy"] = *patch.RuleFallbackStrategy
	}
	if patch.RuleFallbackTaskType != nil {
		updates["rule_fallback_task_type"] = *patch.RuleFallbackTaskType
	}
	if patch.RuleFallbackModelID != nil {
		updates["rule_fallback_model_id"] = *patch.RuleFallbackModelID
	}
	if patch.CrossRoleFallbackEnabled != nil {
		updates["cross_role_fallback_enabled"] = *patch.CrossRoleFallbackEnabled
	}
	if patch.LogFullContent != nil {
		updates["log_full_content"] = *patch.LogFullContent
	}
	return r.updateWithMap(ctx, updates)
}

// joinStrings joins strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	n := len(sep) * (len(strs) - 1)
	for _, s := range strs {
		n += len(s)
	}
	buf := make([]byte, 0, n)
	buf = append(buf, strs[0]...)
	for _, s := range strs[1:] {
		buf = append(buf, sep...)
		buf = append(buf, s...)
	}
	return string(buf)
}

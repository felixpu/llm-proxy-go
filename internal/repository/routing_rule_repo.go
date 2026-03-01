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

// RoutingRuleRepo handles routing rule data access.
type RoutingRuleRepo struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewRoutingRuleRepository creates a new RoutingRuleRepo.
func NewRoutingRuleRepository(db *sql.DB, logger *zap.Logger) *RoutingRuleRepo {
	return &RoutingRuleRepo{db: db, logger: logger}
}

// ListRules retrieves routing rules, optionally filtered by enabled status.
// Results are sorted by priority DESC.
func (r *RoutingRuleRepo) ListRules(ctx context.Context, enabledOnly bool) ([]*models.RoutingRule, error) {
	var query string
	var args []any

	if enabledOnly {
		query = `SELECT id, name, description, keywords, pattern, condition, task_type,
			priority, is_builtin, enabled, hit_count, created_at, updated_at
			FROM routing_rules WHERE enabled = 1 ORDER BY priority DESC, id`
	} else {
		query = `SELECT id, name, description, keywords, pattern, condition, task_type,
			priority, is_builtin, enabled, hit_count, created_at, updated_at
			FROM routing_rules ORDER BY priority DESC, id`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list routing rules: %w", err)
	}
	defer rows.Close()

	var result []*models.RoutingRule
	for rows.Next() {
		rule, err := r.scanRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

// GetRule retrieves a single routing rule by ID.
func (r *RoutingRuleRepo) GetRule(ctx context.Context, id int64) (*models.RoutingRule, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, keywords, pattern, condition, task_type,
			priority, is_builtin, enabled, hit_count, created_at, updated_at
		FROM routing_rules WHERE id = ?
	`, id)

	rule, err := r.scanRuleRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get routing rule: %w", err)
	}
	return rule, nil
}

// AddRule inserts a new routing rule.
func (r *RoutingRuleRepo) AddRule(ctx context.Context, rule *models.RoutingRule) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	keywordsJSON, err := json.Marshal(rule.Keywords)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal keywords: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO routing_rules (name, description, keywords, pattern, condition,
			task_type, priority, is_builtin, enabled, hit_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
	`, rule.Name, rule.Description, string(keywordsJSON), rule.Pattern, rule.Condition,
		rule.TaskType, rule.Priority, boolToInt(rule.IsBuiltin), boolToInt(rule.Enabled), now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to add routing rule: %w", err)
	}
	return result.LastInsertId()
}

// UpdateRule dynamically updates a routing rule.
func (r *RoutingRuleRepo) UpdateRule(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(updates)+1)
	params := make([]any, 0, len(updates)+2)

	for field, value := range updates {
		switch field {
		case "enabled", "is_builtin":
			if b, ok := value.(bool); ok {
				value = boolToInt(b)
			}
		case "keywords":
			if kw, ok := value.([]string); ok {
				j, err := json.Marshal(kw)
				if err != nil {
					return fmt.Errorf("failed to marshal keywords: %w", err)
				}
				value = string(j)
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", field))
		params = append(params, value)
	}

	setClauses = append(setClauses, "updated_at = ?")
	params = append(params, time.Now().UTC().Format("2006-01-02 15:04:05"))
	params = append(params, id)

	query := fmt.Sprintf("UPDATE routing_rules SET %s WHERE id = ?",
		strings.Join(setClauses, ", "))

	_, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to update routing rule: %w", err)
	}
	return nil
}

// DeleteRule deletes a routing rule by ID.
func (r *RoutingRuleRepo) DeleteRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM routing_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete routing rule: %w", err)
	}
	return nil
}

// IncrementHitCount atomically increments the hit count for a rule.
func (r *RoutingRuleRepo) IncrementHitCount(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE routing_rules SET hit_count = hit_count + 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to increment hit count: %w", err)
	}
	return nil
}

// GetStats retrieves routing rule statistics.
func (r *RoutingRuleRepo) GetStats(ctx context.Context) (*models.RuleStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, hit_count FROM routing_rules ORDER BY hit_count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get rule stats: %w", err)
	}
	defer rows.Close()

	var totalRequests int64
	ruleHits := make(map[int64]models.HitStat)

	for rows.Next() {
		var id int64
		var name string
		var hitCount int64
		if err := rows.Scan(&id, &name, &hitCount); err != nil {
			return nil, fmt.Errorf("failed to scan rule stats: %w", err)
		}
		totalRequests += hitCount
		ruleHits[id] = models.HitStat{
			Name:  name,
			Count: hitCount,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Calculate percentages
	if totalRequests > 0 {
		for id, stat := range ruleHits {
			stat.Percentage = float64(stat.Count) / float64(totalRequests) * 100
			ruleHits[id] = stat
		}
	}

	return &models.RuleStats{
		TotalRequests: totalRequests,
		RuleHits:      ruleHits,
	}, nil
}

// ListBuiltinRules retrieves only builtin routing rules.
func (r *RoutingRuleRepo) ListBuiltinRules(ctx context.Context) ([]*models.RoutingRule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, keywords, pattern, condition, task_type,
			priority, is_builtin, enabled, hit_count, created_at, updated_at
		FROM routing_rules WHERE is_builtin = 1 ORDER BY priority DESC, id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list builtin rules: %w", err)
	}
	defer rows.Close()

	var result []*models.RoutingRule
	for rows.Next() {
		rule, err := r.scanRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

// ListCustomRules retrieves only custom (non-builtin) routing rules.
func (r *RoutingRuleRepo) ListCustomRules(ctx context.Context) ([]*models.RoutingRule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, keywords, pattern, condition, task_type,
			priority, is_builtin, enabled, hit_count, created_at, updated_at
		FROM routing_rules WHERE is_builtin = 0 ORDER BY priority DESC, id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom rules: %w", err)
	}
	defer rows.Close()

	var result []*models.RoutingRule
	for rows.Next() {
		rule, err := r.scanRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

// scanRule scans a routing rule from sql.Rows.
func (r *RoutingRuleRepo) scanRule(rows *sql.Rows) (*models.RoutingRule, error) {
	var rule models.RoutingRule
	var keywordsJSON string
	var isBuiltin, enabled int
	var createdAt, updatedAt string

	err := rows.Scan(
		&rule.ID, &rule.Name, &rule.Description, &keywordsJSON,
		&rule.Pattern, &rule.Condition, &rule.TaskType,
		&rule.Priority, &isBuiltin, &enabled, &rule.HitCount,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan routing rule: %w", err)
	}

	rule.IsBuiltin = isBuiltin == 1
	rule.Enabled = enabled == 1
	rule.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	rule.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	if err := json.Unmarshal([]byte(keywordsJSON), &rule.Keywords); err != nil {
		rule.Keywords = []string{}
	}

	return &rule, nil
}

// scanRuleRow scans a routing rule from sql.Row.
func (r *RoutingRuleRepo) scanRuleRow(row *sql.Row) (*models.RoutingRule, error) {
	var rule models.RoutingRule
	var keywordsJSON string
	var isBuiltin, enabled int
	var createdAt, updatedAt string

	err := row.Scan(
		&rule.ID, &rule.Name, &rule.Description, &keywordsJSON,
		&rule.Pattern, &rule.Condition, &rule.TaskType,
		&rule.Priority, &isBuiltin, &enabled, &rule.HitCount,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	rule.IsBuiltin = isBuiltin == 1
	rule.Enabled = enabled == 1
	rule.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	rule.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	if err := json.Unmarshal([]byte(keywordsJSON), &rule.Keywords); err != nil {
		rule.Keywords = []string{}
	}

	return &rule, nil
}

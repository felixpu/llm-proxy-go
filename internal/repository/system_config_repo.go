package repository

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// SystemConfigRepository handles system configuration data access.
// Operates on routing_config, load_balance_config, health_check_config, ui_config tables.
type SystemConfigRepository struct {
	db     *sql.DB
	readDB *sql.DB
}

type SystemRoutingConfigPatch struct {
	DefaultRole *string
}

type SystemLoadBalanceConfigPatch struct {
	Strategy *string
}

type SystemHealthCheckConfigPatch struct {
	Enabled         *bool
	IntervalSeconds *int
	TimeoutSeconds  *int
}

type SystemUIConfigPatch struct {
	DashboardRefreshSeconds *int
	LogsRefreshSeconds      *int
}

// NewSystemConfigRepository creates a new SystemConfigRepository.
func NewSystemConfigRepository(db *sql.DB, readDB ...*sql.DB) *SystemConfigRepository {
	repo := &SystemConfigRepository{db: db, readDB: db}
	if len(readDB) > 0 && readDB[0] != nil {
		repo.readDB = readDB[0]
	}
	return repo
}

// GetRoutingConfig retrieves the routing configuration (single row, id=1).
func (r *SystemConfigRepository) GetRoutingConfig(ctx context.Context) (map[string]any, error) {
	return r.getConfig(ctx, "routing_config")
}

// UpdateRoutingConfigPatch updates the routing configuration.
func (r *SystemConfigRepository) UpdateRoutingConfigPatch(ctx context.Context, patch SystemRoutingConfigPatch) error {
	updates := make(map[string]any)
	if patch.DefaultRole != nil {
		updates["default_role"] = *patch.DefaultRole
	}
	return r.updateConfig(ctx, "routing_config", updates)
}

// GetLoadBalanceConfig retrieves the load balance configuration.
func (r *SystemConfigRepository) GetLoadBalanceConfig(ctx context.Context) (map[string]any, error) {
	return r.getConfig(ctx, "load_balance_config")
}

// UpdateLoadBalanceConfigPatch updates the load balance configuration.
func (r *SystemConfigRepository) UpdateLoadBalanceConfigPatch(ctx context.Context, patch SystemLoadBalanceConfigPatch) error {
	updates := make(map[string]any)
	if patch.Strategy != nil {
		updates["strategy"] = *patch.Strategy
	}
	return r.updateConfig(ctx, "load_balance_config", updates)
}

// GetHealthCheckConfig retrieves the health check configuration.
func (r *SystemConfigRepository) GetHealthCheckConfig(ctx context.Context) (map[string]any, error) {
	return r.getConfig(ctx, "health_check_config")
}

// UpdateHealthCheckConfigPatch updates the health check configuration.
func (r *SystemConfigRepository) UpdateHealthCheckConfigPatch(ctx context.Context, patch SystemHealthCheckConfigPatch) error {
	updates := make(map[string]any)
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if patch.IntervalSeconds != nil {
		updates["interval_seconds"] = *patch.IntervalSeconds
	}
	if patch.TimeoutSeconds != nil {
		updates["timeout_seconds"] = *patch.TimeoutSeconds
	}
	return r.updateConfig(ctx, "health_check_config", updates)
}

// GetUIConfig retrieves the UI configuration.
func (r *SystemConfigRepository) GetUIConfig(ctx context.Context) (map[string]any, error) {
	return r.getConfig(ctx, "ui_config")
}

// UpdateUIConfigPatch updates the UI configuration.
func (r *SystemConfigRepository) UpdateUIConfigPatch(ctx context.Context, patch SystemUIConfigPatch) error {
	updates := make(map[string]any)
	if patch.DashboardRefreshSeconds != nil {
		updates["dashboard_refresh_seconds"] = *patch.DashboardRefreshSeconds
	}
	if patch.LogsRefreshSeconds != nil {
		updates["logs_refresh_seconds"] = *patch.LogsRefreshSeconds
	}
	return r.updateConfig(ctx, "ui_config", updates)
}

// getConfig reads a single-row config table and returns all columns as a map.
func (r *SystemConfigRepository) getConfig(ctx context.Context, table string) (map[string]any, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = 1", table)
	rows, err := r.readDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s: %w", table, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns for %s: %w", table, err)
	}

	if !rows.Next() {
		// Return empty config if no row exists
		return map[string]any{}, nil
	}

	values := make([]any, len(cols))
	valuePtrs := make([]any, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("failed to scan %s: %w", table, err)
	}

	result := make(map[string]any, len(cols))
	for i, col := range cols {
		if col == "id" {
			continue
		}
		val := values[i]
		// Convert []byte to string for SQLite text fields
		if b, ok := val.([]byte); ok {
			result[col] = string(b)
		} else {
			result[col] = val
		}
	}
	return result, nil
}

// validColumnName matches safe SQL column identifiers (letters, digits, underscores).
var validColumnName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// updateConfig dynamically updates fields in a single-row config table.
func (r *SystemConfigRepository) updateConfig(ctx context.Context, table string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(updates))
	params := make([]any, 0, len(updates))
	for field, value := range updates {
		// Validate column name to prevent SQL injection
		if !validColumnName.MatchString(field) {
			return fmt.Errorf("invalid column name: %q", field)
		}
		setClauses = append(setClauses, field+" = ?")
		// Convert bool to int for SQLite INTEGER columns
		if b, ok := value.(bool); ok {
			if b {
				params = append(params, 1)
			} else {
				params = append(params, 0)
			}
		} else {
			params = append(params, value)
		}
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = 1", table, strings.Join(setClauses, ", "))
	_, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", table, err)
	}
	return nil
}

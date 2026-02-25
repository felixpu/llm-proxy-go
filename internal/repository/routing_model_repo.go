package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// RoutingModelRepository handles routing model data access.
type RoutingModelRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewRoutingModelRepository creates a new RoutingModelRepository.
func NewRoutingModelRepository(db *sql.DB, logger *zap.Logger) *RoutingModelRepository {
	return &RoutingModelRepository{db: db, logger: logger}
}

// ListModels retrieves routing models, optionally filtered by provider_id.
func (r *RoutingModelRepository) ListModels(ctx context.Context, providerID *int64) ([]*models.RoutingModel, error) {
	var query string
	var args []any

	if providerID != nil {
		query = `SELECT id, provider_id, model_name, enabled, priority,
			cost_per_mtok_input, cost_per_mtok_output, billing_multiplier,
			description, created_at, updated_at
			FROM routing_models WHERE provider_id = ? ORDER BY priority DESC, id`
		args = append(args, *providerID)
	} else {
		query = `SELECT id, provider_id, model_name, enabled, priority,
			cost_per_mtok_input, cost_per_mtok_output, billing_multiplier,
			description, created_at, updated_at
			FROM routing_models ORDER BY priority DESC, id`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list routing models: %w", err)
	}
	defer rows.Close()

	var result []*models.RoutingModel
	for rows.Next() {
		m, err := r.scanModel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetModel retrieves a single routing model by ID.
func (r *RoutingModelRepository) GetModel(ctx context.Context, id int64) (*models.RoutingModel, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, provider_id, model_name, enabled, priority,
			cost_per_mtok_input, cost_per_mtok_output, billing_multiplier,
			description, created_at, updated_at
		FROM routing_models WHERE id = ?
	`, id)

	m, err := r.scanModelRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get routing model: %w", err)
	}
	return m, nil
}

// GetModelWithProvider retrieves a routing model with its provider info.
func (r *RoutingModelRepository) GetModelWithProvider(ctx context.Context, id int64) (*models.RoutingModelWithProvider, error) {
	var m models.RoutingModelWithProvider
	var enabled int
	var description sql.NullString
	var createdAt, updatedAt string

	err := r.db.QueryRowContext(ctx, `
		SELECT rm.id, rm.provider_id, rm.model_name, rm.enabled, rm.priority,
			rm.cost_per_mtok_input, rm.cost_per_mtok_output, rm.billing_multiplier,
			rm.description, rm.created_at, rm.updated_at,
			p.base_url, p.api_key
		FROM routing_models rm
		JOIN providers p ON rm.provider_id = p.id
		WHERE rm.id = ? AND rm.enabled = 1 AND p.enabled = 1
	`, id).Scan(
		&m.ID, &m.ProviderID, &m.ModelName, &enabled, &m.Priority,
		&m.CostPerMtokInput, &m.CostPerMtokOutput, &m.BillingMultiplier,
		&description, &createdAt, &updatedAt,
		&m.BaseURL, &m.APIKey,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get routing model with provider: %w", err)
	}

	m.Enabled = enabled == 1
	if description.Valid {
		m.Description = description.String
	}
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	m.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &m, nil
}

// AddModel inserts a new routing model.
func (r *RoutingModelRepository) AddModel(ctx context.Context, m *models.RoutingModel) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO routing_models (provider_id, model_name, enabled, priority,
			cost_per_mtok_input, cost_per_mtok_output, billing_multiplier,
			description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.ProviderID, m.ModelName, boolToInt(m.Enabled), m.Priority,
		m.CostPerMtokInput, m.CostPerMtokOutput, m.BillingMultiplier,
		m.Description, now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to add routing model: %w", err)
	}
	return result.LastInsertId()
}

// UpdateModel dynamically updates a routing model.
func (r *RoutingModelRepository) UpdateModel(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(updates)+1)
	params := make([]any, 0, len(updates)+2)

	for field, value := range updates {
		if field == "enabled" {
			if b, ok := value.(bool); ok {
				value = boolToInt(b)
			}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", field))
		params = append(params, value)
	}

	setClauses = append(setClauses, "updated_at = ?")
	params = append(params, time.Now().UTC().Format("2006-01-02 15:04:05"))
	params = append(params, id)

	query := fmt.Sprintf("UPDATE routing_models SET %s WHERE id = ?",
		strings.Join(setClauses, ", "))

	_, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to update routing model: %w", err)
	}
	return nil
}

// DeleteModel deletes a routing model by ID.
func (r *RoutingModelRepository) DeleteModel(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM routing_models WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete routing model: %w", err)
	}
	return nil
}

func (r *RoutingModelRepository) scanModel(rows *sql.Rows) (*models.RoutingModel, error) {
	var m models.RoutingModel
	var enabled int
	var description sql.NullString
	var createdAt, updatedAt string

	err := rows.Scan(
		&m.ID, &m.ProviderID, &m.ModelName, &enabled, &m.Priority,
		&m.CostPerMtokInput, &m.CostPerMtokOutput, &m.BillingMultiplier,
		&description, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan routing model: %w", err)
	}

	m.Enabled = enabled == 1
	if description.Valid {
		m.Description = description.String
	}
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	m.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &m, nil
}

func (r *RoutingModelRepository) scanModelRow(row *sql.Row) (*models.RoutingModel, error) {
	var m models.RoutingModel
	var enabled int
	var description sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&m.ID, &m.ProviderID, &m.ModelName, &enabled, &m.Priority,
		&m.CostPerMtokInput, &m.CostPerMtokOutput, &m.BillingMultiplier,
		&description, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	m.Enabled = enabled == 1
	if description.Valid {
		m.Description = description.String
	}
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	m.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &m, nil
}

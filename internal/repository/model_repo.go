package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// SQLModelRepository implements ModelRepository using database/sql.
type SQLModelRepository struct {
	db *sql.DB
}

// NewModelRepository creates a new SQLModelRepository.
func NewModelRepository(db *sql.DB) *SQLModelRepository {
	return &SQLModelRepository{db: db}
}

func (r *SQLModelRepository) FindByID(ctx context.Context, id int64) (*models.Model, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, role, cost_per_mtok_input, cost_per_mtok_output,
		        billing_multiplier, supports_thinking, enabled, weight, created_at
		 FROM models WHERE id = ?`, id)
	return scanModel(row)
}

func (r *SQLModelRepository) FindByName(ctx context.Context, name string) (*models.Model, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, role, cost_per_mtok_input, cost_per_mtok_output,
		        billing_multiplier, supports_thinking, enabled, weight, created_at
		 FROM models WHERE name = ?`, name)
	return scanModel(row)
}

func (r *SQLModelRepository) FindByRole(ctx context.Context, role models.ModelRole) ([]*models.Model, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, cost_per_mtok_input, cost_per_mtok_output,
		        billing_multiplier, supports_thinking, enabled, weight, created_at
		 FROM models WHERE role = ? AND enabled = 1 ORDER BY weight DESC`, string(role))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModels(rows)
}

func (r *SQLModelRepository) FindAllEnabled(ctx context.Context) ([]*models.Model, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, cost_per_mtok_input, cost_per_mtok_output,
		        billing_multiplier, supports_thinking, enabled, weight, created_at
		 FROM models WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModels(rows)
}

// scanner is an interface for sql.Row and sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanModel(s scanner) (*models.Model, error) {
	var m models.Model
	var role string
	var supportsThinking, enabled int
	var createdAt sql.NullTime

	err := s.Scan(
		&m.ID, &m.Name, &role,
		&m.CostPerMtokInput, &m.CostPerMtokOutput,
		&m.BillingMultiplier, &supportsThinking, &enabled,
		&m.Weight, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	m.Role = models.ModelRole(role)
	m.SupportsThinking = supportsThinking == 1
	m.Enabled = enabled == 1
	if createdAt.Valid {
		m.CreatedAt = createdAt.Time
	} else {
		m.CreatedAt = time.Now()
	}
	return &m, nil
}

func scanModels(rows *sql.Rows) ([]*models.Model, error) {
	var result []*models.Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *SQLModelRepository) FindAll(ctx context.Context) ([]*models.Model, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, cost_per_mtok_input, cost_per_mtok_output,
		        billing_multiplier, supports_thinking, enabled, weight, created_at
		 FROM models ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModels(rows)
}

func (r *SQLModelRepository) Insert(ctx context.Context, m *models.Model) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO models (name, role, cost_per_mtok_input, cost_per_mtok_output,
		        billing_multiplier, supports_thinking, enabled, weight, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		m.Name, string(m.Role), m.CostPerMtokInput, m.CostPerMtokOutput,
		m.BillingMultiplier, boolToInt(m.SupportsThinking), boolToInt(m.Enabled), m.Weight)
	if err != nil {
		return 0, fmt.Errorf("failed to insert model: %w", err)
	}
	return result.LastInsertId()
}

func (r *SQLModelRepository) Update(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(updates))
	params := make([]any, 0, len(updates)+1)
	for field, value := range updates {
		if field == "enabled" || field == "supports_thinking" {
			if b, ok := value.(bool); ok {
				value = boolToInt(b)
			}
		}
		setClauses = append(setClauses, field+" = ?")
		params = append(params, value)
	}
	params = append(params, id)
	query := fmt.Sprintf("UPDATE models SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	_, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}
	return nil
}

func (r *SQLModelRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM models WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}
	return nil
}

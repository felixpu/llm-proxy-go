package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// SQLProviderRepository implements ProviderRepository using database/sql.
type SQLProviderRepository struct {
	db *sql.DB
}

// NewProviderRepository creates a new SQLProviderRepository.
func NewProviderRepository(db *sql.DB) *SQLProviderRepository {
	return &SQLProviderRepository{db: db}
}

func (r *SQLProviderRepository) FindByID(ctx context.Context, id int64) (*models.Provider, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, base_url, api_key, weight, max_concurrent,
		        enabled, description, custom_headers, created_at, updated_at
		 FROM providers WHERE id = ?`, id)
	return scanProvider(row)
}

func (r *SQLProviderRepository) FindByModelID(ctx context.Context, modelID int64) ([]*models.Provider, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.name, p.base_url, p.api_key, p.weight, p.max_concurrent,
		        p.enabled, p.description, p.custom_headers, p.created_at, p.updated_at
		 FROM providers p
		 JOIN provider_models pm ON p.id = pm.provider_id
		 WHERE pm.model_id = ? AND p.enabled = 1
		 ORDER BY p.weight DESC`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProviders(rows)
}

func (r *SQLProviderRepository) FindAllEnabled(ctx context.Context) ([]*models.Provider, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, base_url, api_key, weight, max_concurrent,
		        enabled, description, custom_headers, created_at, updated_at
		 FROM providers WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProviders(rows)
}

func scanProvider(s scanner) (*models.Provider, error) {
	var p models.Provider
	var enabled int
	var description sql.NullString
	var customHeaders sql.NullString
	var createdAt, updatedAt sql.NullTime

	err := s.Scan(
		&p.ID, &p.Name, &p.BaseURL, &p.APIKey,
		&p.Weight, &p.MaxConcurrent, &enabled,
		&description, &customHeaders, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	p.Enabled = enabled == 1
	if description.Valid {
		p.Description = description.String
	}
	if customHeaders.Valid && customHeaders.String != "" {
		if err := json.Unmarshal([]byte(customHeaders.String), &p.CustomHeaders); err != nil {
			return nil, fmt.Errorf("unmarshal custom_headers for provider %d: %w", p.ID, err)
		}
	}
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	} else {
		p.CreatedAt = time.Now()
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	} else {
		p.UpdatedAt = p.CreatedAt
	}
	return &p, nil
}

func scanProviders(rows *sql.Rows) ([]*models.Provider, error) {
	var result []*models.Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *SQLProviderRepository) FindAll(ctx context.Context) ([]*models.Provider, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, base_url, api_key, weight, max_concurrent,
		        enabled, description, custom_headers, created_at, updated_at
		 FROM providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProviders(rows)
}

func (r *SQLProviderRepository) Insert(ctx context.Context, p *models.Provider, modelIDs []int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	customHeadersJSON := ""
	if len(p.CustomHeaders) > 0 {
		if b, err := json.Marshal(p.CustomHeaders); err == nil {
			customHeadersJSON = string(b)
		}
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO providers (name, base_url, api_key, weight, max_concurrent,
		        enabled, description, custom_headers, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.BaseURL, p.APIKey, p.Weight, p.MaxConcurrent,
		boolToInt(p.Enabled), p.Description, customHeadersJSON, now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to insert provider: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	for _, mid := range modelIDs {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO provider_models (provider_id, model_id) VALUES (?, ?)`, id, mid)
		if err != nil {
			return 0, fmt.Errorf("failed to insert provider_model: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}
	return id, nil
}

func (r *SQLProviderRepository) Update(ctx context.Context, id int64, updates map[string]any, modelIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	if len(updates) > 0 {
		setClauses := make([]string, 0, len(updates)+1)
		params := make([]any, 0, len(updates)+2)
		for field, value := range updates {
			if field == "enabled" {
				if b, ok := value.(bool); ok {
					value = boolToInt(b)
				}
			}
			if field == "custom_headers" {
				if m, ok := value.(map[string]string); ok {
					if b, err := json.Marshal(m); err == nil {
						value = string(b)
					}
				}
			}
			setClauses = append(setClauses, field+" = ?")
			params = append(params, value)
		}
		setClauses = append(setClauses, "updated_at = ?")
		params = append(params, time.Now().UTC().Format("2006-01-02 15:04:05"))
		params = append(params, id)
		query := fmt.Sprintf("UPDATE providers SET %s WHERE id = ?", strings.Join(setClauses, ", "))
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return fmt.Errorf("failed to update provider: %w", err)
		}
	}

	if modelIDs != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM provider_models WHERE provider_id = ?`, id); err != nil {
			return fmt.Errorf("failed to delete provider_models: %w", err)
		}
		for _, mid := range modelIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO provider_models (provider_id, model_id) VALUES (?, ?)`, id, mid); err != nil {
				return fmt.Errorf("failed to insert provider_model: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

func (r *SQLProviderRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM provider_models WHERE provider_id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete provider_models: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM providers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

func (r *SQLProviderRepository) GetModelIDsForProvider(ctx context.Context, providerID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT model_id FROM provider_models WHERE provider_id = ? ORDER BY model_id`, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model ids: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

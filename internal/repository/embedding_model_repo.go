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

// EmbeddingModelRepository handles embedding model data access.
type EmbeddingModelRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewEmbeddingModelRepository creates a new EmbeddingModelRepository.
func NewEmbeddingModelRepository(db *sql.DB, logger *zap.Logger) *EmbeddingModelRepository {
	return &EmbeddingModelRepository{db: db, logger: logger}
}

// ListModels retrieves embedding models, optionally filtering enabled only.
func (r *EmbeddingModelRepository) ListModels(ctx context.Context, enabledOnly bool) ([]*models.EmbeddingModel, error) {
	query := `SELECT id, name, dimension, description, fastembed_supported, fastembed_name,
		is_builtin, enabled, sort_order, created_at, updated_at
		FROM embedding_models`
	if enabledOnly {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY sort_order, id`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list embedding models: %w", err)
	}
	defer rows.Close()

	var result []*models.EmbeddingModel
	for rows.Next() {
		m, err := r.scanModel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetModelByName retrieves an embedding model by name.
func (r *EmbeddingModelRepository) GetModelByName(ctx context.Context, name string) (*models.EmbeddingModel, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, dimension, description, fastembed_supported, fastembed_name,
			is_builtin, enabled, sort_order, created_at, updated_at
		FROM embedding_models WHERE name = ?
	`, name)

	m, err := r.scanModelRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get embedding model by name: %w", err)
	}
	return m, nil
}

// AddModel inserts a new embedding model.
func (r *EmbeddingModelRepository) AddModel(ctx context.Context, m *models.EmbeddingModel) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO embedding_models (name, dimension, description, fastembed_supported,
			fastembed_name, is_builtin, enabled, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.Name, m.Dimension, m.Description, boolToInt(m.FastembedSupported),
		m.FastembedName, boolToInt(m.IsBuiltin), boolToInt(m.Enabled),
		m.SortOrder, now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to add embedding model: %w", err)
	}
	return result.LastInsertId()
}

// UpdateModel dynamically updates an embedding model.
func (r *EmbeddingModelRepository) UpdateModel(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	boolFields := map[string]bool{
		"fastembed_supported": true,
		"enabled":             true,
	}

	setClauses := make([]string, 0, len(updates)+1)
	params := make([]any, 0, len(updates)+2)

	for field, value := range updates {
		if boolFields[field] {
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

	query := fmt.Sprintf("UPDATE embedding_models SET %s WHERE id = ?",
		strings.Join(setClauses, ", "))

	_, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to update embedding model: %w", err)
	}
	return nil
}

// DeleteModel deletes an embedding model (builtin models cannot be deleted).
func (r *EmbeddingModelRepository) DeleteModel(ctx context.Context, id int64) error {
	// Check if builtin
	var isBuiltin int
	err := r.db.QueryRowContext(ctx, `SELECT is_builtin FROM embedding_models WHERE id = ?`, id).Scan(&isBuiltin)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("embedding model not found: %d", id)
		}
		return fmt.Errorf("failed to check builtin status: %w", err)
	}
	if isBuiltin == 1 {
		return fmt.Errorf("cannot delete builtin embedding model: %d", id)
	}

	_, err = r.db.ExecContext(ctx, `DELETE FROM embedding_models WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete embedding model: %w", err)
	}
	return nil
}

// GetFastembedMapping returns a map of model name to fastembed name.
func (r *EmbeddingModelRepository) GetFastembedMapping(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT name, fastembed_name FROM embedding_models
		WHERE fastembed_supported = 1 AND fastembed_name IS NOT NULL AND enabled = 1
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get fastembed mapping: %w", err)
	}
	defer rows.Close()

	mapping := make(map[string]string)
	for rows.Next() {
		var name, fastembedName string
		if err := rows.Scan(&name, &fastembedName); err != nil {
			return nil, fmt.Errorf("failed to scan fastembed mapping: %w", err)
		}
		mapping[name] = fastembedName
	}
	return mapping, rows.Err()
}

// GetEnabledModelNames returns names of all enabled embedding models.
func (r *EmbeddingModelRepository) GetEnabledModelNames(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT name FROM embedding_models WHERE enabled = 1 ORDER BY sort_order, id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled model names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan model name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *EmbeddingModelRepository) scanModel(rows *sql.Rows) (*models.EmbeddingModel, error) {
	var m models.EmbeddingModel
	var description, fastembedName sql.NullString
	var fastembedSupported, isBuiltin, enabled int
	var createdAt, updatedAt string

	err := rows.Scan(
		&m.ID, &m.Name, &m.Dimension, &description, &fastembedSupported, &fastembedName,
		&isBuiltin, &enabled, &m.SortOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan embedding model: %w", err)
	}

	if description.Valid {
		m.Description = description.String
	}
	if fastembedName.Valid {
		m.FastembedName = fastembedName.String
	}
	m.FastembedSupported = fastembedSupported == 1
	m.IsBuiltin = isBuiltin == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	m.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &m, nil
}

func (r *EmbeddingModelRepository) scanModelRow(row *sql.Row) (*models.EmbeddingModel, error) {
	var m models.EmbeddingModel
	var description, fastembedName sql.NullString
	var fastembedSupported, isBuiltin, enabled int
	var createdAt, updatedAt string

	err := row.Scan(
		&m.ID, &m.Name, &m.Dimension, &description, &fastembedSupported, &fastembedName,
		&isBuiltin, &enabled, &m.SortOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if description.Valid {
		m.Description = description.String
	}
	if fastembedName.Valid {
		m.FastembedName = fastembedName.String
	}
	m.FastembedSupported = fastembedSupported == 1
	m.IsBuiltin = isBuiltin == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	m.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &m, nil
}

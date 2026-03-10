package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// ModelAliasPatch is a typed partial update payload for model aliases.
type ModelAliasPatch struct {
	AliasName     *string
	TargetModelID *int64
	Enabled       *bool
}

// SQLModelAliasRepository implements ModelAliasRepository using database/sql.
type SQLModelAliasRepository struct {
	db     *sql.DB
	readDB *sql.DB
}

// NewModelAliasRepository creates a new SQLModelAliasRepository.
func NewModelAliasRepository(db *sql.DB, readDB ...*sql.DB) *SQLModelAliasRepository {
	repo := &SQLModelAliasRepository{db: db, readDB: db}
	if len(readDB) > 0 && readDB[0] != nil {
		repo.readDB = readDB[0]
	}
	return repo
}

func (r *SQLModelAliasRepository) FindByID(ctx context.Context, id int64) (*models.ModelAlias, error) {
	row := r.readDB.QueryRowContext(ctx,
		`SELECT id, alias_name, target_model_id, enabled, created_at, updated_at
		 FROM model_aliases WHERE id = ?`, id)
	return scanModelAlias(row)
}

func (r *SQLModelAliasRepository) FindByAliasName(ctx context.Context, aliasName string) (*models.ModelAlias, error) {
	row := r.readDB.QueryRowContext(ctx,
		`SELECT id, alias_name, target_model_id, enabled, created_at, updated_at
		 FROM model_aliases
		 WHERE alias_name = ? COLLATE NOCASE AND enabled = 1`,
		aliasName)
	alias, err := scanModelAlias(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return alias, nil
}

func (r *SQLModelAliasRepository) FindAll(ctx context.Context) ([]*models.ModelAlias, error) {
	rows, err := r.readDB.QueryContext(ctx,
		`SELECT id, alias_name, target_model_id, enabled, created_at, updated_at
		 FROM model_aliases
		 ORDER BY alias_name COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModelAliases(rows)
}

func (r *SQLModelAliasRepository) Insert(ctx context.Context, alias *models.ModelAlias) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO model_aliases (alias_name, target_model_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		alias.AliasName, alias.TargetModelID, boolToInt(alias.Enabled), now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to insert model alias: %w", err)
	}
	return result.LastInsertId()
}

func (r *SQLModelAliasRepository) updateWithMap(ctx context.Context, id int64, updates map[string]any) error {
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
		setClauses = append(setClauses, field+" = ?")
		params = append(params, value)
	}
	setClauses = append(setClauses, "updated_at = ?")
	params = append(params, time.Now().UTC().Format("2006-01-02 15:04:05"))
	params = append(params, id)

	query := fmt.Sprintf("UPDATE model_aliases SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	if _, err := r.db.ExecContext(ctx, query, params...); err != nil {
		return fmt.Errorf("failed to update model alias: %w", err)
	}
	return nil
}

func (r *SQLModelAliasRepository) UpdatePatch(ctx context.Context, id int64, patch ModelAliasPatch) error {
	updates := make(map[string]any)
	if patch.AliasName != nil {
		updates["alias_name"] = *patch.AliasName
	}
	if patch.TargetModelID != nil {
		updates["target_model_id"] = *patch.TargetModelID
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	return r.updateWithMap(ctx, id, updates)
}

func (r *SQLModelAliasRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM model_aliases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete model alias: %w", err)
	}
	return nil
}

func scanModelAlias(s scanner) (*models.ModelAlias, error) {
	var alias models.ModelAlias
	var enabled int
	var createdAt, updatedAt sql.NullTime

	err := s.Scan(
		&alias.ID, &alias.AliasName, &alias.TargetModelID,
		&enabled, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	alias.Enabled = enabled == 1
	if createdAt.Valid {
		alias.CreatedAt = createdAt.Time
	} else {
		alias.CreatedAt = time.Now()
	}
	if updatedAt.Valid {
		alias.UpdatedAt = updatedAt.Time
	} else {
		alias.UpdatedAt = alias.CreatedAt
	}
	return &alias, nil
}

func scanModelAliases(rows *sql.Rows) ([]*models.ModelAlias, error) {
	var result []*models.ModelAlias
	for rows.Next() {
		alias, err := scanModelAlias(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, alias)
	}
	return result, rows.Err()
}

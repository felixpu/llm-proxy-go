package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// SharedState represents a shared state entry
type SharedState struct {
	Key       string
	Value     string
	UpdatedAt time.Time
	UpdatedBy string
}

// SharedStateRepository handles shared state data access
type SharedStateRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewSharedStateRepository creates a new SharedStateRepository
func NewSharedStateRepository(db *sql.DB, logger *zap.Logger) *SharedStateRepository {
	return &SharedStateRepository{
		db:     db,
		logger: logger,
	}
}

// SetState sets or updates a shared state value
func (r *SharedStateRepository) SetState(ctx context.Context, key, value, updatedBy string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO shared_state (key, value, updated_at, updated_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by
	`, key, value, now, updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set shared state: %w", err)
	}

	r.logger.Debug("shared state updated",
		zap.String("key", key),
		zap.String("updated_by", updatedBy))

	return nil
}

// GetState retrieves a shared state value by key
func (r *SharedStateRepository) GetState(ctx context.Context, key string) (*SharedState, error) {
	var state SharedState
	var updatedAt string
	var updatedBy sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT key, value, updated_at, updated_by
		FROM shared_state
		WHERE key = ?
	`, key).Scan(&state.Key, &state.Value, &updatedAt, &updatedBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get shared state: %w", err)
	}

	state.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	if updatedBy.Valid {
		state.UpdatedBy = updatedBy.String
	}

	return &state, nil
}

// DeleteState removes a shared state entry
func (r *SharedStateRepository) DeleteState(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM shared_state WHERE key = ?
	`, key)
	if err != nil {
		return fmt.Errorf("failed to delete shared state: %w", err)
	}
	return nil
}

// GetAllStates returns all shared state entries
func (r *SharedStateRepository) GetAllStates(ctx context.Context) ([]*SharedState, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT key, value, updated_at, updated_by
		FROM shared_state
		ORDER BY key ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get all states: %w", err)
	}
	defer rows.Close()

	var states []*SharedState
	for rows.Next() {
		var state SharedState
		var updatedAt string
		var updatedBy sql.NullString

		err := rows.Scan(&state.Key, &state.Value, &updatedAt, &updatedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to scan state: %w", err)
		}

		state.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		if updatedBy.Valid {
			state.UpdatedBy = updatedBy.String
		}

		states = append(states, &state)
	}

	return states, rows.Err()
}

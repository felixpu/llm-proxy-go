package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// WorkerRecord represents a worker registration record
type WorkerRecord struct {
	ID            int64
	WorkerID      string
	PID           int
	IsPrimary     bool
	LastHeartbeat time.Time
	CreatedAt     time.Time
}

// WorkerRegistryRepository handles worker registration data access
type WorkerRegistryRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewWorkerRegistryRepository creates a new WorkerRegistryRepository
func NewWorkerRegistryRepository(db *sql.DB, logger *zap.Logger) *WorkerRegistryRepository {
	return &WorkerRegistryRepository{
		db:     db,
		logger: logger,
	}
}

// Register inserts a new worker record
func (r *WorkerRegistryRepository) Register(ctx context.Context, workerID string, pid int) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO worker_registry (worker_id, pid, is_primary, last_heartbeat, created_at)
		VALUES (?, ?, 0, ?, ?)
	`, workerID, pid, now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to register worker: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	r.logger.Debug("worker registered",
		zap.String("worker_id", workerID),
		zap.Int("pid", pid),
		zap.Int64("id", id))

	return id, nil
}

// TryBecomePrimary attempts to become the primary worker atomically
// Returns true if this worker became primary, false otherwise
func (r *WorkerRegistryRepository) TryBecomePrimary(ctx context.Context, workerID string) (bool, error) {
	// Atomic operation: become primary only if no other primary exists
	result, err := r.db.ExecContext(ctx, `
		UPDATE worker_registry
		SET is_primary = 1
		WHERE worker_id = ?
		AND NOT EXISTS (
			SELECT 1 FROM worker_registry WHERE is_primary = 1
		)
	`, workerID)
	if err != nil {
		return false, fmt.Errorf("failed to try become primary: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		r.logger.Info("worker became primary", zap.String("worker_id", workerID))
		return true, nil
	}

	// Check if we're already primary
	var isPrimary int
	err = r.db.QueryRowContext(ctx, `
		SELECT is_primary FROM worker_registry WHERE worker_id = ?
	`, workerID).Scan(&isPrimary)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check primary status: %w", err)
	}

	return isPrimary == 1, nil
}

// IsPrimary checks if the specified worker is primary
func (r *WorkerRegistryRepository) IsPrimary(ctx context.Context, workerID string) (bool, error) {
	var isPrimary int
	err := r.db.QueryRowContext(ctx, `
		SELECT is_primary FROM worker_registry WHERE worker_id = ?
	`, workerID).Scan(&isPrimary)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check primary status: %w", err)
	}
	return isPrimary == 1, nil
}

// UpdateHeartbeat updates the heartbeat timestamp for a worker
func (r *WorkerRegistryRepository) UpdateHeartbeat(ctx context.Context, workerID string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err := r.db.ExecContext(ctx, `
		UPDATE worker_registry SET last_heartbeat = ? WHERE worker_id = ?
	`, now, workerID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}
	return nil
}

// IsPrimaryStale checks if the current primary has timed out
func (r *WorkerRegistryRepository) IsPrimaryStale(ctx context.Context, timeoutSeconds int) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM worker_registry
		WHERE is_primary = 1
		AND datetime(last_heartbeat) < datetime('now', '-%d seconds')
	`, timeoutSeconds)).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check primary staleness: %w", err)
	}
	return count > 0, nil
}

// CleanupStale removes workers that haven't sent heartbeat within timeout
func (r *WorkerRegistryRepository) CleanupStale(ctx context.Context, timeoutSeconds int) (int64, error) {
	result, err := r.db.ExecContext(ctx, fmt.Sprintf(`
		DELETE FROM worker_registry
		WHERE datetime(last_heartbeat) < datetime('now', '-%d seconds')
	`, timeoutSeconds))
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup stale workers: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		r.logger.Info("cleaned up stale workers", zap.Int64("count", rowsAffected))
	}

	return rowsAffected, nil
}

// Unregister removes a worker from the registry
func (r *WorkerRegistryRepository) Unregister(ctx context.Context, workerID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM worker_registry WHERE worker_id = ?
	`, workerID)
	if err != nil {
		return fmt.Errorf("failed to unregister worker: %w", err)
	}

	r.logger.Info("worker unregistered", zap.String("worker_id", workerID))
	return nil
}

// GetAllWorkers returns all registered workers
func (r *WorkerRegistryRepository) GetAllWorkers(ctx context.Context) ([]*WorkerRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, worker_id, pid, is_primary, last_heartbeat, created_at
		FROM worker_registry
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get all workers: %w", err)
	}
	defer rows.Close()

	var workers []*WorkerRecord
	for rows.Next() {
		var w WorkerRecord
		var isPrimary int
		var lastHeartbeat, createdAt string

		err := rows.Scan(&w.ID, &w.WorkerID, &w.PID, &isPrimary, &lastHeartbeat, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan worker: %w", err)
		}

		w.IsPrimary = isPrimary == 1
		w.LastHeartbeat, _ = time.Parse("2006-01-02 15:04:05", lastHeartbeat)
		w.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)

		workers = append(workers, &w)
	}

	return workers, rows.Err()
}

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// EmbeddingCacheEntry represents a cached embedding entry
type EmbeddingCacheEntry struct {
	ID             int64
	ContentHash    string
	ContentPreview string
	Embedding      []float64
	TaskType       string
	Reason         string
	HitCount       int
	CreatedAt      time.Time
	LastHitAt      *time.Time
}

// EmbeddingCacheRepository handles embedding cache data access
type EmbeddingCacheRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewEmbeddingCacheRepository creates a new EmbeddingCacheRepository
func NewEmbeddingCacheRepository(db *sql.DB, logger *zap.Logger) *EmbeddingCacheRepository {
	return &EmbeddingCacheRepository{
		db:     db,
		logger: logger,
	}
}

// SaveCache saves or updates an embedding cache entry
func (r *EmbeddingCacheRepository) SaveCache(ctx context.Context, contentHash, contentPreview string, embedding []float64, taskType, reason string) error {
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO routing_embedding_cache (content_hash, content_preview, embedding, task_type, reason, hit_count, created_at)
		VALUES (?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP)
		ON CONFLICT(content_hash) DO UPDATE SET
			embedding = excluded.embedding,
			task_type = excluded.task_type,
			reason = excluded.reason,
			created_at = CURRENT_TIMESTAMP
	`, contentHash, contentPreview, string(embeddingJSON), taskType, reason)
	if err != nil {
		return fmt.Errorf("failed to save cache: %w", err)
	}

	r.logger.Debug("embedding cache saved",
		zap.String("content_hash", contentHash),
		zap.String("task_type", taskType))

	return nil
}

// GetExactMatch retrieves a cache entry by exact content hash match
func (r *EmbeddingCacheRepository) GetExactMatch(ctx context.Context, contentHash string, ttlSeconds int) (*EmbeddingCacheEntry, error) {
	if ttlSeconds <= 0 {
		return nil, nil
	}

	var entry EmbeddingCacheEntry
	var embeddingJSON string
	var createdAt string
	var lastHitAt sql.NullString

	query := fmt.Sprintf(`
		SELECT id, content_hash, content_preview, embedding, task_type, reason, hit_count, created_at, last_hit_at
		FROM routing_embedding_cache
		WHERE content_hash = ?
		AND datetime(created_at) >= datetime('now', '-%d seconds')
	`, ttlSeconds)

	err := r.db.QueryRowContext(ctx, query, contentHash).Scan(
		&entry.ID, &entry.ContentHash, &entry.ContentPreview,
		&embeddingJSON, &entry.TaskType, &entry.Reason,
		&entry.HitCount, &createdAt, &lastHitAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get exact match: %w", err)
	}

	if err := json.Unmarshal([]byte(embeddingJSON), &entry.Embedding); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
	}

	entry.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	if lastHitAt.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", lastHitAt.String)
		entry.LastHitAt = &t
	}

	return &entry, nil
}

// FindAllEmbeddings retrieves all embeddings within TTL for similarity search
func (r *EmbeddingCacheRepository) FindAllEmbeddings(ctx context.Context, ttlSeconds int) ([]*EmbeddingCacheEntry, error) {
	if ttlSeconds <= 0 {
		return nil, nil
	}

	query := fmt.Sprintf(`
		SELECT id, content_hash, embedding, task_type, reason
		FROM routing_embedding_cache
		WHERE datetime(created_at) >= datetime('now', '-%d seconds')
	`, ttlSeconds)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find all embeddings: %w", err)
	}
	defer rows.Close()

	var entries []*EmbeddingCacheEntry
	for rows.Next() {
		var entry EmbeddingCacheEntry
		var embeddingJSON string

		err := rows.Scan(&entry.ID, &entry.ContentHash, &embeddingJSON, &entry.TaskType, &entry.Reason)
		if err != nil {
			return nil, fmt.Errorf("failed to scan embedding: %w", err)
		}

		if err := json.Unmarshal([]byte(embeddingJSON), &entry.Embedding); err != nil {
			r.logger.Warn("failed to unmarshal embedding", zap.Error(err), zap.Int64("id", entry.ID))
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// UpdateHitCount increments the hit count for a cache entry
func (r *EmbeddingCacheRepository) UpdateHitCount(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE routing_embedding_cache
		SET hit_count = hit_count + 1, last_hit_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("failed to update hit count: %w", err)
	}
	return nil
}

// UpdateHitCountByHash increments the hit count by content hash
func (r *EmbeddingCacheRepository) UpdateHitCountByHash(ctx context.Context, contentHash string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE routing_embedding_cache
		SET hit_count = hit_count + 1, last_hit_at = CURRENT_TIMESTAMP
		WHERE content_hash = ?
	`, contentHash)
	if err != nil {
		return fmt.Errorf("failed to update hit count by hash: %w", err)
	}
	return nil
}

// CleanupExpired removes expired cache entries
func (r *EmbeddingCacheRepository) CleanupExpired(ctx context.Context, ttlSeconds int) (int64, error) {
	var result sql.Result
	var err error

	if ttlSeconds <= 0 {
		// Delete all if TTL is 0 or negative
		result, err = r.db.ExecContext(ctx, `DELETE FROM routing_embedding_cache`)
	} else {
		result, err = r.db.ExecContext(ctx, fmt.Sprintf(`
			DELETE FROM routing_embedding_cache
			WHERE datetime(created_at) < datetime('now', '-%d seconds')
		`, ttlSeconds))
	}

	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		r.logger.Info("cleaned up expired cache entries", zap.Int64("count", rowsAffected))
	}

	return rowsAffected, nil
}

// GetTopEntries retrieves the most frequently accessed cache entries
func (r *EmbeddingCacheRepository) GetTopEntries(ctx context.Context, sortBy string, limit int) ([]*EmbeddingCacheEntry, error) {
	// Validate sort field
	validSortFields := map[string]bool{
		"hit_count":   true,
		"created_at":  true,
		"last_hit_at": true,
	}
	if !validSortFields[sortBy] {
		sortBy = "hit_count"
	}

	query := fmt.Sprintf(`
		SELECT id, content_hash, content_preview, task_type, reason, hit_count, created_at, last_hit_at
		FROM routing_embedding_cache
		ORDER BY %s DESC
		LIMIT ?
	`, sortBy)

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top entries: %w", err)
	}
	defer rows.Close()

	var entries []*EmbeddingCacheEntry
	for rows.Next() {
		var entry EmbeddingCacheEntry
		var createdAt string
		var lastHitAt sql.NullString

		err := rows.Scan(
			&entry.ID, &entry.ContentHash, &entry.ContentPreview,
			&entry.TaskType, &entry.Reason, &entry.HitCount,
			&createdAt, &lastHitAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}

		entry.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		if lastHitAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", lastHitAt.String)
			entry.LastHitAt = &t
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// DeleteAll removes all cache entries
func (r *EmbeddingCacheRepository) DeleteAll(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM routing_embedding_cache`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete all: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	r.logger.Info("deleted all cache entries", zap.Int64("count", rowsAffected))
	return rowsAffected, nil
}

// Count returns the total number of cache entries
func (r *EmbeddingCacheRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routing_embedding_cache`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count cache entries: %w", err)
	}
	return count, nil
}

// GetStats returns cache statistics
func (r *EmbeddingCacheRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var totalHits int64
	var avgHitCount float64
	var maxHitCount int64

	query := `
		SELECT
			COALESCE(SUM(hit_count), 0) as total_hits,
			COALESCE(AVG(hit_count), 0) as avg_hit_count,
			COALESCE(MAX(hit_count), 0) as max_hit_count
		FROM routing_embedding_cache
	`

	err := r.db.QueryRowContext(ctx, query).Scan(&totalHits, &avgHitCount, &maxHitCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return map[string]interface{}{
		"total_hits":    totalHits,
		"avg_hit_count": avgHitCount,
		"max_hit_count": maxHitCount,
	}, nil
}

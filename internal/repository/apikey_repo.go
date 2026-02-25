package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// SQLAPIKeyRepository implements APIKeyRepository using database/sql.
type SQLAPIKeyRepository struct {
	db *sql.DB
}

// NewAPIKeyRepository creates a new SQLAPIKeyRepository.
func NewAPIKeyRepository(db *sql.DB) *SQLAPIKeyRepository {
	return &SQLAPIKeyRepository{db: db}
}

func (r *SQLAPIKeyRepository) FindByKeyHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, key_hash, key_full, key_prefix, name, is_active, created_at, last_used_at, expires_at
		 FROM api_keys WHERE key_hash = ?`, keyHash)

	var k models.APIKey
	var isActive int
	var keyFull sql.NullString
	var lastUsed, expires sql.NullTime

	err := row.Scan(
		&k.ID, &k.UserID, &k.KeyHash, &keyFull, &k.KeyPrefix, &k.Name,
		&isActive, &k.CreatedAt, &lastUsed, &expires,
	)
	if err != nil {
		return nil, err
	}

	k.IsActive = isActive == 1
	if keyFull.Valid {
		k.KeyFull = keyFull.String
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		k.ExpiresAt = &expires.Time
	}
	return &k, nil
}

func (r *SQLAPIKeyRepository) FindByID(ctx context.Context, id int64) (*models.APIKey, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, key_hash, key_full, key_prefix, name, is_active, created_at, last_used_at, expires_at
		 FROM api_keys WHERE id = ?`, id)

	var k models.APIKey
	var isActive int
	var keyFull sql.NullString
	var lastUsed, expires sql.NullTime

	err := row.Scan(
		&k.ID, &k.UserID, &k.KeyHash, &keyFull, &k.KeyPrefix, &k.Name,
		&isActive, &k.CreatedAt, &lastUsed, &expires,
	)
	if err != nil {
		return nil, err
	}

	k.IsActive = isActive == 1
	if keyFull.Valid {
		k.KeyFull = keyFull.String
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		k.ExpiresAt = &expires.Time
	}
	return &k, nil
}

func (r *SQLAPIKeyRepository) FindByUserID(ctx context.Context, userID int64) ([]*models.APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, key_hash, key_full, key_prefix, name, is_active, created_at, last_used_at, expires_at
		 FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		var isActive int
		var keyFull sql.NullString
		var lastUsed, expires sql.NullTime

		if err := rows.Scan(
			&k.ID, &k.UserID, &k.KeyHash, &keyFull, &k.KeyPrefix, &k.Name,
			&isActive, &k.CreatedAt, &lastUsed, &expires,
		); err != nil {
			return nil, err
		}

		k.IsActive = isActive == 1
		if keyFull.Valid {
			k.KeyFull = keyFull.String
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			k.ExpiresAt = &expires.Time
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (r *SQLAPIKeyRepository) FindAll(ctx context.Context) ([]*models.APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, key_hash, key_full, key_prefix, name, is_active, created_at, last_used_at, expires_at
		 FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		var isActive int
		var keyFull sql.NullString
		var lastUsed, expires sql.NullTime

		if err := rows.Scan(
			&k.ID, &k.UserID, &k.KeyHash, &keyFull, &k.KeyPrefix, &k.Name,
			&isActive, &k.CreatedAt, &lastUsed, &expires,
		); err != nil {
			return nil, err
		}

		k.IsActive = isActive == 1
		if keyFull.Valid {
			k.KeyFull = keyFull.String
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			k.ExpiresAt = &expires.Time
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (r *SQLAPIKeyRepository) Insert(ctx context.Context, key *models.APIKey) (int64, error) {
	now := time.Now().UTC()
	if key.CreatedAt.IsZero() {
		key.CreatedAt = now
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO api_keys (user_id, key_hash, key_full, key_prefix, name, is_active, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		key.UserID, key.KeyHash, key.KeyFull, key.KeyPrefix, key.Name,
		boolToInt(key.IsActive), key.CreatedAt, key.ExpiresAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLAPIKeyRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = ? WHERE id = ?`,
		time.Now().UTC(), id)
	return err
}

func (r *SQLAPIKeyRepository) Revoke(ctx context.Context, id int64, userID *int64) error {
	if userID != nil {
		_, err := r.db.ExecContext(ctx,
			`UPDATE api_keys SET is_active = 0 WHERE id = ? AND user_id = ?`,
			id, *userID)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET is_active = 0 WHERE id = ?`, id)
	return err
}

func (r *SQLAPIKeyRepository) Delete(ctx context.Context, id int64, userID *int64) error {
	if userID != nil {
		_, err := r.db.ExecContext(ctx,
			`DELETE FROM api_keys WHERE id = ? AND user_id = ?`,
			id, *userID)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = ?`, id)
	return err
}

func (r *SQLAPIKeyRepository) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE expires_at IS NOT NULL AND expires_at < ?`,
		time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}


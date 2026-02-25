package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Session represents a user session.
type Session struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
	IPAddress string
	UserAgent string
}

// SessionRepository handles session data access.
type SessionRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewSessionRepository creates a new SessionRepository.
func NewSessionRepository(db *sql.DB, logger *zap.Logger) *SessionRepository {
	return &SessionRepository{db: db, logger: logger}
}

// GenerateSessionToken generates a cryptographically secure session token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// CreateSession inserts a new session record.
func (r *SessionRepository) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time, ipAddress, userAgent string) (int64, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token, expires_at, created_at, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, token, expiresAt.UTC().Format("2006-01-02 15:04:05"), now, ipAddress, userAgent)
	if err != nil {
		return 0, fmt.Errorf("failed to create session: %w", err)
	}
	return result.LastInsertId()
}

// FindValidSession finds a valid (non-expired, active user) session by token.
// Returns nil if not found or expired.
func (r *SessionRepository) FindValidSession(ctx context.Context, token string) (*Session, string, string, error) {
	var s Session
	var username string
	var role string
	var expiresAt, createdAt string
	var ipAddress, userAgent sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.token, s.expires_at, s.created_at,
			s.ip_address, s.user_agent, u.username, u.role
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token = ?
		AND datetime(s.expires_at) > datetime('now')
		AND u.is_active = 1
	`, token).Scan(
		&s.ID, &s.UserID, &s.Token, &expiresAt, &createdAt,
		&ipAddress, &userAgent, &username, &role,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", "", nil
		}
		return nil, "", "", fmt.Errorf("failed to find valid session: %w", err)
	}

	s.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresAt)
	s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	if ipAddress.Valid {
		s.IPAddress = ipAddress.String
	}
	if userAgent.Valid {
		s.UserAgent = userAgent.String
	}

	return &s, username, role, nil
}

// DeleteByToken deletes a session by token.
func (r *SessionRepository) DeleteByToken(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeleteByUserID deletes all sessions for a user.
func (r *SessionRepository) DeleteByUserID(ctx context.Context, userID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete user sessions: %w", err)
	}
	return result.RowsAffected()
}

// CleanupExpired removes expired sessions.
func (r *SessionRepository) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE datetime(expires_at) <= datetime('now')
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		r.logger.Info("cleaned up expired sessions", zap.Int64("count", rows))
	}
	return rows, nil
}

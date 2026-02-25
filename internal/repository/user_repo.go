package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// SQLUserRepository implements UserRepository using database/sql.
type SQLUserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new SQLUserRepository.
func NewUserRepository(db *sql.DB) *SQLUserRepository {
	return &SQLUserRepository{db: db}
}

func (r *SQLUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, role, is_active, created_at, updated_at
		 FROM users WHERE id = ?`, id)

	var u models.User
	var role string
	var isActive int

	err := row.Scan(&u.ID, &u.Username, &role, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	u.Role = models.UserRole(role)
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *SQLUserRepository) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, role, is_active, created_at, updated_at
		 FROM users WHERE username = ?`, username)

	var u models.User
	var role string
	var isActive int

	err := row.Scan(&u.ID, &u.Username, &role, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	u.Role = models.UserRole(role)
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *SQLUserRepository) FindByUsernameWithHash(ctx context.Context, username string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, is_active, created_at, updated_at
		 FROM users WHERE username = ?`, username)

	var u models.User
	var role string
	var isActive int

	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	u.Role = models.UserRole(role)
	u.IsActive = isActive == 1
	return &u, nil
}

func (r *SQLUserRepository) Insert(ctx context.Context, user *models.User) (int64, error) {
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		user.Username, user.PasswordHash, string(user.Role),
		boolToInt(user.IsActive), user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FindAll returns all users with pagination.
func (r *SQLUserRepository) FindAll(ctx context.Context, offset, limit int) ([]*models.User, int64, error) {
	// Get total count
	var total int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get users
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, username, role, is_active, created_at, updated_at
		 FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		var role string
		var isActive int
		if err := rows.Scan(&u.ID, &u.Username, &role, &isActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		u.Role = models.UserRole(role)
		u.IsActive = isActive == 1
		users = append(users, &u)
	}
	return users, total, rows.Err()
}

// Update updates a user record.
func (r *SQLUserRepository) Update(ctx context.Context, user *models.User) error {
	user.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = ?, role = ?, is_active = ?, updated_at = ? WHERE id = ?`,
		user.Username, string(user.Role), boolToInt(user.IsActive), user.UpdatedAt, user.ID)
	return err
}

// UpdatePassword updates a user's password hash.
func (r *SQLUserRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, time.Now().UTC(), userID)
	return err
}

// Delete removes a user by ID.
func (r *SQLUserRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// CountByRole counts users with a specific role.
func (r *SQLUserRepository) CountByRole(ctx context.Context, role models.UserRole) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ?`, string(role)).Scan(&count)
	return count, err
}

// FindByIDWithHash returns a user by ID including password hash (for auth).
func (r *SQLUserRepository) FindByIDWithHash(ctx context.Context, id int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, is_active, created_at, updated_at
		 FROM users WHERE id = ?`, id)

	var u models.User
	var role string
	var isActive int

	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &isActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	u.Role = models.UserRole(role)
	u.IsActive = isActive == 1
	return &u, nil
}

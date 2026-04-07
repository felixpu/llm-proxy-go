// Package database provides SQLite database connection management and migrations.
package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

func sqliteDSN(path string, readOnly bool) string {
	query := url.Values{}
	if readOnly {
		query.Set("mode", "ro")
	} else {
		query.Add("_pragma", "journal_mode(WAL)")
	}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(ON)")
	return fmt.Sprintf("file:%s?%s", path, query.Encode())
}

// DBOptions holds optional database pool configuration.
type DBOptions struct {
	ConnMaxLifetime time.Duration
}

// New creates a new database connection with the given path.
func New(path string) (*sql.DB, error) {
	return NewWithOptions(path, DBOptions{})
}

// NewWithOptions creates a new database connection with custom pool options.
func NewWithOptions(path string, opts DBOptions) (*sql.DB, error) {
	dsn := sqliteDSN(path, false)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite supports many readers in WAL mode but still only one writer.
	// Serializing the write pool avoids concurrent writer lock contention.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if opts.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(opts.ConnMaxLifetime)
	}

	// Verify connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return conn, nil
}

// NewReadOnly creates a read-only database connection for query-heavy workloads.
// Using a separate pool prevents expensive analytical queries from starving
// latency-sensitive write operations (e.g. proxy auth, log inserts).
func NewReadOnly(path string) (*sql.DB, error) {
	return NewReadOnlyWithOptions(path, DBOptions{})
}

// NewReadOnlyWithOptions creates a read-only connection with custom pool options.
func NewReadOnlyWithOptions(path string, opts DBOptions) (*sql.DB, error) {
	dsn := sqliteDSN(path, true)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only database: %w", err)
	}

	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(3)
	if opts.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(opts.ConnMaxLifetime)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping read-only database: %w", err)
	}

	return conn, nil
}

// Package database provides SQLite database connection management and migrations.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	once sync.Once
)

// New creates a new database connection with the given path.
func New(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool — leave headroom for the read-only pool.
	conn.SetMaxOpenConns(15)
	conn.SetMaxIdleConns(5)

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
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON&mode=ro", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only database: %w", err)
	}

	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(3)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping read-only database: %w", err)
	}

	return conn, nil
}

// GetDB returns the global database instance (singleton).
func GetDB() *sql.DB {
	return db
}

// InitDB initializes the global database instance.
func InitDB(path string) error {
	var initErr error
	once.Do(func() {
		var err error
		db, err = New(path)
		if err != nil {
			initErr = err
			return
		}
		log.Printf("Database initialized: %s", path)
	})
	return initErr
}

// Close closes the global database connection.
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

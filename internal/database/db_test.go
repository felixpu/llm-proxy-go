//go:build !integration && !e2e
// +build !integration,!e2e

package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ConfiguresSerializedWritePool(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "write.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	stats := db.Stats()
	assert.Equal(t, 1, stats.MaxOpenConnections)
}

func TestNew_EnablesSQLitePragmas(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pragmas.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var journalMode string
	require.NoError(t, db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode))
	assert.Equal(t, "wal", strings.ToLower(journalMode))

	var busyTimeout int
	require.NoError(t, db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout))
	assert.Equal(t, 5000, busyTimeout)

	var foreignKeys int
	require.NoError(t, db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys))
	assert.Equal(t, 1, foreignKeys)

	plainDB, err := sql.Open("sqlite", "file:"+dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = plainDB.Close() })

	require.NoError(t, plainDB.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode))
	assert.Equal(t, "wal", strings.ToLower(journalMode))
}

func TestNewReadOnly_ConfiguresReadPool(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "read.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	readDB, err := NewReadOnly(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readDB.Close() })

	stats := readDB.Stats()
	assert.Equal(t, 10, stats.MaxOpenConnections)

	var busyTimeout int
	require.NoError(t, readDB.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout))
	assert.Equal(t, 5000, busyTimeout)

	_, err = readDB.Exec(`CREATE TABLE should_fail (id INTEGER)`)
	require.Error(t, err)
}

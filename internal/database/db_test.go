//go:build !integration && !e2e
// +build !integration,!e2e

package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestNewWithOptions_AppliesConnMaxLifetime(t *testing.T) {
	opts := DBOptions{ConnMaxLifetime: 20 * time.Millisecond}
	db, err := NewWithOptions(filepath.Join(t.TempDir(), "lifetime.db"), opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.Ping())
	time.Sleep(50 * time.Millisecond)

	assertConnMaxLifetimeClosesConnections(t, db, func() error {
		return db.Ping()
	})
}

func TestNewReadOnlyWithOptions_AppliesConnMaxLifetime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "lifetime_ro.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	opts := DBOptions{ConnMaxLifetime: 20 * time.Millisecond}
	readDB, err := NewReadOnlyWithOptions(dbPath, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readDB.Close() })

	require.NoError(t, readDB.QueryRow(`SELECT 1`).Scan(new(int)))
	time.Sleep(50 * time.Millisecond)

	assertConnMaxLifetimeClosesConnections(t, readDB, func() error {
		return readDB.QueryRow(`SELECT 1`).Scan(new(int))
	})
}

func TestNew_SetsWALAutocheckpoint(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "wal_ckpt.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var autoCkpt int
	require.NoError(t, db.QueryRow(`PRAGMA wal_autocheckpoint`).Scan(&autoCkpt))
	assert.Equal(t, 1000, autoCkpt, "default wal_autocheckpoint should be 1000")
}

func assertConnMaxLifetimeClosesConnections(t *testing.T, db *sql.DB, touch func() error) {
	t.Helper()

	require.Eventually(t, func() bool {
		if err := touch(); err != nil {
			return false
		}
		return db.Stats().MaxLifetimeClosed > 0
	}, time.Second, 20*time.Millisecond, "expected ConnMaxLifetime to recycle at least one connection")
}

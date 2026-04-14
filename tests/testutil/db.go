package testutil

import (
	"database/sql"
	"testing"

	internaltestutil "github.com/user/llm-proxy-go/internal/testutil"
)

// NewTestDB delegates to internal/testutil to avoid schema drift.
func NewTestDB(t testing.TB) *sql.DB {
	t.Helper()
	return internaltestutil.NewTestDB(t)
}

// NewTestDBWithDefaults delegates to internal/testutil to avoid schema drift.
func NewTestDBWithDefaults(t testing.TB) *sql.DB {
	t.Helper()
	return internaltestutil.NewTestDBWithDefaults(t)
}

// NewFileBackedTestDBPair delegates to internal/testutil to avoid schema drift.
func NewFileBackedTestDBPair(t testing.TB) (*sql.DB, *sql.DB) {
	t.Helper()
	return internaltestutil.NewFileBackedTestDBPair(t)
}

// NewFileBackedTestDBPairWithDefaults delegates to internal/testutil to avoid schema drift.
func NewFileBackedTestDBPairWithDefaults(t testing.TB) (*sql.DB, *sql.DB) {
	t.Helper()
	return internaltestutil.NewFileBackedTestDBPairWithDefaults(t)
}

// SeedTestData delegates to internal/testutil to avoid fixture drift.
func SeedTestData(t testing.TB, db *sql.DB) {
	t.Helper()
	internaltestutil.SeedTestData(t, db)
}

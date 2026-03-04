package testutil

import (
	"database/sql"
	"testing"

	internaltestutil "github.com/user/llm-proxy-go/internal/testutil"
)

// NewTestDB delegates to internal/testutil to avoid schema drift.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return internaltestutil.NewTestDB(t)
}

// NewTestDBWithDefaults delegates to internal/testutil to avoid schema drift.
func NewTestDBWithDefaults(t *testing.T) *sql.DB {
	t.Helper()
	return internaltestutil.NewTestDBWithDefaults(t)
}

// SeedTestData delegates to internal/testutil to avoid fixture drift.
func SeedTestData(t *testing.T, db *sql.DB) {
	t.Helper()
	internaltestutil.SeedTestData(t, db)
}

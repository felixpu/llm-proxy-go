//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestBackupHandler_ExportImport_ModelAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sourceDB := testutil.NewTestDB(t)
	testutil.SeedTestData(t, sourceDB)
	insertModelAlias(t, sourceDB, "claude-sonnet-4-6", "claude-sonnet-4", true)
	insertModelAlias(t, sourceDB, "claude-opus-4-6", "claude-opus-4", false)

	exportHandler := NewBackupHandler(sourceDB, nil)
	exportBody := exportBackup(t, exportHandler)

	var backup BackupData
	require.NoError(t, json.Unmarshal(exportBody, &backup))
	require.Len(t, backup.ModelAliases, 2)
	assert.Equal(t, "claude-opus-4-6", backup.ModelAliases[0].AliasName)
	assert.Equal(t, "claude-opus-4", backup.ModelAliases[0].TargetModelName)
	assert.False(t, backup.ModelAliases[0].Enabled)
	assert.Equal(t, "claude-sonnet-4-6", backup.ModelAliases[1].AliasName)
	assert.Equal(t, "claude-sonnet-4", backup.ModelAliases[1].TargetModelName)
	assert.True(t, backup.ModelAliases[1].Enabled)

	targetDB := testutil.NewTestDB(t)
	importHandler := NewBackupHandler(targetDB, nil)
	importBackup(t, importHandler, exportBody)

	aliases := fetchImportedAliases(t, targetDB)
	require.Len(t, aliases, 2)
	assert.Equal(t, importedAlias{AliasName: "claude-opus-4-6", TargetModelName: "claude-opus-4", Enabled: false}, aliases[0])
	assert.Equal(t, importedAlias{AliasName: "claude-sonnet-4-6", TargetModelName: "claude-sonnet-4", Enabled: true}, aliases[1])
}

type importedAlias struct {
	AliasName       string
	TargetModelName string
	Enabled         bool
}

func insertModelAlias(t *testing.T, db *sql.DB, aliasName, targetModelName string, enabled bool) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO model_aliases (alias_name, target_model_id, enabled)
		SELECT ?, id, ? FROM models WHERE name = ?`,
		aliasName, boolInt(enabled), targetModelName,
	)
	require.NoError(t, err)
}

func exportBackup(t *testing.T, handler *BackupHandler) []byte {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/config/backup/export", nil)

	handler.Export(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.Bytes()
}

func importBackup(t *testing.T, handler *BackupHandler, payload []byte) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/config/backup/import", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Import(c)

	require.Equal(t, http.StatusOK, recorder.Code)
}

func fetchImportedAliases(t *testing.T, db *sql.DB) []importedAlias {
	t.Helper()

	rows, err := db.Query(`
		SELECT ma.alias_name, m.name, ma.enabled
		FROM model_aliases ma
		JOIN models m ON m.id = ma.target_model_id
		ORDER BY ma.alias_name COLLATE NOCASE ASC`)
	require.NoError(t, err)
	defer rows.Close()

	var aliases []importedAlias
	for rows.Next() {
		var alias importedAlias
		var enabled int
		require.NoError(t, rows.Scan(&alias.AliasName, &alias.TargetModelName, &enabled))
		alias.Enabled = enabled == 1
		aliases = append(aliases, alias)
	}
	require.NoError(t, rows.Err())
	return aliases
}

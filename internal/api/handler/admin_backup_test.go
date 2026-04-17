//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	assert.Empty(t, backup.ModelAliases[0].ProviderRef)
	assert.Empty(t, backup.ModelAliases[0].ProviderName)
	assert.False(t, backup.ModelAliases[0].Enabled)
	assert.Equal(t, "claude-sonnet-4-6", backup.ModelAliases[1].AliasName)
	assert.Equal(t, "claude-sonnet-4", backup.ModelAliases[1].TargetModelName)
	assert.Empty(t, backup.ModelAliases[1].ProviderRef)
	assert.Empty(t, backup.ModelAliases[1].ProviderName)
	assert.True(t, backup.ModelAliases[1].Enabled)

	targetDB := testutil.NewTestDB(t)
	importHandler := NewBackupHandler(targetDB, nil)
	importBackup(t, importHandler, exportBody)

	aliases := fetchImportedAliases(t, targetDB)
	require.Len(t, aliases, 2)
	assert.Equal(t, importedAlias{AliasName: "claude-opus-4-6", TargetModelName: "claude-opus-4", ProviderName: "", Enabled: false}, aliases[0])
	assert.Equal(t, importedAlias{AliasName: "claude-sonnet-4-6", TargetModelName: "claude-sonnet-4", ProviderName: "", Enabled: true}, aliases[1])
}

func TestBackupHandler_ExportImport_ModelAliases_WithProviderScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sourceDB := testutil.NewTestDB(t)
	testutil.SeedTestData(t, sourceDB)
	insertModelAliasWithProvider(t, sourceDB, "claude-sonnet-4-6", "claude-sonnet-4", "anthropic-primary", true)
	insertModelAlias(t, sourceDB, "claude-opus-4-6", "claude-opus-4", true)

	exportHandler := NewBackupHandler(sourceDB, nil)
	exportBody := exportBackup(t, exportHandler)

	var backup BackupData
	require.NoError(t, json.Unmarshal(exportBody, &backup))
	require.Len(t, backup.ModelAliases, 2)
	assert.NotEmpty(t, backup.ModelAliases[1].ProviderRef)
	assert.Equal(t, "anthropic-primary", backup.ModelAliases[1].ProviderName)

	targetDB := testutil.NewTestDB(t)
	importHandler := NewBackupHandler(targetDB, nil)
	importBackup(t, importHandler, exportBody)

	aliases := fetchImportedAliases(t, targetDB)
	require.Len(t, aliases, 2)
	assert.Equal(t, importedAlias{AliasName: "claude-opus-4-6", TargetModelName: "claude-opus-4", ProviderName: "", Enabled: true}, aliases[0])
	assert.Equal(t, importedAlias{AliasName: "claude-sonnet-4-6", TargetModelName: "claude-sonnet-4", ProviderName: "anthropic-primary", Enabled: true}, aliases[1])
}

func TestBackupHandler_ExportImport_ModelAliasProviderRef_WithDuplicateProviderName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sourceDB := testutil.NewTestDB(t)
	testutil.SeedTestData(t, sourceDB)

	// Insert a second provider with the same name to simulate ambiguous provider_name.
	res, err := sourceDB.Exec(`
		INSERT INTO providers (name, base_url, api_key, weight, max_concurrent, enabled, description)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"anthropic-primary", "https://duplicate-provider.example.com", "sk-dup-provider", 1, 5, 1, "Duplicate Name Provider")
	require.NoError(t, err)
	dupProviderID, err := res.LastInsertId()
	require.NoError(t, err)

	_, err = sourceDB.Exec(`INSERT INTO provider_models (provider_id, model_id) VALUES (?, ?)`, dupProviderID, 2)
	require.NoError(t, err)

	insertModelAliasWithProviderID(t, sourceDB, "claude-sonnet-4-6", "claude-sonnet-4", dupProviderID, true)

	exportHandler := NewBackupHandler(sourceDB, nil)
	exportBody := exportBackup(t, exportHandler)

	var backup BackupData
	require.NoError(t, json.Unmarshal(exportBody, &backup))
	require.NotEmpty(t, backup.ModelAliases)
	require.Equal(t, "provider:"+sqlInt64ToString(dupProviderID), backup.ModelAliases[0].ProviderRef)

	targetDB := testutil.NewTestDB(t)
	importHandler := NewBackupHandler(targetDB, nil)
	importBackup(t, importHandler, exportBody)

	baseURL := fetchAliasProviderBaseURL(t, targetDB, "claude-sonnet-4-6")
	assert.Equal(t, "https://duplicate-provider.example.com", baseURL)
}

type importedAlias struct {
	AliasName       string
	TargetModelName string
	ProviderName    string
	Enabled         bool
}

func insertModelAlias(t *testing.T, db *sql.DB, aliasName, targetModelName string, enabled bool) {
	t.Helper()
	insertModelAliasWithProvider(t, db, aliasName, targetModelName, "", enabled)
}

func insertModelAliasWithProvider(t *testing.T, db *sql.DB, aliasName, targetModelName, providerName string, enabled bool) {
	t.Helper()

	var err error
	if providerName == "" {
		_, err = db.Exec(`
			INSERT INTO model_aliases (alias_name, target_model_id, provider_id, enabled)
			SELECT ?, id, NULL, ? FROM models WHERE name = ?`,
			aliasName, boolInt(enabled), targetModelName,
		)
	} else {
		_, err = db.Exec(`
			INSERT INTO model_aliases (alias_name, target_model_id, provider_id, enabled)
			SELECT ?, m.id, p.id, ?
			FROM models m
			JOIN providers p ON p.name = ?
			WHERE m.name = ?`,
			aliasName, boolInt(enabled), providerName, targetModelName,
		)
	}
	require.NoError(t, err)
}

func insertModelAliasWithProviderID(t *testing.T, db *sql.DB, aliasName, targetModelName string, providerID int64, enabled bool) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO model_aliases (alias_name, target_model_id, provider_id, enabled)
		SELECT ?, m.id, ?, ?
		FROM models m
		WHERE m.name = ?`,
		aliasName, providerID, boolInt(enabled), targetModelName,
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
		SELECT ma.alias_name, m.name, p.name, ma.enabled
		FROM model_aliases ma
		JOIN models m ON m.id = ma.target_model_id
		LEFT JOIN providers p ON p.id = ma.provider_id
		ORDER BY ma.alias_name COLLATE NOCASE ASC`)
	require.NoError(t, err)
	defer rows.Close()

	var aliases []importedAlias
	for rows.Next() {
		var alias importedAlias
		var providerName sql.NullString
		var enabled int
		require.NoError(t, rows.Scan(&alias.AliasName, &alias.TargetModelName, &providerName, &enabled))
		if providerName.Valid {
			alias.ProviderName = providerName.String
		}
		alias.Enabled = enabled == 1
		aliases = append(aliases, alias)
	}
	require.NoError(t, rows.Err())
	return aliases
}

func fetchAliasProviderBaseURL(t *testing.T, db *sql.DB, aliasName string) string {
	t.Helper()

	var baseURL string
	err := db.QueryRow(`
		SELECT p.base_url
		FROM model_aliases ma
		JOIN providers p ON p.id = ma.provider_id
		WHERE ma.alias_name = ? COLLATE NOCASE
		LIMIT 1`,
		aliasName,
	).Scan(&baseURL)
	require.NoError(t, err)
	return baseURL
}

func sqlInt64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}

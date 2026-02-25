package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// BackupHandler handles configuration backup and restore.
type BackupHandler struct {
	db *sql.DB
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(db *sql.DB) *BackupHandler {
	return &BackupHandler{db: db}
}

// --- Backup data structures (override json:"-" fields) ---

// BackupData is the top-level export envelope.
type BackupData struct {
	Version         int                    `json:"version"`
	ExportedAt      string                 `json:"exported_at"`
	Models          []backupModel          `json:"models"`
	Providers       []backupProvider       `json:"providers"`
	Users           []backupUser           `json:"users"`
	APIKeys         []backupAPIKey         `json:"api_keys"`
	RoutingModels   []backupRoutingModel   `json:"routing_models"`
	RoutingRules    []backupRoutingRule    `json:"routing_rules"`
	RoutingLLMConfig map[string]any        `json:"routing_llm_config"`
	EmbeddingModels []backupEmbeddingModel `json:"embedding_models"`
	SystemConfig    backupSystemConfig     `json:"system_config"`
}

type backupModel struct {
	Name              string  `json:"name"`
	Role              string  `json:"role"`
	CostPerMtokInput  float64 `json:"cost_per_mtok_input"`
	CostPerMtokOutput float64 `json:"cost_per_mtok_output"`
	BillingMultiplier float64 `json:"billing_multiplier"`
	SupportsThinking  bool    `json:"supports_thinking"`
	Enabled           bool    `json:"enabled"`
	Weight            int     `json:"weight"`
}

type backupProvider struct {
	Name          string   `json:"name"`
	BaseURL       string   `json:"base_url"`
	APIKey        string   `json:"api_key"`
	Weight        int      `json:"weight"`
	MaxConcurrent int      `json:"max_concurrent"`
	Enabled       bool     `json:"enabled"`
	Description   string   `json:"description,omitempty"`
	ModelNames    []string `json:"model_names"`
}

type backupUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	IsActive     bool   `json:"is_active"`
}

type backupAPIKey struct {
	Name      string  `json:"name"`
	KeyHash   string  `json:"key_hash"`
	KeyFull   string  `json:"key_full"`
	KeyPrefix string  `json:"key_prefix"`
	Username  string  `json:"username"`
	IsActive  bool    `json:"is_active"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

type backupRoutingModel struct {
	ProviderName      string  `json:"provider_name"`
	ModelName         string  `json:"model_name"`
	Enabled           bool    `json:"enabled"`
	Priority          int     `json:"priority"`
	CostPerMtokInput  float64 `json:"cost_per_mtok_input"`
	CostPerMtokOutput float64 `json:"cost_per_mtok_output"`
	BillingMultiplier float64 `json:"billing_multiplier"`
	Description       string  `json:"description,omitempty"`
}

type backupRoutingRule struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Pattern     string   `json:"pattern"`
	Condition   string   `json:"condition"`
	TaskType    string   `json:"task_type"`
	Priority    int      `json:"priority"`
	IsBuiltin   bool     `json:"is_builtin"`
	Enabled     bool     `json:"enabled"`
}

type backupEmbeddingModel struct {
	Name               string `json:"name"`
	Dimension          int    `json:"dimension"`
	Description        string `json:"description,omitempty"`
	FastembedSupported bool   `json:"fastembed_supported"`
	FastembedName      string `json:"fastembed_name,omitempty"`
	IsBuiltin          bool   `json:"is_builtin"`
	Enabled            bool   `json:"enabled"`
	SortOrder          int    `json:"sort_order"`
}

type backupSystemConfig struct {
	Routing     map[string]any `json:"routing"`
	LoadBalance map[string]any `json:"load_balance"`
	HealthCheck map[string]any `json:"health_check"`
	UI          map[string]any `json:"ui"`
}

// Export handles GET /api/config/backup/export - exports all config as JSON file.
func (h *BackupHandler) Export(c *gin.Context) {
	ctx := c.Request.Context()
	data := BackupData{Version: 1, ExportedAt: time.Now().UTC().Format(time.RFC3339)}

	var err error
	if data.Models, err = h.exportModels(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export models: %v", err)})
		return
	}
	if data.Providers, err = h.exportProviders(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export providers: %v", err)})
		return
	}
	if data.Users, err = h.exportUsers(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export users: %v", err)})
		return
	}
	if data.APIKeys, err = h.exportAPIKeys(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export api_keys: %v", err)})
		return
	}
	if data.RoutingModels, err = h.exportRoutingModels(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export routing_models: %v", err)})
		return
	}
	if data.RoutingRules, err = h.exportRoutingRules(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export routing_rules: %v", err)})
		return
	}
	if data.RoutingLLMConfig, err = h.exportSingletonTable(ctx, "routing_llm_config"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export routing_llm_config: %v", err)})
		return
	}
	if data.EmbeddingModels, err = h.exportEmbeddingModels(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("export embedding_models: %v", err)})
		return
	}
	data.SystemConfig.Routing, _ = h.exportSingletonTable(ctx, "routing_config")
	data.SystemConfig.LoadBalance, _ = h.exportSingletonTable(ctx, "load_balance_config")
	data.SystemConfig.HealthCheck, _ = h.exportSingletonTable(ctx, "health_check_config")
	data.SystemConfig.UI, _ = h.exportSingletonTable(ctx, "ui_config")

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="llm-proxy-backup-%s.json"`,
		time.Now().Format("20060102-150405")))
	c.JSON(http.StatusOK, data)
}

func (h *BackupHandler) exportModels(ctx context.Context) ([]backupModel, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT name, role, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, supports_thinking, enabled, weight FROM models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupModel
	for rows.Next() {
		var m backupModel
		var st, en int
		if err := rows.Scan(&m.Name, &m.Role, &m.CostPerMtokInput, &m.CostPerMtokOutput, &m.BillingMultiplier, &st, &en, &m.Weight); err != nil {
			return nil, err
		}
		m.SupportsThinking = st == 1
		m.Enabled = en == 1
		result = append(result, m)
	}
	return result, rows.Err()
}

func (h *BackupHandler) exportProviders(ctx context.Context) ([]backupProvider, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT id, name, base_url, api_key, weight, max_concurrent, enabled, COALESCE(description,'') FROM providers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupProvider
	for rows.Next() {
		var p backupProvider
		var id int64
		var en int
		if err := rows.Scan(&id, &p.Name, &p.BaseURL, &p.APIKey, &p.Weight, &p.MaxConcurrent, &en, &p.Description); err != nil {
			return nil, err
		}
		p.Enabled = en == 1
		// Fetch associated model names
		mrows, err := h.db.QueryContext(ctx, `SELECT m.name FROM provider_models pm JOIN models m ON pm.model_id = m.id WHERE pm.provider_id = ?`, id)
		if err != nil {
			return nil, err
		}
		for mrows.Next() {
			var mn string
			if err := mrows.Scan(&mn); err != nil {
				mrows.Close()
				return nil, err
			}
			p.ModelNames = append(p.ModelNames, mn)
		}
		mrows.Close()
		result = append(result, p)
	}
	return result, rows.Err()
}

func (h *BackupHandler) exportUsers(ctx context.Context) ([]backupUser, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT username, password_hash, role, is_active FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupUser
	for rows.Next() {
		var u backupUser
		var active int
		if err := rows.Scan(&u.Username, &u.PasswordHash, &u.Role, &active); err != nil {
			return nil, err
		}
		u.IsActive = active == 1
		result = append(result, u)
	}
	return result, rows.Err()
}

func (h *BackupHandler) exportAPIKeys(ctx context.Context) ([]backupAPIKey, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT ak.name, ak.key_hash, ak.key_full, ak.key_prefix, u.username, ak.is_active, ak.expires_at FROM api_keys ak JOIN users u ON ak.user_id = u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupAPIKey
	for rows.Next() {
		var k backupAPIKey
		var active int
		var expiresAt sql.NullString
		if err := rows.Scan(&k.Name, &k.KeyHash, &k.KeyFull, &k.KeyPrefix, &k.Username, &active, &expiresAt); err != nil {
			return nil, err
		}
		k.IsActive = active == 1
		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.String
		}
		result = append(result, k)
	}
	return result, rows.Err()
}

func (h *BackupHandler) exportRoutingModels(ctx context.Context) ([]backupRoutingModel, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT p.name, rm.model_name, rm.enabled, rm.priority, rm.cost_per_mtok_input, rm.cost_per_mtok_output, rm.billing_multiplier, COALESCE(rm.description,'') FROM routing_models rm JOIN providers p ON rm.provider_id = p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupRoutingModel
	for rows.Next() {
		var m backupRoutingModel
		var en int
		if err := rows.Scan(&m.ProviderName, &m.ModelName, &en, &m.Priority, &m.CostPerMtokInput, &m.CostPerMtokOutput, &m.BillingMultiplier, &m.Description); err != nil {
			return nil, err
		}
		m.Enabled = en == 1
		result = append(result, m)
	}
	return result, rows.Err()
}

func (h *BackupHandler) exportRoutingRules(ctx context.Context) ([]backupRoutingRule, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT name, COALESCE(description,''), COALESCE(keywords,'[]'), COALESCE(pattern,''), COALESCE(condition,''), task_type, priority, is_builtin, enabled FROM routing_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupRoutingRule
	for rows.Next() {
		var r backupRoutingRule
		var keywordsJSON string
		var builtin, en int
		if err := rows.Scan(&r.Name, &r.Description, &keywordsJSON, &r.Pattern, &r.Condition, &r.TaskType, &r.Priority, &builtin, &en); err != nil {
			return nil, err
		}
		r.IsBuiltin = builtin == 1
		r.Enabled = en == 1
		_ = json.Unmarshal([]byte(keywordsJSON), &r.Keywords)
		if r.Keywords == nil {
			r.Keywords = []string{}
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (h *BackupHandler) exportEmbeddingModels(ctx context.Context) ([]backupEmbeddingModel, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT name, dimension, COALESCE(description,''), fastembed_supported, COALESCE(fastembed_name,''), is_builtin, enabled, sort_order FROM embedding_models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupEmbeddingModel
	for rows.Next() {
		var m backupEmbeddingModel
		var fs, builtin, en int
		if err := rows.Scan(&m.Name, &m.Dimension, &m.Description, &fs, &m.FastembedName, &builtin, &en, &m.SortOrder); err != nil {
			return nil, err
		}
		m.FastembedSupported = fs == 1
		m.IsBuiltin = builtin == 1
		m.Enabled = en == 1
		result = append(result, m)
	}
	return result, rows.Err()
}

// exportSingletonTable reads all columns (except id) from a single-row config table.
func (h *BackupHandler) exportSingletonTable(ctx context.Context, table string) (map[string]any, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = 1", table)
	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		return map[string]any{}, nil
	}
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	result := make(map[string]any, len(cols))
	for i, col := range cols {
		if col == "id" {
			continue
		}
		if b, ok := values[i].([]byte); ok {
			result[col] = string(b)
		} else {
			result[col] = values[i]
		}
	}
	return result, nil
}

// Import handles POST /api/config/backup/import - restores config from JSON.
func (h *BackupHandler) Import(c *gin.Context) {
	var data BackupData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid JSON: %v", err)})
		return
	}
	if data.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported backup version: %d", data.Version)})
		return
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("begin transaction: %v", err)})
		return
	}
	defer tx.Rollback()

	ctx := c.Request.Context()

	// 1. Clear dependent tables first (foreign key order)
	clearTables := []string{
		"provider_models", "api_keys", "routing_models",
		"routing_rules", "embedding_models", "models", "providers", "users",
	}
	for _, t := range clearTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", t)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("clear %s: %v", t, err)})
			return
		}
	}

	// 2. Import models → build name→ID map
	modelIDs := make(map[string]int64)
	for _, m := range data.Models {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO models (name, role, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, supports_thinking, enabled, weight) VALUES (?,?,?,?,?,?,?,?)`,
			m.Name, m.Role, m.CostPerMtokInput, m.CostPerMtokOutput, m.BillingMultiplier, boolInt(m.SupportsThinking), boolInt(m.Enabled), m.Weight)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("insert model %s: %v", m.Name, err)})
			return
		}
		id, _ := res.LastInsertId()
		modelIDs[m.Name] = id
	}

	// 3. Import providers → build name→ID map, then insert provider_models
	providerIDs := make(map[string]int64)
	if err := h.importProviders(ctx, tx, data.Providers, modelIDs, providerIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 4. Import users → build username→ID map
	userIDs := make(map[string]int64)
	for _, u := range data.Users {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO users (username, password_hash, role, is_active) VALUES (?,?,?,?)`,
			u.Username, u.PasswordHash, u.Role, boolInt(u.IsActive))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("insert user %s: %v", u.Username, err)})
			return
		}
		id, _ := res.LastInsertId()
		userIDs[u.Username] = id
	}

	// 5. Import API keys (resolve username → user_id)
	for _, k := range data.APIKeys {
		uid, ok := userIDs[k.Username]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("api_key %s references unknown user %s", k.Name, k.Username)})
			return
		}
		expiresAt := sql.NullString{}
		if k.ExpiresAt != nil {
			expiresAt = sql.NullString{String: *k.ExpiresAt, Valid: true}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO api_keys (user_id, key_hash, key_full, key_prefix, name, is_active, expires_at) VALUES (?,?,?,?,?,?,?)`,
			uid, k.KeyHash, k.KeyFull, k.KeyPrefix, k.Name, boolInt(k.IsActive), expiresAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("insert api_key %s: %v", k.Name, err)})
			return
		}
	}

	// 6. Import routing models (resolve provider_name → provider_id)
	for _, rm := range data.RoutingModels {
		pid, ok := providerIDs[rm.ProviderName]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("routing_model %s references unknown provider %s", rm.ModelName, rm.ProviderName)})
			return
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO routing_models (provider_id, model_name, enabled, priority, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, description) VALUES (?,?,?,?,?,?,?,?)`,
			pid, rm.ModelName, boolInt(rm.Enabled), rm.Priority, rm.CostPerMtokInput, rm.CostPerMtokOutput, rm.BillingMultiplier, rm.Description); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("insert routing_model %s: %v", rm.ModelName, err)})
			return
		}
	}

	// 7. Import routing rules
	for _, r := range data.RoutingRules {
		kw, _ := json.Marshal(r.Keywords)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled) VALUES (?,?,?,?,?,?,?,?,?)`,
			r.Name, r.Description, string(kw), r.Pattern, r.Condition, r.TaskType, r.Priority, boolInt(r.IsBuiltin), boolInt(r.Enabled)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("insert routing_rule %s: %v", r.Name, err)})
			return
		}
	}

	// 8. Import embedding models
	for _, m := range data.EmbeddingModels {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, enabled, sort_order) VALUES (?,?,?,?,?,?,?,?)`,
			m.Name, m.Dimension, m.Description, boolInt(m.FastembedSupported), m.FastembedName, boolInt(m.IsBuiltin), boolInt(m.Enabled), m.SortOrder); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("insert embedding_model %s: %v", m.Name, err)})
			return
		}
	}

	// 9. Update singleton config tables
	if err := h.importSingletonTable(ctx, tx, "routing_llm_config", data.RoutingLLMConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("update routing_llm_config: %v", err)})
		return
	}
	if err := h.importSingletonTable(ctx, tx, "routing_config", data.SystemConfig.Routing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("update routing_config: %v", err)})
		return
	}
	if err := h.importSingletonTable(ctx, tx, "load_balance_config", data.SystemConfig.LoadBalance); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("update load_balance_config: %v", err)})
		return
	}
	if err := h.importSingletonTable(ctx, tx, "health_check_config", data.SystemConfig.HealthCheck); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("update health_check_config: %v", err)})
		return
	}
	if err := h.importSingletonTable(ctx, tx, "ui_config", data.SystemConfig.UI); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("update ui_config: %v", err)})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("commit: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配置导入成功"})
}

// importProviders inserts providers and their provider_models associations.
func (h *BackupHandler) importProviders(ctx context.Context, tx *sql.Tx, providers []backupProvider, modelIDs map[string]int64, providerIDs map[string]int64) error {
	for _, p := range providers {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO providers (name, base_url, api_key, weight, max_concurrent, enabled, description) VALUES (?,?,?,?,?,?,?)`,
			p.Name, p.BaseURL, p.APIKey, p.Weight, p.MaxConcurrent, boolInt(p.Enabled), p.Description)
		if err != nil {
			return fmt.Errorf("insert provider %s: %v", p.Name, err)
		}
		pid, _ := res.LastInsertId()
		providerIDs[p.Name] = pid

		// Insert provider_models associations
		for _, mn := range p.ModelNames {
			mid, ok := modelIDs[mn]
			if !ok {
				return fmt.Errorf("provider %s references unknown model %s", p.Name, mn)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO provider_models (provider_id, model_id) VALUES (?,?)`, pid, mid); err != nil {
				return fmt.Errorf("insert provider_model %s/%s: %v", p.Name, mn, err)
			}
		}
	}
	return nil
}

// importSingletonTable updates a single-row config table with the given values.
func (h *BackupHandler) importSingletonTable(ctx context.Context, tx *sql.Tx, table string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(values))
	params := make([]any, 0, len(values))
	for col, val := range values {
		if col == "id" {
			continue
		}
		setClauses = append(setClauses, col+" = ?")
		params = append(params, val)
	}
	if len(setClauses) == 0 {
		return nil
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = 1", table, strings.Join(setClauses, ", "))
	_, err := tx.ExecContext(ctx, query, params...)
	return err
}

// boolInt converts bool to SQLite integer (1/0).
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Package testutil provides test utilities and fixtures for the LLM proxy service.
package testutil

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/require"
)

// NewTestDB creates an in-memory SQLite database with full schema for testing.
// The database is automatically closed when the test completes.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=ON")
	require.NoError(t, err, "failed to open test database")

	t.Cleanup(func() {
		db.Close()
	})

	// Run schema creation
	err = createSchema(db)
	require.NoError(t, err, "failed to create schema")

	return db
}

// NewTestDBWithDefaults creates a test database with default configuration data.
func NewTestDBWithDefaults(t *testing.T) *sql.DB {
	t.Helper()

	db := NewTestDB(t)

	err := insertDefaults(db)
	require.NoError(t, err, "failed to insert defaults")

	return db
}

// createSchema creates all tables for testing.
func createSchema(db *sql.DB) error {
	schema := `
-- Proxy configuration (singleton)
CREATE TABLE IF NOT EXISTS proxy_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    host TEXT DEFAULT '0.0.0.0',
    port INTEGER DEFAULT 8000
);

-- Health check configuration (singleton)
CREATE TABLE IF NOT EXISTS health_check_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER DEFAULT 1,
    interval_seconds INTEGER DEFAULT 60,
    timeout_seconds INTEGER DEFAULT 10
);

-- Load balance configuration (singleton)
CREATE TABLE IF NOT EXISTS load_balance_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    strategy TEXT DEFAULT 'conversation_hash'
);

-- Routing configuration (singleton)
CREATE TABLE IF NOT EXISTS routing_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    default_role TEXT DEFAULT 'default'
);

-- Models table
CREATE TABLE IF NOT EXISTS models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    role TEXT NOT NULL,
    cost_per_mtok_input REAL DEFAULT 0,
    cost_per_mtok_output REAL DEFAULT 0,
    billing_multiplier REAL DEFAULT 1.0,
    supports_thinking INTEGER DEFAULT 0,
    enabled INTEGER DEFAULT 1,
    weight INTEGER DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Providers table
CREATE TABLE IF NOT EXISTS providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    weight INTEGER DEFAULT 1,
    max_concurrent INTEGER DEFAULT 10,
    enabled INTEGER DEFAULT 1,
    description TEXT,
    custom_headers TEXT DEFAULT '' NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Provider-Model association
CREATE TABLE IF NOT EXISTS provider_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL,
    model_id INTEGER NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE,
    UNIQUE(provider_id, model_id)
);

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    is_active INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ip_address TEXT,
    user_agent TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- API Keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    key_hash TEXT UNIQUE NOT NULL,
    key_full TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- UI configuration (singleton)
CREATE TABLE IF NOT EXISTS ui_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    dashboard_refresh_seconds INTEGER DEFAULT 30,
    logs_refresh_seconds INTEGER DEFAULT 15
);

-- LLM routing configuration (singleton)
CREATE TABLE IF NOT EXISTS routing_llm_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER DEFAULT 0,
    primary_model_id INTEGER,
    fallback_model_id INTEGER,
    timeout_seconds INTEGER DEFAULT 30,
    cache_enabled INTEGER DEFAULT 1,
    cache_ttl_seconds INTEGER DEFAULT 300,
    cache_ttl_l3_seconds INTEGER DEFAULT 604800,
    max_tokens INTEGER DEFAULT 1024,
    temperature REAL DEFAULT 0.0,
    retry_count INTEGER DEFAULT 2,
    semantic_cache_enabled INTEGER DEFAULT 1,
    embedding_model_id INTEGER,
    similarity_threshold REAL DEFAULT 0.82,
    local_embedding_model TEXT DEFAULT 'paraphrase-multilingual-MiniLM-L12-v2',
    force_smart_routing INTEGER DEFAULT 0,
    rule_based_routing_enabled INTEGER DEFAULT 1,
    rule_fallback_strategy TEXT DEFAULT 'default',
    rule_fallback_task_type TEXT DEFAULT 'default',
    rule_fallback_model_id INTEGER,
    log_full_content INTEGER DEFAULT 1
);

-- Routing models table
CREATE TABLE IF NOT EXISTS routing_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL,
    model_name TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    priority INTEGER DEFAULT 0,
    cost_per_mtok_input REAL DEFAULT 0,
    cost_per_mtok_output REAL DEFAULT 0,
    billing_multiplier REAL DEFAULT 1.0,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

-- Embedding models configuration
CREATE TABLE IF NOT EXISTS embedding_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    dimension INTEGER NOT NULL,
    description TEXT,
    fastembed_supported INTEGER DEFAULT 0,
    fastembed_name TEXT,
    is_builtin INTEGER DEFAULT 0,
    enabled INTEGER DEFAULT 1,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Worker registry
CREATE TABLE IF NOT EXISTS worker_registry (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT UNIQUE NOT NULL,
    pid INTEGER NOT NULL,
    is_primary INTEGER DEFAULT 0,
    last_heartbeat TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Shared state
CREATE TABLE IF NOT EXISTS shared_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT
);

-- Request logs table
CREATE TABLE IF NOT EXISTS request_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT UNIQUE NOT NULL,
    user_id INTEGER NOT NULL,
    api_key_id INTEGER,
    model_name TEXT NOT NULL,
    endpoint_name TEXT NOT NULL,
    task_type TEXT,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    latency_ms REAL DEFAULT 0,
    cost REAL DEFAULT 0,
    status_code INTEGER,
    success INTEGER DEFAULT 1,
    stream INTEGER DEFAULT 0,
    message_preview TEXT DEFAULT '',
    request_content TEXT DEFAULT '',
    response_content TEXT DEFAULT '',
    routing_method TEXT DEFAULT '',
    routing_reason TEXT DEFAULT '',
    matched_rule_id INTEGER,
    matched_rule_name TEXT DEFAULT '',
    all_matches TEXT DEFAULT '[]',
    is_inaccurate INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL
);

-- Cache stats timeseries
CREATE TABLE IF NOT EXISTS routing_cache_stats_timeseries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    l1_hits INTEGER DEFAULT 0,
    l1_misses INTEGER DEFAULT 0,
    l1_size INTEGER DEFAULT 0,
    l2_hits INTEGER DEFAULT 0,
    l2_misses INTEGER DEFAULT 0,
    l3_hits INTEGER DEFAULT 0,
    l3_misses INTEGER DEFAULT 0,
    llm_calls INTEGER DEFAULT 0,
    llm_errors INTEGER DEFAULT 0,
    period_seconds INTEGER DEFAULT 60
);

-- Embedding cache table
CREATE TABLE IF NOT EXISTS routing_embedding_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content_hash TEXT UNIQUE NOT NULL,
    content_preview TEXT,
    embedding TEXT NOT NULL,
    task_type TEXT,
    reason TEXT,
    hit_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_hit_at TIMESTAMP
);

-- Routing rules table
CREATE TABLE IF NOT EXISTS routing_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    keywords TEXT DEFAULT '[]',
    pattern TEXT DEFAULT '',
    condition TEXT DEFAULT '',
    task_type TEXT NOT NULL,
    priority INTEGER DEFAULT 50,
    is_builtin INTEGER DEFAULT 0,
    enabled INTEGER DEFAULT 1,
    hit_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON provider_models(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_models_model_id ON provider_models(model_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_routing_models_provider_id ON routing_models(provider_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON request_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
`
	_, err := db.Exec(schema)
	return err
}

// insertDefaults inserts default configuration data.
func insertDefaults(db *sql.DB) error {
	defaults := `
INSERT OR IGNORE INTO proxy_config (id, host, port) VALUES (1, '0.0.0.0', 8000);
INSERT OR IGNORE INTO health_check_config (id, enabled, interval_seconds, timeout_seconds) VALUES (1, 1, 60, 10);
INSERT OR IGNORE INTO load_balance_config (id, strategy) VALUES (1, 'conversation_hash');
INSERT OR IGNORE INTO routing_config (id, default_role) VALUES (1, 'default');
INSERT OR IGNORE INTO ui_config (id, dashboard_refresh_seconds, logs_refresh_seconds) VALUES (1, 30, 15);
INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0);
`
	_, err := db.Exec(defaults)
	return err
}

// SeedTestData populates the database with sample test data.
func SeedTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	// Insert test users
	_, err := db.Exec(`
		INSERT INTO users (username, password_hash, role, is_active)
		VALUES
			('admin', '$2a$10$hashedpassword1', 'admin', 1),
			('testuser', '$2a$10$hashedpassword2', 'user', 1),
			('inactive', '$2a$10$hashedpassword3', 'user', 0)
	`)
	require.NoError(t, err)

	// Insert test models
	_, err = db.Exec(`
		INSERT INTO models (name, role, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, supports_thinking, enabled, weight)
		VALUES
			('claude-3-haiku', 'simple', 0.25, 1.25, 1.0, 0, 1, 100),
			('claude-sonnet-4', 'default', 3.0, 15.0, 1.0, 0, 1, 100),
			('claude-opus-4', 'complex', 15.0, 75.0, 1.0, 1, 1, 100),
			('disabled-model', 'default', 1.0, 5.0, 1.0, 0, 0, 50)
	`)
	require.NoError(t, err)

	// Insert test providers
	_, err = db.Exec(`
		INSERT INTO providers (name, base_url, api_key, weight, max_concurrent, enabled, description)
		VALUES
			('anthropic-primary', 'https://api.anthropic.com', 'sk-ant-test-key-1', 2, 10, 1, 'Primary Anthropic'),
			('anthropic-backup', 'https://api.anthropic.com', 'sk-ant-test-key-2', 1, 5, 1, 'Backup Anthropic'),
			('disabled-provider', 'https://disabled.example.com', 'sk-disabled', 1, 5, 0, 'Disabled')
	`)
	require.NoError(t, err)

	// Link providers to models
	_, err = db.Exec(`
		INSERT INTO provider_models (provider_id, model_id)
		VALUES
			(1, 1), (1, 2), (1, 3),
			(2, 1), (2, 2)
	`)
	require.NoError(t, err)

	// Insert test API keys
	_, err = db.Exec(`
		INSERT INTO api_keys (user_id, key_hash, key_full, key_prefix, name, is_active)
		VALUES
			(1, 'hash_admin_key_1', 'sk-admin-full-key', 'sk-admin', 'Admin Key', 1),
			(2, 'hash_user_key_1', 'sk-user-full-key', 'sk-user', 'User Key', 1),
			(2, 'hash_user_key_2', 'sk-user-revoked', 'sk-rev', 'Revoked Key', 0)
	`)
	require.NoError(t, err)
}

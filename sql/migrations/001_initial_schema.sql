-- Initial schema: Core tables for LLM proxy service
-- Migrated from Python version's schema.py

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

-- Providers table (unified business + routing providers)
CREATE TABLE IF NOT EXISTS providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    weight INTEGER DEFAULT 1,
    max_concurrent INTEGER DEFAULT 10,
    enabled INTEGER DEFAULT 1,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Provider-Model association (many-to-many)
CREATE TABLE IF NOT EXISTS provider_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL,
    model_id INTEGER NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE,
    UNIQUE(provider_id, model_id)
);

-- Legacy endpoints table (kept for data migration)
CREATE TABLE IF NOT EXISTS endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    weight INTEGER DEFAULT 1,
    max_concurrent INTEGER DEFAULT 10,
    enabled INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE,
    UNIQUE(model_id, name)
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
    logs_refresh_seconds INTEGER DEFAULT 30
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
    force_smart_routing INTEGER DEFAULT 0
);

-- Routing providers table (legacy, data migrated to providers)
CREATE TABLE IF NOT EXISTS routing_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    weight INTEGER DEFAULT 1,
    max_concurrent INTEGER DEFAULT 10,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

-- Worker registry (multi-worker coordination)
CREATE TABLE IF NOT EXISTS worker_registry (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT UNIQUE NOT NULL,
    pid INTEGER NOT NULL,
    is_primary INTEGER DEFAULT 0,
    last_heartbeat TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Shared state (cross-worker data sharing)
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
    text_hash TEXT UNIQUE NOT NULL,
    embedding BLOB NOT NULL,
    model_name TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_hit_at TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_endpoints_model_id ON endpoints(model_id);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON provider_models(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_models_model_id ON provider_models(model_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_routing_models_provider_id ON routing_models(provider_id);
CREATE INDEX IF NOT EXISTS idx_worker_registry_heartbeat ON worker_registry(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_worker_registry_primary ON worker_registry(is_primary);
CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON request_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_model_name ON request_logs(model_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_endpoint_name ON request_logs(endpoint_name);
CREATE INDEX IF NOT EXISTS idx_cache_stats_timestamp ON routing_cache_stats_timeseries(timestamp);

-- Default configuration data
INSERT OR IGNORE INTO proxy_config (id, host, port) VALUES (1, '0.0.0.0', 8000);
INSERT OR IGNORE INTO health_check_config (id, enabled, interval_seconds, timeout_seconds) VALUES (1, 1, 60, 10);
INSERT OR IGNORE INTO load_balance_config (id, strategy) VALUES (1, 'conversation_hash');
INSERT OR IGNORE INTO routing_config (id, default_role) VALUES (1, 'default');
INSERT OR IGNORE INTO ui_config (id, dashboard_refresh_seconds, logs_refresh_seconds) VALUES (1, 30, 30);
INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0);

-- Built-in embedding models
INSERT OR IGNORE INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, sort_order)
VALUES ('paraphrase-multilingual-MiniLM-L12-v2', 384, '多语言通用模型，中英文支持好（推荐）', 1, 'sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2', 1, 0);
INSERT OR IGNORE INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, sort_order)
VALUES ('paraphrase-multilingual-mpnet-base-v2', 768, '多语言高精度模型', 0, NULL, 1, 1);
INSERT OR IGNORE INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, sort_order)
VALUES ('all-MiniLM-L6-v2', 384, '轻量英文模型', 1, 'sentence-transformers/all-MiniLM-L6-v2', 1, 2);
INSERT OR IGNORE INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, sort_order)
VALUES ('all-MiniLM-L12-v2', 384, '英文模型，比 L6 更准确', 1, 'sentence-transformers/all-MiniLM-L12-v2', 1, 3);
INSERT OR IGNORE INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, sort_order)
VALUES ('BAAI/bge-small-zh-v1.5', 512, '中文优化小模型', 1, 'BAAI/bge-small-zh-v1.5', 1, 4);
INSERT OR IGNORE INTO embedding_models (name, dimension, description, fastembed_supported, fastembed_name, is_builtin, sort_order)
VALUES ('BAAI/bge-base-zh-v1.5', 768, '中文优化基础模型', 1, 'BAAI/bge-base-zh-v1.5', 1, 5);

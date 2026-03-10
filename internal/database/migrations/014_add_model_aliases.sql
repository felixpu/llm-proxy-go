CREATE TABLE IF NOT EXISTS model_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alias_name TEXT NOT NULL COLLATE NOCASE,
    target_model_id INTEGER NOT NULL,
    enabled INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (target_model_id) REFERENCES models(id) ON DELETE CASCADE,
    UNIQUE(alias_name)
);

CREATE INDEX IF NOT EXISTS idx_model_aliases_target_model_id ON model_aliases(target_model_id);

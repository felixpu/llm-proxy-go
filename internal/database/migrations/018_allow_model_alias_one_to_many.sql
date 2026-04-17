-- Allow one alias name to map to multiple target models.
-- SQLite does not support dropping UNIQUE constraints directly, so rebuild table.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS model_aliases_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alias_name TEXT NOT NULL COLLATE NOCASE,
    target_model_id INTEGER NOT NULL,
    provider_id INTEGER,
    enabled INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (target_model_id) REFERENCES models(id) ON DELETE CASCADE,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

INSERT INTO model_aliases_new (id, alias_name, target_model_id, provider_id, enabled, created_at, updated_at)
SELECT id, alias_name, target_model_id, NULL AS provider_id, enabled, created_at, updated_at
FROM model_aliases;

DROP TABLE model_aliases;
ALTER TABLE model_aliases_new RENAME TO model_aliases;

CREATE INDEX IF NOT EXISTS idx_model_aliases_target_model_id ON model_aliases(target_model_id);
CREATE INDEX IF NOT EXISTS idx_model_aliases_alias_name_enabled ON model_aliases(alias_name COLLATE NOCASE, enabled);
CREATE INDEX IF NOT EXISTS idx_model_aliases_provider_id ON model_aliases(provider_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_model_aliases_alias_target_provider ON model_aliases(
    alias_name COLLATE NOCASE,
    target_model_id,
    IFNULL(provider_id, 0)
);

PRAGMA foreign_keys = ON;

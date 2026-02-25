-- Fix routing_embedding_cache table schema to match repository code
-- Drop old table and recreate with correct columns

DROP TABLE IF EXISTS routing_embedding_cache;

CREATE TABLE IF NOT EXISTS routing_embedding_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content_hash TEXT UNIQUE NOT NULL,
    content_preview TEXT,
    embedding TEXT NOT NULL,
    task_type TEXT NOT NULL,
    reason TEXT,
    hit_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_hit_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_embedding_cache_content_hash ON routing_embedding_cache(content_hash);
CREATE INDEX IF NOT EXISTS idx_embedding_cache_created_at ON routing_embedding_cache(created_at);

-- Request log model trace fields for requested/resolved model visibility
-- SQLite does not support ALTER TABLE ... ADD COLUMN IF NOT EXISTS

ALTER TABLE request_logs ADD COLUMN requested_model TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN resolved_model TEXT DEFAULT '';

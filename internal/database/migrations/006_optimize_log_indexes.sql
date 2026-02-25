-- Composite indexes to speed up log queries filtered by created_at + common columns.
-- created_at is stored as ISO 8601 text ("2006-01-02 15:04:05"), so string comparison
-- is equivalent to temporal ordering and these indexes can be used for range scans.

CREATE INDEX IF NOT EXISTS idx_request_logs_created_model ON request_logs(created_at, model_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_endpoint ON request_logs(created_at, endpoint_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_success ON request_logs(created_at, success);

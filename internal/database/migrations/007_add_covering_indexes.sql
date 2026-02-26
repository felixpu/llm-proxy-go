-- 007_add_covering_indexes.sql
-- Add covering indexes to speed up common log queries.

-- Descending index for ORDER BY created_at DESC pagination queries.
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at_desc ON request_logs(created_at DESC);

-- Composite index for inaccurate log queries (ListInaccurate, GetRoutingAggregation).
CREATE INDEX IF NOT EXISTS idx_request_logs_inaccurate_created ON request_logs(is_inaccurate, created_at DESC);

-- Composite index for success + time range filtering.
CREATE INDEX IF NOT EXISTS idx_request_logs_success_created ON request_logs(success, created_at DESC);

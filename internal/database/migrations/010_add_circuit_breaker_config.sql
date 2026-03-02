-- 010: Add circuit breaker configuration to health_check_config table
-- Adds columns for circuit breaker thresholds and cooldown settings

ALTER TABLE health_check_config ADD COLUMN cb_enabled INTEGER DEFAULT 1 NOT NULL;
ALTER TABLE health_check_config ADD COLUMN cb_consecutive_failures INTEGER DEFAULT 5 NOT NULL;
ALTER TABLE health_check_config ADD COLUMN cb_permanent_error_threshold INTEGER DEFAULT 3 NOT NULL;
ALTER TABLE health_check_config ADD COLUMN cb_cooldown_seconds INTEGER DEFAULT 60 NOT NULL;
ALTER TABLE health_check_config ADD COLUMN cb_half_open_max_requests INTEGER DEFAULT 3 NOT NULL;

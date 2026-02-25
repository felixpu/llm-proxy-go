-- Update logs page default refresh interval from 30s to 15s
-- Improves real-time monitoring experience

UPDATE ui_config
SET logs_refresh_seconds = 15
WHERE id = 1;

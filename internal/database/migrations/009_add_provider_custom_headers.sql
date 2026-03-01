-- 009: Add custom_headers column to providers table
-- Stores JSON map of custom HTTP headers to send with upstream requests
ALTER TABLE providers ADD COLUMN custom_headers TEXT DEFAULT '' NOT NULL;

-- Add cache statistics columns for Anthropic Prompt Caching support
ALTER TABLE request_logs ADD COLUMN cache_creation_input_tokens INTEGER DEFAULT 0;
ALTER TABLE request_logs ADD COLUMN cache_read_input_tokens INTEGER DEFAULT 0;

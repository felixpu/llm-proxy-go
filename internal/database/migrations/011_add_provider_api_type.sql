-- Add api_type column to providers table
-- Supported values: 'auto', 'anthropic_messages', 'anthropic_responses', 'openai_chat'
-- Default 'auto' means auto-detect on first use
ALTER TABLE providers ADD COLUMN api_type TEXT DEFAULT 'auto' NOT NULL;

-- Add api_type column to routing_models table (for routing LLM calls)
-- Inherits from provider if not specified (empty string means inherit)
ALTER TABLE routing_models ADD COLUMN api_type TEXT DEFAULT '' NOT NULL;

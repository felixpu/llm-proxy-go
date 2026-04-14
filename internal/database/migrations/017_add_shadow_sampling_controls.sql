ALTER TABLE routing_llm_config ADD COLUMN shadow_sample_rate REAL DEFAULT 0.2;
ALTER TABLE routing_llm_config ADD COLUMN shadow_max_qps INTEGER DEFAULT 20;

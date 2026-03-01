-- Routing analysis reports table for storing LLM-based analysis results.
CREATE TABLE IF NOT EXISTS routing_analysis_reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_used TEXT NOT NULL,
    time_range_start DATETIME,
    time_range_end DATETIME,
    total_logs INTEGER NOT NULL DEFAULT 0,
    analyzed_logs INTEGER NOT NULL DEFAULT 0,
    report TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

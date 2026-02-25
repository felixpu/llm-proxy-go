-- Add routing rules table and rule-based routing configuration
-- Supports: keyword matching, regex patterns, condition expressions (DSL)

-- Routing rules table
CREATE TABLE IF NOT EXISTS routing_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    keywords TEXT DEFAULT '[]',       -- JSON array of keyword strings
    pattern TEXT DEFAULT '',          -- Regex pattern
    condition TEXT DEFAULT '',        -- DSL condition expression
    task_type TEXT NOT NULL,          -- simple / default / complex
    priority INTEGER DEFAULT 50,
    is_builtin INTEGER DEFAULT 0,
    enabled INTEGER DEFAULT 1,
    hit_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_routing_rules_enabled ON routing_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_routing_rules_task_type ON routing_rules(task_type);
CREATE INDEX IF NOT EXISTS idx_routing_rules_priority ON routing_rules(priority DESC);

-- Add rule-based routing fields to routing_llm_config
ALTER TABLE routing_llm_config ADD COLUMN rule_based_routing_enabled INTEGER DEFAULT 1;
ALTER TABLE routing_llm_config ADD COLUMN rule_fallback_strategy TEXT DEFAULT 'default';
ALTER TABLE routing_llm_config ADD COLUMN rule_fallback_task_type TEXT DEFAULT 'default';
ALTER TABLE routing_llm_config ADD COLUMN rule_fallback_model_id INTEGER;

-- Seed builtin rules
INSERT OR IGNORE INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled)
VALUES (
    'architecture_keywords',
    '架构设计类关键词匹配',
    '["架构","设计","重构","优化","规划","创建项目","微服务"]',
    '',
    '',
    'complex',
    100,
    1,
    1
);

INSERT OR IGNORE INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled)
VALUES (
    'long_message',
    '超长消息自动归类为复杂任务',
    '[]',
    '',
    'len(message) > 3000',
    'complex',
    80,
    1,
    1
);

INSERT OR IGNORE INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled)
VALUES (
    'multi_file_operation',
    '多文件/批量操作匹配',
    '[]',
    '(?i)(多个文件|批量|所有.*文件|整个.*目录)',
    '',
    'complex',
    90,
    1,
    1
);

INSERT OR IGNORE INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled)
VALUES (
    'simple_operations',
    '简单操作关键词匹配（短消息且不含分析）',
    '["列出","查看","翻译","转换","格式化","显示"]',
    '',
    'len(message) < 200 AND NOT contains(message, "分析")',
    'simple',
    100,
    1,
    1
);

INSERT OR IGNORE INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled)
VALUES (
    'file_listing',
    '文件列表操作模式匹配',
    '[]',
    '^(ls|列出|显示|查看).*(文件|目录|文件夹)',
    '',
    'simple',
    90,
    1,
    1
);

INSERT OR IGNORE INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled)
VALUES (
    'code_with_analysis',
    '包含代码块的分析/解释请求',
    '[]',
    '',
    'has_code_block(message) AND (contains(message, "分析") OR contains(message, "解释"))',
    'default',
    70,
    1,
    1
);

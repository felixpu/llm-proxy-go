-- 增强请求日志表：添加路由决策详情字段
-- 支持路由方式追踪、规则匹配记录、完整内容存储
-- 注意：此迁移依赖 schema_migrations 表防止重复执行
--       SQLite 不支持 ALTER TABLE ADD COLUMN IF NOT EXISTS

-- 添加新字段到 request_logs 表
ALTER TABLE request_logs ADD COLUMN message_preview TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN request_content TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN response_content TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN routing_method TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN routing_reason TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN matched_rule_id INTEGER;
ALTER TABLE request_logs ADD COLUMN matched_rule_name TEXT DEFAULT '';
ALTER TABLE request_logs ADD COLUMN all_matches TEXT DEFAULT '[]';
ALTER TABLE request_logs ADD COLUMN is_inaccurate INTEGER DEFAULT 0;

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_request_logs_routing_method ON request_logs(routing_method);
CREATE INDEX IF NOT EXISTS idx_request_logs_matched_rule_id ON request_logs(matched_rule_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_is_inaccurate ON request_logs(is_inaccurate);

-- 在 routing_llm_config 表添加完整内容记录开关
ALTER TABLE routing_llm_config ADD COLUMN log_full_content INTEGER DEFAULT 1;

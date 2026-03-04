# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased] - 2026-03-04

### 新增功能
- feat(frontend): 路由分析页面增强 - 按规则分组 + 禁用状态提示 (fb0c55c)
- feat(frontend): 日志列表卡片式重构 + Rich Tooltip (75f2c57)
- feat(service): add routing analyzer v2 with batched analysis and improved prompts (09b0d5b)
- feat(models): enhance analysis report structure with warnings and priority (ebfbe37)
- feat(api): add delete analysis report endpoint (a41d96c)
- feat(service): enhance health checker with improved circuit breaker and error classification (8b7408e)
- feat(frontend): 添加 Prompt Caching 统计展示 (fec33fd)
- feat: 添加熔断器（Circuit Breaker）支持 (378fed3)
- feat: 添加 Provider API 类型支持和适配器系统 (5be8e73)
- feat: 添加 Anthropic Prompt Caching 完整支持 (f4bbfa6)

### 问题修复
- fix(service): improve API endpoint detection and error messages (715c394)
- fix(service): fix routing analysis empty content and inaccurate total_logs (098f833)
- fix: 修正缓存命中率计算逻辑 (f0811ae)

### 重构
- refactor(service): 统一路由消息提取逻辑 + 跨角色回退 (bd9ac23)
- refactor(repository): include all logs in analysis query, not just routed ones (77a0e64)
- refactor(service): extract shared LLM call logic and improve API adapter (be0e667)

### 文档更新
- docs: add health check improvement proposal with circuit breaker design (704e129)

### UI/样式优化
- style(frontend): enhance routing analysis UI with modern design (00d4b25)
- style(frontend): improve code formatting and add pagination support for analysis reports (b98cc6a)

### 测试
- test(service): update proxy tests to use new API adapter structure (62bc99f)

<!-- Last updated: fb0c55c -->

# 更新记录 - 2026-03-04

## 变更概述
本次更新主要包含三大核心功能：路由分析器 v2 增强、Anthropic Prompt Caching 完整支持、以及熔断器（Circuit Breaker）机制。同时对前端 UI 进行了卡片式重构，提升用户体验。

## 详细变更

### 新增功能

#### 路由分析器 v2
- 批量分析支持，改进提示词质量
- 按规则分组展示，支持禁用状态提示
- 增强分析报告结构，添加警告和优先级
- 支持删除分析报告
- 改进 API 端点检测和错误消息

#### Anthropic Prompt Caching
- 完整支持 Anthropic Prompt Caching 功能
- 前端展示缓存统计（缓存写入、缓存读取、命中率）
- 修正缓存命中率计算逻辑

#### 熔断器（Circuit Breaker）
- 添加熔断器支持，提升系统稳定性
- 改进健康检查器，支持错误分类
- 详细设计文档（docs/proposals/）

#### Provider API 适配器系统
- 添加 Provider API 类型支持
- 提取共享 LLM 调用逻辑
- 改进 API 适配器结构

### 问题修复
- 修正路由分析空内容和不准确的 total_logs 统计
- 改进 API 端点检测和错误消息
- 修正缓存命中率计算逻辑

### 重构
- 统一路由消息提取逻辑，支持跨角色回退
- 分析查询包含所有日志，不仅限于已路由的日志
- 提取共享 LLM 调用逻辑，改进 API 适配器

### 文档更新
- 添加健康检查改进提案（熔断器设计）

### UI/样式优化
- 日志列表卡片式重构，添加 Rich Tooltip
- 路由分析 UI 现代化设计
- 改进代码格式，添加分页支持

### 测试
- 更新 proxy 测试以使用新的 API 适配器结构

## 影响范围
**需要人工补充**
- 影响的模块：
  - 路由分析器（service/routing_analyzer_v2.go）
  - 消息提取器（service/message_extractor.go）
  - 代理服务（service/proxy.go）
  - 健康检查器（service/health_checker.go）
  - 前端页面（frontend/js/vue/pages/）
- 破坏性变更：
  - 无
- 迁移指南：
  - 无需迁移，向后兼容

## 相关 Commit
- fb0c55c - feat(frontend): 路由分析页面增强 - 按规则分组 + 禁用状态提示
- bd9ac23 - refactor(service): 统一路由消息提取逻辑 + 跨角色回退
- 75f2c57 - feat(frontend): 日志列表卡片式重构 + Rich Tooltip
- 704e129 - docs: add health check improvement proposal with circuit breaker design
- 09b0d5b - feat(service): add routing analyzer v2 with batched analysis and improved prompts
- 715c394 - fix(service): improve API endpoint detection and error messages
- 77a0e64 - refactor(repository): include all logs in analysis query, not just routed ones
- ebfbe37 - feat(models): enhance analysis report structure with warnings and priority
- 00d4b25 - style(frontend): enhance routing analysis UI with modern design
- 098f833 - fix(service): fix routing analysis empty content and inaccurate total_logs
- 62bc99f - test(service): update proxy tests to use new API adapter structure
- b98cc6a - style(frontend): improve code formatting and add pagination support for analysis reports
- a41d96c - feat(api): add delete analysis report endpoint
- 8b7408e - feat(service): enhance health checker with improved circuit breaker and error classification
- be0e667 - refactor(service): extract shared LLM call logic and improve API adapter
- f0811ae - fix: 修正缓存命中率计算逻辑
- 378fed3 - feat: 添加熔断器（Circuit Breaker）支持
- 5be8e73 - feat: 添加 Provider API 类型支持和适配器系统
- f4bbfa6 - feat: 添加 Anthropic Prompt Caching 完整支持
- fec33fd - feat(frontend): 添加 Prompt Caching 统计展示

---
生成时间：2026-03-04
Commit 范围：HEAD~20..fb0c55c

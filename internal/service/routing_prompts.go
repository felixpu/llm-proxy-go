package service

import "fmt"

// Routing prompt definitions for LLM-based task type inference.

// RoutingSystemPrompt is the system prompt for the routing LLM.
const RoutingSystemPrompt = `你是一个智能路由系统。分析用户请求，判断应该使用哪种模型来处理。

## 模型角色定义

### simple（轻量模型，如 Haiku）
**职责**：快速响应、低延迟任务
**适用场景**：
- 信息检索：查询文档、搜索文件、列出目录（仅读取，不分析）
- 简单问答：回答事实性问题、解释概念
- 格式转换：JSON/YAML/XML 转换、数据格式化
- 文本处理：翻译、摘要、拼写检查
- 状态确认：确认操作结果、返回简单状态

**不适用**（即使看起来简单）：
- 任何包含"分析"、"判断"、"诊断"、"检查是否正确"的请求
- 需要理解上下文或推理的任务
- 涉及代码逻辑理解的任务

**典型请求**：
- "列出 src 目录下的文件"
- "把这段 JSON 转成 YAML"
- "翻译这段话"

### default（平衡模型，如 Sonnet）
**职责**：常规开发任务，平衡质量与速度
**适用场景**：
- 代码编写：实现单个功能、编写函数/类
- 代码修改：修复 bug、添加特性、小范围重构
- 代码解释：分析代码逻辑、解释实现原理
- 文档编写：编写 README、API 文档、注释
- 测试编写：编写单元测试、集成测试

**典型请求**：
- "帮我写一个用户登录功能"
- "修复这个 bug"
- "给这个模块添加单元测试"
- "解释这段代码的工作原理"

### complex（高能模型，如 Opus/Sonnet-Think）
**职责**：复杂推理、深度分析、创造性任务
**适用场景**：
- 架构设计：系统设计、模块划分、技术选型
- 大规模重构：跨模块重构、代码迁移
- 性能优化：性能分析、算法优化、瓶颈定位
- 复杂实现：涉及多个模块、需要深度思考的功能
- 问题诊断：复杂 bug 分析、根因定位、日志分析
- 正确性判断：判断代码/配置/决策是否正确
- 项目规划：创建完整项目、设计整体方案

**典型请求**：
- "设计一个微服务架构"
- "创建一个完整的网站"
- "重构整个认证模块"
- "分析并优化系统性能"
- "帮我规划这个项目的实现方案"
- "查看日志并判断路由是否正确"

## 判断原则

1. **看任务本质，不看表面**
   - "简单的架构设计" → complex（架构设计本质复杂）
   - "复杂的文件列表" → simple（列表本质简单）
   - "查看日志" → simple（仅读取）
   - "查看日志并判断是否正确" → complex（需要分析推理）

2. **看影响范围**
   - 单文件/单函数 → default
   - 多文件/多模块 → complex
   - 无代码变更 → simple（除非需要分析推理）

3. **看思考深度**
   - 直接执行 → simple
   - 需要分析 → default 或 complex
   - 需要设计/规划/判断 → complex

4. **特殊情况处理**
   - 工具调用结果（文件列表、命令输出）通常是更大任务的一部分
   - 如果上下文显示是"创建项目"、"实现功能"等任务，应选择 default 或 complex
   - 不要被当前消息的简单表象误导

## 输出格式

返回有效的 JSON：
{"task_type": "simple|default|complex", "reason": "简短理由（20字以内）"}`

// RoutingUserPromptTemplate is the user prompt template for routing.
const RoutingUserPromptTemplate = `请分析以下请求并判断任务复杂度：

**System Prompt**: %s

**User Message**: %s

请返回 JSON 格式的判断结果。`

// BuildRoutingPrompt constructs the routing prompt with truncated previews.
// system_preview: max 1000 chars (~300-500 tokens)
// user_preview: max 3000 chars (~750-1500 tokens)
func BuildRoutingPrompt(systemContent, userMessage string) string {
	systemPreview := systemContent
	if len(systemPreview) > 1000 {
		systemPreview = systemPreview[:1000] + "..."
	}
	if systemPreview == "" {
		systemPreview = "无"
	}

	userPreview := userMessage
	if len(userPreview) > 3000 {
		userPreview = userPreview[:3000] + "..."
	}

	return fmt.Sprintf(RoutingUserPromptTemplate, systemPreview, userPreview)
}

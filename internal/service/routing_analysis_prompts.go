package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

// AnalysisSystemPrompt defines the LLM's role for routing analysis.
const AnalysisSystemPrompt = `你是一个路由规则分析专家。分析请求日志数据，识别路由规则的问题并提供优化建议。

**重要：你必须只返回有效的 JSON 格式，不要包含任何解释性文本、Markdown 标记或其他内容。**

你的响应必须是一个纯 JSON 对象，直接以 { 开头，以 } 结尾。不要使用 Markdown 代码块（三个反引号加 json），不要添加任何前缀或后缀文本。

## 你的任务

根据提供的路由规则和请求日志数据，分析以下方面：
1. 规则是否准确匹配了请求（false_positive / false_negative）
2. 规则优先级是否合理（priority_conflict）
3. 是否有冗余规则（redundant_rule）
4. 是否有规则过于宽泛（overly_broad）
5. **直接指定模型的请求是否应该添加路由规则**（missing_rule）

## 路由方式说明

日志中的 routing_method 字段表示路由方式：
- **rule**: 通过规则匹配路由（有 matched_rule_name）
- **llm**: 规则未匹配，LLM 语义路由回退
- **空值**: 用户直接指定模型名称，未使用路由系统

**重要**：routing_method 为空的请求也需要分析！这些请求可能：
- 存在明显的模式，应该添加路由规则
- 频繁使用某个模型，提示需要优化默认路由
- 绕过了路由系统，可能导致成本或性能问题

## 问题类型

- **false_positive**: 规则匹配了不应匹配的请求（例如简单查询被路由到 complex 模型）
- **false_negative**: 请求应被某规则匹配但未匹配（例如复杂任务被路由到 simple 模型）
- **priority_conflict**: 多条规则匹配同一请求，优先级设置不当
- **redundant_rule**: 两条规则功能重叠，可以合并
- **overly_broad**: 规则匹配范围过大，误匹配率高
- **missing_rule**: 大量直接指定模型的请求存在明显模式，应添加规则

## 严重程度

- **high**: 直接影响用户体验或成本（如复杂任务用了 simple 模型）
- **medium**: 可能导致次优路由但不严重
- **low**: 优化建议，非必须修复

## 建议类型

- **modify**: 修改现有规则（关键词、正则、条件、优先级）— 必须提供完整 rule_spec
- **add**: 添加新规则覆盖未匹配场景 — 必须提供完整 rule_spec
- **delete**: 删除冗余或无效规则 — 不需要 rule_spec
- **reorder**: 调整规则优先级 — rule_spec 中只需 priority

注意：is_builtin=true 的内置规则不可直接修改或删除，系统会自动创建同名自定义规则覆盖。

## 建议优先级

每条建议必须包含优先级，帮助用户决定处理顺序：
- **high**: 紧急优化，直接影响成本或用户体验（如误匹配导致成本浪费 >20%）
- **medium**: 重要优化，可改善路由准确性（如 LLM 回退率 >30%）
- **low**: 可选优化，锦上添花（如规则可读性改进）

## 输出格式

**你的响应必须是且只能是以下 JSON 对象，不要有任何其他文本：**

{
  "summary": {
    "rule_match_rate": 0.75,
    "llm_fallback_rate": 0.15,
    "inaccurate_rate": 0.05,
    "top_task_types": {"default": 50, "simple": 30, "complex": 20}
  },
  "issues": [
    {
      "type": "false_positive",
      "severity": "high",
      "rule_name": "rule_name_here",
      "description": "描述问题",
      "examples": ["示例请求1", "示例请求2"]
    }
  ],
  "recommendations": [
    {
      "action": "modify|add|delete|reorder",
      "rule_name": "rule_name_here",
      "priority": "high|medium|low",
      "reason": "为什么需要这个优化（业务价值、影响范围）",
      "description": "具体怎么做（操作步骤）",
      "details": "补充说明（可选）",
      "rule_spec": {
        "keywords": ["关键词1", "关键词2"],
        "pattern": "正则表达式（可选）",
        "condition": "DSL条件（可选）",
        "task_type": "simple|default|complex",
        "priority": 80,
        "enabled": true
      }
    }
  ],
  "conclusion": "总结分析结果和主要建议"
}

**关键要求：**
1. 顶层 key 必须且只能是 summary、issues、recommendations、conclusion 这四个
2. 不要嵌套在其他 key 下（如 "analysis"、"result"、"data" 等）
3. 不要使用 Markdown 代码块（三个反引号加 json）
4. 不要添加任何解释性文本
5. 直接返回有效的 JSON 对象
6. **CRITICAL**: recommendations 数组必须存在，不能为 null。如果没有建议，返回空数组 []
7. **CRITICAL**: 对于每个识别的问题（issues），必须提供至少一条对应的优化建议（recommendations）
8. **CRITICAL**: 建议类型为 add 或 modify 时，必须提供完整的 rule_spec 对象（包含 keywords、task_type、priority、enabled 字段）`

// AnalysisUserPromptTemplate is the user prompt template for analysis.
const AnalysisUserPromptTemplate = `请分析以下路由规则和请求日志数据：

## 当前路由规则

%s

## 统计概览

- 总日志数: %d
- 分析日志数: %d
- 规则匹配数: %d（%.1f%%）
- LLM 回退数: %d（%.1f%%）
- 直接指定模型数: %d（%.1f%%）
- 标记不准确数: %d（%.1f%%）

## 请求日志数据

%s

请返回 JSON 格式的分析结果。`

// BuildAnalysisPrompt constructs the full analysis prompt.
func BuildAnalysisPrompt(
	rules []*models.RoutingRule,
	entries []*models.ExtractedLogEntry,
	totalLogs, analyzedLogs int,
) string {
	// Format rules
	rulesText := formatRulesForPrompt(rules)

	// Compute stats
	var ruleMatch, llmFallback, directModel, inaccurate int
	for _, e := range entries {
		switch e.RoutingMethod {
		case "rule":
			ruleMatch++
		case "llm":
			llmFallback++
		case "":
			directModel++
		}
		if e.IsInaccurate {
			inaccurate++
		}
	}
	total := len(entries)
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return float64(n) * 100.0 / float64(total)
	}

	// Format log entries (truncate to fit context)
	logsText := formatLogsForPrompt(entries)

	return fmt.Sprintf(AnalysisUserPromptTemplate,
		rulesText,
		totalLogs, analyzedLogs,
		ruleMatch, pct(ruleMatch),
		llmFallback, pct(llmFallback),
		directModel, pct(directModel),
		inaccurate, pct(inaccurate),
		logsText,
	)
}

func formatRulesForPrompt(rules []*models.RoutingRule) string {
	if len(rules) == 0 {
		return "（无自定义规则）"
	}
	var b strings.Builder
	for i, r := range rules {
		if i > 0 {
			b.WriteString("\n")
		}
		status := "enabled"
		if !r.Enabled {
			status = "disabled"
		}
		builtinTag := ""
		if r.IsBuiltin {
			builtinTag = ", is_builtin=true"
		}
		b.WriteString(fmt.Sprintf("- **%s** [%s, priority=%d, task=%s%s]",
			r.Name, status, r.Priority, r.TaskType, builtinTag))
		if len(r.Keywords) > 0 {
			b.WriteString(fmt.Sprintf("\n  keywords: %s", strings.Join(r.Keywords, ", ")))
		}
		if r.Pattern != "" {
			b.WriteString(fmt.Sprintf("\n  pattern: %s", r.Pattern))
		}
		if r.Condition != "" {
			b.WriteString(fmt.Sprintf("\n  condition: %s", r.Condition))
		}
		if r.Description != "" {
			b.WriteString(fmt.Sprintf("\n  desc: %s", r.Description))
		}
	}
	return b.String()
}

func formatLogsForPrompt(entries []*models.ExtractedLogEntry) string {
	var b strings.Builder
	maxBytes := 60000 // Keep prompt within reasonable size
	for i, e := range entries {
		line := formatSingleEntry(i+1, e)
		if b.Len()+len(line) > maxBytes {
			b.WriteString(fmt.Sprintf("\n... (truncated, %d more entries)", len(entries)-i))
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func formatSingleEntry(idx int, e *models.ExtractedLogEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n### #%d (id=%d)\n", idx, e.ID))
	b.WriteString(fmt.Sprintf("- task: %s | method: %s", e.TaskType, e.RoutingMethod))
	if e.MatchedRuleName != "" {
		b.WriteString(fmt.Sprintf(" | rule: %s", e.MatchedRuleName))
	}
	if e.IsInaccurate {
		b.WriteString(" | **INACCURATE**")
	}
	b.WriteString("\n")
	if e.MessageSummary != "" {
		b.WriteString(fmt.Sprintf("- context: %s\n", e.MessageSummary))
	}
	msg := e.UserMessage
	if len(msg) > 300 {
		msg = msg[:300] + "..."
	}
	b.WriteString(fmt.Sprintf("- message: %s\n", msg))
	return b.String()
}

// extractAnalysisJSON extracts a JSON object from LLM response text.
// Unlike extractJSON (designed for simple task_type responses), this handles
// nested JSON objects with arrays and sub-objects.
func extractAnalysisJSON(text string) string {
	text = strings.TrimSpace(text)

	// Strategy 1: Direct parse - check if entire response is valid JSON
	if json.Valid([]byte(text)) && strings.HasPrefix(text, "{") {
		return text
	}

	// Strategy 2: Try markdown code block (```json ... ```)
	// Use greedy matching for the last ``` to handle nested code blocks
	re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n(\\{.+\\})\\s*\\n?```")
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		extracted := strings.TrimSpace(m[1])
		if json.Valid([]byte(extracted)) {
			return extracted
		}
	}

	// Strategy 3: Find the outermost JSON object using brace counting
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(text); i++ {
		if escaped {
			escaped = false
			continue
		}
		ch := text[i]
		if ch == '\\' && inStr {
			escaped = true
			continue
		}
		if ch == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				extracted := text[start : i+1]
				if json.Valid([]byte(extracted)) {
					return extracted
				}
				return ""
			}
		}
	}
	return ""
}

// ParseAnalysisResponse extracts the structured report from LLM response text.
func ParseAnalysisResponse(text string) (*models.AnalysisReport, error) {
	jsonStr := extractAnalysisJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in analysis response (response length: %d)", len(text))
	}

	// Try parsing the expected format first
	var raw struct {
		Summary         *models.AnalysisSummary          `json:"summary"`
		Issues          []models.AnalysisIssue           `json:"issues"`
		Recommendations []models.AnalysisRecommendation  `json:"recommendations"`
		Conclusion      string                           `json:"conclusion"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parse analysis JSON: %w (JSON preview: %s)", err, truncateForLog(jsonStr, 200))
	}

	// If all fields are empty, the LLM may have wrapped output in a nested key.
	// Try common wrapper keys: "analysis", "result", "report", "response", "data".
	if raw.Summary == nil && len(raw.Issues) == 0 && len(raw.Recommendations) == 0 && raw.Conclusion == "" {
		raw = tryUnwrapNestedJSON(jsonStr, raw)
	}

	// Ensure slices are never nil so JSON serialization produces [] instead of null.
	issues := raw.Issues
	if issues == nil {
		issues = []models.AnalysisIssue{}
	}
	recs := raw.Recommendations
	if recs == nil {
		recs = []models.AnalysisRecommendation{}
	}

	return &models.AnalysisReport{
		Summary:         raw.Summary,
		Issues:          issues,
		Recommendations: recs,
		Conclusion:      raw.Conclusion,
	}, nil
}

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// tryUnwrapNestedJSON attempts to find the expected fields inside a nested wrapper key.
func tryUnwrapNestedJSON(jsonStr string, fallback struct {
	Summary         *models.AnalysisSummary          `json:"summary"`
	Issues          []models.AnalysisIssue           `json:"issues"`
	Recommendations []models.AnalysisRecommendation  `json:"recommendations"`
	Conclusion      string                           `json:"conclusion"`
}) struct {
	Summary         *models.AnalysisSummary          `json:"summary"`
	Issues          []models.AnalysisIssue           `json:"issues"`
	Recommendations []models.AnalysisRecommendation  `json:"recommendations"`
	Conclusion      string                           `json:"conclusion"`
} {
	// Parse into a generic map to inspect top-level keys
	var generic map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &generic); err != nil {
		return fallback
	}

	// Common wrapper keys that LLMs may use
	wrapperKeys := []string{"analysis", "result", "report", "response", "data"}
	for _, key := range wrapperKeys {
		inner, ok := generic[key]
		if !ok {
			continue
		}
		var unwrapped struct {
			Summary         *models.AnalysisSummary          `json:"summary"`
			Issues          []models.AnalysisIssue           `json:"issues"`
			Recommendations []models.AnalysisRecommendation  `json:"recommendations"`
			Conclusion      string                           `json:"conclusion"`
		}
		if err := json.Unmarshal(inner, &unwrapped); err == nil {
			if unwrapped.Summary != nil || len(unwrapped.Issues) > 0 || len(unwrapped.Recommendations) > 0 || unwrapped.Conclusion != "" {
				return unwrapped
			}
		}
	}

	return fallback
}

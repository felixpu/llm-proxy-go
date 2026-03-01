package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

// AnalysisSystemPrompt defines the LLM's role for routing analysis.
const AnalysisSystemPrompt = `你是一个路由规则分析专家。分析请求日志数据，识别路由规则的问题并提供优化建议。

## 你的任务

根据提供的路由规则和请求日志数据，分析以下方面：
1. 规则是否准确匹配了请求（false_positive / false_negative）
2. 规则优先级是否合理（priority_conflict）
3. 是否有冗余规则（redundant_rule）
4. 是否有规则过于宽泛（overly_broad）

## 问题类型

- **false_positive**: 规则匹配了不应匹配的请求（例如简单查询被路由到 complex 模型）
- **false_negative**: 请求应被某规则匹配但未匹配（例如复杂任务被路由到 simple 模型）
- **priority_conflict**: 多条规则匹配同一请求，优先级设置不当
- **redundant_rule**: 两条规则功能重叠，可以合并
- **overly_broad**: 规则匹配范围过大，误匹配率高

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

## 输出格式

返回有效的 JSON：
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
      "description": "修改建议",
      "details": "具体修改内容",
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
}`

// AnalysisUserPromptTemplate is the user prompt template for analysis.
const AnalysisUserPromptTemplate = `请分析以下路由规则和请求日志数据：

## 当前路由规则

%s

## 统计概览

- 总日志数: %d
- 分析日志数: %d
- 规则匹配数: %d（%.1f%%）
- LLM 回退数: %d（%.1f%%）
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
	var ruleMatch, llmFallback, inaccurate int
	for _, e := range entries {
		switch e.RoutingMethod {
		case "rule":
			ruleMatch++
		case "llm":
			llmFallback++
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
	maxBytes := 15000 // Keep prompt within reasonable size
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

// ParseAnalysisResponse extracts the structured report from LLM response text.
func ParseAnalysisResponse(text string) (*models.AnalysisReport, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in analysis response")
	}

	var raw struct {
		Summary         *models.AnalysisSummary          `json:"summary"`
		Issues          []models.AnalysisIssue           `json:"issues"`
		Recommendations []models.AnalysisRecommendation  `json:"recommendations"`
		Conclusion      string                           `json:"conclusion"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parse analysis JSON: %w", err)
	}

	return &models.AnalysisReport{
		Summary:         raw.Summary,
		Issues:          raw.Issues,
		Recommendations: raw.Recommendations,
		Conclusion:      raw.Conclusion,
	}, nil
}

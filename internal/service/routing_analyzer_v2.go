package service

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// AnalysisStrategy defines how to handle large log volumes.
type AnalysisStrategy string

const (
	// StrategySinglePass: Analyze all logs in one LLM call (up to context limit)
	StrategySinglePass AnalysisStrategy = "single_pass"

	// StrategyBatched: Split logs into batches, analyze separately, then merge
	StrategyBatched AnalysisStrategy = "batched"

	// StrategyHierarchical: First aggregate statistics, then deep-dive on issues
	StrategyHierarchical AnalysisStrategy = "hierarchical"
)

// AnalysisConfig controls analysis behavior.
type AnalysisConfig struct {
	// MaxLogsPerBatch: Maximum logs to send in one LLM call
	MaxLogsPerBatch int

	// MaxPromptTokens: Estimated max tokens for prompt (leave room for response)
	MaxPromptTokens int

	// Strategy: How to handle large volumes
	Strategy AnalysisStrategy

	// PrioritizeInaccurate: Show full content for inaccurate logs
	PrioritizeInaccurate bool
}

// DefaultAnalysisConfig returns sensible defaults for Claude Opus/Sonnet.
func DefaultAnalysisConfig() AnalysisConfig {
	return AnalysisConfig{
		MaxLogsPerBatch:      500,  // Increased from 200
		MaxPromptTokens:      150000, // ~75% of 200K context for Claude
		Strategy:             StrategyHierarchical,
		PrioritizeInaccurate: true,
	}
}

// EstimatePromptTokens roughly estimates token count (1 token ≈ 4 chars for Chinese/English mix).
func EstimatePromptTokens(text string) int {
	return len(text) / 4
}

// BuildHierarchicalPrompt creates a two-phase analysis prompt:
// Phase 1: Statistical overview with aggregated patterns
// Phase 2: Detailed issue analysis with examples
func BuildHierarchicalPrompt(
	rules []*models.RoutingRule,
	entries []*models.ExtractedLogEntry,
	totalLogs, analyzedLogs int,
	config AnalysisConfig,
) string {
	var b strings.Builder

	// Clear instruction at the beginning
	b.WriteString("请分析以下路由规则和请求日志数据，识别问题并提供优化建议。\n\n")

	// Phase 1: Statistical Overview
	b.WriteString("# 路由规则分析数据\n\n")
	b.WriteString("## 统计概览\n\n")
	b.WriteString("**注意：以下统计均基于分析样本计算，非全量日志。**\n\n")
	b.WriteString(fmt.Sprintf("- 总日志数: %d\n", totalLogs))
	b.WriteString(fmt.Sprintf("- 分析日志数: %d\n", analyzedLogs))

	// Aggregate statistics
	stats := aggregateLogStats(entries)
	b.WriteString(fmt.Sprintf("- 规则匹配率: %.1f%% (%d/%d)\n",
		stats.RuleMatchRate*100, stats.RuleMatchCount, len(entries)))
	b.WriteString(fmt.Sprintf("- LLM 回退率: %.1f%% (%d/%d)\n",
		stats.LLMFallbackRate*100, stats.LLMFallbackCount, len(entries)))
	b.WriteString(fmt.Sprintf("- 不准确率: %.1f%% (%d/%d)\n",
		stats.InaccurateRate*100, stats.InaccurateCount, len(entries)))

	// Task type distribution
	b.WriteString("\n### 任务类型分布\n\n")
	for taskType, count := range stats.TaskTypeDistribution {
		pct := float64(count) * 100.0 / float64(len(entries))
		b.WriteString(fmt.Sprintf("- %s: %d (%.1f%%)\n", taskType, count, pct))
	}

	// Rule match distribution
	b.WriteString("\n### 规则匹配分布\n\n")
	for ruleName, count := range stats.RuleMatchDistribution {
		pct := float64(count) * 100.0 / float64(stats.RuleMatchCount)
		b.WriteString(fmt.Sprintf("- %s: %d (%.1f%%)\n", ruleName, count, pct))
	}

	// Phase 2: Current Rules
	b.WriteString("\n## 当前路由规则\n\n")
	b.WriteString(formatRulesForPrompt(rules))

	// Phase 3: Detailed Log Examples
	b.WriteString("\n## 详细日志样本\n\n")
	b.WriteString(formatLogsWithPriority(entries, config))

	// Clear output format requirement at the end
	b.WriteString("\n\n---\n\n")
	b.WriteString("请严格按照 System Prompt 中定义的 JSON 格式返回分析结果。")
	b.WriteString("不要返回任何解释性文本，只返回纯 JSON 对象。")

	return b.String()
}

// LogStats holds aggregated statistics.
type LogStats struct {
	RuleMatchCount        int
	RuleMatchRate         float64
	LLMFallbackCount      int
	LLMFallbackRate       float64
	InaccurateCount       int
	InaccurateRate        float64
	TaskTypeDistribution  map[string]int
	RuleMatchDistribution map[string]int
}

// aggregateLogStats computes statistics from log entries.
func aggregateLogStats(entries []*models.ExtractedLogEntry) LogStats {
	stats := LogStats{
		TaskTypeDistribution:  make(map[string]int),
		RuleMatchDistribution: make(map[string]int),
	}

	for _, e := range entries {
		// Routing method
		switch e.RoutingMethod {
		case "rule":
			stats.RuleMatchCount++
			if e.MatchedRuleName != "" {
				stats.RuleMatchDistribution[e.MatchedRuleName]++
			}
		case "llm":
			stats.LLMFallbackCount++
		}

		// Inaccurate
		if e.IsInaccurate {
			stats.InaccurateCount++
		}

		// Task type
		stats.TaskTypeDistribution[e.TaskType]++
	}

	total := len(entries)
	if total > 0 {
		stats.RuleMatchRate = float64(stats.RuleMatchCount) / float64(total)
		stats.LLMFallbackRate = float64(stats.LLMFallbackCount) / float64(total)
		stats.InaccurateRate = float64(stats.InaccurateCount) / float64(total)
	}

	return stats
}

// formatLogsWithPriority formats logs with priority for inaccurate ones.
func formatLogsWithPriority(entries []*models.ExtractedLogEntry, config AnalysisConfig) string {
	var b strings.Builder

	// Separate inaccurate and normal logs
	var inaccurate, normal []*models.ExtractedLogEntry
	for _, e := range entries {
		if e.IsInaccurate {
			inaccurate = append(inaccurate, e)
		} else {
			normal = append(normal, e)
		}
	}

	// Show all inaccurate logs with full content
	if len(inaccurate) > 0 {
		b.WriteString("### 不准确的日志（需重点关注）\n\n")
		for i, e := range inaccurate {
			b.WriteString(formatDetailedEntry(i+1, e, true))
		}
	}

	// Show sample of normal logs
	if len(normal) > 0 {
		b.WriteString("\n### 正常日志样本\n\n")
		maxNormal := 200 // Show up to 200 normal logs for better analysis coverage
		if config.MaxPromptTokens > 0 {
			// Dynamic limit based on token budget: reserve ~40% for normal logs
			tokenBudget := config.MaxPromptTokens * 40 / 100
			// Estimate ~100 tokens per log entry
			dynamicMax := tokenBudget / 100
			if dynamicMax > maxNormal {
				maxNormal = dynamicMax
			}
		}
		if len(normal) > maxNormal {
			// Sample evenly
			step := float64(len(normal)) / float64(maxNormal)
			for i := 0; i < maxNormal; i++ {
				idx := int(float64(i) * step)
				b.WriteString(formatDetailedEntry(i+1, normal[idx], false))
			}
			b.WriteString(fmt.Sprintf("\n... (省略 %d 条正常日志)\n", len(normal)-maxNormal))
		} else {
			for i, e := range normal {
				b.WriteString(formatDetailedEntry(i+1, e, false))
			}
		}
	}

	return b.String()
}

// formatDetailedEntry formats a single log entry with optional full content.
func formatDetailedEntry(idx int, e *models.ExtractedLogEntry, showFullMessage bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n#### #%d (id=%d)\n", idx, e.ID))
	b.WriteString(fmt.Sprintf("- **任务类型**: %s\n", e.TaskType))
	b.WriteString(fmt.Sprintf("- **路由方式**: %s", e.RoutingMethod))
	if e.MatchedRuleName != "" {
		b.WriteString(fmt.Sprintf(" (规则: %s)", e.MatchedRuleName))
	}
	b.WriteString("\n")

	if e.IsInaccurate {
		b.WriteString("- **标记**: ⚠️ 不准确\n")
	}

	if e.MessageSummary != "" {
		b.WriteString(fmt.Sprintf("- **上下文**: %s\n", e.MessageSummary))
	}

	msg := e.UserMessage
	if !showFullMessage && len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	b.WriteString(fmt.Sprintf("- **消息内容**: %s\n", msg))

	return b.String()
}

// runSinglePassAnalysis performs analysis in a single LLM call (for small datasets).
func (a *RoutingAnalyzer) runSinglePassAnalysis(
	ctx context.Context,
	taskID string,
	rules []*models.RoutingRule,
	entries []*models.ExtractedLogEntry,
	totalLogs int,
	modelCfg *models.RoutingModelWithProvider,
	config AnalysisConfig,
) (*models.AnalysisReport, error) {
	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "building_prompt"
		t.Progress = 40
	})

	// Build hierarchical prompt
	userPrompt := BuildHierarchicalPrompt(rules, entries, totalLogs, len(entries), config)

	a.updateTask(taskID, func(t *models.AnalysisTask) {
		t.Stage = "calling_llm"
		t.Progress = 50
	})

	// Call LLM
	llmResponse, err := a.callAnalysisModel(ctx, userPrompt, modelCfg)
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	a.logger.Debug("LLM analysis raw response",
		zap.String("task_id", taskID),
		zap.Int("response_length", len(llmResponse)),
		zap.String("response_preview", truncateForLog(llmResponse, 500)),
	)

	// Parse response
	report, err := ParseAnalysisResponse(llmResponse)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return report, nil
}

// BatchedAnalysis splits logs into batches and analyzes separately.
func (a *RoutingAnalyzer) runBatchedAnalysis(
	ctx context.Context,
	taskID string,
	rules []*models.RoutingRule,
	entries []*models.ExtractedLogEntry,
	totalLogs int,
	modelCfg *models.RoutingModelWithProvider,
	config AnalysisConfig,
) (*models.AnalysisReport, error) {
	batchSize := config.MaxLogsPerBatch
	numBatches := (len(entries) + batchSize - 1) / batchSize

	// Memory monitoring: record baseline
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	baselineAlloc := memStats.Alloc
	const maxMemoryIncreaseMB = 500 // 500MB threshold

	a.logger.Info("starting batched analysis",
		zap.String("task_id", taskID),
		zap.Int("total_entries", len(entries)),
		zap.Int("batch_size", batchSize),
		zap.Int("num_batches", numBatches),
		zap.Uint64("baseline_memory_mb", baselineAlloc/1024/1024))

	var allIssues []models.AnalysisIssue
	var allRecommendations []models.AnalysisRecommendation
	var summaries []*models.AnalysisSummary
	var warnings []string
	var failedBatches int
	var firstError error
	var firstRawResponse string

	for i := 0; i < numBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[start:end]

		// Memory check before processing batch
		runtime.ReadMemStats(&memStats)
		currentAlloc := memStats.Alloc
		memoryIncreaseMB := (currentAlloc - baselineAlloc) / 1024 / 1024

		if memoryIncreaseMB > maxMemoryIncreaseMB {
			warning := fmt.Sprintf("⚠️ 内存使用超过阈值 (%d MB)，在批次 %d/%d 处提前终止分析",
				memoryIncreaseMB, i+1, numBatches)
			warnings = append(warnings, warning)
			a.logger.Warn("memory threshold exceeded, stopping batched analysis",
				zap.String("task_id", taskID),
				zap.Int("batch", i+1),
				zap.Uint64("memory_increase_mb", memoryIncreaseMB))
			break
		}

		a.updateTask(taskID, func(t *models.AnalysisTask) {
			t.Stage = fmt.Sprintf("analyzing_batch_%d/%d", i+1, numBatches)
			t.Progress = 45 + int(float64(i)*40.0/float64(numBatches))
		})

		// Build prompt for this batch
		prompt := BuildHierarchicalPrompt(rules, batch, totalLogs, len(batch), config)

		// Call LLM
		llmResponse, err := a.callAnalysisModel(ctx, prompt, modelCfg)
		if err != nil {
			failedBatches++
			if firstError == nil {
				firstError = fmt.Errorf("LLM call failed: %w", err)
			}
			warning := fmt.Sprintf("批次 %d/%d LLM 调用失败: %v", i+1, numBatches, err)
			warnings = append(warnings, warning)
			a.logger.Warn("batch LLM call failed",
				zap.Int("batch", i+1),
				zap.Error(err))
			continue
		}

		// Parse response
		report, err := ParseAnalysisResponse(llmResponse)
		if err != nil {
			failedBatches++
			if firstError == nil {
				firstError = fmt.Errorf("response parse failed: %w", err)
				// Truncate response for logging
				if len(llmResponse) > 500 {
					firstRawResponse = llmResponse[:500] + "..."
				} else {
					firstRawResponse = llmResponse
				}
			}
			warning := fmt.Sprintf("批次 %d/%d 响应解析失败: %v", i+1, numBatches, err)
			warnings = append(warnings, warning)
			a.logger.Warn("failed to parse batch response",
				zap.Int("batch", i+1),
				zap.Error(err),
				zap.String("raw_response_preview", truncateString(llmResponse, 200)))
			continue
		}

		// Accumulate results
		if report.Summary != nil {
			summaries = append(summaries, report.Summary)
		}
		allIssues = append(allIssues, report.Issues...)
		allRecommendations = append(allRecommendations, report.Recommendations...)

		// Log memory usage after each batch
		if i%5 == 0 || i == numBatches-1 {
			runtime.ReadMemStats(&memStats)
			a.logger.Debug("batch memory usage",
				zap.Int("batch", i+1),
				zap.Uint64("current_alloc_mb", memStats.Alloc/1024/1024),
				zap.Uint64("increase_mb", (memStats.Alloc-baselineAlloc)/1024/1024))
		}
	}

	// Final memory report
	runtime.ReadMemStats(&memStats)
	finalMemoryIncreaseMB := (memStats.Alloc - baselineAlloc) / 1024 / 1024
	a.logger.Info("batched analysis completed",
		zap.String("task_id", taskID),
		zap.Int("successful_batches", numBatches-failedBatches),
		zap.Int("failed_batches", failedBatches),
		zap.Uint64("final_memory_increase_mb", finalMemoryIncreaseMB))

	// Check if too many batches failed
	failureRate := float64(failedBatches) / float64(numBatches)
	if failureRate > 0.5 {
		errMsg := fmt.Sprintf("批量分析失败率过高 (%.1f%%, %d/%d 批次失败)，无法生成可靠的分析报告",
			failureRate*100, failedBatches, numBatches)
		if firstError != nil {
			errMsg += fmt.Sprintf("\n\n第一个批次失败原因: %v", firstError)
			if firstRawResponse != "" {
				errMsg += fmt.Sprintf("\n\nLLM 原始响应（前 500 字符）:\n%s", firstRawResponse)
			}
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Add summary warning if some batches failed
	if failedBatches > 0 {
		warnings = append([]string{
			fmt.Sprintf("⚠️ 部分批次分析失败 (%d/%d)，结果可能不完整", failedBatches, numBatches),
		}, warnings...)
	}

	// Merge results
	mergedSummary := mergeSummaries(summaries, len(entries))
	deduplicatedIssues := deduplicateIssues(allIssues)
	deduplicatedRecs := deduplicateRecommendations(allRecommendations)

	return &models.AnalysisReport{
		Summary:         mergedSummary,
		Issues:          deduplicatedIssues,
		Recommendations: deduplicatedRecs,
		Conclusion:      generateMergedConclusion(deduplicatedIssues, deduplicatedRecs),
		Warnings:        warnings,
	}, nil
}

// mergeSummaries combines multiple batch summaries into one.
func mergeSummaries(summaries []*models.AnalysisSummary, totalEntries int) *models.AnalysisSummary {
	if len(summaries) == 0 {
		return nil
	}

	merged := &models.AnalysisSummary{
		TopTaskTypes: make(map[string]int),
	}

	var totalRuleMatch, totalLLMFallback, totalInaccurate float64
	for _, s := range summaries {
		totalRuleMatch += s.RuleMatchRate
		totalLLMFallback += s.LLMFallbackRate
		totalInaccurate += s.InaccurateRate

		for taskType, count := range s.TopTaskTypes {
			merged.TopTaskTypes[taskType] += count
		}
	}

	n := float64(len(summaries))
	merged.RuleMatchRate = totalRuleMatch / n
	merged.LLMFallbackRate = totalLLMFallback / n
	merged.InaccurateRate = totalInaccurate / n

	return merged
}

// deduplicateIssues removes duplicate issues based on rule_name + type.
func deduplicateIssues(issues []models.AnalysisIssue) []models.AnalysisIssue {
	seen := make(map[string]bool)
	var result []models.AnalysisIssue

	for _, issue := range issues {
		key := fmt.Sprintf("%s:%s", issue.RuleName, issue.Type)
		if !seen[key] {
			seen[key] = true
			result = append(result, issue)
		}
	}

	return result
}

// deduplicateRecommendations removes duplicate recommendations based on rule_name + action.
func deduplicateRecommendations(recs []models.AnalysisRecommendation) []models.AnalysisRecommendation {
	seen := make(map[string]bool)
	var result []models.AnalysisRecommendation

	for _, rec := range recs {
		key := fmt.Sprintf("%s:%s", rec.RuleName, rec.Action)
		if !seen[key] {
			seen[key] = true
			result = append(result, rec)
		}
	}

	return result
}

// generateMergedConclusion creates a summary conclusion from merged results.
func generateMergedConclusion(issues []models.AnalysisIssue, recs []models.AnalysisRecommendation) string {
	highIssues := 0
	for _, issue := range issues {
		if issue.Severity == "high" {
			highIssues++
		}
	}

	if highIssues > 0 {
		return fmt.Sprintf("发现 %d 个高优先级问题和 %d 条优化建议，建议优先处理高优先级问题。",
			highIssues, len(recs))
	}

	if len(issues) > 0 {
		return fmt.Sprintf("发现 %d 个问题和 %d 条优化建议，整体路由规则运行良好，建议根据优先级逐步优化。",
			len(issues), len(recs))
	}

	return "路由规则运行良好，未发现明显问题。"
}

// truncateString truncates a string to maxLen characters for logging.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

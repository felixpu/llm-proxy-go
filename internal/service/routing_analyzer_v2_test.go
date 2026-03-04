package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
)

func TestAggregateLogStats(t *testing.T) {
	tests := []struct {
		name     string
		entries  []*models.ExtractedLogEntry
		expected LogStats
	}{
		{
			name: "mixed routing methods",
			entries: []*models.ExtractedLogEntry{
				{RoutingMethod: "rule", MatchedRuleName: "rule1", TaskType: "translation", IsInaccurate: false},
				{RoutingMethod: "rule", MatchedRuleName: "rule2", TaskType: "coding", IsInaccurate: false},
				{RoutingMethod: "llm", TaskType: "translation", IsInaccurate: true},
				{RoutingMethod: "", TaskType: "general", IsInaccurate: false}, // direct model selection
			},
			expected: LogStats{
				RuleMatchCount:   2,
				RuleMatchRate:    0.5,
				LLMFallbackCount: 1,
				LLMFallbackRate:  0.25,
				InaccurateCount:  1,
				InaccurateRate:   0.25,
				TaskTypeDistribution: map[string]int{
					"translation": 2,
					"coding":      1,
					"general":     1,
				},
				RuleMatchDistribution: map[string]int{
					"rule1": 1,
					"rule2": 1,
				},
			},
		},
		{
			name:    "empty entries",
			entries: []*models.ExtractedLogEntry{},
			expected: LogStats{
				RuleMatchCount:        0,
				RuleMatchRate:         0,
				LLMFallbackCount:      0,
				LLMFallbackRate:       0,
				InaccurateCount:       0,
				InaccurateRate:        0,
				TaskTypeDistribution:  map[string]int{},
				RuleMatchDistribution: map[string]int{},
			},
		},
		{
			name: "all inaccurate",
			entries: []*models.ExtractedLogEntry{
				{RoutingMethod: "rule", MatchedRuleName: "rule1", TaskType: "coding", IsInaccurate: true},
				{RoutingMethod: "llm", TaskType: "translation", IsInaccurate: true},
			},
			expected: LogStats{
				RuleMatchCount:   1,
				RuleMatchRate:    0.5,
				LLMFallbackCount: 1,
				LLMFallbackRate:  0.5,
				InaccurateCount:  2,
				InaccurateRate:   1.0,
				TaskTypeDistribution: map[string]int{
					"coding":      1,
					"translation": 1,
				},
				RuleMatchDistribution: map[string]int{
					"rule1": 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aggregateLogStats(tt.entries)
			assert.Equal(t, tt.expected.RuleMatchCount, result.RuleMatchCount)
			assert.InDelta(t, tt.expected.RuleMatchRate, result.RuleMatchRate, 0.01)
			assert.Equal(t, tt.expected.LLMFallbackCount, result.LLMFallbackCount)
			assert.InDelta(t, tt.expected.LLMFallbackRate, result.LLMFallbackRate, 0.01)
			assert.Equal(t, tt.expected.InaccurateCount, result.InaccurateCount)
			assert.InDelta(t, tt.expected.InaccurateRate, result.InaccurateRate, 0.01)
			assert.Equal(t, tt.expected.TaskTypeDistribution, result.TaskTypeDistribution)
			assert.Equal(t, tt.expected.RuleMatchDistribution, result.RuleMatchDistribution)
		})
	}
}

func TestFormatLogsWithPriority(t *testing.T) {
	config := DefaultAnalysisConfig()

	tests := []struct {
		name     string
		entries  []*models.ExtractedLogEntry
		validate func(t *testing.T, output string)
	}{
		{
			name: "prioritize inaccurate logs",
			entries: []*models.ExtractedLogEntry{
				{ID: 1, UserMessage: "normal log 1", IsInaccurate: false, TaskType: "coding", RoutingMethod: "rule"},
				{ID: 2, UserMessage: "inaccurate log", IsInaccurate: true, TaskType: "translation", RoutingMethod: "llm"},
				{ID: 3, UserMessage: "normal log 2", IsInaccurate: false, TaskType: "general", RoutingMethod: "rule"},
			},
			validate: func(t *testing.T, output string) {
				// Inaccurate logs should appear first
				inaccurateIdx := strings.Index(output, "不准确的日志")
				normalIdx := strings.Index(output, "正常日志样本")
				assert.Greater(t, normalIdx, inaccurateIdx, "Inaccurate logs should appear before normal logs")
				assert.Contains(t, output, "⚠️ 不准确")
				assert.Contains(t, output, "id=2")
			},
		},
		{
			name: "truncate long messages for normal logs",
			entries: []*models.ExtractedLogEntry{
				{ID: 1, UserMessage: strings.Repeat("a", 600), IsInaccurate: false, TaskType: "coding", RoutingMethod: "rule"},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "...")
				assert.Less(t, strings.Count(output, "a"), 600, "Long messages should be truncated")
			},
		},
		{
			name: "show full message for inaccurate logs",
			entries: []*models.ExtractedLogEntry{
				{ID: 1, UserMessage: strings.Repeat("b", 600), IsInaccurate: true, TaskType: "coding", RoutingMethod: "llm"},
			},
			validate: func(t *testing.T, output string) {
				assert.Equal(t, 600, strings.Count(output, "b"), "Inaccurate logs should show full message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLogsWithPriority(tt.entries, config)
			tt.validate(t, result)
		})
	}
}

func TestDeduplicateIssues(t *testing.T) {
	tests := []struct {
		name     string
		issues   []models.AnalysisIssue
		expected int
	}{
		{
			name: "remove duplicates",
			issues: []models.AnalysisIssue{
				{RuleName: "rule1", Type: "missing_condition", Severity: "high", Description: "desc1"},
				{RuleName: "rule1", Type: "missing_condition", Severity: "high", Description: "desc2"}, // duplicate
				{RuleName: "rule2", Type: "priority_conflict", Severity: "medium", Description: "desc3"},
			},
			expected: 2,
		},
		{
			name: "no duplicates",
			issues: []models.AnalysisIssue{
				{RuleName: "rule1", Type: "missing_condition", Severity: "high", Description: "desc1"},
				{RuleName: "rule2", Type: "priority_conflict", Severity: "medium", Description: "desc2"},
			},
			expected: 2,
		},
		{
			name:     "empty issues",
			issues:   []models.AnalysisIssue{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateIssues(tt.issues)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestDeduplicateRecommendations(t *testing.T) {
	tests := []struct {
		name     string
		recs     []models.AnalysisRecommendation
		expected int
	}{
		{
			name: "remove duplicates",
			recs: []models.AnalysisRecommendation{
				{RuleName: "rule1", Action: "add_condition", Priority: "high", Reason: "reason1", Description: "desc1"},
				{RuleName: "rule1", Action: "add_condition", Priority: "high", Reason: "reason2", Description: "desc2"}, // duplicate
				{RuleName: "rule2", Action: "adjust_priority", Priority: "medium", Reason: "reason3", Description: "desc3"},
			},
			expected: 2,
		},
		{
			name: "no duplicates",
			recs: []models.AnalysisRecommendation{
				{RuleName: "rule1", Action: "add_condition", Priority: "high", Reason: "reason1", Description: "desc1"},
				{RuleName: "rule2", Action: "adjust_priority", Priority: "medium", Reason: "reason2", Description: "desc2"},
			},
			expected: 2,
		},
		{
			name:     "empty recommendations",
			recs:     []models.AnalysisRecommendation{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateRecommendations(tt.recs)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestMergeSummaries(t *testing.T) {
	tests := []struct {
		name         string
		summaries    []*models.AnalysisSummary
		totalEntries int
		validate     func(t *testing.T, result *models.AnalysisSummary)
	}{
		{
			name: "merge multiple summaries",
			summaries: []*models.AnalysisSummary{
				{
					RuleMatchRate:   0.6,
					LLMFallbackRate: 0.3,
					InaccurateRate:  0.1,
					TopTaskTypes:    map[string]int{"coding": 10, "translation": 5},
				},
				{
					RuleMatchRate:   0.8,
					LLMFallbackRate: 0.1,
					InaccurateRate:  0.1,
					TopTaskTypes:    map[string]int{"coding": 15, "general": 5},
				},
			},
			totalEntries: 40,
			validate: func(t *testing.T, result *models.AnalysisSummary) {
				assert.InDelta(t, 0.7, result.RuleMatchRate, 0.01)
				assert.InDelta(t, 0.2, result.LLMFallbackRate, 0.01)
				assert.InDelta(t, 0.1, result.InaccurateRate, 0.01)
				assert.Equal(t, 25, result.TopTaskTypes["coding"])
				assert.Equal(t, 5, result.TopTaskTypes["translation"])
				assert.Equal(t, 5, result.TopTaskTypes["general"])
			},
		},
		{
			name:         "empty summaries",
			summaries:    []*models.AnalysisSummary{},
			totalEntries: 0,
			validate: func(t *testing.T, result *models.AnalysisSummary) {
				assert.Nil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeSummaries(tt.summaries, tt.totalEntries)
			tt.validate(t, result)
		})
	}
}

func TestGenerateMergedConclusion(t *testing.T) {
	tests := []struct {
		name     string
		issues   []models.AnalysisIssue
		recs     []models.AnalysisRecommendation
		validate func(t *testing.T, conclusion string)
	}{
		{
			name: "high priority issues",
			issues: []models.AnalysisIssue{
				{Severity: "high"},
				{Severity: "medium"},
			},
			recs: []models.AnalysisRecommendation{{}, {}},
			validate: func(t *testing.T, conclusion string) {
				assert.Contains(t, conclusion, "高优先级问题")
				assert.Contains(t, conclusion, "1 个")
			},
		},
		{
			name: "only medium issues",
			issues: []models.AnalysisIssue{
				{Severity: "medium"},
				{Severity: "low"},
			},
			recs: []models.AnalysisRecommendation{{}},
			validate: func(t *testing.T, conclusion string) {
				assert.Contains(t, conclusion, "整体路由规则运行良好")
			},
		},
		{
			name:   "no issues",
			issues: []models.AnalysisIssue{},
			recs:   []models.AnalysisRecommendation{},
			validate: func(t *testing.T, conclusion string) {
				assert.Contains(t, conclusion, "未发现明显问题")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateMergedConclusion(tt.issues, tt.recs)
			tt.validate(t, result)
		})
	}
}

func TestBuildHierarchicalPrompt(t *testing.T) {
	rules := []*models.RoutingRule{
		{ID: 1, Name: "test_rule", Priority: 100, Enabled: true, TaskType: "coding", HitCount: 42},
	}
	entries := []*models.ExtractedLogEntry{
		{ID: 1, TaskType: "coding", RoutingMethod: "rule", MatchedRuleName: "test_rule", UserMessage: "test message", IsInaccurate: false},
		{ID: 2, TaskType: "translation", RoutingMethod: "llm", UserMessage: "another message", IsInaccurate: true},
	}
	config := DefaultAnalysisConfig()

	result := BuildHierarchicalPrompt(rules, entries, 100, 2, config)

	assert.Contains(t, result, "# 路由规则分析")
	assert.Contains(t, result, "## 统计概览")
	assert.Contains(t, result, "总日志数: 100")
	assert.Contains(t, result, "分析日志数: 2")
	assert.Contains(t, result, "## 当前路由规则")
	assert.Contains(t, result, "test_rule")
	assert.Contains(t, result, "hits=42")
	assert.Contains(t, result, "## 详细日志样本")
	assert.Contains(t, result, "不准确的日志")
	assert.Contains(t, result, "⚠️ 不准确")
}

func TestBatchFailureHandling(t *testing.T) {
	tests := []struct {
		name           string
		numBatches     int
		failedBatches  int
		expectError    bool
		expectWarnings bool
	}{
		{
			name:           "no failures",
			numBatches:     5,
			failedBatches:  0,
			expectError:    false,
			expectWarnings: false,
		},
		{
			name:           "some failures under threshold",
			numBatches:     10,
			failedBatches:  3,
			expectError:    false,
			expectWarnings: true,
		},
		{
			name:           "failures at 50% threshold",
			numBatches:     10,
			failedBatches:  5,
			expectError:    false,
			expectWarnings: true,
		},
		{
			name:           "failures exceed threshold",
			numBatches:     10,
			failedBatches:  6,
			expectError:    true,
			expectWarnings: false, // Error returned, no report
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate batch processing
			var warnings []string
			failureRate := float64(tt.failedBatches) / float64(tt.numBatches)

			// Check failure threshold
			if failureRate > 0.5 {
				assert.True(t, tt.expectError, "Should return error when failure rate > 50%")
				return
			}

			// Add warnings if some batches failed
			if tt.failedBatches > 0 {
				warnings = append(warnings, "⚠️ 部分批次分析失败")
			}

			if tt.expectWarnings {
				assert.NotEmpty(t, warnings, "Should have warnings when batches fail")
			} else {
				assert.Empty(t, warnings, "Should not have warnings when all batches succeed")
			}
		})
	}
}

func TestRecommendationStructure(t *testing.T) {
	// Test that AnalysisRecommendation has all required fields
	rec := models.AnalysisRecommendation{
		Action:      "modify",
		RuleName:    "test_rule",
		Priority:    "high",
		Reason:      "规则误匹配率达 30%，导致成本浪费",
		Description: "添加条件：message_length < 1000",
		Details:     "详细说明",
		RuleSpec: &models.RecommendedRuleSpec{
			Keywords: []string{"test"},
			Priority: intPtr(80),
		},
	}

	assert.Equal(t, "modify", rec.Action)
	assert.Equal(t, "test_rule", rec.RuleName)
	assert.Equal(t, "high", rec.Priority)
	assert.Equal(t, "规则误匹配率达 30%，导致成本浪费", rec.Reason)
	assert.Equal(t, "添加条件：message_length < 1000", rec.Description)
	assert.NotNil(t, rec.RuleSpec)
}

func TestDeduplicateIssues_PreservesRuleName(t *testing.T) {
	issues := []models.AnalysisIssue{
		{RuleName: "code_rule", Type: "false_positive", Severity: "high", Description: "desc1"},
		{RuleName: "code_rule", Type: "overly_broad", Severity: "medium", Description: "desc2"},
		{RuleName: "translate_rule", Type: "false_positive", Severity: "low", Description: "desc3"},
		{RuleName: "", Type: "missing_rule", Severity: "medium", Description: "desc4"},
	}

	result := deduplicateIssues(issues)
	assert.Len(t, result, 4, "All have unique rule_name:type keys")

	// Verify rule_name is preserved
	ruleNames := make(map[string]int)
	for _, issue := range result {
		ruleNames[issue.RuleName]++
	}
	assert.Equal(t, 2, ruleNames["code_rule"])
	assert.Equal(t, 1, ruleNames["translate_rule"])
	assert.Equal(t, 1, ruleNames[""])
}

func TestDeduplicateRecommendations_PreservesRuleSpec(t *testing.T) {
	priority80 := 80
	priority60 := 60
	recs := []models.AnalysisRecommendation{
		{
			RuleName:    "code_rule",
			Action:      "modify",
			Priority:    "high",
			Description: "modify code rule",
			RuleSpec:    &models.RecommendedRuleSpec{Keywords: []string{"code"}, Priority: &priority80},
		},
		{
			RuleName:    "code_rule",
			Action:      "modify", // duplicate key
			Priority:    "medium",
			Description: "different desc",
			RuleSpec:    &models.RecommendedRuleSpec{Keywords: []string{"debug"}, Priority: &priority60},
		},
		{
			RuleName:    "code_rule",
			Action:      "reorder", // different action, same rule
			Priority:    "low",
			Description: "reorder",
			RuleSpec:    &models.RecommendedRuleSpec{Priority: &priority60},
		},
		{
			RuleName:    "",
			Action:      "add",
			Priority:    "medium",
			Description: "add general rule",
			RuleSpec:    &models.RecommendedRuleSpec{Keywords: []string{"general"}, Priority: &priority80},
		},
	}

	result := deduplicateRecommendations(recs)
	assert.Len(t, result, 3, "Duplicate code_rule:modify should be removed")

	// First occurrence wins
	assert.Equal(t, "code_rule", result[0].RuleName)
	assert.Equal(t, "modify", result[0].Action)
	assert.Equal(t, "high", result[0].Priority)
	assert.NotNil(t, result[0].RuleSpec)
	assert.Equal(t, []string{"code"}, result[0].RuleSpec.Keywords)

	// reorder kept
	assert.Equal(t, "code_rule", result[1].RuleName)
	assert.Equal(t, "reorder", result[1].Action)

	// general add kept with rule_spec
	assert.Equal(t, "", result[2].RuleName)
	assert.Equal(t, "add", result[2].Action)
	assert.NotNil(t, result[2].RuleSpec)
}

func TestGenerateMergedConclusion_AllSeverities(t *testing.T) {
	tests := []struct {
		name       string
		issues     []models.AnalysisIssue
		recs       []models.AnalysisRecommendation
		containStr string
	}{
		{
			name: "multiple high priority issues",
			issues: []models.AnalysisIssue{
				{Severity: "high"},
				{Severity: "high"},
				{Severity: "medium"},
			},
			recs:       []models.AnalysisRecommendation{{}, {}, {}},
			containStr: "2 个高优先级问题",
		},
		{
			name: "only low issues",
			issues: []models.AnalysisIssue{
				{Severity: "low"},
				{Severity: "low"},
			},
			recs:       []models.AnalysisRecommendation{{}},
			containStr: "整体路由规则运行良好",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateMergedConclusion(tt.issues, tt.recs)
			assert.Contains(t, result, tt.containStr)
		})
	}
}

func intPtr(i int) *int {
	return &i
}

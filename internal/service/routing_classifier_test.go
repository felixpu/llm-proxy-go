//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
)

func TestRoutingClassifier_BuiltinRules(t *testing.T) {
	classifier := NewRoutingClassifier(nil)

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		// Complex keywords
		{
			name:     "architecture keyword",
			message:  "帮我设计一个微服务架构",
			expected: string(models.ModelRoleComplex),
		},
		{
			name:     "refactor keyword",
			message:  "重构整个数据库模块",
			expected: string(models.ModelRoleComplex),
		},
		{
			name:     "design keyword",
			message:  "设计一个用户认证系统",
			expected: string(models.ModelRoleComplex),
		},

		// Simple keywords
		{
			name:     "list keyword",
			message:  "列出所有文件",
			expected: string(models.ModelRoleSimple),
		},
		{
			name:     "translate keyword",
			message:  "翻译这段话",
			expected: string(models.ModelRoleSimple),
		},
		{
			name:     "format keyword",
			message:  "格式化这段代码",
			expected: string(models.ModelRoleSimple),
		},

		// Default fallback
		{
			name:     "no keyword match - default",
			message:  "帮我看看这段代码",
			expected: string(models.ModelRoleDefault),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.message)
			assert.Equal(t, tt.expected, result.TaskType,
				"message: %s, reason: %s", tt.message, result.Reason)
		})
	}
}

func TestRoutingClassifier_CustomRules(t *testing.T) {
	customRules := []*models.RoutingRule{
		{
			ID:       100,
			Name:     "custom_api_design",
			Keywords: []string{"API", "接口设计"},
			TaskType: "complex",
			Priority: 200,
			Enabled:  true,
		},
		{
			ID:       101,
			Name:     "custom_quick_check",
			Pattern:  `^(检查|查看)\s*\S+$`,
			TaskType: "simple",
			Priority: 150,
			Enabled:  true,
		},
		{
			ID:       102,
			Name:     "custom_disabled",
			Keywords: []string{"禁用规则"},
			TaskType: "complex",
			Priority: 300,
			Enabled:  false, // disabled
		},
	}

	classifier := NewRoutingClassifier(customRules)

	tests := []struct {
		name     string
		message  string
		expected string
		ruleID   int64 // expected matched rule ID, 0 for builtin/fallback
	}{
		{
			name:     "custom keyword match",
			message:  "帮我设计一个API接口",
			expected: string(models.ModelRoleComplex),
			ruleID:   100,
		},
		{
			name:     "custom pattern match",
			message:  "检查 main.go",
			expected: string(models.ModelRoleSimple),
			ruleID:   101,
		},
		{
			name:     "disabled rule should not match",
			message:  "禁用规则测试",
			expected: string(models.ModelRoleDefault), // falls through to default
		},
		{
			name:     "custom rule higher priority than builtin",
			message:  "设计一个API接口",
			expected: string(models.ModelRoleComplex),
			ruleID:   100, // custom rule (priority 200) > builtin (priority 100)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.message)
			assert.Equal(t, tt.expected, result.TaskType,
				"message: %s, reason: %s", tt.message, result.Reason)
			if tt.ruleID > 0 && result.Rule != nil {
				assert.Equal(t, tt.ruleID, result.Rule.ID)
			}
		})
	}
}

func TestRoutingClassifier_ConditionExpressions(t *testing.T) {
	customRules := []*models.RoutingRule{
		{
			ID:        200,
			Name:      "long_with_code",
			Condition: `len(message) > 500 AND has_code_block(message)`,
			TaskType:  "complex",
			Priority:  150,
			Enabled:   true,
		},
		{
			ID:        201,
			Name:      "short_simple",
			Condition: `len(message) < 20 AND NOT has_code_block(message)`,
			TaskType:  "simple",
			Priority:  140,
			Enabled:   true,
		},
		{
			ID:        202,
			Name:      "multi_question",
			Condition: `count(message, "？") > 2`,
			TaskType:  "complex",
			Priority:  150, // higher than short_simple (140)
			Enabled:   true,
		},
	}

	classifier := NewRoutingClassifier(customRules)

	tests := []struct {
		name     string
		message  string
		expected string
		ruleID   int64
	}{
		{
			name:     "long message with code block",
			message:  string(make([]byte, 600)) + "\n```go\nfunc main() {}\n```",
			expected: string(models.ModelRoleComplex),
			ruleID:   200,
		},
		{
			name:     "short message",
			message:  "你好",
			expected: string(models.ModelRoleSimple),
			ruleID:   201,
		},
		{
			name:     "multiple questions",
			message:  "这是什么？那是什么？为什么？怎么办？",
			expected: string(models.ModelRoleComplex),
			ruleID:   202,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.message)
			assert.Equal(t, tt.expected, result.TaskType,
				"message: %s, reason: %s", tt.message, result.Reason)
			if tt.ruleID > 0 && result.Rule != nil {
				assert.Equal(t, tt.ruleID, result.Rule.ID)
			}
		})
	}
}

func TestRoutingClassifier_PriorityOrdering(t *testing.T) {
	// Two rules match the same message, higher priority wins
	customRules := []*models.RoutingRule{
		{
			ID:       300,
			Name:     "low_priority",
			Keywords: []string{"测试"},
			TaskType: "simple",
			Priority: 50,
			Enabled:  true,
		},
		{
			ID:       301,
			Name:     "high_priority",
			Keywords: []string{"测试"},
			TaskType: "complex",
			Priority: 200,
			Enabled:  true,
		},
	}

	classifier := NewRoutingClassifier(customRules)
	result := classifier.Classify("测试消息")

	assert.Equal(t, string(models.ModelRoleComplex), result.TaskType)
	require.NotNil(t, result.Rule)
	assert.Equal(t, int64(301), result.Rule.ID)
	assert.True(t, len(result.Matches) >= 2, "should have multiple matches")
}

func TestRoutingClassifier_PatternMatching(t *testing.T) {
	customRules := []*models.RoutingRule{
		{
			ID:       400,
			Name:     "multi_file_pattern",
			Pattern:  `(?i)(多个文件|批量|所有.*文件|整个.*目录)`,
			TaskType: "complex",
			Priority: 90,
			Enabled:  true,
		},
	}

	classifier := NewRoutingClassifier(customRules)

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "multi file operation",
			message:  "修改多个文件的导入路径",
			expected: string(models.ModelRoleComplex),
		},
		{
			name:     "batch operation",
			message:  "批量更新数据库记录",
			expected: string(models.ModelRoleComplex),
		},
		{
			name:     "all files",
			message:  "检查所有Go文件的错误处理",
			expected: string(models.ModelRoleComplex),
		},
		{
			name:     "no pattern match",
			message:  "修改一个文件",
			expected: string(models.ModelRoleDefault),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.message)
			assert.Equal(t, tt.expected, result.TaskType)
		})
	}
}

func TestRoutingClassifier_CombinedMatching(t *testing.T) {
	// Rule with keywords + condition + pattern — all must be evaluated
	customRules := []*models.RoutingRule{
		{
			ID:        500,
			Name:      "keyword_and_condition",
			Keywords:  []string{"分析"},
			Condition: `len(message) > 10`,
			TaskType:  "complex",
			Priority:  180,
			Enabled:   true,
		},
	}

	classifier := NewRoutingClassifier(customRules)

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "keyword match + condition met",
			message:  "分析这段代码的性能问题并给出优化建议",
			expected: string(models.ModelRoleComplex),
		},
		{
			name:     "keyword match but condition not met",
			message:  "分析代码",
			expected: string(models.ModelRoleDefault), // len < 10, condition fails
		},
		{
			name:     "no keyword match",
			message:  "这是一段很长的消息但是没有关键词匹配",
			expected: string(models.ModelRoleDefault),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.message)
			assert.Equal(t, tt.expected, result.TaskType,
				"message: %s, reason: %s", tt.message, result.Reason)
		})
	}
}

func TestRoutingClassifier_EmptyMessage(t *testing.T) {
	classifier := NewRoutingClassifier(nil)

	result := classifier.Classify("")
	assert.Equal(t, string(models.ModelRoleDefault), result.TaskType)
}

func TestRoutingClassifier_TestMessage(t *testing.T) {
	// TestMessage returns all matches, not just the winner
	classifier := NewRoutingClassifier(nil)

	result := classifier.TestMessage("帮我设计一个微服务架构，需要考虑高可用性")
	assert.Equal(t, string(models.ModelRoleComplex), result.TaskType)
	assert.NotEmpty(t, result.Reason)
	// TestMessage should populate Matches with all hits
	assert.NotNil(t, result.Matches)
}

func BenchmarkRoutingClassifier_Classify(b *testing.B) {
	classifier := NewRoutingClassifier(nil)
	message := "帮我设计一个微服务架构，需要考虑高可用性和扩展性"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = classifier.Classify(message)
	}
}

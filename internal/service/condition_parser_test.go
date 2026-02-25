//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConditionParser_BasicFunctions(t *testing.T) {
	parser := NewConditionParser()

	tests := []struct {
		name      string
		condition string
		message   string
		expected  bool
	}{
		// len() function
		{
			name:      "len greater than - true",
			condition: "len(message) > 100",
			message:   string(make([]byte, 150)),
			expected:  true,
		},
		{
			name:      "len greater than - false",
			condition: "len(message) > 100",
			message:   "short message",
			expected:  false,
		},
		{
			name:      "len less than - true",
			condition: "len(message) < 50",
			message:   "short",
			expected:  true,
		},
		{
			name:      "len equals",
			condition: "len(message) == 5",
			message:   "hello",
			expected:  true,
		},
		{
			name:      "len greater or equal",
			condition: "len(message) >= 5",
			message:   "hello",
			expected:  true,
		},
		{
			name:      "len less or equal",
			condition: "len(message) <= 5",
			message:   "hello",
			expected:  true,
		},

		// contains() function
		{
			name:      "contains - true",
			condition: `contains(message, "架构")`,
			message:   "帮我设计一个系统架构",
			expected:  true,
		},
		{
			name:      "contains - false",
			condition: `contains(message, "架构")`,
			message:   "帮我列出文件",
			expected:  false,
		},
		{
			name:      "contains - case sensitive",
			condition: `contains(message, "Hello")`,
			message:   "hello world",
			expected:  false,
		},

		// matches() function - regex
		{
			name:      "matches - simple pattern",
			condition: `matches(message, "\\d{3,}")`,
			message:   "订单号是12345",
			expected:  true,
		},
		{
			name:      "matches - no match",
			condition: `matches(message, "\\d{3,}")`,
			message:   "没有数字",
			expected:  false,
		},
		{
			name:      "matches - word boundary",
			condition: `matches(message, "(?i)design")`,
			message:   "Please Design a system",
			expected:  true,
		},

		// has_code_block() function
		{
			name:      "has_code_block - triple backticks",
			condition: "has_code_block(message)",
			message:   "看看这段代码\n```go\nfunc main() {}\n```",
			expected:  true,
		},
		{
			name:      "has_code_block - no code block",
			condition: "has_code_block(message)",
			message:   "普通文本消息",
			expected:  false,
		},
		{
			name:      "has_code_block - inline code only",
			condition: "has_code_block(message)",
			message:   "使用 `fmt.Println` 打印",
			expected:  false,
		},

		// count() function
		{
			name:      "count - multiple occurrences",
			condition: `count(message, "？") > 2`,
			message:   "这是什么？那是什么？为什么？",
			expected:  true,
		},
		{
			name:      "count - single occurrence",
			condition: `count(message, "？") > 2`,
			message:   "这是什么？",
			expected:  false,
		},
		{
			name:      "count - equals",
			condition: `count(message, "a") == 3`,
			message:   "banana",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Evaluate(tt.condition, tt.message)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionParser_LogicalOperators(t *testing.T) {
	parser := NewConditionParser()

	tests := []struct {
		name      string
		condition string
		message   string
		expected  bool
	}{
		// AND operator
		{
			name:      "AND - both true",
			condition: `len(message) >= 10 AND contains(message, "设计")`,
			message:   "帮我设计一个系统架构",
			expected:  true,
		},
		{
			name:      "AND - first false",
			condition: `len(message) > 100 AND contains(message, "设计")`,
			message:   "帮我设计",
			expected:  false,
		},
		{
			name:      "AND - second false",
			condition: `len(message) > 5 AND contains(message, "架构")`,
			message:   "帮我设计",
			expected:  false,
		},

		// OR operator
		{
			name:      "OR - first true",
			condition: `contains(message, "列出") OR contains(message, "查看")`,
			message:   "列出所有文件",
			expected:  true,
		},
		{
			name:      "OR - second true",
			condition: `contains(message, "列出") OR contains(message, "查看")`,
			message:   "查看日志",
			expected:  true,
		},
		{
			name:      "OR - both false",
			condition: `contains(message, "列出") OR contains(message, "查看")`,
			message:   "设计系统",
			expected:  false,
		},

		// NOT operator
		{
			name:      "NOT - negates true",
			condition: `NOT contains(message, "分析")`,
			message:   "列出文件",
			expected:  true,
		},
		{
			name:      "NOT - negates false",
			condition: `NOT contains(message, "分析")`,
			message:   "分析这段代码",
			expected:  false,
		},

		// Combined operators
		{
			name:      "AND with NOT",
			condition: `len(message) < 200 AND NOT contains(message, "分析")`,
			message:   "列出文件",
			expected:  true,
		},
		{
			name:      "Complex combination",
			condition: `(len(message) > 500 AND has_code_block(message)) OR contains(message, "重构")`,
			message:   "帮我重构这个模块",
			expected:  true,
		},
		{
			name:      "Nested parentheses",
			condition: `(contains(message, "设计") OR contains(message, "架构")) AND len(message) > 3`,
			message:   "设计系统",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Evaluate(tt.condition, tt.message)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConditionParser_EdgeCases(t *testing.T) {
	parser := NewConditionParser()

	tests := []struct {
		name      string
		condition string
		message   string
		expected  bool
		wantErr   bool
	}{
		// Empty conditions
		{
			name:      "empty condition",
			condition: "",
			message:   "any message",
			expected:  true, // Empty condition always matches
			wantErr:   false,
		},
		{
			name:      "whitespace only condition",
			condition: "   ",
			message:   "any message",
			expected:  true,
			wantErr:   false,
		},

		// Empty message
		{
			name:      "empty message with len check",
			condition: "len(message) == 0",
			message:   "",
			expected:  true,
			wantErr:   false,
		},

		// Unicode handling
		{
			name:      "unicode characters in contains",
			condition: `contains(message, "你好")`,
			message:   "你好世界",
			expected:  true,
			wantErr:   false,
		},
		{
			name:      "unicode length",
			condition: "len(message) == 4",
			message:   "你好世界",
			expected:  true, // 4 runes
			wantErr:   false,
		},

		// Invalid syntax
		{
			name:      "invalid function",
			condition: "invalid_func(message)",
			message:   "test",
			expected:  false,
			wantErr:   true,
		},
		{
			name:      "unclosed parenthesis",
			condition: "len(message > 5",
			message:   "test",
			expected:  false,
			wantErr:   true,
		},
		{
			name:      "invalid regex",
			condition: `matches(message, "[invalid")`,
			message:   "test",
			expected:  false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Evaluate(tt.condition, tt.message)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConditionParser_OperatorPrecedence(t *testing.T) {
	parser := NewConditionParser()

	tests := []struct {
		name      string
		condition string
		message   string
		expected  bool
	}{
		// NOT has highest precedence
		{
			name:      "NOT before AND",
			condition: `NOT contains(message, "a") AND contains(message, "b")`,
			message:   "b",
			expected:  true, // (NOT contains "a") AND (contains "b") = true AND true
		},
		// AND before OR
		{
			name:      "AND before OR - left",
			condition: `contains(message, "a") AND contains(message, "b") OR contains(message, "c")`,
			message:   "c",
			expected:  true, // (a AND b) OR c = false OR true = true
		},
		{
			name:      "AND before OR - right",
			condition: `contains(message, "c") OR contains(message, "a") AND contains(message, "b")`,
			message:   "c",
			expected:  true, // c OR (a AND b) = true OR false = true
		},
		// Parentheses override precedence
		{
			name:      "parentheses override",
			condition: `contains(message, "a") AND (contains(message, "b") OR contains(message, "c"))`,
			message:   "ac",
			expected:  true, // a AND (b OR c) = true AND true = true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Evaluate(tt.condition, tt.message)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkConditionParser_SimpleCondition(b *testing.B) {
	parser := NewConditionParser()
	message := "帮我设计一个系统架构"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Evaluate(`contains(message, "架构")`, message)
	}
}

func BenchmarkConditionParser_ComplexCondition(b *testing.B) {
	parser := NewConditionParser()
	message := "帮我设计一个系统架构，需要考虑高可用性和扩展性"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Evaluate(
			`(len(message) > 10 AND contains(message, "设计")) OR (has_code_block(message) AND NOT contains(message, "简单"))`,
			message,
		)
	}
}

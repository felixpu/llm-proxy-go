package service

import (
	"testing"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestExtractAnalysisJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "code block with nested JSON",
			input: "Here is the analysis:\n```json\n" +
				`{"summary": {"rule_match_rate": 0.75}, "issues": [{"type": "false_positive"}], "conclusion": "test"}` +
				"\n```\nDone.",
			expected: `{"summary": {"rule_match_rate": 0.75}, "issues": [{"type": "false_positive"}], "conclusion": "test"}`,
		},
		{
			name: "text before JSON with nested objects",
			input: "Analysis complete. Result:\n" +
				`{"summary": {"rule_match_rate": 0.75, "top_task_types": {"default": 50}}, "issues": [], "recommendations": [], "conclusion": "Good"}`,
			expected: `{"summary": {"rule_match_rate": 0.75, "top_task_types": {"default": 50}}, "issues": [], "recommendations": [], "conclusion": "Good"}`,
		},
		{
			name: "direct JSON at start",
			input: `{"summary": null, "issues": [{"type": "test", "severity": "high"}], "recommendations": [], "conclusion": ""}`,
			expected: `{"summary": null, "issues": [{"type": "test", "severity": "high"}], "recommendations": [], "conclusion": ""}`,
		},
		{
			name: "JSON with arrays and nested objects",
			input: `{"summary": {"rule_match_rate": 0.8}, "issues": [{"type": "a", "examples": ["x", "y"]}], "recommendations": [{"action": "modify", "rule_spec": {"keywords": ["k1", "k2"]}}], "conclusion": "Done"}`,
			expected: `{"summary": {"rule_match_rate": 0.8}, "issues": [{"type": "a", "examples": ["x", "y"]}], "recommendations": [{"action": "modify", "rule_spec": {"keywords": ["k1", "k2"]}}], "conclusion": "Done"}`,
		},
		{
			name: "JSON with escaped quotes",
			input: `{"summary": null, "issues": [], "recommendations": [], "conclusion": "Use \"quotes\" carefully"}`,
			expected: `{"summary": null, "issues": [], "recommendations": [], "conclusion": "Use \"quotes\" carefully"}`,
		},
		{
			name:     "no JSON",
			input:    "This is just plain text without any JSON",
			expected: "",
		},
		{
			name:     "incomplete JSON",
			input:    `{"summary": {"rule_match_rate": 0.75`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAnalysisJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAnalysisResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		checkResult func(t *testing.T, report *models.AnalysisReport)
	}{
		{
			name: "valid complete response",
			input: `{"summary": {"rule_match_rate": 0.75, "llm_fallback_rate": 0.15, "inaccurate_rate": 0.05, "top_task_types": {"default": 50}}, ` +
				`"issues": [{"type": "false_positive", "severity": "high", "rule_name": "test_rule", "description": "desc", "examples": ["ex1"]}], ` +
				`"recommendations": [{"action": "modify", "rule_name": "test_rule", "description": "desc", "details": "details", "rule_spec": {"keywords": ["k1"]}}], ` +
				`"conclusion": "Analysis complete"}`,
			expectError: false,
			checkResult: func(t *testing.T, report *models.AnalysisReport) {
				assert.NotNil(t, report.Summary)
				assert.Equal(t, 0.75, report.Summary.RuleMatchRate)
				assert.Len(t, report.Issues, 1)
				assert.Equal(t, "false_positive", report.Issues[0].Type)
				assert.Len(t, report.Recommendations, 1)
				assert.Equal(t, "modify", report.Recommendations[0].Action)
				assert.Equal(t, "Analysis complete", report.Conclusion)
			},
		},
		{
			name: "response with null summary",
			input: `{"summary": null, "issues": [], "recommendations": [], "conclusion": ""}`,
			expectError: false,
			checkResult: func(t *testing.T, report *models.AnalysisReport) {
				assert.Nil(t, report.Summary)
				assert.Empty(t, report.Issues)
				assert.Empty(t, report.Recommendations)
				assert.Empty(t, report.Conclusion)
			},
		},
		{
			name:        "no JSON in response",
			input:       "This is just text without JSON",
			expectError: true,
		},
		{
			name:        "invalid JSON",
			input:       `{"summary": invalid}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := ParseAnalysisResponse(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, report)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, report)
				if tt.checkResult != nil {
					tt.checkResult(t, report)
				}
			}
		})
	}
}

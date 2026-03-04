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
			name:  "response with null summary",
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

func TestParseAnalysisResponse_RuleSpecFields(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		checkResult func(t *testing.T, report *models.AnalysisReport)
	}{
		{
			name: "recommendation with full rule_spec",
			input: `{"summary": null, "issues": [], "recommendations": [` +
				`{"action": "add", "rule_name": "new_code_rule", "priority": "high", "reason": "missing rule", "description": "add rule",` +
				` "rule_spec": {"keywords": ["code", "debug"], "pattern": "^write.*code", "condition": "message_length > 100",` +
				` "task_type": "complex", "priority": 80, "enabled": true}}], "conclusion": "done"}`,
			checkResult: func(t *testing.T, report *models.AnalysisReport) {
				assert.Len(t, report.Recommendations, 1)
				rec := report.Recommendations[0]
				assert.Equal(t, "add", rec.Action)
				assert.Equal(t, "new_code_rule", rec.RuleName)
				assert.Equal(t, "high", rec.Priority)
				assert.Equal(t, "missing rule", rec.Reason)
				assert.NotNil(t, rec.RuleSpec)
				assert.Equal(t, []string{"code", "debug"}, rec.RuleSpec.Keywords)
				assert.Equal(t, "^write.*code", rec.RuleSpec.Pattern)
				assert.Equal(t, "message_length > 100", rec.RuleSpec.Condition)
				assert.Equal(t, "complex", rec.RuleSpec.TaskType)
				assert.NotNil(t, rec.RuleSpec.Priority)
				assert.Equal(t, 80, *rec.RuleSpec.Priority)
				assert.NotNil(t, rec.RuleSpec.Enabled)
				assert.True(t, *rec.RuleSpec.Enabled)
			},
		},
		{
			name: "recommendation without rule_spec (delete action)",
			input: `{"summary": null, "issues": [], "recommendations": [` +
				`{"action": "delete", "rule_name": "obsolete_rule", "priority": "low", "reason": "redundant", "description": "remove rule"}` +
				`], "conclusion": "done"}`,
			checkResult: func(t *testing.T, report *models.AnalysisReport) {
				assert.Len(t, report.Recommendations, 1)
				rec := report.Recommendations[0]
				assert.Equal(t, "delete", rec.Action)
				assert.Equal(t, "obsolete_rule", rec.RuleName)
				assert.Nil(t, rec.RuleSpec)
			},
		},
		{
			name: "recommendation with reorder rule_spec (priority only)",
			input: `{"summary": null, "issues": [], "recommendations": [` +
				`{"action": "reorder", "rule_name": "my_rule", "priority": "medium", "reason": "conflict", "description": "raise priority",` +
				` "rule_spec": {"priority": 90}}], "conclusion": "done"}`,
			checkResult: func(t *testing.T, report *models.AnalysisReport) {
				assert.Len(t, report.Recommendations, 1)
				rec := report.Recommendations[0]
				assert.Equal(t, "reorder", rec.Action)
				assert.NotNil(t, rec.RuleSpec)
				assert.NotNil(t, rec.RuleSpec.Priority)
				assert.Equal(t, 90, *rec.RuleSpec.Priority)
				// Other fields should be zero/nil
				assert.Empty(t, rec.RuleSpec.Keywords)
				assert.Empty(t, rec.RuleSpec.Pattern)
				assert.Empty(t, rec.RuleSpec.TaskType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := ParseAnalysisResponse(tt.input)
			assert.NoError(t, err)
			assert.NotNil(t, report)
			tt.checkResult(t, report)
		})
	}
}

func TestParseAnalysisResponse_IssueTypes(t *testing.T) {
	// Verify all issue types the frontend expects can be parsed correctly
	issueTypes := []string{
		"false_positive", "false_negative", "priority_conflict",
		"redundant_rule", "overly_broad", "missing_rule",
	}

	for _, issueType := range issueTypes {
		t.Run(issueType, func(t *testing.T) {
			input := `{"summary": null, "issues": [{"type": "` + issueType +
				`", "severity": "high", "rule_name": "test_rule", "description": "test"}], ` +
				`"recommendations": [], "conclusion": ""}`
			report, err := ParseAnalysisResponse(input)
			assert.NoError(t, err)
			assert.Len(t, report.Issues, 1)
			assert.Equal(t, issueType, report.Issues[0].Type)
			assert.Equal(t, "test_rule", report.Issues[0].RuleName)
		})
	}
}

func TestParseAnalysisResponse_ActionTypes(t *testing.T) {
	// Verify all action types the frontend expects can be parsed correctly
	actionTypes := []string{"modify", "add", "delete", "reorder"}

	for _, action := range actionTypes {
		t.Run(action, func(t *testing.T) {
			input := `{"summary": null, "issues": [], "recommendations": [` +
				`{"action": "` + action + `", "rule_name": "test_rule", "priority": "high", ` +
				`"reason": "test", "description": "test"}], "conclusion": ""}`
			report, err := ParseAnalysisResponse(input)
			assert.NoError(t, err)
			assert.Len(t, report.Recommendations, 1)
			assert.Equal(t, action, report.Recommendations[0].Action)
		})
	}
}

func TestParseAnalysisResponse_GroupableByRuleName(t *testing.T) {
	// Verify issues and recommendations with the same rule_name can be grouped
	input := `{"summary": null, "issues": [` +
		`{"type": "false_positive", "severity": "high", "rule_name": "code_review_rule", "description": "desc1"},` +
		`{"type": "overly_broad", "severity": "medium", "rule_name": "code_review_rule", "description": "desc2"},` +
		`{"type": "missing_rule", "severity": "low", "description": "desc3"}` +
		`], "recommendations": [` +
		`{"action": "modify", "rule_name": "code_review_rule", "priority": "high", "reason": "r1", "description": "d1", "rule_spec": {"keywords": ["review"]}},` +
		`{"action": "add", "rule_name": "translation_rule", "priority": "medium", "reason": "r2", "description": "d2", "rule_spec": {"keywords": ["translate"]}},` +
		`{"action": "add", "priority": "low", "reason": "r3", "description": "d3"}` +
		`], "conclusion": "done"}`

	report, err := ParseAnalysisResponse(input)
	assert.NoError(t, err)

	// Group issues by rule_name
	issueGroups := make(map[string][]models.AnalysisIssue)
	for _, issue := range report.Issues {
		key := issue.RuleName
		if key == "" {
			key = "__general__"
		}
		issueGroups[key] = append(issueGroups[key], issue)
	}

	// Group recommendations by rule_name
	recGroups := make(map[string][]models.AnalysisRecommendation)
	for _, rec := range report.Recommendations {
		key := rec.RuleName
		if key == "" {
			key = "__general__"
		}
		recGroups[key] = append(recGroups[key], rec)
	}

	// code_review_rule: 2 issues + 1 recommendation
	assert.Len(t, issueGroups["code_review_rule"], 2)
	assert.Len(t, recGroups["code_review_rule"], 1)

	// translation_rule: 0 issues + 1 recommendation
	assert.Empty(t, issueGroups["translation_rule"])
	assert.Len(t, recGroups["translation_rule"], 1)

	// general: 1 issue + 1 recommendation (no rule_name)
	assert.Len(t, issueGroups["__general__"], 1)
	assert.Len(t, recGroups["__general__"], 1)
}

func TestParseAnalysisResponse_NilRecommendationsBecomesEmptySlice(t *testing.T) {
	// Frontend depends on recommendations being [] not null
	input := `{"summary": null, "issues": [], "conclusion": "done"}`
	report, err := ParseAnalysisResponse(input)
	assert.NoError(t, err)
	assert.NotNil(t, report.Recommendations, "recommendations should be empty slice, not nil")
	assert.NotNil(t, report.Issues, "issues should be empty slice, not nil")
}

func TestParseAnalysisResponse_WrappedInNestedKey(t *testing.T) {
	// LLMs sometimes wrap in {"analysis": {...}} or {"result": {...}}
	input := `{"analysis": {"summary": null, "issues": [{"type": "false_positive", "severity": "high", "rule_name": "r1", "description": "d1"}], ` +
		`"recommendations": [{"action": "modify", "rule_name": "r1", "priority": "high", "reason": "reason", "description": "desc", ` +
		`"rule_spec": {"keywords": ["k1"], "task_type": "complex", "priority": 80}}], "conclusion": "done"}}`

	report, err := ParseAnalysisResponse(input)
	assert.NoError(t, err)
	assert.Len(t, report.Issues, 1)
	assert.Equal(t, "r1", report.Issues[0].RuleName)
	assert.Len(t, report.Recommendations, 1)
	assert.Equal(t, "r1", report.Recommendations[0].RuleName)
	assert.NotNil(t, report.Recommendations[0].RuleSpec)
	assert.Equal(t, 80, *report.Recommendations[0].RuleSpec.Priority)
}

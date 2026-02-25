//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestNewLLMRouter(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	router := NewLLMRouter(db, nil, logger)
	assert.NotNil(t, router)
	assert.NotNil(t, router.configRepo)
	assert.NotNil(t, router.modelRepo)
	assert.NotNil(t, router.embeddingRepo)
	assert.NotNil(t, router.routingCache)
	assert.NotNil(t, router.ruleRepo)
	assert.NotNil(t, router.client)
}

func TestExtractSystemContent(t *testing.T) {
	tests := []struct {
		name     string
		req      *models.AnthropicRequest
		expected string
	}{
		{
			name:     "nil system",
			req:      &models.AnthropicRequest{},
			expected: "",
		},
		{
			name: "string system",
			req: &models.AnthropicRequest{
				System: &models.SystemPrompt{Text: "You are a helpful assistant.", IsArray: false},
			},
			expected: "You are a helpful assistant.",
		},
		{
			name: "array system",
			req: &models.AnthropicRequest{
				System: &models.SystemPrompt{
					Blocks: []models.ContentPart{
						{Type: "text", Text: "Part 1"},
						{Type: "text", Text: "Part 2"},
					},
					IsArray: true,
				},
			},
			expected: "Part 1 Part 2",
		},
		{
			name: "empty system",
			req: &models.AnthropicRequest{
				System: &models.SystemPrompt{Text: "", IsArray: false},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSystemContent(tt.req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractLastUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		req      *models.AnthropicRequest
		expected string
	}{
		{
			name:     "empty messages",
			req:      &models.AnthropicRequest{Messages: []models.Message{}},
			expected: "",
		},
		{
			name: "single user message",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{Text: "Hello"}},
				},
			},
			expected: "Hello",
		},
		{
			name: "multiple messages - get last user",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{Text: "First"}},
					{Role: "assistant", Content: models.MessageContent{Text: "Response"}},
					{Role: "user", Content: models.MessageContent{Text: "Second"}},
				},
			},
			expected: "Second",
		},
		{
			name: "last message is assistant",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{Text: "Question"}},
					{Role: "assistant", Content: models.MessageContent{Text: "Answer"}},
				},
			},
			expected: "Question",
		},
		{
			name: "multiple content parts",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{
						Parts: []models.ContentPart{
							{Type: "text", Text: "Part 1"},
							{Type: "text", Text: "Part 2"},
						},
						IsArray: true,
					}},
				},
			},
			expected: "Part 1\nPart 2",
		},
		{
			name: "skip non-text content",
			req: &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: models.MessageContent{
						Parts: []models.ContentPart{
							{Type: "image", Text: ""},
							{Type: "text", Text: "Actual text"},
						},
						IsArray: true,
					}},
				},
			},
			expected: "Actual text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLastUserMessage(tt.req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseModelRole(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected models.ModelRole
	}{
		{"simple", "simple", models.ModelRoleSimple},
		{"complex", "complex", models.ModelRoleComplex},
		{"default", "default", models.ModelRoleDefault},
		{"uppercase", "SIMPLE", models.ModelRoleSimple},
		{"mixed case", "Complex", models.ModelRoleComplex},
		{"with spaces", "  simple  ", models.ModelRoleSimple},
		{"unknown", "unknown", models.ModelRoleDefault},
		{"empty", "", models.ModelRoleDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseModelRole(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "direct JSON",
			input:    `{"task_type": "simple", "reason": "test"}`,
			expected: `{"task_type": "simple", "reason": "test"}`,
		},
		{
			name:     "markdown code block",
			input:    "```json\n{\"task_type\": \"complex\", \"reason\": \"test\"}\n```",
			expected: `{"task_type": "complex", "reason": "test"}`,
		},
		{
			name:     "code block without lang",
			input:    "```\n{\"task_type\": \"simple\", \"reason\": \"test\"}\n```",
			expected: `{"task_type": "simple", "reason": "test"}`,
		},
		{
			name:     "embedded in text",
			input:    `Based on analysis, {"task_type": "complex", "reason": "needs deep thinking"} is the result.`,
			expected: `{"task_type": "complex", "reason": "needs deep thinking"}`,
		},
		{
			name:     "no JSON",
			input:    "This is just plain text without any JSON",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRoutingDecision(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType models.ModelRole
		wantErr  bool
	}{
		{
			name:     "valid simple",
			input:    `{"task_type": "simple", "reason": "basic query"}`,
			wantType: models.ModelRoleSimple,
		},
		{
			name:     "valid complex",
			input:    `{"task_type": "complex", "reason": "architecture design"}`,
			wantType: models.ModelRoleComplex,
		},
		{
			name:     "valid default",
			input:    `{"task_type": "default", "reason": "general task"}`,
			wantType: models.ModelRoleDefault,
		},
		{
			name:     "markdown wrapped",
			input:    "```json\n{\"task_type\": \"simple\", \"reason\": \"test\"}\n```",
			wantType: models.ModelRoleSimple,
		},
		{
			name:    "no JSON",
			input:   "I think this is a simple task",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `{"task_type": }`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := parseRoutingDecision(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, decision)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, decision)
				assert.Equal(t, tt.wantType, decision.TaskType)
				assert.False(t, decision.FromCache)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
		{"zero max", "hello", 0, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestLLMRouter_InferTaskType_RuleBasedDisabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Ensure default row exists, then disable rule-based routing
	_, execErr := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	assert.NoError(t, execErr)
	_, execErr = db.Exec(`UPDATE routing_llm_config SET rule_based_routing_enabled = 0, enabled = 0 WHERE id = 1`)
	assert.NoError(t, execErr)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	// Both routing methods disabled - should use fallback
	taskType, decision, err := router.InferTaskType(t.Context(), req)
	assert.NoError(t, err)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "fallback")
}

func TestLLMRouter_InferTaskType_RuleMatch(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Enable rule-based routing (default)
	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "帮我设计一个微服务架构"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	assert.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "matched rule")
	assert.Equal(t, "rule", decision.CacheType)
}

func TestLLMRouter_InferTaskType_NoRuleMatch_FallbackDefault(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Enable rule-based routing with default fallback (default config)
	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello, how are you?"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	assert.NoError(t, err)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "fallback")
}

func TestLLMRouter_InferTaskType_NoRuleMatch_FallbackUserChoice(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Ensure default row exists, then set fallback to user choice with complex task type
	_, execErr := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	assert.NoError(t, execErr)
	_, execErr = db.Exec(`UPDATE routing_llm_config SET rule_fallback_strategy = 'user', rule_fallback_task_type = 'complex' WHERE id = 1`)
	assert.NoError(t, execErr)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Random message"}},
		},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	assert.NoError(t, err)
	assert.Equal(t, models.ModelRoleComplex, taskType)
	assert.NotNil(t, decision)
	assert.Contains(t, decision.Reason, "user-configured")
}

func TestLLMRouter_InferTaskType_EmptyMessage(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zap.NewNop()

	// Ensure default row exists, then enable routing config
	_, execErr := db.Exec(`INSERT OR IGNORE INTO routing_llm_config (id, enabled) VALUES (1, 0)`)
	assert.NoError(t, execErr)
	_, execErr = db.Exec(`UPDATE routing_llm_config SET enabled = 1 WHERE id = 1`)
	assert.NoError(t, execErr)

	router := NewLLMRouter(db, nil, logger)

	req := &models.AnthropicRequest{
		Messages: []models.Message{},
	}

	taskType, decision, err := router.InferTaskType(t.Context(), req)
	assert.NoError(t, err)
	assert.Equal(t, models.ModelRoleDefault, taskType)
	assert.Nil(t, decision)
}

//go:build !integration && !e2e
// +build !integration,!e2e

package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemPrompt_UnmarshalJSON_String(t *testing.T) {
	input := `"You are a helpful assistant."`
	var sp SystemPrompt
	err := json.Unmarshal([]byte(input), &sp)

	require.NoError(t, err)
	assert.False(t, sp.IsArray)
	assert.Equal(t, "You are a helpful assistant.", sp.Text)
	assert.Empty(t, sp.Blocks)
}

func TestSystemPrompt_UnmarshalJSON_Array(t *testing.T) {
	input := `[{"type":"text","text":"You are helpful."},{"type":"text","text":"Be concise."}]`
	var sp SystemPrompt
	err := json.Unmarshal([]byte(input), &sp)

	require.NoError(t, err)
	assert.True(t, sp.IsArray)
	assert.Len(t, sp.Blocks, 2)
	assert.Equal(t, "You are helpful.", sp.Blocks[0].Text)
	assert.Equal(t, "Be concise.", sp.Blocks[1].Text)
}

func TestSystemPrompt_UnmarshalJSON_Invalid(t *testing.T) {
	input := `123`
	var sp SystemPrompt
	err := json.Unmarshal([]byte(input), &sp)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system must be a string or array")
}

func TestSystemPrompt_MarshalJSON_String(t *testing.T) {
	sp := SystemPrompt{Text: "Hello", IsArray: false}
	data, err := json.Marshal(sp)

	require.NoError(t, err)
	assert.JSONEq(t, `"Hello"`, string(data))
}

func TestSystemPrompt_MarshalJSON_Array(t *testing.T) {
	sp := SystemPrompt{
		Blocks:  []ContentPart{{Type: "text", Text: "Hello"}, {Type: "text", Text: "World"}},
		IsArray: true,
	}
	data, err := json.Marshal(sp)

	require.NoError(t, err)
	assert.JSONEq(t, `[{"type":"text","text":"Hello"},{"type":"text","text":"World"}]`, string(data))
}

func TestSystemPrompt_String_FromText(t *testing.T) {
	sp := &SystemPrompt{Text: "Hello world", IsArray: false}
	assert.Equal(t, "Hello world", sp.String())
}

func TestSystemPrompt_String_FromBlocks(t *testing.T) {
	sp := &SystemPrompt{
		Blocks:  []ContentPart{{Type: "text", Text: "Hello"}, {Type: "text", Text: "World"}},
		IsArray: true,
	}
	assert.Equal(t, "Hello World", sp.String())
}

func TestSystemPrompt_String_SkipsNonTextBlocks(t *testing.T) {
	sp := &SystemPrompt{
		Blocks:  []ContentPart{{Type: "image", Text: ""}, {Type: "text", Text: "Only text"}},
		IsArray: true,
	}
	assert.Equal(t, "Only text", sp.String())
}

func TestSystemPrompt_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		sp       *SystemPrompt
		expected bool
	}{
		{"nil", nil, true},
		{"empty string", &SystemPrompt{Text: "", IsArray: false}, true},
		{"non-empty string", &SystemPrompt{Text: "hi", IsArray: false}, false},
		{"empty array", &SystemPrompt{Blocks: nil, IsArray: true}, true},
		{"non-empty array", &SystemPrompt{Blocks: []ContentPart{{Type: "text", Text: "hi"}}, IsArray: true}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.sp.IsEmpty())
		})
	}
}

func TestAnthropicRequest_UnmarshalJSON_WithStringSystem(t *testing.T) {
	input := `{"model":"claude-3","messages":[],"max_tokens":100,"system":"You are helpful."}`
	var req AnthropicRequest
	err := json.Unmarshal([]byte(input), &req)

	require.NoError(t, err)
	require.NotNil(t, req.System)
	assert.False(t, req.System.IsArray)
	assert.Equal(t, "You are helpful.", req.System.String())
}

func TestAnthropicRequest_UnmarshalJSON_WithArraySystem(t *testing.T) {
	input := `{"model":"claude-3","messages":[],"max_tokens":100,"system":[{"type":"text","text":"Be helpful."}]}`
	var req AnthropicRequest
	err := json.Unmarshal([]byte(input), &req)

	require.NoError(t, err)
	require.NotNil(t, req.System)
	assert.True(t, req.System.IsArray)
	assert.Equal(t, "Be helpful.", req.System.String())
}

func TestAnthropicRequest_UnmarshalJSON_WithoutSystem(t *testing.T) {
	input := `{"model":"claude-3","messages":[],"max_tokens":100}`
	var req AnthropicRequest
	err := json.Unmarshal([]byte(input), &req)

	require.NoError(t, err)
	assert.Nil(t, req.System)
}

func TestAnthropicRequest_UnmarshalJSON_WithNullSystem(t *testing.T) {
	input := `{"model":"claude-3","messages":[],"max_tokens":100,"system":null}`
	var req AnthropicRequest
	err := json.Unmarshal([]byte(input), &req)

	require.NoError(t, err)
	assert.Nil(t, req.System)
}

func TestCacheControl_MarshalJSON(t *testing.T) {
	cc := &CacheControl{Type: "ephemeral"}
	data, err := json.Marshal(cc)

	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(data))
}

func TestContentPart_WithCacheControl_MarshalJSON(t *testing.T) {
	part := ContentPart{
		Type:         "text",
		Text:         "Hello",
		CacheControl: &CacheControl{Type: "ephemeral"},
	}
	data, err := json.Marshal(part)

	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"text","text":"Hello","cache_control":{"type":"ephemeral"}}`, string(data))
}

func TestContentPart_WithCacheControl_UnmarshalJSON(t *testing.T) {
	input := `{"type":"text","text":"Hello","cache_control":{"type":"ephemeral"}}`
	var part ContentPart
	err := json.Unmarshal([]byte(input), &part)

	require.NoError(t, err)
	assert.Equal(t, "text", part.Type)
	assert.Equal(t, "Hello", part.Text)
	require.NotNil(t, part.CacheControl)
	assert.Equal(t, "ephemeral", part.CacheControl.Type)
}

func TestTool_WithCacheControl_MarshalJSON(t *testing.T) {
	tool := Tool{
		Name:         "get_weather",
		Description:  "Get weather info",
		InputSchema:  map[string]interface{}{"type": "object"},
		CacheControl: &CacheControl{Type: "ephemeral"},
	}
	data, err := json.Marshal(tool)

	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"get_weather","description":"Get weather info","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}`, string(data))
}

func TestTool_WithCacheControl_UnmarshalJSON(t *testing.T) {
	input := `{"name":"get_weather","description":"Get weather info","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}`
	var tool Tool
	err := json.Unmarshal([]byte(input), &tool)

	require.NoError(t, err)
	assert.Equal(t, "get_weather", tool.Name)
	require.NotNil(t, tool.CacheControl)
	assert.Equal(t, "ephemeral", tool.CacheControl.Type)
}

func TestUsage_WithCacheTokens_MarshalJSON(t *testing.T) {
	usage := Usage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     30,
	}
	data, err := json.Marshal(usage)

	require.NoError(t, err)
	assert.JSONEq(t, `{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":30}`, string(data))
}

func TestUsage_WithCacheTokens_UnmarshalJSON(t *testing.T) {
	input := `{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":30}`
	var usage Usage
	err := json.Unmarshal([]byte(input), &usage)

	require.NoError(t, err)
	assert.Equal(t, 100, usage.InputTokens)
	assert.Equal(t, 50, usage.OutputTokens)
	assert.Equal(t, 20, usage.CacheCreationInputTokens)
	assert.Equal(t, 30, usage.CacheReadInputTokens)
}

func TestAnthropicRequest_WithCacheControl_RoundTrip(t *testing.T) {
	// Test that a full request with cache_control preserves all fields through marshal/unmarshal
	req := AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role: "user",
				Content: MessageContent{
					Parts: []ContentPart{
						{
							Type:         "text",
							Text:         "Hello",
							CacheControl: &CacheControl{Type: "ephemeral"},
						},
					},
					IsArray: true,
				},
			},
		},
		System: &SystemPrompt{
			Blocks: []ContentPart{
				{
					Type:         "text",
					Text:         "You are helpful",
					CacheControl: &CacheControl{Type: "ephemeral"},
				},
			},
			IsArray: true,
		},
		Tools: []Tool{
			{
				Name:         "get_weather",
				InputSchema:  map[string]interface{}{"type": "object"},
				CacheControl: &CacheControl{Type: "ephemeral"},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Unmarshal back
	var req2 AnthropicRequest
	err = json.Unmarshal(data, &req2)
	require.NoError(t, err)

	// Verify cache_control is preserved
	require.NotNil(t, req2.Messages[0].Content.Parts[0].CacheControl)
	assert.Equal(t, "ephemeral", req2.Messages[0].Content.Parts[0].CacheControl.Type)

	require.NotNil(t, req2.System)
	require.NotNil(t, req2.System.Blocks[0].CacheControl)
	assert.Equal(t, "ephemeral", req2.System.Blocks[0].CacheControl.Type)

	require.NotNil(t, req2.Tools[0].CacheControl)
	assert.Equal(t, "ephemeral", req2.Tools[0].CacheControl.Type)
}

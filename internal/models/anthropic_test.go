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

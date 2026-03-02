package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAdapter(t *testing.T) {
	tests := []struct {
		name     string
		apiType  APIType
		expected string // endpoint path
	}{
		{
			name:     "Anthropic Messages",
			apiType:  APITypeAnthropicMessages,
			expected: "/v1/messages",
		},
		{
			name:     "Anthropic Responses",
			apiType:  APITypeAnthropicResponses,
			expected: "/v1/responses",
		},
		{
			name:     "OpenAI Chat",
			apiType:  APITypeOpenAIChat,
			expected: "/v1/chat/completions",
		},
		{
			name:     "Unknown (fallback to OpenAI)",
			apiType:  APIType("unknown"),
			expected: "/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := GetAdapter(tt.apiType)
			assert.Equal(t, tt.expected, adapter.GetEndpoint())
		})
	}
}

func TestDetermineAPIType(t *testing.T) {
	tests := []struct {
		name            string
		modelAPIType    string
		providerAPIType string
		expected        APIType
	}{
		{
			name:            "Model-level takes priority",
			modelAPIType:    "anthropic_messages",
			providerAPIType: "openai_chat",
			expected:        APITypeAnthropicMessages,
		},
		{
			name:            "Provider-level when model is empty",
			modelAPIType:    "",
			providerAPIType: "anthropic_responses",
			expected:        APITypeAnthropicResponses,
		},
		{
			name:            "Provider-level when model is auto",
			modelAPIType:    "auto",
			providerAPIType: "openai_chat",
			expected:        APITypeOpenAIChat,
		},
		{
			name:            "Auto when both are auto",
			modelAPIType:    "auto",
			providerAPIType: "auto",
			expected:        APITypeAuto,
		},
		{
			name:            "Auto when both are empty",
			modelAPIType:    "",
			providerAPIType: "",
			expected:        APITypeAuto,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineAPIType(tt.modelAPIType, tt.providerAPIType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnthropicMessagesAdapter(t *testing.T) {
	adapter := &AnthropicMessagesAdapter{}

	t.Run("GetEndpoint", func(t *testing.T) {
		assert.Equal(t, "/v1/messages", adapter.GetEndpoint())
	})

	t.Run("BuildRequestBody", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		options := RequestOptions{
			Model:       "claude-3-opus-20240229",
			MaxTokens:   100,
			Temperature: 0.7,
			Stream:      false,
		}

		body, err := adapter.BuildRequestBody(messages, options)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "claude-3-opus-20240229")
		assert.Contains(t, string(body), "Hello")
	})

	t.Run("ParseResponse", func(t *testing.T) {
		responseBody := `{"content":[{"text":"Hello, world!"}]}`
		resp, err := adapter.ParseResponse([]byte(responseBody))
		assert.NoError(t, err)
		assert.Equal(t, "Hello, world!", resp.Content)
	})
}

func TestAnthropicResponsesAdapter(t *testing.T) {
	adapter := &AnthropicResponsesAdapter{}

	t.Run("GetEndpoint", func(t *testing.T) {
		assert.Equal(t, "/v1/responses", adapter.GetEndpoint())
	})

	t.Run("BuildRequestBody", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "Test"},
		}
		options := RequestOptions{
			Model:       "claude-opus-4-6",
			MaxTokens:   50,
			Temperature: 0.0,
			Stream:      false,
		}

		body, err := adapter.BuildRequestBody(messages, options)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "claude-opus-4-6")
		assert.Contains(t, string(body), "Test")
	})
}

func TestOpenAIChatAdapter(t *testing.T) {
	adapter := &OpenAIChatAdapter{}

	t.Run("GetEndpoint", func(t *testing.T) {
		assert.Equal(t, "/v1/chat/completions", adapter.GetEndpoint())
	})

	t.Run("BuildRequestBody", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "Hi"},
		}
		options := RequestOptions{
			Model:       "gpt-4",
			MaxTokens:   200,
			Temperature: 0.5,
			Stream:      true,
		}

		body, err := adapter.BuildRequestBody(messages, options)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "gpt-4")
		assert.Contains(t, string(body), "Hi")
	})

	t.Run("ParseResponse", func(t *testing.T) {
		responseBody := `{"choices":[{"message":{"content":"Response text"}}]}`
		resp, err := adapter.ParseResponse([]byte(responseBody))
		assert.NoError(t, err)
		assert.Equal(t, "Response text", resp.Content)
	})
}

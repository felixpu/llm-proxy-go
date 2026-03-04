package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
)

func TestExtractRoutingMessage_SimpleText(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: msgText("Hello, please help me write code")},
		},
	}
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "Hello, please help me write code", result)
}

func TestExtractRoutingMessage_MultipleMessages(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: msgText("first message")},
			{Role: "assistant", Content: msgText("response")},
			{Role: "user", Content: msgText("second message")},
		},
	}
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "second message", result)
}

func TestExtractRoutingMessage_StripSystemInjections(t *testing.T) {
	text := `<system-reminder>some system context</system-reminder>Please translate this.`
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: msgText(text)},
		},
	}
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "Please translate this.", result)
}

func TestExtractRoutingMessage_StripAllInjectionTags(t *testing.T) {
	tags := []string{
		"system-reminder",
		"command-name",
		"command-message",
		"command-args",
		"local-command-caveat",
		"local-command-stdout",
	}
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			text := "<" + tag + ">injected</" + tag + ">actual content"
			req := &models.AnthropicRequest{
				Messages: []models.Message{
					{Role: "user", Content: msgText(text)},
				},
			}
			result := ExtractRoutingMessage(req)
			assert.Equal(t, "actual content", result)
		})
	}
}

func TestExtractRoutingMessage_MultiContentParts(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{
				Role: "user",
				Content: msgParts(
					models.ContentPart{Type: "text", Text: "part one"},
					models.ContentPart{Type: "text", Text: "part two"},
				),
			},
		},
	}
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "part one\npart two", result)
}

func TestExtractRoutingMessage_SkipToolResultOnlyMessages(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: msgText("real question")},
			{Role: "assistant", Content: msgText("response")},
			{
				Role: "user",
				Content: msgParts(
					models.ContentPart{Type: "tool_result", ToolUseID: "tool1"},
				),
			},
		},
	}
	// The last user message is tool_result only — should skip it and find "real question"
	// But ExtractRoutingMessage does NOT skip tool_result-only messages, it looks for text parts.
	// Since the tool_result message has no text parts, it moves to the previous user message.
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "real question", result)
}

func TestExtractRoutingMessage_EmptyMessages(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{},
	}
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "", result)
}

func TestExtractRoutingMessage_NoUserMessages(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "assistant", Content: msgText("only assistant")},
		},
	}
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "", result)
}

func TestExtractRoutingMessage_EmptyTextParts(t *testing.T) {
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{
				Role: "user",
				Content: msgParts(
					models.ContentPart{Type: "text", Text: ""},
					models.ContentPart{Type: "image", Text: ""},
				),
			},
			{Role: "user", Content: msgText("fallback message")},
		},
	}
	// First user message (from end) has no non-empty text parts, so skip it.
	// Wait, order is reversed. The last message is "fallback message" which has text.
	// Actually no — the first message in the slice is parts, the second is "fallback message".
	// ExtractRoutingMessage iterates from the end, so it finds "fallback message" first.
	result := ExtractRoutingMessage(req)
	assert.Equal(t, "fallback message", result)
}

func TestExtractFromLog_UsesExtractRoutingMessage(t *testing.T) {
	// Build a request with system injection tags that should be stripped
	reqJSON := `{
		"model": "claude-3-sonnet",
		"messages": [
			{"role": "user", "content": "<system-reminder>ctx</system-reminder>Please help me debug this code"},
			{"role": "assistant", "content": "Sure"},
			{"role": "user", "content": "Now translate this to Chinese"}
		],
		"max_tokens": 1024
	}`

	log := &models.RequestLog{
		ID:             42,
		TaskType:       "translation",
		RoutingMethod:  "rule",
		RequestContent: reqJSON,
	}

	extractor := &MessageExtractor{}
	entry := extractor.ExtractFromLog(log)

	// ExtractFromLog should use ExtractRoutingMessage, which returns the last user message
	assert.Equal(t, "Now translate this to Chinese", entry.UserMessage)

	// Also verify that ExtractRoutingMessage returns the same result
	var req models.AnthropicRequest
	require.NoError(t, json.Unmarshal([]byte(reqJSON), &req))
	directResult := ExtractRoutingMessage(&req)
	assert.Equal(t, directResult, entry.UserMessage)
}

func TestExtractFromLog_FallsBackToMessagePreview(t *testing.T) {
	log := &models.RequestLog{
		ID:             1,
		TaskType:       "default",
		RequestContent: "",
		MessagePreview: "preview text",
	}

	extractor := &MessageExtractor{}
	entry := extractor.ExtractFromLog(log)
	assert.Equal(t, "preview text", entry.UserMessage)
}

func TestExtractFromLog_InvalidJSON(t *testing.T) {
	log := &models.RequestLog{
		ID:             1,
		TaskType:       "default",
		RequestContent: "not json",
		MessagePreview: "preview fallback",
	}

	extractor := &MessageExtractor{}
	entry := extractor.ExtractFromLog(log)
	assert.Equal(t, "preview fallback", entry.UserMessage)
}

func TestExtractFromLog_TruncatesLongMessages(t *testing.T) {
	longMsg := make([]byte, 600)
	for i := range longMsg {
		longMsg[i] = 'a'
	}
	reqJSON := `{"model":"test","messages":[{"role":"user","content":"` + string(longMsg) + `"}],"max_tokens":100}`

	log := &models.RequestLog{
		ID:             1,
		RequestContent: reqJSON,
	}

	extractor := &MessageExtractor{}
	entry := extractor.ExtractFromLog(log)
	assert.Len(t, entry.UserMessage, 503) // 500 + "..."
	assert.True(t, entry.UserMessage[500:] == "...")
}

// msgText creates a MessageContent from a plain string.
func msgText(s string) models.MessageContent {
	return models.MessageContent{Text: s, IsArray: false}
}

// msgParts creates a MessageContent from content parts.
func msgParts(parts ...models.ContentPart) models.MessageContent {
	return models.MessageContent{Parts: parts, IsArray: true}
}

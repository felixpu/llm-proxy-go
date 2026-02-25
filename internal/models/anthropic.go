// Package models defines request/response types for the Anthropic API.
package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AnthropicRequest represents a request to the Anthropic Messages API.
type AnthropicRequest struct {
	Model         string            `json:"model"`
	Messages      []Message         `json:"messages"`
	MaxTokens     int               `json:"max_tokens"`
	Stream        bool              `json:"stream,omitempty"`
	System        *SystemPrompt     `json:"system,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	TopP          *float64          `json:"top_p,omitempty"`
	TopK          *int              `json:"top_k,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Tools         []Tool            `json:"tools,omitempty"`
	ToolChoice    *ToolChoice       `json:"tool_choice,omitempty"`
	Thinking      *ThinkingConfig   `json:"thinking,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

// ContentPart represents a part of message content.
// Supports text, image, tool_use, tool_result, and thinking types.
type ContentPart struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
	// tool_use fields
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Input interface{} `json:"input,omitempty"`
	// tool_result fields
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   interface{} `json:"content,omitempty"` // Can be string or []ContentPart
	IsError   *bool       `json:"is_error,omitempty"`
	// thinking field (extended thinking)
	Thinking string `json:"thinking,omitempty"`
}

// MessageContent represents message content that can be either a string or an array of content parts.
// Anthropic API supports both formats:
//   - string: "Hello, how are you?"
//   - array:  [{"type": "text", "text": "Hello"}, {"type": "image", ...}]
type MessageContent struct {
	Text    string        // Plain text when string format
	Parts   []ContentPart // Content parts when array format
	IsArray bool          // Tracks original format for faithful re-serialization
}

// UnmarshalJSON handles both string and array formats.
func (m *MessageContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		m.Text = str
		m.IsArray = false
		return nil
	}

	// Try array of content parts
	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		m.Parts = parts
		m.IsArray = true
		return nil
	}

	return fmt.Errorf("content must be a string or array of content parts")
}

// MarshalJSON preserves the original format.
func (m MessageContent) MarshalJSON() ([]byte, error) {
	if m.IsArray {
		return json.Marshal(m.Parts)
	}
	return json.Marshal(m.Text)
}

// String returns the combined text content.
func (m *MessageContent) String() string {
	if !m.IsArray {
		return m.Text
	}
	var parts []string
	for _, part := range m.Parts {
		if part.Type == "text" && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, " ")
}

// GetParts returns content as []ContentPart (converts string to single text part if needed).
func (m *MessageContent) GetParts() []ContentPart {
	if m.IsArray {
		return m.Parts
	}
	if m.Text == "" {
		return nil
	}
	return []ContentPart{{Type: "text", Text: m.Text}}
}

// ImageSource represents an image source.
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// Tool represents a tool definition.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

// ToolChoice represents tool choice configuration.
type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// ThinkingConfig represents extended thinking configuration.
type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// AnthropicResponse represents a response from the Anthropic Messages API.
type AnthropicResponse struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Role         string        `json:"role"`
	Content      []ContentPart `json:"content"`
	Model        string        `json:"model"`
	StopReason   string        `json:"stop_reason,omitempty"`
	StopSequence string        `json:"stop_sequence,omitempty"`
	Usage        Usage         `json:"usage"`
}

// Usage represents token usage statistics.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent represents a Server-Sent Event for streaming responses.
type StreamEvent struct {
	Type  string      `json:"type"`
	Index int         `json:"index,omitempty"`
	Delta interface{} `json:"delta,omitempty"`
	Usage *Usage      `json:"usage,omitempty"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Type  string     `json:"type"`
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error details.
type ErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SystemPrompt represents a system prompt that can be either a string or an array of content blocks.
// Anthropic API supports both formats:
//   - string: "You are a helpful assistant."
//   - array:  [{"type": "text", "text": "You are a helpful assistant."}]
type SystemPrompt struct {
	Text    string        // Plain text when string format
	Blocks  []ContentPart // Content blocks when array format
	IsArray bool          // Tracks original format for faithful re-serialization
}

// UnmarshalJSON handles both string and array formats.
func (s *SystemPrompt) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Text = str
		s.IsArray = false
		return nil
	}

	// Try array of content blocks
	var blocks []ContentPart
	if err := json.Unmarshal(data, &blocks); err == nil {
		s.Blocks = blocks
		s.IsArray = true
		return nil
	}

	return fmt.Errorf("system must be a string or array of content blocks")
}

// MarshalJSON preserves the original format.
func (s SystemPrompt) MarshalJSON() ([]byte, error) {
	if s.IsArray {
		return json.Marshal(s.Blocks)
	}
	return json.Marshal(s.Text)
}

// String returns the combined text content (used for routing decisions).
func (s *SystemPrompt) String() string {
	if !s.IsArray {
		return s.Text
	}
	var parts []string
	for _, block := range s.Blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, " ")
}

// IsEmpty returns true if the system prompt has no content.
func (s *SystemPrompt) IsEmpty() bool {
	if s == nil {
		return true
	}
	if s.IsArray {
		return len(s.Blocks) == 0
	}
	return s.Text == ""
}

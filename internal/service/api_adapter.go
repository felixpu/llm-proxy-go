package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIType represents supported API types
type APIType string

const (
	APITypeAuto               APIType = "auto"
	APITypeAnthropicMessages  APIType = "anthropic_messages"
	APITypeAnthropicResponses APIType = "anthropic_responses"
	APITypeOpenAIChat         APIType = "openai_chat"
)

// APIAdapter handles different API formats
type APIAdapter interface {
	// GetEndpoint returns the API endpoint path (e.g., "/v1/messages")
	GetEndpoint() string

	// SetAuthHeaders sets authentication headers on the request
	SetAuthHeaders(req *http.Request, apiKey string)

	// BuildRequestBody builds the request body for the API
	BuildRequestBody(messages []Message, options RequestOptions) ([]byte, error)

	// ParseResponse parses the API response
	ParseResponse(body []byte) (*Response, error)
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestOptions contains request parameters
type RequestOptions struct {
	Model       string
	MaxTokens   int
	Temperature float64
	Stream      bool
}

// Response represents a parsed API response
type Response struct {
	Content string
	// Additional fields can be added as needed
}

// GetAdapter returns the appropriate adapter for the API type
func GetAdapter(apiType APIType) APIAdapter {
	switch apiType {
	case APITypeAnthropicMessages:
		return &AnthropicMessagesAdapter{}
	case APITypeAnthropicResponses:
		return &AnthropicResponsesAdapter{}
	case APITypeOpenAIChat:
		return &OpenAIChatAdapter{}
	default:
		return &OpenAIChatAdapter{} // fallback
	}
}

// determineAPIType decides which API type to use based on model and provider config
func determineAPIType(modelAPIType, providerAPIType string) APIType {
	// Priority 1: Model-level configuration
	if modelAPIType != "" && modelAPIType != "auto" {
		return APIType(modelAPIType)
	}

	// Priority 2: Provider-level configuration
	if providerAPIType != "" && providerAPIType != "auto" {
		return APIType(providerAPIType)
	}

	// Priority 3: Auto-detect
	return APITypeAuto
}

// DetectAPIType attempts to detect the API type by trying different endpoints
func DetectAPIType(ctx context.Context, baseURL, apiKey string) (APIType, error) {
	// Try Anthropic Responses API first (newest)
	if tryEndpoint(ctx, baseURL, apiKey, APITypeAnthropicResponses) {
		return APITypeAnthropicResponses, nil
	}

	// Try Anthropic Messages API
	if tryEndpoint(ctx, baseURL, apiKey, APITypeAnthropicMessages) {
		return APITypeAnthropicMessages, nil
	}

	// Try OpenAI Chat Completions API
	if tryEndpoint(ctx, baseURL, apiKey, APITypeOpenAIChat) {
		return APITypeOpenAIChat, nil
	}

	return "", fmt.Errorf("unable to detect API type: all endpoints returned errors. Please manually set api_type in provider configuration")
}

// tryEndpoint sends a minimal test request to check if the endpoint is supported
func tryEndpoint(ctx context.Context, baseURL, apiKey string, apiType APIType) bool {
	adapter := GetAdapter(apiType)
	url := strings.TrimRight(baseURL, "/") + adapter.GetEndpoint()

	// Build minimal test request
	messages := []Message{{Role: "user", Content: "test"}}
	options := RequestOptions{
		Model:       "test",
		MaxTokens:   1,
		Temperature: 0.0,
		Stream:      false,
	}

	bodyBytes, err := adapter.BuildRequestBody(messages, options)
	if err != nil {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	adapter.SetAuthHeaders(req, apiKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Read response to check for specific error messages
	body, _ := io.ReadAll(resp.Body)

	// If we get a 400 with "Unsupported" or "not supported", this endpoint is not supported
	if resp.StatusCode == 400 {
		bodyStr := string(body)
		if strings.Contains(bodyStr, "Unsupported") || strings.Contains(bodyStr, "not supported") {
			return false
		}
	}

	// If we get 401/403, the endpoint exists but auth failed (which is fine for detection)
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return true
	}

	// If we get 200 or other non-400 errors, assume the endpoint is supported
	if resp.StatusCode != 400 {
		return true
	}

	return false
}

// AnthropicMessagesAdapter implements Anthropic Messages API
type AnthropicMessagesAdapter struct{}

func (a *AnthropicMessagesAdapter) GetEndpoint() string {
	return "/v1/messages"
}

func (a *AnthropicMessagesAdapter) SetAuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (a *AnthropicMessagesAdapter) BuildRequestBody(messages []Message, options RequestOptions) ([]byte, error) {
	body := map[string]interface{}{
		"model":       options.Model,
		"messages":    messages,
		"max_tokens":  options.MaxTokens,
		"temperature": options.Temperature,
		"stream":      options.Stream,
	}
	return json.Marshal(body)
}

func (a *AnthropicMessagesAdapter) ParseResponse(body []byte) (*Response, error) {
	var resp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Content) > 0 {
		return &Response{Content: resp.Content[0].Text}, nil
	}
	return &Response{}, nil
}

// AnthropicResponsesAdapter implements Anthropic Responses API
type AnthropicResponsesAdapter struct{}

func (a *AnthropicResponsesAdapter) GetEndpoint() string {
	return "/v1/responses"
}

func (a *AnthropicResponsesAdapter) SetAuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (a *AnthropicResponsesAdapter) BuildRequestBody(messages []Message, options RequestOptions) ([]byte, error) {
	body := map[string]interface{}{
		"model":       options.Model,
		"messages":    messages,
		"max_tokens":  options.MaxTokens,
		"temperature": options.Temperature,
		"stream":      options.Stream,
	}
	return json.Marshal(body)
}

func (a *AnthropicResponsesAdapter) ParseResponse(body []byte) (*Response, error) {
	var resp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Content) > 0 {
		return &Response{Content: resp.Content[0].Text}, nil
	}
	return &Response{}, nil
}

// OpenAIChatAdapter implements OpenAI Chat Completions API
type OpenAIChatAdapter struct{}

func (a *OpenAIChatAdapter) GetEndpoint() string {
	return "/v1/chat/completions"
}

func (a *OpenAIChatAdapter) SetAuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func (a *OpenAIChatAdapter) BuildRequestBody(messages []Message, options RequestOptions) ([]byte, error) {
	body := map[string]interface{}{
		"model":       options.Model,
		"messages":    messages,
		"max_tokens":  options.MaxTokens,
		"temperature": options.Temperature,
		"stream":      options.Stream,
	}
	return json.Marshal(body)
}

func (a *OpenAIChatAdapter) ParseResponse(body []byte) (*Response, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Choices) > 0 {
		return &Response{Content: resp.Choices[0].Message.Content}, nil
	}
	return &Response{}, nil
}

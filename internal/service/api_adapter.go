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

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// APIType represents supported API types
type APIType string

const (
	APITypeAuto               APIType = "auto"
	APITypeAnthropicMessages  APIType = "anthropic_messages"
	APITypeAnthropicResponses APIType = "anthropic_responses"
	APITypeOpenAIChat         APIType = "openai_chat"
)

// DefaultAnthropicVersion is the default API version for Anthropic endpoints.
// Shared by adapters and proxy layer to avoid inconsistency.
const DefaultAnthropicVersion = "2023-06-01"

// maxDetectResponseBytes limits response body size during API type detection
// to prevent unbounded reads from malicious or misconfigured servers.
const maxDetectResponseBytes = 4096

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

// ValidAPITypes lists all supported API type values.
var ValidAPITypes = map[APIType]bool{
	APITypeAuto:               true,
	APITypeAnthropicMessages:  true,
	APITypeAnthropicResponses: true,
	APITypeOpenAIChat:         true,
}

// IsValidAPIType returns whether the given string is a valid API type.
func IsValidAPIType(s string) bool {
	return ValidAPITypes[APIType(s)]
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

// detectClient is a shared HTTP client for API type detection, reusing connections.
var detectClient = &http.Client{Timeout: 5 * time.Second}

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

	resp, err := detectClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Limit response body size to prevent unbounded reads from malicious servers
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDetectResponseBytes))

	switch resp.StatusCode {
	case 200, 401, 403:
		// 200: Success, 401/403: Auth failed but endpoint exists
		return true
	case 404:
		// Endpoint does not exist
		return false
	case 400:
		// Check if it's an "unsupported endpoint" error
		bodyStr := string(body)
		return !strings.Contains(bodyStr, "Unsupported") && !strings.Contains(bodyStr, "not supported")
	default:
		// For 5xx or other errors, assume endpoint is not supported
		return false
	}
}

// baseAnthropicAdapter contains shared logic for all Anthropic API adapters.
type baseAnthropicAdapter struct{}

func (a *baseAnthropicAdapter) SetAuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", DefaultAnthropicVersion)
}

func (a *baseAnthropicAdapter) BuildRequestBody(messages []Message, options RequestOptions) ([]byte, error) {
	body := map[string]interface{}{
		"model":       options.Model,
		"messages":    messages,
		"max_tokens":  options.MaxTokens,
		"temperature": options.Temperature,
		"stream":      options.Stream,
	}
	return json.Marshal(body)
}

func (a *baseAnthropicAdapter) ParseResponse(body []byte) (*Response, error) {
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

// AnthropicMessagesAdapter implements Anthropic Messages API
type AnthropicMessagesAdapter struct {
	baseAnthropicAdapter
}

func (a *AnthropicMessagesAdapter) GetEndpoint() string {
	return "/v1/messages"
}

// AnthropicResponsesAdapter implements Anthropic Responses API
type AnthropicResponsesAdapter struct {
	baseAnthropicAdapter
}

func (a *AnthropicResponsesAdapter) GetEndpoint() string {
	return "/v1/responses"
}

func (a *AnthropicResponsesAdapter) BuildRequestBody(messages []Message, options RequestOptions) ([]byte, error) {
	instructions, input := splitResponsesMessages(messages)
	body := map[string]interface{}{
		"model":             options.Model,
		"input":             input,
		"max_output_tokens": options.MaxTokens,
		"temperature":       options.Temperature,
		"stream":            options.Stream,
	}
	if instructions != "" {
		body["instructions"] = instructions
	}
	return json.Marshal(body)
}

func (a *AnthropicResponsesAdapter) ParseResponse(body []byte) (*Response, error) {
	resp, err := parseAnthropicResponsesResponse(body)
	if err != nil {
		return nil, err
	}

	var parts []string
	for _, part := range resp.Content {
		if part.Type == "text" && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}

	return &Response{Content: strings.Join(parts, "\n")}, nil
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

func splitResponsesMessages(messages []Message) (string, []Message) {
	var instructions []string
	input := make([]Message, 0, len(messages))

	for _, msg := range messages {
		if msg.Role == "system" {
			if strings.TrimSpace(msg.Content) != "" {
				instructions = append(instructions, msg.Content)
			}
			continue
		}
		input = append(input, msg)
	}

	return strings.Join(instructions, "\n\n"), input
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

// LLMCallParams encapsulates parameters for a standard LLM API call.
type LLMCallParams struct {
	ModelCfg   *models.RoutingModelWithProvider
	Messages   []Message
	Options    RequestOptions
	Client     *http.Client
	Logger     *zap.Logger
	LogContext string // e.g. "routing" or "analysis" for log messages
}

// CallLLMModel resolves the API type, builds the request, calls the endpoint,
// and returns the parsed response content. Shared by routing and analysis callers.
func CallLLMModel(ctx context.Context, params LLMCallParams) (string, error) {
	modelCfg := params.ModelCfg

	// Determine API type
	apiType := determineAPIType(modelCfg.APIType, modelCfg.ProviderAPIType)
	if apiType == APITypeAuto {
		detected, err := DetectAPIType(ctx, modelCfg.BaseURL, modelCfg.APIKey)
		if err != nil {
			return "", fmt.Errorf("无法自动检测 API 类型。请在 Provider 配置中手动设置 api_type 字段为以下值之一：\n"+
				"  - anthropic_messages (Anthropic Messages API, /v1/messages)\n"+
				"  - anthropic_responses (Anthropic Responses API, /v1/responses)\n"+
				"  - openai_chat (OpenAI Chat Completions API, /v1/chat/completions)\n"+
				"原始错误: %w", err)
		}
		apiType = detected
		if params.Logger != nil {
			params.Logger.Info("Auto-detected API type",
				zap.String("context", params.LogContext),
				zap.String("model", modelCfg.ModelName),
				zap.String("api_type", string(apiType)))
		}
	}

	adapter := GetAdapter(apiType)

	bodyBytes, err := adapter.BuildRequestBody(params.Messages, params.Options)
	if err != nil {
		return "", fmt.Errorf("marshal %s request: %w", params.LogContext, err)
	}

	url := fmt.Sprintf("%s%s", modelCfg.BaseURL, adapter.GetEndpoint())

	// Debug logging for analysis calls
	if params.Logger != nil && params.LogContext == "analysis" {
		params.Logger.Info("Calling LLM for analysis",
			zap.String("url", url),
			zap.String("api_type", string(apiType)),
			zap.String("model", modelCfg.ModelName),
			zap.String("base_url", modelCfg.BaseURL),
			zap.String("endpoint", adapter.GetEndpoint()))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create %s request: %w", params.LogContext, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	adapter.SetAuthHeaders(httpReq, modelCfg.APIKey)

	resp, err := params.Client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%s API call failed: %w", params.LogContext, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s response: %w", params.LogContext, err)
	}

	if resp.StatusCode != http.StatusOK {
		// Check if it's a 404 endpoint not found error
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("API 端点不存在 (404)。当前使用的 API 类型为 %s，端点为 %s。"+
				"请检查 Provider 配置中的 api_type 字段是否正确。支持的类型："+
				"anthropic_messages (/v1/messages), anthropic_responses (/v1/responses), openai_chat (/v1/chat/completions)",
				apiType, adapter.GetEndpoint())
		}
		// Check if it's an unsupported endpoint error
		if resp.StatusCode == 400 {
			var errResp struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.Unmarshal(respBody, &errResp)
			if strings.Contains(errResp.Error.Message, "not supported") ||
				strings.Contains(errResp.Error.Message, "Unsupported") {
				return "", fmt.Errorf("API 端点不支持: %s。当前使用的 API 类型为 %s，请检查 Provider 配置中的 api_type 字段是否正确",
					errResp.Error.Message, apiType)
			}
		}
		return "", fmt.Errorf("%s API returned status %d: %s", params.LogContext, resp.StatusCode, truncateCallResp(string(respBody), 500))
	}

	parsedResp, err := adapter.ParseResponse(respBody)
	if err != nil {
		return "", fmt.Errorf("parse %s response: %w", params.LogContext, err)
	}

	return parsedResp.Content, nil
}

// truncateCallResp truncates a string to maxLen for error messages.
func truncateCallResp(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

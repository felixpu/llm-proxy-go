package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	detectTimeout    = 15 * time.Second
	anthropicVersion = "2023-06-01"
)

// DetectedModel represents a detected model from a provider.
type DetectedModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
}

// DetectionResult represents the result of model detection.
type DetectionResult struct {
	Success   bool            `json:"success"`
	Models    []DetectedModel `json:"models"`
	APIFormat string          `json:"api_format,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ModelDetector detects available models from a provider.
// Strategy: try OpenAI format first, then Anthropic format.
type ModelDetector struct {
	client *http.Client
	logger *zap.Logger
}

// NewModelDetector creates a new ModelDetector.
func NewModelDetector(logger *zap.Logger) *ModelDetector {
	return &ModelDetector{
		client: &http.Client{Timeout: detectTimeout},
		logger: logger,
	}
}

// normalizeBaseURL ensures the URL doesn't end with "/" or "/v1".
func normalizeBaseURL(baseURL string) string {
	url := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(url, "/v1") {
		url = url[:len(url)-3]
	}
	return url
}

// Detect detects available models from a provider.
// Tries OpenAI format first, then Anthropic format.
func (d *ModelDetector) Detect(ctx context.Context, baseURL, apiKey string) *DetectionResult {
	modelsURL := normalizeBaseURL(baseURL) + "/v1/models"

	// Strategy 1: OpenAI format
	result, status := d.tryOpenAIFormat(ctx, modelsURL, apiKey)
	if result != nil {
		return result
	}

	// Strategy 2: Anthropic format
	result, _ = d.tryAnthropicFormat(ctx, modelsURL, apiKey)
	if result != nil {
		return result
	}

	errMsg := fmt.Sprintf("无法检测模型列表（HTTP %d），请检查 Base URL 和 API Key 是否正确", status)
	return &DetectionResult{Success: false, Models: []DetectedModel{}, Error: errMsg}
}

// tryOpenAIFormat tries to detect models using OpenAI API format.
func (d *ModelDetector) tryOpenAIFormat(ctx context.Context, url, apiKey string) (*DetectionResult, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			return &DetectionResult{Success: false, Error: "连接超时，请检查网络或 Base URL"}, 0
		}
		if isConnectionError(err) {
			return &DetectionResult{Success: false, Error: "网络连接失败，请检查 Base URL"}, 0
		}
		return nil, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0
	}

	var data struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			OwnedBy     string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, 0
	}

	models := make([]DetectedModel, 0, len(data.Data))
	for _, m := range data.Data {
		models = append(models, DetectedModel{
			ID:          m.ID,
			DisplayName: m.DisplayName,
			OwnedBy:     m.OwnedBy,
		})
	}

	return &DetectionResult{Success: true, Models: models, APIFormat: "openai"}, http.StatusOK
}

// tryAnthropicFormat tries to detect models using Anthropic API format.
func (d *ModelDetector) tryAnthropicFormat(ctx context.Context, url, apiKey string) (*DetectionResult, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := d.client.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			return &DetectionResult{Success: false, Error: "连接超时，请检查网络或 Base URL"}, 0
		}
		if isConnectionError(err) {
			return &DetectionResult{Success: false, Error: "网络连接失败，请检查 Base URL"}, 0
		}
		return nil, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0
	}

	var data struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			CreatedBy   string `json:"created_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, 0
	}

	models := make([]DetectedModel, 0, len(data.Data))
	for _, m := range data.Data {
		models = append(models, DetectedModel{
			ID:          m.ID,
			DisplayName: m.DisplayName,
			OwnedBy:     m.CreatedBy, // Anthropic uses created_by
		})
	}

	return &DetectionResult{Success: true, Models: models, APIFormat: "anthropic"}, http.StatusOK
}

// isTimeoutError checks if the error is a timeout error.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "Timeout")
}

// isConnectionError checks if the error is a connection error.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable")
}

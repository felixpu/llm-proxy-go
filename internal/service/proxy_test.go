//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func TestNewProxyService(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)

	ps := NewProxyService(hc, lb, nil, logger)
	assert.NotNil(t, ps.client)
	assert.NotNil(t, ps.streamClient)
	assert.NotNil(t, ps.healthChecker)
	assert.NotNil(t, ps.loadBalancer)
}

func TestProxyService_ProxyRequest_NoHealthyEndpoints(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	// Nil selection (no endpoint selected)
	resp, meta, err := ps.ProxyRequest(context.Background(), req, nil, nil, []*models.Endpoint{})
	assert.Nil(t, resp)
	assert.Nil(t, meta)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoint selected")
}

func TestProxyService_ProxyRequest_Success(t *testing.T) {
	// Create mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("x-api-key"))

		// Return mock response
		resp := models.AnthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-sonnet-20240229",
			Content: []models.ContentPart{
				{Type: "text", Text: "Hello! How can I help you?"},
			},
			StopReason: "end_turn",
			Usage:      models.Usage{InputTokens: 10, OutputTokens: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{Enabled: true}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	// Create endpoint pointing to mock server
	ep := &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    "test-provider",
			BaseURL: upstream.URL,
			APIKey:  "test-key",
			Enabled: true,
		},
		Model: &models.Model{
			ID:                1,
			Name:              "claude-3-sonnet",
			Role:              models.ModelRoleDefault,
			CostPerMtokInput:  3.0,
			CostPerMtokOutput: 15.0,
			BillingMultiplier: 1.0,
			Enabled:           true,
		},
		Status: models.EndpointHealthy,
	}

	// Register endpoint as healthy
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep,
		Model:    ep.Model,
		TaskType: ep.Model.Role,
	}
	resp, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, meta)

	assert.Equal(t, "msg_123", resp.ID)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 20, resp.Usage.OutputTokens)

	assert.NotEmpty(t, meta.RequestID)
	assert.Equal(t, "claude-3-sonnet", meta.SelectedModel)
	assert.Equal(t, "test-provider", meta.SelectedEndpoint)
	assert.Equal(t, 10, meta.InputTokens)
	assert.Equal(t, 20, meta.OutputTokens)
	assert.GreaterOrEqual(t, meta.LatencyMs, float64(0))
}

func TestProxyService_ProxyRequest_UpstreamError(t *testing.T) {
	// Create mock upstream server that returns error
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Invalid request"}}`))
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	ep := createProxyTestEndpoint(upstream.URL)
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep,
		Model:    ep.Model,
		TaskType: ep.Model.Role,
	}
	resp, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep})
	assert.Nil(t, resp)
	assert.Nil(t, meta)
	assert.Error(t, err)

	// Should be UpstreamError
	var upErr *UpstreamError
	require.True(t, errors.As(err, &upErr))
	assert.Equal(t, http.StatusBadRequest, upErr.StatusCode)
}

func TestProxyService_ProxyRequest_ServerError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Internal error"}}`))
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	ep := createProxyTestEndpoint(upstream.URL)
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep,
		Model:    ep.Model,
		TaskType: ep.Model.Role,
	}
	resp, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep})
	assert.Nil(t, resp)
	assert.Nil(t, meta)
	assert.Error(t, err)

	// With retry logic, 500 errors trigger retry. Since there's only one endpoint,
	// the error is wrapped as "all endpoints failed for model...". Use errors.As to unwrap.
	var upErr *UpstreamError
	require.True(t, errors.As(err, &upErr), "expected UpstreamError, got: %v", err)
	assert.Equal(t, http.StatusInternalServerError, upErr.StatusCode)
}

func TestProxyService_ProxyStreamRequest_NoHealthyEndpoints(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Stream:    true,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	ch, meta, err := ps.ProxyStreamRequest(context.Background(), req, nil, nil, []*models.Endpoint{})
	assert.Nil(t, ch)
	assert.Nil(t, meta)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoint selected")
}

func TestProxyService_ProxyStreamRequest_UpstreamError(t *testing.T) {
	// 401 is now retryable — with only one endpoint, it should exhaust retries
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"Invalid API key"}}`))
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	ep := createProxyTestEndpoint(upstream.URL)
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep,
		Model:    ep.Model,
		TaskType: ep.Model.Role,
	}
	ch, meta, err := ps.ProxyStreamRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep})
	assert.Nil(t, ch)
	assert.Nil(t, meta)
	assert.Error(t, err)

	// 401 is retryable, so with one endpoint it wraps as "all endpoints failed"
	var upErr *UpstreamError
	require.True(t, errors.As(err, &upErr), "expected UpstreamError, got: %v", err)
	assert.Equal(t, http.StatusUnauthorized, upErr.StatusCode)
}

func TestProxyService_StreamModelNameMapping(t *testing.T) {
	// Test that streaming requests correctly map model names
	var receivedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body to verify model name
		var req models.AnthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedModel = req.Model

		// Return SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// Send a simple SSE event
		w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-3-sonnet-20240229\",\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n"))
		flusher.Flush()

		w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"))
		flusher.Flush()

		w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{Enabled: true}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	ep := &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    "test-provider",
			BaseURL: upstream.URL,
			APIKey:  "test-key",
			Enabled: true,
		},
		Model: &models.Model{
			ID:                1,
			Name:              "claude-3-sonnet-20240229", // Actual model name
			Role:              models.ModelRoleDefault,
			CostPerMtokInput:  3.0,
			CostPerMtokOutput: 15.0,
			BillingMultiplier: 1.0,
			Enabled:           true,
		},
		Status: models.EndpointHealthy,
	}
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	// Client sends request with "auto" model
	req := &models.AnthropicRequest{
		Model:     "auto", // Client uses "auto"
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep,
		Model:    ep.Model,
		TaskType: ep.Model.Role,
	}

	ch, meta, err := ps.ProxyStreamRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep})
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.NotNil(t, meta)

	// Consume the stream
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
	}

	// Verify that upstream received the endpoint's model name, not "auto"
	assert.Equal(t, "claude-3-sonnet-20240229", receivedModel, "upstream should receive endpoint's model name in stream request")
	assert.Equal(t, "claude-3-sonnet-20240229", meta.SelectedModel, "metadata should reflect selected model")
}

func TestUpstreamError_Error(t *testing.T) {
	err := &UpstreamError{StatusCode: 400, Body: []byte("bad request")}
	assert.Equal(t, "upstream returned status 400", err.Error())
}

func TestHeaderOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		key      string
		def      string
		expected string
	}{
		{
			name:     "header exists",
			headers:  http.Header{"X-Custom": []string{"value"}},
			key:      "X-Custom",
			def:      "default",
			expected: "value",
		},
		{
			name:     "header missing",
			headers:  http.Header{},
			key:      "X-Custom",
			def:      "default",
			expected: "default",
		},
		{
			name:     "nil headers",
			headers:  nil,
			key:      "X-Custom",
			def:      "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := headerOrDefault(tt.headers, tt.key, tt.def)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCopyAnthropicHeaders(t *testing.T) {
	src := http.Header{
		"Anthropic-Beta":    []string{"beta-feature"},
		"Anthropic-Version": []string{"2023-06-01"}, // Should NOT be copied
		"Content-Type":      []string{"application/json"},
		"Anthropic-Custom":  []string{"custom-value"},
	}
	dst := http.Header{}

	copyAnthropicHeaders(src, dst)

	assert.Equal(t, "beta-feature", dst.Get("Anthropic-Beta"))
	assert.Equal(t, "custom-value", dst.Get("Anthropic-Custom"))
	assert.Empty(t, dst.Get("Anthropic-Version")) // Should not be copied
	assert.Empty(t, dst.Get("Content-Type"))      // Should not be copied
}

func TestMsSince(t *testing.T) {
	// Just verify it returns a positive value
	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	ms := msSince(start)
	assert.GreaterOrEqual(t, ms, float64(10))
}

func TestProxy_CalculateCost(t *testing.T) {
	model := &models.Model{
		CostPerMtokInput:  3.0,  // $3 per million input tokens
		CostPerMtokOutput: 15.0, // $15 per million output tokens
		BillingMultiplier: 1.0,
	}

	usage := models.Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	cost := calculateCost(model, usage)
	// Input: 1000/1M * 3 = 0.003
	// Output: 500/1M * 15 * 1 = 0.0075
	// Total: 0.0105
	assert.InDelta(t, 0.0105, cost, 0.0001)
}

func TestCalculateCostFromTokens(t *testing.T) {
	model := &models.Model{
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 2.0, // 2x multiplier
	}

	cost := calculateCostFromTokens(model, 1000, 500)
	// Input: 1000/1M * 3 = 0.003
	// Output: 500/1M * 15 * 2 = 0.015
	// Total: 0.018
	assert.InDelta(t, 0.018, cost, 0.0001)
}

func TestProxyService_ModelNameMapping(t *testing.T) {
	// Test that the proxy correctly maps client's model name to endpoint's model name
	var receivedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body to verify model name
		var req models.AnthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedModel = req.Model

		// Return mock response
		resp := models.AnthropicResponse{
			ID:         "msg_123",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-3-sonnet-20240229",
			Content:    []models.ContentPart{{Type: "text", Text: "Hello!"}},
			StopReason: "end_turn",
			Usage:      models.Usage{InputTokens: 10, OutputTokens: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{Enabled: true}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	ep := &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    "test-provider",
			BaseURL: upstream.URL,
			APIKey:  "test-key",
			Enabled: true,
		},
		Model: &models.Model{
			ID:                1,
			Name:              "claude-3-sonnet-20240229", // Actual model name
			Role:              models.ModelRoleDefault,
			CostPerMtokInput:  3.0,
			CostPerMtokOutput: 15.0,
			BillingMultiplier: 1.0,
			Enabled:           true,
		},
		Status: models.EndpointHealthy,
	}
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	// Client sends request with "auto" model
	req := &models.AnthropicRequest{
		Model:     "auto", // Client uses "auto"
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep,
		Model:    ep.Model,
		TaskType: ep.Model.Role,
	}

	resp, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, meta)

	// Verify that upstream received the endpoint's model name, not "auto"
	assert.Equal(t, "claude-3-sonnet-20240229", receivedModel, "upstream should receive endpoint's model name")
	assert.Equal(t, "claude-3-sonnet-20240229", meta.SelectedModel, "metadata should reflect selected model")
}

// Helper function to create test endpoint
func createProxyTestEndpoint(baseURL string) *models.Endpoint {
	return &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    "test-provider",
			BaseURL: baseURL,
			APIKey:  "test-key",
			Enabled: true,
		},
		Model: &models.Model{
			ID:                1,
			Name:              "claude-3-sonnet",
			Role:              models.ModelRoleDefault,
			CostPerMtokInput:  3.0,
			CostPerMtokOutput: 15.0,
			BillingMultiplier: 1.0,
			Enabled:           true,
		},
		Status: models.EndpointHealthy,
	}
}

// registerHealthyEndpoints registers endpoints as healthy in the HealthChecker.
func registerHealthyEndpoints(hc *HealthChecker, endpoints []*models.Endpoint) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	for _, ep := range endpoints {
		name := EndpointName(ep)
		hc.states[name] = &EndpointState{
			Name:   name,
			Status: models.EndpointHealthy,
		}
	}
}

// TestIsRetryableStatusCode verifies the status code classification logic.
func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{400, false}, // Bad request
		{401, true},  // Unauthorized (invalid key)
		{402, true},  // Payment required (insufficient balance)
		{403, true},  // Forbidden (quota/permission)
		{404, false}, // Not found
		{408, true},  // Request timeout
		{413, false}, // Payload too large
		{422, false}, // Unprocessable entity
		{429, true},  // Too many requests (rate limit)
		{500, true},  // Internal server error
		{502, true},  // Bad gateway
		{503, true},  // Service unavailable
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			result := isRetryableStatusCode(tt.code)
			assert.Equal(t, tt.retryable, result, "status %d retryable=%v", tt.code, tt.retryable)
		})
	}
}

// TestProxyService_ProxyRequest_RetryOn403 verifies that 403 triggers fallback to alternative endpoints.
func TestProxyService_ProxyRequest_RetryOn403(t *testing.T) {
	// First provider returns 403 (quota exceeded)
	provider1Calls := 0
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider1Calls++
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"type":"error","error":{"type":"permission_error","message":"Quota exceeded"}}`))
	}))
	defer upstream1.Close()

	// Second provider succeeds
	provider2Calls := 0
	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider2Calls++
		resp := models.AnthropicResponse{
			ID:         "msg_123",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-3-sonnet",
			Content:    []models.ContentPart{{Type: "text", Text: "Success from provider2"}},
			StopReason: "end_turn",
			Usage:      models.Usage{InputTokens: 10, OutputTokens: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream2.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	model := &models.Model{
		ID:                1,
		Name:              "claude-3-sonnet",
		Role:              models.ModelRoleDefault,
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 1.0,
		Enabled:           true,
	}

	ep1 := &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    "provider1",
			BaseURL: upstream1.URL,
			APIKey:  "key1",
			Enabled: true,
		},
		Model:  model,
		Status: models.EndpointHealthy,
	}

	ep2 := &models.Endpoint{
		Provider: &models.Provider{
			ID:      2,
			Name:    "provider2",
			BaseURL: upstream2.URL,
			APIKey:  "key2",
			Enabled: true,
		},
		Model:  model,
		Status: models.EndpointHealthy,
	}

	registerHealthyEndpoints(hc, []*models.Endpoint{ep1, ep2})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep1,
		Model:    model,
		TaskType: model.Role,
	}

	resp, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep1, ep2})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, meta)

	assert.Equal(t, 1, provider1Calls, "provider1 should be called once")
	assert.Equal(t, 1, provider2Calls, "provider2 should be called once after fallback")
	assert.Equal(t, "provider2", meta.SelectedEndpoint, "should fallback to provider2")
	assert.Equal(t, "Success from provider2", resp.Content[0].Text)
}

// TestProxyService_ProxyStreamRequest_RetryOn403 verifies that 403 triggers fallback in streaming requests.
func TestProxyService_ProxyStreamRequest_RetryOn403(t *testing.T) {
	provider1Calls := 0
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider1Calls++
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"type":"error","error":{"type":"permission_error","message":"Quota exceeded"}}`))
	}))
	defer upstream1.Close()

	provider2Calls := 0
	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider2Calls++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n"))
		flusher.Flush()

		w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Success\"}}\n\n"))
		flusher.Flush()

		w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n"))
		flusher.Flush()
	}))
	defer upstream2.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	model := &models.Model{
		ID:                1,
		Name:              "claude-3-sonnet",
		Role:              models.ModelRoleDefault,
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 1.0,
		Enabled:           true,
	}

	ep1 := &models.Endpoint{
		Provider: &models.Provider{ID: 1, Name: "provider1", BaseURL: upstream1.URL, APIKey: "key1", Enabled: true},
		Model:    model,
		Status:   models.EndpointHealthy,
	}

	ep2 := &models.Endpoint{
		Provider: &models.Provider{ID: 2, Name: "provider2", BaseURL: upstream2.URL, APIKey: "key2", Enabled: true},
		Model:    model,
		Status:   models.EndpointHealthy,
	}

	registerHealthyEndpoints(hc, []*models.Endpoint{ep1, ep2})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep1,
		Model:    model,
		TaskType: model.Role,
	}

	ch, meta, err := ps.ProxyStreamRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep1, ep2})
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.NotNil(t, meta)

	// Consume stream
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
	}

	assert.Equal(t, 1, provider1Calls, "provider1 should be called once")
	assert.Equal(t, 1, provider2Calls, "provider2 should be called once after fallback")
	assert.Equal(t, "provider2", meta.SelectedEndpoint, "should fallback to provider2")
}

// TestProxyService_ProxyRequest_NoRetryOn400 verifies that 400 does NOT trigger retry.
func TestProxyService_ProxyRequest_NoRetryOn400(t *testing.T) {
	provider1Calls := 0
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider1Calls++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Bad request"}}`))
	}))
	defer upstream1.Close()

	provider2Calls := 0
	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider2Calls++
		resp := models.AnthropicResponse{
			ID:         "msg_123",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-3-sonnet",
			Content:    []models.ContentPart{{Type: "text", Text: "Should not reach here"}},
			StopReason: "end_turn",
			Usage:      models.Usage{InputTokens: 10, OutputTokens: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream2.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	model := &models.Model{
		ID:                1,
		Name:              "claude-3-sonnet",
		Role:              models.ModelRoleDefault,
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 1.0,
		Enabled:           true,
	}

	ep1 := &models.Endpoint{
		Provider: &models.Provider{ID: 1, Name: "provider1", BaseURL: upstream1.URL, APIKey: "key1", Enabled: true},
		Model:    model,
		Status:   models.EndpointHealthy,
	}

	ep2 := &models.Endpoint{
		Provider: &models.Provider{ID: 2, Name: "provider2", BaseURL: upstream2.URL, APIKey: "key2", Enabled: true},
		Model:    model,
		Status:   models.EndpointHealthy,
	}

	registerHealthyEndpoints(hc, []*models.Endpoint{ep1, ep2})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello"}},
		},
	}

	selection := &EndpointSelectionResult{
		Endpoint: ep1,
		Model:    model,
		TaskType: model.Role,
	}

	resp, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep1, ep2})
	assert.Nil(t, resp)
	assert.Nil(t, meta)
	assert.Error(t, err)

	// Should be UpstreamError with 400
	var upErr *UpstreamError
	require.True(t, errors.As(err, &upErr))
	assert.Equal(t, http.StatusBadRequest, upErr.StatusCode)

	// Verify no retry happened
	assert.Equal(t, 1, provider1Calls, "provider1 should be called once")
	assert.Equal(t, 0, provider2Calls, "provider2 should NOT be called (400 is non-retryable)")
}

// TestStreamLatency verifies TTFB calculation logic.
func TestStreamLatency(t *testing.T) {
	start := time.Now()
	time.Sleep(10 * time.Millisecond)

	t.Run("with first byte time", func(t *testing.T) {
		firstByte := start.Add(5 * time.Millisecond)
		latency := streamLatency(firstByte, start)
		assert.Equal(t, float64(5), latency, "should use TTFB")
	})

	t.Run("without first byte time (zero)", func(t *testing.T) {
		latency := streamLatency(time.Time{}, start)
		assert.GreaterOrEqual(t, latency, float64(10), "should fallback to msSince(start)")
	})
}

// TestBuildStreamMeta verifies metadata construction for stream chunks.
func TestBuildStreamMeta(t *testing.T) {
	meta := &ProxyMetadata{
		RequestID:        "req-123",
		SelectedModel:    "claude-3-sonnet",
		SelectedEndpoint: "provider1",
		Stream:           true,
		StatusCode:       200,
	}
	ep := &models.Endpoint{
		Model: &models.Model{
			CostPerMtokInput:  3.0,
			CostPerMtokOutput: 15.0,
			BillingMultiplier: 1.0,
		},
	}

	result := buildStreamMeta(meta, ep, false, 42.0, 100, 50)

	assert.Equal(t, "req-123", result.RequestID)
	assert.Equal(t, float64(42), result.LatencyMs)
	assert.Equal(t, 100, result.InputTokens)
	assert.Equal(t, 50, result.OutputTokens)
	assert.False(t, result.Success)
	assert.Greater(t, result.Cost, float64(0))

	// Verify original meta is not mutated
	assert.Equal(t, float64(0), meta.LatencyMs)
	assert.Equal(t, 0, meta.InputTokens)
}

// TestProxyService_StreamContextCancel verifies stats are updated when context is cancelled.
func TestProxyService_StreamContextCancel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// Send first chunk
		w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n"))
		flusher.Flush()

		// Block until client disconnects
		<-r.Context().Done()
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{Enabled: true}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	ep := createProxyTestEndpoint(upstream.URL)
	registerHealthyEndpoints(hc, []*models.Endpoint{ep})

	req := &models.AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 100,
		Messages:  []models.Message{{Role: "user", Content: models.MessageContent{Text: "Hello"}}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	selection := &EndpointSelectionResult{Endpoint: ep, Model: ep.Model, TaskType: ep.Model.Role}

	ch, _, err := ps.ProxyStreamRequest(ctx, req, http.Header{}, selection, []*models.Endpoint{ep})
	require.NoError(t, err)

	// Read first data chunk to ensure stream started
	chunk := <-ch
	require.NoError(t, chunk.Err)
	require.False(t, chunk.Done)

	// Cancel context
	cancel()

	// Drain remaining chunks and find the final one
	var finalChunk StreamChunk
	for c := range ch {
		finalChunk = c
	}

	assert.True(t, finalChunk.Done, "final chunk should be Done")
	assert.Error(t, finalChunk.Err, "final chunk should have context error")
	assert.NotNil(t, finalChunk.Meta, "final chunk should have metadata")
	assert.False(t, finalChunk.Meta.Success, "cancelled request should not be successful")
	assert.GreaterOrEqual(t, finalChunk.Meta.LatencyMs, float64(0), "latency should be set")
}

// TestProxyService_RetryUsesPerAttemptTiming verifies each retry attempt measures its own latency.
func TestProxyService_RetryUsesPerAttemptTiming(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: slow then fail with retryable error
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"type":"error"}`))
			return
		}
		// Second call: fast success
		resp := models.AnthropicResponse{
			ID:         "msg_ok",
			Type:       "message",
			Role:       "assistant",
			Model:      "claude-3-sonnet",
			Content:    []models.ContentPart{{Type: "text", Text: "OK"}},
			StopReason: "end_turn",
			Usage:      models.Usage{InputTokens: 5, OutputTokens: 3},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ps := NewProxyService(hc, lb, nil, logger)

	model := &models.Model{
		ID: 1, Name: "claude-3-sonnet", Role: models.ModelRoleDefault,
		CostPerMtokInput: 3.0, CostPerMtokOutput: 15.0, BillingMultiplier: 1.0, Enabled: true,
	}
	ep1 := &models.Endpoint{
		Provider: &models.Provider{ID: 1, Name: "p1", BaseURL: upstream.URL, APIKey: "k1", Enabled: true},
		Model: model, Status: models.EndpointHealthy,
	}
	ep2 := &models.Endpoint{
		Provider: &models.Provider{ID: 2, Name: "p2", BaseURL: upstream.URL, APIKey: "k2", Enabled: true},
		Model: model, Status: models.EndpointHealthy,
	}
	registerHealthyEndpoints(hc, []*models.Endpoint{ep1, ep2})

	req := &models.AnthropicRequest{
		Model: "claude-3-sonnet", MaxTokens: 100,
		Messages: []models.Message{{Role: "user", Content: models.MessageContent{Text: "Hi"}}},
	}
	selection := &EndpointSelectionResult{Endpoint: ep1, Model: model, TaskType: model.Role}

	_, meta, err := ps.ProxyRequest(context.Background(), req, http.Header{}, selection, []*models.Endpoint{ep1, ep2})
	require.NoError(t, err)

	// The successful retry's latency should be less than the first attempt's 50ms sleep
	// (it measures only the second attempt, not cumulative time)
	assert.Less(t, meta.LatencyMs, float64(50),
		"retry latency should measure only the successful attempt, not cumulative time")
}

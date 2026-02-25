//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.example.com", "https://api.example.com"},
		{"https://api.example.com/", "https://api.example.com"},
		{"https://api.example.com/v1", "https://api.example.com"},
		{"https://api.example.com/v1/", "https://api.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeBaseURL(tt.input))
		})
	}
}

func TestModelDetector_Detect_OpenAIFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		resp := map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4", "display_name": "GPT-4", "owned_by": "openai"},
				{"id": "gpt-3.5-turbo", "owned_by": "openai"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	detector := NewModelDetector(zap.NewNop())
	result := detector.Detect(context.Background(), server.URL, "test-key")

	require.True(t, result.Success)
	assert.Equal(t, "openai", result.APIFormat)
	assert.Len(t, result.Models, 2)
	assert.Equal(t, "gpt-4", result.Models[0].ID)
	assert.Equal(t, "GPT-4", result.Models[0].DisplayName)
}

func TestModelDetector_Detect_AnthropicFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First request (OpenAI) returns 401, second (Anthropic) succeeds
		if r.Header.Get("x-api-key") != "" {
			assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
			resp := map[string]any{
				"data": []map[string]any{
					{"id": "claude-3-opus", "display_name": "Claude 3 Opus", "created_by": "anthropic"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	detector := NewModelDetector(zap.NewNop())
	result := detector.Detect(context.Background(), server.URL, "test-key")

	require.True(t, result.Success)
	assert.Equal(t, "anthropic", result.APIFormat)
	assert.Len(t, result.Models, 1)
	assert.Equal(t, "claude-3-opus", result.Models[0].ID)
	assert.Equal(t, "anthropic", result.Models[0].OwnedBy)
}

func TestModelDetector_Detect_BothFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	detector := NewModelDetector(zap.NewNop())
	result := detector.Detect(context.Background(), server.URL, "bad-key")

	assert.False(t, result.Success)
	assert.Empty(t, result.Models)
	assert.Contains(t, result.Error, "无法检测模型列表")
}

func TestModelDetector_Detect_ConnectionError(t *testing.T) {
	detector := NewModelDetector(zap.NewNop())
	result := detector.Detect(context.Background(), "http://127.0.0.1:1", "key")

	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
}

func TestModelDetector_Detect_URLWithTrailingV1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		resp := map[string]any{
			"data": []map[string]any{
				{"id": "model-1"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	detector := NewModelDetector(zap.NewNop())
	// Pass URL with /v1 suffix — should not produce /v1/v1/models
	result := detector.Detect(context.Background(), server.URL+"/v1", "key")

	require.True(t, result.Success)
	assert.Len(t, result.Models, 1)
}

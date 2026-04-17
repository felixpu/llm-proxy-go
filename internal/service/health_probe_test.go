package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
)

func TestProbeEndpointStatus_UsesModelsProbeWithAuthHeaders(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotXAPIKey string
	var gotVersion string
	var gotCustom string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotXAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotCustom = r.Header.Get("X-Custom-Gateway")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL,
			APIKey:  "test-key",
			CustomHeaders: map[string]string{
				"X-Custom-Gateway": "gw-header",
			},
		},
		Model: &models.Model{Name: "gpt-4o-mini"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointHealthy, status)
	assert.Empty(t, errMsg)
	assert.Equal(t, "/v1/models", gotPath)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "test-key", gotXAPIKey)
	assert.Equal(t, DefaultAnthropicVersion, gotVersion)
	assert.Equal(t, "gw-header", gotCustom)
}

func TestProbeEndpointStatus_FallbackToMessagesProbeWhenModelsUnsupported(t *testing.T) {
	var modelsCalls int32
	var messagesCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.WriteHeader(http.StatusNotFound)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			assert.Equal(t, http.MethodPost, r.Method)
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `claude-sonnet-4-6`)
			w.WriteHeader(http.StatusBadRequest) // validation error should be treated as healthy
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL + "/v1",
			APIKey:  "test-key",
		},
		Model: &models.Model{Name: "claude-sonnet-4-6"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointHealthy, status)
	assert.Empty(t, errMsg)
	assert.Equal(t, int32(1), atomic.LoadInt32(&modelsCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&messagesCalls))
}

func TestProbeEndpointStatus_401IsUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL,
			APIKey:  "bad-key",
		},
		Model: &models.Model{Name: "gpt-4o-mini"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointUnhealthy, status)
	assert.Contains(t, errMsg, "auth failed")
}

func TestProbeEndpointStatus_404AfterFallbackIsUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL + "/v1",
			APIKey:  "test-key",
		},
		Model: &models.Model{Name: "gpt-4o-mini"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointUnhealthy, status)
	assert.Contains(t, errMsg, "endpoint not found")
}

func TestProbeEndpointStatus_OpenAIChatFallbackValidationErrorHealthy(t *testing.T) {
	var modelsCalls int32
	var chatCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			atomic.AddInt32(&chatCalls, 1)
			assert.Equal(t, http.MethodPost, r.Method)
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `gpt-4o-mini`)
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL,
			APIKey:  "test-key",
			APIType: string(APITypeOpenAIChat),
		},
		Model: &models.Model{Name: "gpt-4o-mini"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointHealthy, status)
	assert.Empty(t, errMsg)
	assert.Equal(t, int32(1), atomic.LoadInt32(&modelsCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&chatCalls))
}

func TestProbeEndpointStatus_MessagesFallbackModelNotFound404Unhealthy(t *testing.T) {
	var modelsCalls int32
	var messagesCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.WriteHeader(http.StatusNotFound)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			assert.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL,
			APIKey:  "test-key",
			APIType: string(APITypeAnthropicMessages),
		},
		Model: &models.Model{Name: "claude-opus-4-6"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointUnhealthy, status)
	assert.Contains(t, errMsg, "model unavailable")
	assert.Equal(t, int32(1), atomic.LoadInt32(&modelsCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&messagesCalls))
}

func TestProbeEndpointStatus_MessagesFallbackModelNotAllowed403Unhealthy(t *testing.T) {
	var modelsCalls int32
	var messagesCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.WriteHeader(http.StatusNotFound)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			assert.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"type":"model_not_allowed","message":"Model is not allowed for this virtual key"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL,
			APIKey:  "test-key",
			APIType: string(APITypeAnthropicMessages),
		},
		Model: &models.Model{Name: "claude-sonnet-4-6"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointUnhealthy, status)
	assert.Contains(t, errMsg, "model unavailable")
	assert.Equal(t, int32(1), atomic.LoadInt32(&modelsCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&messagesCalls))
}

func TestProbeEndpointStatus_5xxIsUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	ep := &models.Endpoint{
		Provider: &models.Provider{
			Name:    "p1",
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
		Model: &models.Model{Name: "claude-sonnet-4-6"},
	}

	status, errMsg := probeEndpointStatus(context.Background(), client, ep)
	assert.Equal(t, models.EndpointUnhealthy, status)
	assert.Contains(t, errMsg, "upstream error")
}

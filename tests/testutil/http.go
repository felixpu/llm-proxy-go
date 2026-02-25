package testutil

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// CurrentUser represents the authenticated user in context.
type CurrentUser struct {
	UserID       int64
	Username     string
	Role         string
	APIKeyPrefix *string
	APIKeyID     *int64
}

// TestServerConfig holds configuration for creating a test server.
type TestServerConfig struct {
	DB            *sql.DB
	Logger        *zap.Logger
	Authenticated bool
	User          *CurrentUser
}

// NewTestLogger creates a no-op logger for testing.
func NewTestLogger() *zap.Logger {
	return zap.NewNop()
}

// NewTestRouter creates a Gin router configured for testing.
func NewTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// NewTestContext creates a Gin context for testing.
func NewTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

// NewTestContextWithRequest creates a Gin context with a request.
func NewTestContextWithRequest(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var req *http.Request
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	c.Request = req

	return c, w
}

// SetCurrentUser sets the current user in the Gin context.
func SetCurrentUser(c *gin.Context, user *CurrentUser) {
	c.Set("user_id", user.UserID)
	c.Set("username", user.Username)
	c.Set("role", user.Role)
	if user.APIKeyPrefix != nil {
		c.Set("api_key_prefix", *user.APIKeyPrefix)
	}
	if user.APIKeyID != nil {
		c.Set("api_key_id", *user.APIKeyID)
	}
}

// AdminUser returns a CurrentUser with admin role.
func AdminUser() *CurrentUser {
	return &CurrentUser{
		UserID:   1,
		Username: "admin",
		Role:     "admin",
	}
}

// RegularUser returns a CurrentUser with user role.
func RegularUser() *CurrentUser {
	return &CurrentUser{
		UserID:   2,
		Username: "testuser",
		Role:     "user",
	}
}

// APIKeyUser returns a CurrentUser authenticated via API key.
func APIKeyUser() *CurrentUser {
	prefix := "sk-test"
	keyID := int64(1)
	return &CurrentUser{
		UserID:       2,
		Username:     "testuser",
		Role:         "user",
		APIKeyPrefix: &prefix,
		APIKeyID:     &keyID,
	}
}

// MakeJSONRequest creates an HTTP request with JSON body.
func MakeJSONRequest(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()

	var req *http.Request
	var err error

	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err)
		req, err = http.NewRequest(method, url, bytes.NewReader(jsonBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		require.NoError(t, err)
	}

	return req
}

// MakeAuthenticatedRequest creates an authenticated HTTP request.
func MakeAuthenticatedRequest(t *testing.T, method, url string, body any, token string) *http.Request {
	t.Helper()

	req := MakeJSONRequest(t, method, url, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// MakeAPIKeyRequest creates an HTTP request with API key authentication.
func MakeAPIKeyRequest(t *testing.T, method, url string, body any, apiKey string) *http.Request {
	t.Helper()

	req := MakeJSONRequest(t, method, url, body)
	req.Header.Set("X-API-Key", apiKey)
	return req
}

// MockUpstreamServer creates a mock upstream server for testing proxy functionality.
func MockUpstreamServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})

	return server
}

// MockUpstreamResponse returns a handler that responds with the given status and body.
func MockUpstreamResponse(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			json.NewEncoder(w).Encode(body)
		}
	}
}

// MockAnthropicResponse returns a mock Anthropic API response.
func MockAnthropicResponse() map[string]any {
	return map[string]any{
		"id":   "msg_test_12345",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{
				"type": "text",
				"text": "Hello! How can I help you today?",
			},
		},
		"model":         "claude-sonnet-4",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 15,
		},
	}
}

// MockStreamingResponse returns a mock streaming response handler.
func MockStreamingResponse() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4"}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			w.Write([]byte(event + "\n"))
		}
	}
}

// ContextWithTimeout returns a context with a timeout for testing.
func ContextWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5000)
	t.Cleanup(cancel)
	return ctx
}

// ToJSON converts a value to JSON bytes.
func ToJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// FromJSON unmarshals JSON bytes to a value.
func FromJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	err := json.Unmarshal(data, v)
	require.NoError(t, err)
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}

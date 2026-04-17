package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

type healthProbeRequest struct {
	method                 string
	url                    string
	body                   []byte
	treatValidationAsHealth bool
}

const healthProbeInvalidModel = "__llm_proxy_health_probe_invalid_model__"

func probeEndpointStatus(ctx context.Context, client *http.Client, ep *models.Endpoint) (models.EndpointStatus, string) {
	if ep == nil || ep.Provider == nil {
		return models.EndpointUnhealthy, "endpoint provider is nil"
	}

	// Prefer probing /v1/models to validate API surface and auth with low cost.
	// Fall back to API-type-specific validation probes when /v1/models is unsupported (404/405).
	probeURL := normalizeBaseURL(ep.Provider.BaseURL) + "/v1/models"
	statusCode, bodySnippet, err := executeHealthProbe(ctx, client, ep, healthProbeRequest{
		method: http.MethodGet,
		url:    probeURL,
	})
	if err != nil {
		return models.EndpointUnhealthy, err.Error()
	}

	if statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed {
		for _, fallback := range buildFallbackProbes(ep.Provider) {
			statusCode, bodySnippet, err = executeHealthProbe(ctx, client, ep, fallback)
			if err != nil {
				return models.EndpointUnhealthy, err.Error()
			}

			status, errMsg := classifyProbeStatus(statusCode, bodySnippet, fallback.treatValidationAsHealth)
			if status == models.EndpointHealthy {
				return status, errMsg
			}

			// Try the next fallback probe only when endpoint path/method is unsupported.
			// For auth/rate-limit/5xx and other errors, return immediately.
			if statusCode != http.StatusNotFound && statusCode != http.StatusMethodNotAllowed {
				return status, errMsg
			}
		}
	}

	return classifyProbeStatus(statusCode, bodySnippet, false)
}

func buildFallbackProbes(provider *models.Provider) []healthProbeRequest {
	baseURL := normalizeBaseURL(provider.BaseURL)
	messagesProbeBody := []byte(`{"model":"` + healthProbeInvalidModel + `","max_tokens":1,"messages":[{"role":"user","content":"health-check"}]}`)
	responsesProbeBody := []byte(`{"model":"` + healthProbeInvalidModel + `","max_tokens":1,"input":"health-check"}`)
	openAIChatProbeBody := []byte(`{"model":"` + healthProbeInvalidModel + `","max_tokens":1,"messages":[{"role":"user","content":"health-check"}]}`)

	switch strings.TrimSpace(provider.APIType) {
	case string(APITypeAnthropicMessages), "":
		return []healthProbeRequest{{
			method:                 http.MethodPost,
			url:                    baseURL + "/v1/messages",
			body:                   messagesProbeBody,
			treatValidationAsHealth: true,
		}}
	case string(APITypeAnthropicResponses):
		return []healthProbeRequest{{
			method:                 http.MethodPost,
			url:                    baseURL + "/v1/responses",
			body:                   responsesProbeBody,
			treatValidationAsHealth: true,
		}}
	case string(APITypeOpenAIChat):
		return []healthProbeRequest{{
			method:                 http.MethodPost,
			url:                    baseURL + "/v1/chat/completions",
			body:                   openAIChatProbeBody,
			treatValidationAsHealth: true,
		}}
	default:
		// Unknown/auto fallback: try all known endpoints in conservative order.
		return []healthProbeRequest{
			{
				method:                 http.MethodPost,
				url:                    baseURL + "/v1/messages",
				body:                   messagesProbeBody,
				treatValidationAsHealth: true,
			},
			{
				method:                 http.MethodPost,
				url:                    baseURL + "/v1/responses",
				body:                   responsesProbeBody,
				treatValidationAsHealth: true,
			},
			{
				method:                 http.MethodPost,
				url:                    baseURL + "/v1/chat/completions",
				body:                   openAIChatProbeBody,
				treatValidationAsHealth: true,
			},
		}
	}
}

func executeHealthProbe(ctx context.Context, client *http.Client, ep *models.Endpoint, probe healthProbeRequest) (int, string, error) {
	reqBody := bytes.NewReader(probe.body)
	req, err := http.NewRequestWithContext(ctx, probe.method, probe.url, reqBody)
	if err != nil {
		return 0, "", err
	}

	req.Header.Set("Accept", "application/json")
	if len(probe.body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if ep.Provider.APIKey != "" {
		// Send both common auth headers for compatibility with OpenAI-like and
		// Anthropic-like upstreams and third-party gateways.
		req.Header.Set("Authorization", "Bearer "+ep.Provider.APIKey)
		req.Header.Set("x-api-key", ep.Provider.APIKey)
	}
	req.Header.Set("anthropic-version", DefaultAnthropicVersion)
	applyCustomHeaders(ep.Provider.CustomHeaders, req.Header)

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return resp.StatusCode, strings.TrimSpace(string(body)), nil
}

func classifyProbeStatus(statusCode int, bodySnippet string, treatValidationAsHealth bool) (models.EndpointStatus, string) {
	formatErr := func(prefix string) string {
		if bodySnippet == "" {
			return prefix
		}
		return fmt.Sprintf("%s: %s", prefix, bodySnippet)
	}

	switch {
	case statusCode >= http.StatusOK && statusCode < http.StatusBadRequest:
		return models.EndpointHealthy, ""
	case treatValidationAsHealth && (statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity):
		return models.EndpointHealthy, ""
	case treatValidationAsHealth && statusCode == http.StatusNotFound && containsModelError([]byte(bodySnippet)):
		return models.EndpointHealthy, ""
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return models.EndpointUnhealthy, formatErr(fmt.Sprintf("health probe auth failed (status %d)", statusCode))
	case statusCode == http.StatusNotFound:
		return models.EndpointUnhealthy, formatErr("health probe endpoint not found (status 404)")
	case statusCode == http.StatusMethodNotAllowed:
		return models.EndpointUnhealthy, formatErr("health probe method not allowed (status 405)")
	case statusCode == http.StatusTooManyRequests:
		return models.EndpointUnhealthy, formatErr(fmt.Sprintf("health probe rate limited (status %d)", statusCode))
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return models.EndpointUnhealthy, formatErr(fmt.Sprintf("health probe client error (status %d)", statusCode))
	case statusCode >= http.StatusInternalServerError:
		return models.EndpointUnhealthy, formatErr(fmt.Sprintf("health probe upstream error (status %d)", statusCode))
	default:
		return models.EndpointUnhealthy, formatErr(fmt.Sprintf("health probe unknown status (status %d)", statusCode))
	}

}

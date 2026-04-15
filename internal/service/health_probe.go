package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

func probeEndpointStatus(ctx context.Context, client *http.Client, ep *models.Endpoint) (models.EndpointStatus, string) {
	if ep == nil || ep.Provider == nil {
		return models.EndpointUnhealthy, "endpoint provider is nil"
	}

	// Prefer probing /v1/models to validate API surface and auth with low cost.
	// Fall back to base_url only when /v1/models is unsupported (404/405).
	probeURL := normalizeBaseURL(ep.Provider.BaseURL) + "/v1/models"
	statusCode, bodySnippet, err := executeHealthProbe(ctx, client, ep, probeURL)
	if err != nil {
		return models.EndpointUnhealthy, err.Error()
	}

	if statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed {
		fallbackURL := strings.TrimRight(ep.Provider.BaseURL, "/")
		if fallbackURL == "" {
			fallbackURL = ep.Provider.BaseURL
		}
		statusCode, bodySnippet, err = executeHealthProbe(ctx, client, ep, fallbackURL)
		if err != nil {
			return models.EndpointUnhealthy, err.Error()
		}
	}

	return classifyProbeStatus(statusCode, bodySnippet)
}

func executeHealthProbe(ctx context.Context, client *http.Client, ep *models.Endpoint, targetURL string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, "", err
	}

	req.Header.Set("Accept", "application/json")
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

func classifyProbeStatus(statusCode int, bodySnippet string) (models.EndpointStatus, string) {
	formatErr := func(prefix string) string {
		if bodySnippet == "" {
			return prefix
		}
		return fmt.Sprintf("%s: %s", prefix, bodySnippet)
	}

	switch {
	case statusCode >= http.StatusOK && statusCode < http.StatusBadRequest:
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

package service

import (
	"context"
	"net/http"

	"github.com/user/llm-proxy-go/internal/models"
)

func probeEndpointStatus(ctx context.Context, client *http.Client, ep *models.Endpoint) (models.EndpointStatus, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.Provider.BaseURL, nil)
	if err != nil {
		return models.EndpointUnhealthy, err.Error()
	}
	req.Header.Set("x-api-key", ep.Provider.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return models.EndpointUnhealthy, err.Error()
	}
	defer resp.Body.Close()

	// 401 = invalid key, 403 = quota/permission, <400 = healthy, >=400 = unhealthy
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return models.EndpointUnhealthy, ""
	case resp.StatusCode == http.StatusForbidden:
		return models.EndpointUnhealthy, ""
	case resp.StatusCode < 400:
		return models.EndpointHealthy, ""
	default:
		return models.EndpointUnhealthy, ""
	}
}

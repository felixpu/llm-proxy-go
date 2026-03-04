package service

import (
	"context"

	"github.com/user/llm-proxy-go/internal/models"
)

// RoutingConfigProvider reads routing configuration.
type RoutingConfigProvider interface {
	GetConfig(ctx context.Context) (*models.RoutingConfig, error)
}

type routingConfigContextKey struct{}

type routingConfigCacheEntry struct {
	cfg *models.RoutingConfig
	err error
}

// GetOrLoadRoutingConfig returns routing config from request context cache if available.
// On cache miss, it loads once via provider and stores result (including error) in context.
func GetOrLoadRoutingConfig(ctx context.Context, provider RoutingConfigProvider) (context.Context, *models.RoutingConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if cached, ok := ctx.Value(routingConfigContextKey{}).(routingConfigCacheEntry); ok {
		return ctx, cached.cfg, cached.err
	}

	if provider == nil {
		return ctx, nil, nil
	}

	cfg, err := provider.GetConfig(ctx)
	ctx = context.WithValue(ctx, routingConfigContextKey{}, routingConfigCacheEntry{
		cfg: cfg,
		err: err,
	})
	return ctx, cfg, err
}

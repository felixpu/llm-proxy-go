package handler

import (
	"context"

	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

// ContentLoggingPolicy controls whether full request/response content should be logged.
type ContentLoggingPolicy struct {
	configProvider service.RoutingConfigProvider
	logger         *zap.Logger
}

// NewContentLoggingPolicy creates a content logging policy.
func NewContentLoggingPolicy(configProvider service.RoutingConfigProvider, logger *zap.Logger) *ContentLoggingPolicy {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ContentLoggingPolicy{
		configProvider: configProvider,
		logger:         logger,
	}
}

// ShouldLogFullContent returns whether request/response full content logging is enabled.
func (p *ContentLoggingPolicy) ShouldLogFullContent(ctx context.Context) bool {
	if p == nil || p.configProvider == nil {
		return false
	}

	_, cfg, err := service.GetOrLoadRoutingConfig(ctx, p.configProvider)
	if err != nil {
		p.logger.Warn("failed to get routing config for content logging", zap.Error(err))
		return false
	}
	return cfg != nil && cfg.LogFullContent
}

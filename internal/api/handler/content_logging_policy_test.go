//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

type stubPolicyConfigProvider struct {
	cfg   *models.RoutingConfig
	err   error
	calls int
}

func (s *stubPolicyConfigProvider) GetConfig(ctx context.Context) (*models.RoutingConfig, error) {
	s.calls++
	return s.cfg, s.err
}

func TestContentLoggingPolicy_ShouldLogFullContent(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		provider := &stubPolicyConfigProvider{
			cfg: &models.RoutingConfig{LogFullContent: true},
		}
		policy := NewContentLoggingPolicy(provider, zap.NewNop())
		assert.True(t, policy.ShouldLogFullContent(context.Background()))
		assert.Equal(t, 1, provider.calls)
	})

	t.Run("disabled", func(t *testing.T) {
		provider := &stubPolicyConfigProvider{
			cfg: &models.RoutingConfig{LogFullContent: false},
		}
		policy := NewContentLoggingPolicy(provider, zap.NewNop())
		assert.False(t, policy.ShouldLogFullContent(context.Background()))
		assert.Equal(t, 1, provider.calls)
	})

	t.Run("error", func(t *testing.T) {
		provider := &stubPolicyConfigProvider{err: errors.New("db error")}
		policy := NewContentLoggingPolicy(provider, zap.NewNop())
		assert.False(t, policy.ShouldLogFullContent(context.Background()))
		assert.Equal(t, 1, provider.calls)
	})

	t.Run("nil provider", func(t *testing.T) {
		policy := NewContentLoggingPolicy(nil, zap.NewNop())
		assert.False(t, policy.ShouldLogFullContent(context.Background()))
	})
}

//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
)

type stubRoutingConfigProvider struct {
	cfg   *models.RoutingConfig
	err   error
	calls int
}

func (s *stubRoutingConfigProvider) GetConfig(ctx context.Context) (*models.RoutingConfig, error) {
	s.calls++
	return s.cfg, s.err
}

func TestGetOrLoadRoutingConfig_CachesSuccess(t *testing.T) {
	provider := &stubRoutingConfigProvider{cfg: models.DefaultRoutingConfig()}
	ctx := context.Background()

	ctx, cfg1, err1 := GetOrLoadRoutingConfig(ctx, provider)
	require.NoError(t, err1)
	require.NotNil(t, cfg1)

	_, cfg2, err2 := GetOrLoadRoutingConfig(ctx, provider)
	require.NoError(t, err2)
	require.NotNil(t, cfg2)

	assert.Equal(t, 1, provider.calls, "provider should be called once per request context")
	assert.Equal(t, cfg1, cfg2)
}

func TestGetOrLoadRoutingConfig_CachesError(t *testing.T) {
	provider := &stubRoutingConfigProvider{err: errors.New("db unavailable")}
	ctx := context.Background()

	ctx, cfg1, err1 := GetOrLoadRoutingConfig(ctx, provider)
	require.Error(t, err1)
	assert.Nil(t, cfg1)

	_, cfg2, err2 := GetOrLoadRoutingConfig(ctx, provider)
	require.Error(t, err2)
	assert.Nil(t, cfg2)

	assert.Equal(t, 1, provider.calls, "provider error result should also be cached")
	assert.Equal(t, err1.Error(), err2.Error())
}

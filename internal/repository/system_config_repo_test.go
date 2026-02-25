//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestSystemConfigRepository_GetRoutingConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	config, err := repo.GetRoutingConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify default value
	assert.Equal(t, "default", config["default_role"])
}

func TestSystemConfigRepository_UpdateRoutingConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	err := repo.UpdateRoutingConfig(ctx, map[string]any{
		"default_role": "simple",
	})
	require.NoError(t, err)

	config, err := repo.GetRoutingConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "simple", config["default_role"])
}

func TestSystemConfigRepository_GetLoadBalanceConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	config, err := repo.GetLoadBalanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, "conversation_hash", config["strategy"])
}

func TestSystemConfigRepository_UpdateLoadBalanceConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	err := repo.UpdateLoadBalanceConfig(ctx, map[string]any{
		"strategy": "round_robin",
	})
	require.NoError(t, err)

	config, err := repo.GetLoadBalanceConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "round_robin", config["strategy"])
}

func TestSystemConfigRepository_GetHealthCheckConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	config, err := repo.GetHealthCheckConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, int64(1), config["enabled"])
	assert.Equal(t, int64(60), config["interval_seconds"])
	assert.Equal(t, int64(10), config["timeout_seconds"])
}

func TestSystemConfigRepository_UpdateHealthCheckConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	err := repo.UpdateHealthCheckConfig(ctx, map[string]any{
		"enabled":          0,
		"interval_seconds": 120,
		"timeout_seconds":  30,
	})
	require.NoError(t, err)

	config, err := repo.GetHealthCheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), config["enabled"])
	assert.Equal(t, int64(120), config["interval_seconds"])
	assert.Equal(t, int64(30), config["timeout_seconds"])
}

func TestSystemConfigRepository_GetUIConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	config, err := repo.GetUIConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, int64(30), config["dashboard_refresh_seconds"])
	assert.Equal(t, int64(15), config["logs_refresh_seconds"])
}

func TestSystemConfigRepository_UpdateUIConfig(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	err := repo.UpdateUIConfig(ctx, map[string]any{
		"dashboard_refresh_seconds": 60,
		"logs_refresh_seconds":      15,
	})
	require.NoError(t, err)

	config, err := repo.GetUIConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(60), config["dashboard_refresh_seconds"])
	assert.Equal(t, int64(15), config["logs_refresh_seconds"])
}

func TestSystemConfigRepository_EmptyUpdates(t *testing.T) {
	db := testutil.NewTestDBWithDefaults(t)
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	// Empty updates should not error
	err := repo.UpdateRoutingConfig(ctx, map[string]any{})
	assert.NoError(t, err)

	err = repo.UpdateLoadBalanceConfig(ctx, map[string]any{})
	assert.NoError(t, err)

	err = repo.UpdateHealthCheckConfig(ctx, map[string]any{})
	assert.NoError(t, err)

	err = repo.UpdateUIConfig(ctx, map[string]any{})
	assert.NoError(t, err)
}

func TestSystemConfigRepository_NoRow(t *testing.T) {
	db := testutil.NewTestDB(t) // No defaults
	repo := NewSystemConfigRepository(db)
	ctx := context.Background()

	// Should return empty map when no row exists
	config, err := repo.GetRoutingConfig(ctx)
	require.NoError(t, err)
	assert.Empty(t, config)
}

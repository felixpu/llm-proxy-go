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

	defaultRole := "simple"
	err := repo.UpdateRoutingConfigPatch(ctx, SystemRoutingConfigPatch{
		DefaultRole: &defaultRole,
	})
	require.NoError(t, err)

	config, err := repo.GetRoutingConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "simple", config["default_role"])
}

func TestSystemConfigRepository_ReadsUseReadDB(t *testing.T) {
	db, readDB := testutil.NewFileBackedTestDBPairWithDefaults(t)
	repo := NewSystemConfigRepository(db, readDB)
	ctx := context.Background()

	require.NoError(t, db.Close())

	config, err := repo.GetLoadBalanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "conversation_hash", config["strategy"])
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

	strategy := "round_robin"
	err := repo.UpdateLoadBalanceConfigPatch(ctx, SystemLoadBalanceConfigPatch{
		Strategy: &strategy,
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

	enabled := false
	interval := 120
	timeout := 30
	err := repo.UpdateHealthCheckConfigPatch(ctx, SystemHealthCheckConfigPatch{
		Enabled:         &enabled,
		IntervalSeconds: &interval,
		TimeoutSeconds:  &timeout,
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

	dashboardRefresh := 60
	logsRefresh := 15
	err := repo.UpdateUIConfigPatch(ctx, SystemUIConfigPatch{
		DashboardRefreshSeconds: &dashboardRefresh,
		LogsRefreshSeconds:      &logsRefresh,
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
	err := repo.UpdateRoutingConfigPatch(ctx, SystemRoutingConfigPatch{})
	assert.NoError(t, err)

	err = repo.UpdateLoadBalanceConfigPatch(ctx, SystemLoadBalanceConfigPatch{})
	assert.NoError(t, err)

	err = repo.UpdateHealthCheckConfigPatch(ctx, SystemHealthCheckConfigPatch{})
	assert.NoError(t, err)

	err = repo.UpdateUIConfigPatch(ctx, SystemUIConfigPatch{})
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

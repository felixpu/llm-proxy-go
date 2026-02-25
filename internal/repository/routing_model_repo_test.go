//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestRoutingModelRepository_ListModels(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRoutingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	// Insert routing models
	seedRoutingModels(t, db)

	tests := []struct {
		name       string
		providerID *int64
		wantCount  int
	}{
		{"all models", nil, 3},
		{"provider 1 models", func() *int64 { id := int64(1); return &id }(), 2},
		{"provider 2 models", func() *int64 { id := int64(2); return &id }(), 1},
		{"non-existing provider", func() *int64 { id := int64(999); return &id }(), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modelList, err := repo.ListModels(ctx, tt.providerID)
			require.NoError(t, err)
			assert.Len(t, modelList, tt.wantCount)
		})
	}
}

func TestRoutingModelRepository_GetModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRoutingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingModels(t, db)

	tests := []struct {
		name    string
		id      int64
		wantNil bool
	}{
		{"existing model", 1, false},
		{"non-existing model", 999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := repo.GetModel(ctx, tt.id)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, model)
			} else {
				assert.NotNil(t, model)
				assert.Equal(t, tt.id, model.ID)
			}
		})
	}
}

func TestRoutingModelRepository_AddModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRoutingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		model   *models.RoutingModel
		wantErr bool
	}{
		{
			name: "valid model",
			model: &models.RoutingModel{
				ProviderID:        1,
				ModelName:         "claude-sonnet-4",
				Enabled:           true,
				Priority:          10,
				CostPerMtokInput:  3.0,
				CostPerMtokOutput: 15.0,
				BillingMultiplier: 1.0,
				Description:       "Test routing model",
			},
			wantErr: false,
		},
		{
			name: "disabled model",
			model: &models.RoutingModel{
				ProviderID:        2,
				ModelName:         "claude-3-haiku",
				Enabled:           false,
				Priority:          5,
				CostPerMtokInput:  0.25,
				CostPerMtokOutput: 1.25,
				BillingMultiplier: 1.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.AddModel(ctx, tt.model)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				// Verify insertion
				found, err := repo.GetModel(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, tt.model.ModelName, found.ModelName)
				assert.Equal(t, tt.model.Enabled, found.Enabled)
				assert.Equal(t, tt.model.Priority, found.Priority)
			}
		})
	}
}

func TestRoutingModelRepository_UpdateModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRoutingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingModels(t, db)

	tests := []struct {
		name    string
		id      int64
		updates map[string]any
		verify  func(t *testing.T, m *models.RoutingModel)
	}{
		{
			name: "update model name",
			id:   1,
			updates: map[string]any{
				"model_name": "claude-sonnet-4-updated",
			},
			verify: func(t *testing.T, m *models.RoutingModel) {
				assert.Equal(t, "claude-sonnet-4-updated", m.ModelName)
			},
		},
		{
			name: "update enabled to false",
			id:   1,
			updates: map[string]any{
				"enabled": false,
			},
			verify: func(t *testing.T, m *models.RoutingModel) {
				assert.False(t, m.Enabled)
			},
		},
		{
			name: "update priority",
			id:   2,
			updates: map[string]any{
				"priority": 20,
			},
			verify: func(t *testing.T, m *models.RoutingModel) {
				assert.Equal(t, 20, m.Priority)
			},
		},
		{
			name: "update cost",
			id:   1,
			updates: map[string]any{
				"cost_per_mtok_input":  5.0,
				"cost_per_mtok_output": 25.0,
			},
			verify: func(t *testing.T, m *models.RoutingModel) {
				assert.Equal(t, 5.0, m.CostPerMtokInput)
				assert.Equal(t, 25.0, m.CostPerMtokOutput)
			},
		},
		{
			name:    "empty updates",
			id:      1,
			updates: map[string]any{},
			verify:  func(t *testing.T, m *models.RoutingModel) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpdateModel(ctx, tt.id, tt.updates)
			require.NoError(t, err)

			if len(tt.updates) > 0 {
				model, err := repo.GetModel(ctx, tt.id)
				require.NoError(t, err)
				tt.verify(t, model)
			}
		})
	}
}

func TestRoutingModelRepository_DeleteModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewRoutingModelRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingModels(t, db)

	// Delete model
	err := repo.DeleteModel(ctx, 3)
	require.NoError(t, err)

	// Verify deletion
	model, err := repo.GetModel(ctx, 3)
	require.NoError(t, err)
	assert.Nil(t, model)
}

// seedRoutingModels inserts test routing models.
func seedRoutingModels(t *testing.T, db *sql.DB) {
	t.Helper()

	queries := []string{
		`INSERT INTO routing_models (provider_id, model_name, enabled, priority, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, description, created_at, updated_at)
		 VALUES (1, 'claude-sonnet-4', 1, 10, 3.0, 15.0, 1.0, 'Primary routing model', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO routing_models (provider_id, model_name, enabled, priority, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, description, created_at, updated_at)
		 VALUES (1, 'claude-3-haiku', 1, 5, 0.25, 1.25, 1.0, 'Simple routing model', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO routing_models (provider_id, model_name, enabled, priority, cost_per_mtok_input, cost_per_mtok_output, billing_multiplier, description, created_at, updated_at)
		 VALUES (2, 'claude-opus-4', 1, 15, 15.0, 75.0, 1.0, 'Complex routing model', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
}

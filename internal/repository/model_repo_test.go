//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestModelRepository_FindByID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		wantNil bool
		wantErr bool
	}{
		{"existing model", 1, false, false},
		{"non-existing model", 999, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := repo.FindByID(ctx, tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, model)
				assert.Equal(t, tt.id, model.ID)
			}
		})
	}
}

func TestModelRepository_FindByName(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		modelName string
		wantRole  models.ModelRole
		wantErr   bool
	}{
		{"haiku model", "claude-3-haiku", models.ModelRoleSimple, false},
		{"sonnet model", "claude-sonnet-4", models.ModelRoleDefault, false},
		{"opus model", "claude-opus-4", models.ModelRoleComplex, false},
		{"non-existing", "nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := repo.FindByName(ctx, tt.modelName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, model)
				assert.Equal(t, tt.modelName, model.Name)
				assert.Equal(t, tt.wantRole, model.Role)
			}
		})
	}
}

func TestModelRepository_FindByRole(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		role      models.ModelRole
		wantCount int
	}{
		{"simple models", models.ModelRoleSimple, 1},
		{"default models", models.ModelRoleDefault, 1}, // disabled-model is not enabled
		{"complex models", models.ModelRoleComplex, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modelList, err := repo.FindByRole(ctx, tt.role)
			require.NoError(t, err)
			assert.Len(t, modelList, tt.wantCount)
			for _, m := range modelList {
				assert.Equal(t, tt.role, m.Role)
				assert.True(t, m.Enabled)
			}
		})
	}
}

func TestModelRepository_FindAllEnabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	modelList, err := repo.FindAllEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, modelList, 3) // 3 enabled models

	for _, m := range modelList {
		assert.True(t, m.Enabled)
	}
}

func TestModelRepository_FindAll(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	modelList, err := repo.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, modelList, 4) // All 4 models including disabled
}

func TestModelRepository_Insert(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewModelRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		model   *models.Model
		wantErr bool
	}{
		{
			name: "valid model",
			model: &models.Model{
				Name:              "gpt-4",
				Role:              models.ModelRoleComplex,
				CostPerMtokInput:  30.0,
				CostPerMtokOutput: 60.0,
				BillingMultiplier: 1.0,
				SupportsThinking:  true,
				Enabled:           true,
				Weight:            100,
			},
			wantErr: false,
		},
		{
			name: "simple model",
			model: &models.Model{
				Name:              "gpt-3.5-turbo",
				Role:              models.ModelRoleSimple,
				CostPerMtokInput:  0.5,
				CostPerMtokOutput: 1.5,
				BillingMultiplier: 1.0,
				SupportsThinking:  false,
				Enabled:           true,
				Weight:            50,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.Insert(ctx, tt.model)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				// Verify insertion
				found, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, tt.model.Name, found.Name)
				assert.Equal(t, tt.model.Role, found.Role)
				assert.Equal(t, tt.model.SupportsThinking, found.SupportsThinking)
			}
		})
	}
}

func TestModelRepository_Insert_DuplicateName(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	model := &models.Model{
		Name:    "claude-3-haiku", // Already exists
		Role:    models.ModelRoleSimple,
		Enabled: true,
	}

	_, err := repo.Insert(ctx, model)
	assert.Error(t, err) // Should fail due to UNIQUE constraint
}

func TestModelRepository_Update(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		updates map[string]any
		verify  func(t *testing.T, m *models.Model)
	}{
		{
			name: "update name",
			id:   1,
			updates: map[string]any{
				"name": "claude-3-haiku-updated",
			},
			verify: func(t *testing.T, m *models.Model) {
				assert.Equal(t, "claude-3-haiku-updated", m.Name)
			},
		},
		{
			name: "update role",
			id:   1,
			updates: map[string]any{
				"role": "default",
			},
			verify: func(t *testing.T, m *models.Model) {
				assert.Equal(t, models.ModelRoleDefault, m.Role)
			},
		},
		{
			name: "update enabled to false",
			id:   2,
			updates: map[string]any{
				"enabled": false,
			},
			verify: func(t *testing.T, m *models.Model) {
				assert.False(t, m.Enabled)
			},
		},
		{
			name: "update supports_thinking",
			id:   2,
			updates: map[string]any{
				"supports_thinking": true,
			},
			verify: func(t *testing.T, m *models.Model) {
				assert.True(t, m.SupportsThinking)
			},
		},
		{
			name: "update cost",
			id:   1,
			updates: map[string]any{
				"cost_per_mtok_input":  0.5,
				"cost_per_mtok_output": 2.5,
			},
			verify: func(t *testing.T, m *models.Model) {
				assert.Equal(t, 0.5, m.CostPerMtokInput)
				assert.Equal(t, 2.5, m.CostPerMtokOutput)
			},
		},
		{
			name:    "empty updates",
			id:      1,
			updates: map[string]any{},
			verify:  func(t *testing.T, m *models.Model) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Update(ctx, tt.id, tt.updates)
			require.NoError(t, err)

			if len(tt.updates) > 0 {
				model, err := repo.FindByID(ctx, tt.id)
				require.NoError(t, err)
				tt.verify(t, model)
			}
		})
	}
}

func TestModelRepository_Delete(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewModelRepository(db)
	ctx := context.Background()

	// Delete model
	err := repo.Delete(ctx, 4) // disabled-model
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.FindByID(ctx, 4)
	assert.Error(t, err)
}

func TestModelRepository_Delete_NonExisting(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewModelRepository(db)
	ctx := context.Background()

	// Delete non-existing model should not error
	err := repo.Delete(ctx, 999)
	assert.NoError(t, err)
}

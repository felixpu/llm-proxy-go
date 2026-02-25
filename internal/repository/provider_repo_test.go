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

func TestProviderRepository_FindByID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      int64
		wantNil bool
		wantErr bool
	}{
		{"existing provider", 1, false, false},
		{"non-existing provider", 999, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := repo.FindByID(ctx, tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.id, provider.ID)
			}
		})
	}
}

func TestProviderRepository_FindByModelID(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	tests := []struct {
		name      string
		modelID   int64
		wantCount int
	}{
		{"model with 2 providers", 1, 2}, // claude-3-haiku linked to 2 providers
		{"model with 2 providers", 2, 2}, // claude-sonnet-4 linked to 2 providers
		{"model with 1 provider", 3, 1},  // claude-opus-4 linked to 1 provider
		{"model with no providers", 4, 0}, // disabled-model not linked
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers, err := repo.FindByModelID(ctx, tt.modelID)
			require.NoError(t, err)
			assert.Len(t, providers, tt.wantCount)
			for _, p := range providers {
				assert.True(t, p.Enabled)
			}
		})
	}
}

func TestProviderRepository_FindAllEnabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	providers, err := repo.FindAllEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, providers, 2) // 2 enabled providers

	for _, p := range providers {
		assert.True(t, p.Enabled)
	}
}

func TestProviderRepository_FindAll(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	providers, err := repo.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, providers, 3) // All 3 providers including disabled
}

func TestProviderRepository_Insert(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	tests := []struct {
		name     string
		provider *models.Provider
		modelIDs []int64
		wantErr  bool
	}{
		{
			name: "provider with models",
			provider: &models.Provider{
				Name:          "openai",
				BaseURL:       "https://api.openai.com",
				APIKey:        "sk-openai-key",
				Weight:        1,
				MaxConcurrent: 10,
				Enabled:       true,
				Description:   "OpenAI Provider",
			},
			modelIDs: []int64{1, 2},
			wantErr:  false,
		},
		{
			name: "provider without models",
			provider: &models.Provider{
				Name:          "azure",
				BaseURL:       "https://azure.openai.com",
				APIKey:        "sk-azure-key",
				Weight:        1,
				MaxConcurrent: 5,
				Enabled:       true,
			},
			modelIDs: nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.Insert(ctx, tt.provider, tt.modelIDs)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				// Verify insertion
				found, err := repo.FindByID(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, tt.provider.Name, found.Name)
				assert.Equal(t, tt.provider.BaseURL, found.BaseURL)

				// Verify model associations
				if len(tt.modelIDs) > 0 {
					modelIDs, err := repo.GetModelIDsForProvider(ctx, id)
					require.NoError(t, err)
					assert.ElementsMatch(t, tt.modelIDs, modelIDs)
				}
			}
		})
	}
}

func TestProviderRepository_Update(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	tests := []struct {
		name     string
		id       int64
		updates  map[string]any
		modelIDs []int64
		verify   func(t *testing.T, p *models.Provider)
	}{
		{
			name: "update name",
			id:   1,
			updates: map[string]any{
				"name": "anthropic-updated",
			},
			modelIDs: nil,
			verify: func(t *testing.T, p *models.Provider) {
				assert.Equal(t, "anthropic-updated", p.Name)
			},
		},
		{
			name: "update enabled to false",
			id:   2,
			updates: map[string]any{
				"enabled": false,
			},
			modelIDs: nil,
			verify: func(t *testing.T, p *models.Provider) {
				assert.False(t, p.Enabled)
			},
		},
		{
			name:     "update model associations",
			id:       1,
			updates:  map[string]any{},
			modelIDs: []int64{1}, // Change from {1,2,3} to just {1}
			verify: func(t *testing.T, p *models.Provider) {
				// Verify in separate check
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Update(ctx, tt.id, tt.updates, tt.modelIDs)
			require.NoError(t, err)

			provider, err := repo.FindByID(ctx, tt.id)
			require.NoError(t, err)
			tt.verify(t, provider)

			if tt.modelIDs != nil {
				modelIDs, err := repo.GetModelIDsForProvider(ctx, tt.id)
				require.NoError(t, err)
				assert.ElementsMatch(t, tt.modelIDs, modelIDs)
			}
		})
	}
}

func TestProviderRepository_Delete(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	// Delete provider
	err := repo.Delete(ctx, 3) // disabled-provider
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.FindByID(ctx, 3)
	assert.Error(t, err)
}

func TestProviderRepository_GetModelIDsForProvider(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)
	repo := NewProviderRepository(db)
	ctx := context.Background()

	tests := []struct {
		name       string
		providerID int64
		wantIDs    []int64
	}{
		{"provider with 3 models", 1, []int64{1, 2, 3}},
		{"provider with 2 models", 2, []int64{1, 2}},
		{"provider with no models", 3, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := repo.GetModelIDsForProvider(ctx, tt.providerID)
			require.NoError(t, err)
			if tt.wantIDs == nil {
				assert.Empty(t, ids)
			} else {
				assert.ElementsMatch(t, tt.wantIDs, ids)
			}
		})
	}
}

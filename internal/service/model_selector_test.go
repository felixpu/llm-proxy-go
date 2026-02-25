//go:build !integration && !e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func TestSelectModelByWeight(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	ms := NewModelSelector(hc, logger)

	tests := []struct {
		name      string
		modelList []*models.Model
		wantModel *models.Model
		wantNil   bool
	}{
		{
			name:      "empty list returns nil",
			modelList: []*models.Model{},
			wantNil:   true,
		},
		{
			name: "single model returns that model",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: 5},
			},
			wantModel: &models.Model{ID: 1, Name: "model-1", Weight: 5},
		},
		{
			name: "multiple models with different weights returns highest weight",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: 3},
				{ID: 2, Name: "model-2", Weight: 10},
				{ID: 3, Name: "model-3", Weight: 5},
			},
			wantModel: &models.Model{ID: 2, Name: "model-2", Weight: 10},
		},
		{
			name: "multiple models with same weight returns first",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: 5},
				{ID: 2, Name: "model-2", Weight: 5},
				{ID: 3, Name: "model-3", Weight: 5},
			},
			wantModel: &models.Model{ID: 1, Name: "model-1", Weight: 5},
		},
		{
			name: "all models with zero weight returns first",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: 0},
				{ID: 2, Name: "model-2", Weight: 0},
				{ID: 3, Name: "model-3", Weight: 0},
			},
			wantModel: &models.Model{ID: 1, Name: "model-1", Weight: 0},
		},
		{
			name: "mixed zero and positive weights returns highest positive",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: 0},
				{ID: 2, Name: "model-2", Weight: 8},
				{ID: 3, Name: "model-3", Weight: 0},
			},
			wantModel: &models.Model{ID: 2, Name: "model-2", Weight: 8},
		},
		{
			name: "negative weights are ignored",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: -5},
				{ID: 2, Name: "model-2", Weight: 3},
				{ID: 3, Name: "model-3", Weight: -10},
			},
			wantModel: &models.Model{ID: 2, Name: "model-2", Weight: 3},
		},
		{
			name: "all negative weights returns first",
			modelList: []*models.Model{
				{ID: 1, Name: "model-1", Weight: -5},
				{ID: 2, Name: "model-2", Weight: -3},
			},
			wantModel: &models.Model{ID: 1, Name: "model-1", Weight: -5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ms.SelectModelByWeight(tt.modelList)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantModel.ID, result.ID)
				assert.Equal(t, tt.wantModel.Name, result.Name)
				assert.Equal(t, tt.wantModel.Weight, result.Weight)
			}
		})
	}
}

// TestSelectModelByWeightDeterministic verifies that the function is deterministic
// by calling it multiple times with the same input and expecting the same result.
func TestSelectModelByWeightDeterministic(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	ms := NewModelSelector(hc, logger)

	modelList := []*models.Model{
		{ID: 1, Name: "model-1", Weight: 3},
		{ID: 2, Name: "model-2", Weight: 10},
		{ID: 3, Name: "model-3", Weight: 5},
	}

	// Call the function 100 times and verify it always returns the same model
	var firstResult *models.Model
	for range 100 {
		result := ms.SelectModelByWeight(modelList)
		if firstResult == nil {
			firstResult = result
		} else {
			assert.Equal(t, firstResult.ID, result.ID, "SelectModelByWeight should be deterministic")
		}
	}

	// Verify it's the highest weight model
	assert.Equal(t, int64(2), firstResult.ID)
	assert.Equal(t, "model-2", firstResult.Name)
	assert.Equal(t, 10, firstResult.Weight)
}

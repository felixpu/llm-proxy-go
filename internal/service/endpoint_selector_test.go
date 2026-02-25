//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func TestFindModelByName(t *testing.T) {
	logger := zap.NewNop()
	hc := NewHealthChecker(config.HealthCheckConfig{}, logger)
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)
	ms := NewModelSelector(hc, logger)
	es := NewEndpointSelector(ms, hc, lb, nil, nil, logger)

	endpoints := []*models.Endpoint{
		{
			Model:    &models.Model{ID: 1, Name: "claude-sonnet-4-20250514", Role: models.ModelRoleDefault, Enabled: true},
			Provider: &models.Provider{ID: 1, Name: "provider-1"},
		},
		{
			Model:    &models.Model{ID: 2, Name: "claude-opus-4-20250514", Role: models.ModelRoleComplex, Enabled: true},
			Provider: &models.Provider{ID: 2, Name: "provider-2"},
		},
		{
			Model:    &models.Model{ID: 3, Name: "claude-haiku-4-5-20251001", Role: models.ModelRoleSimple, Enabled: true},
			Provider: &models.Provider{ID: 3, Name: "provider-3"},
		},
	}

	tests := []struct {
		name        string
		requestName string
		wantModel   string
		wantNil     bool
	}{
		{"exact match", "claude-sonnet-4-20250514", "claude-sonnet-4-20250514", false},
		{"exact match case insensitive", "Claude-Sonnet-4-20250514", "claude-sonnet-4-20250514", false},
		{"exact match haiku", "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001", false},
		{"exact match opus", "claude-opus-4-20250514", "claude-opus-4-20250514", false},
		{"not configured model returns nil", "claude-sonnet-4-20250101", "", true},
		{"completely unknown model returns nil", "gpt-4o", "", true},
		{"empty name returns nil", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := es.findModelByName(tt.requestName, endpoints)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantModel, result.Name)
			}
		})
	}
}

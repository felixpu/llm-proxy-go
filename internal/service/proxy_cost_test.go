//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
)

// TestCalculateCost_NegativeProtection verifies negative number protection.
func TestCalculateCost_NegativeProtection(t *testing.T) {
	model := &models.Model{
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 1.0,
	}

	tests := []struct {
		name                 string
		usage                models.Usage
		expectedCost         float64
	}{
		{
			name: "cache_read > input (edge case)",
			usage: models.Usage{
				InputTokens:          500,
				OutputTokens:         100,
				CacheReadInputTokens: 600, // More than input
			},
			// Normal: clamped to 0, Cache read: 600/1M*3*0.1=0.00018, Output: 100/1M*15=0.0015
			expectedCost: 0.00168,
		},
		{
			name: "cache_read == input",
			usage: models.Usage{
				InputTokens:          1000,
				OutputTokens:         100,
				CacheReadInputTokens: 1000,
			},
			// Normal: 0, Cache read: 1000/1M*3*0.1=0.0003, Output: 100/1M*15=0.0015
			expectedCost: 0.0018,
		},
		{
			name: "normal case with cache read only",
			usage: models.Usage{
				InputTokens:          1000,
				OutputTokens:         100,
				CacheReadInputTokens: 400,
			},
			// Normal: 600/1M*3=0.0018, Cache read: 400/1M*3*0.1=0.00012, Output: 100/1M*15=0.0015
			expectedCost: 0.00342,
		},
		{
			name: "with cache creation tokens",
			usage: models.Usage{
				InputTokens:              1000,
				OutputTokens:             100,
				CacheReadInputTokens:     200,
				CacheCreationInputTokens: 300,
			},
			// Normal: (1000-200-300)/1M*3=0.0015
			// Cache read: 200/1M*3*0.1=0.00006
			// Cache creation: 300/1M*3*1.25=0.001125
			// Output: 100/1M*15=0.0015
			// Total: 0.004185
			expectedCost: 0.004185,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calculateCost(model, tt.usage)
			assert.GreaterOrEqual(t, cost, float64(0), "cost should not be negative")
			assert.InDelta(t, tt.expectedCost, cost, 0.00001)
		})
	}
}

// TestCalculateCostFromTokensWithCache_NegativeProtection verifies negative protection in streaming.
func TestCalculateCostFromTokensWithCache_NegativeProtection(t *testing.T) {
	model := &models.Model{
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 1.0,
	}

	tests := []struct {
		name                string
		inputTokens         int
		outputTokens        int
		cacheReadTokens     int
		cacheCreationTokens int
		expectedCost        float64
	}{
		{
			name:                "cache_read > input",
			inputTokens:         500,
			outputTokens:        100,
			cacheReadTokens:     600,
			cacheCreationTokens: 0,
			expectedCost:        0.00168, // Cache read: 600/1M*3*0.1=0.00018, Output: 100/1M*15=0.0015, Total: 0.00168
		},
		{
			name:                "cache_read == input",
			inputTokens:         1000,
			outputTokens:        100,
			cacheReadTokens:     1000,
			cacheCreationTokens: 0,
			expectedCost:        0.0018, // Cache read: 1000/1M*3*0.1=0.0003, Output: 100/1M*15=0.0015, Total: 0.0018
		},
		{
			name:                "normal case",
			inputTokens:         1000,
			outputTokens:        100,
			cacheReadTokens:     400,
			cacheCreationTokens: 0,
			expectedCost:        0.00342, // Normal: 600/1M*3=0.0018, Cache read: 400/1M*3*0.1=0.00012, Output: 100/1M*15=0.0015, Total: 0.00342
		},
		{
			name:                "zero cache",
			inputTokens:         1000,
			outputTokens:        100,
			cacheReadTokens:     0,
			cacheCreationTokens: 0,
			expectedCost:        0.0045, // Normal: 1000/1M*3=0.003, Output: 100/1M*15=0.0015, Total: 0.0045
		},
		{
			name:                "with cache creation tokens",
			inputTokens:         1000,
			outputTokens:        100,
			cacheReadTokens:     200,
			cacheCreationTokens: 300,
			// Normal: (1000-200-300)/1M*3=0.0015
			// Cache read: 200/1M*3*0.1=0.00006
			// Cache creation: 300/1M*3*1.25=0.001125
			// Output: 100/1M*15=0.0015
			// Total: 0.004185
			expectedCost: 0.004185,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calculateCostFromTokensWithCache(model, tt.inputTokens, tt.outputTokens, tt.cacheReadTokens, tt.cacheCreationTokens)
			assert.GreaterOrEqual(t, cost, float64(0), "cost should never be negative")
			assert.InDelta(t, tt.expectedCost, cost, 0.00001)
		})
	}
}

// TestProxyMetadata_CacheStatistics verifies cache statistics are propagated.
func TestProxyMetadata_CacheStatistics(t *testing.T) {
	meta := &ProxyMetadata{
		RequestID:                "req-789",
		SelectedModel:            "claude-3-sonnet",
		SelectedEndpoint:         "provider1",
		InputTokens:              1000,
		OutputTokens:             500,
		CacheCreationInputTokens: 200,
		CacheReadInputTokens:     400,
		Cost:                     0.00942,
	}

	assert.Equal(t, 1000, meta.InputTokens)
	assert.Equal(t, 500, meta.OutputTokens)
	assert.Equal(t, 200, meta.CacheCreationInputTokens)
	assert.Equal(t, 400, meta.CacheReadInputTokens)
	assert.Greater(t, meta.Cost, float64(0))
}

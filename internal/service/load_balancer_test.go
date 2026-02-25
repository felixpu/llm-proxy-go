//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
)

func TestNewLoadBalancer(t *testing.T) {
	tests := []struct {
		name     string
		strategy models.LoadBalanceStrategy
	}{
		{"round robin", models.StrategyRoundRobin},
		{"weighted", models.StrategyWeighted},
		{"least connections", models.StrategyLeastConnections},
		{"conversation hash", models.StrategyConversationHash},
		{"unknown defaults to weighted", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := NewLoadBalancerWithStrategy(tt.strategy)
			require.NotNil(t, lb)
			require.NotNil(t, lb.cachedStrategy)
		})
	}
}

func TestLoadBalancer_Select_Empty(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)

	result := lb.Select([]*models.Endpoint{}, nil)
	assert.Nil(t, result)
}

func TestLoadBalancer_Select_Single(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)

	ep := createTestEndpoint("provider1", "model1", 1)
	result := lb.Select([]*models.Endpoint{ep}, nil)

	assert.Equal(t, ep, result)
}

func TestRoundRobinBalancer(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyRoundRobin)

	ep1 := createTestEndpoint("provider1", "model1", 1)
	ep2 := createTestEndpoint("provider2", "model1", 1)
	ep3 := createTestEndpoint("provider3", "model1", 1)
	endpoints := []*models.Endpoint{ep1, ep2, ep3}

	// Should cycle through endpoints
	results := make([]string, 6)
	for i := 0; i < 6; i++ {
		selected := lb.Select(endpoints, nil)
		results[i] = selected.Provider.Name
	}

	// Should see round-robin pattern
	assert.Equal(t, "provider1", results[0])
	assert.Equal(t, "provider2", results[1])
	assert.Equal(t, "provider3", results[2])
	assert.Equal(t, "provider1", results[3])
	assert.Equal(t, "provider2", results[4])
	assert.Equal(t, "provider3", results[5])
}

func TestWeightedBalancer(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyWeighted)

	ep1 := createTestEndpoint("provider1", "model1", 3) // Weight 3
	ep2 := createTestEndpoint("provider2", "model1", 1) // Weight 1
	endpoints := []*models.Endpoint{ep1, ep2}

	// Run many selections and count distribution
	counts := make(map[string]int)
	iterations := 1000
	for i := 0; i < iterations; i++ {
		selected := lb.Select(endpoints, nil)
		counts[selected.Provider.Name]++
	}

	// provider1 should be selected ~75% of the time (weight 3/4)
	// Allow some variance due to randomness
	assert.Greater(t, counts["provider1"], counts["provider2"])
	assert.Greater(t, counts["provider1"], iterations/2) // Should be > 50%
}

func TestWeightedBalancer_ZeroWeight(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyWeighted)

	ep1 := createTestEndpoint("provider1", "model1", 0)
	ep2 := createTestEndpoint("provider2", "model1", 0)
	endpoints := []*models.Endpoint{ep1, ep2}

	// Should still select something (random fallback)
	selected := lb.Select(endpoints, nil)
	assert.NotNil(t, selected)
}

func TestConversationHashBalancer(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyConversationHash)

	ep1 := createTestEndpoint("provider1", "model1", 1)
	ep2 := createTestEndpoint("provider2", "model1", 1)
	endpoints := []*models.Endpoint{ep1, ep2}

	// Same conversation should always select same endpoint
	req := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello world"}},
		},
	}

	first := lb.Select(endpoints, req)
	for i := 0; i < 10; i++ {
		selected := lb.Select(endpoints, req)
		assert.Equal(t, first.Provider.Name, selected.Provider.Name)
	}
}

func TestConversationHashBalancer_DifferentConversations(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyConversationHash)

	ep1 := createTestEndpoint("provider1", "model1", 1)
	ep2 := createTestEndpoint("provider2", "model1", 1)
	endpoints := []*models.Endpoint{ep1, ep2}

	// Different conversations may select different endpoints
	req1 := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Hello world"}},
		},
	}
	req2 := &models.AnthropicRequest{
		Messages: []models.Message{
			{Role: "user", Content: models.MessageContent{Text: "Goodbye world"}},
		},
	}

	// Just verify both work without error
	selected1 := lb.Select(endpoints, req1)
	selected2 := lb.Select(endpoints, req2)
	assert.NotNil(t, selected1)
	assert.NotNil(t, selected2)
}

func TestConversationHashBalancer_NilRequest(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyConversationHash)

	ep1 := createTestEndpoint("provider1", "model1", 1)
	ep2 := createTestEndpoint("provider2", "model1", 1)
	endpoints := []*models.Endpoint{ep1, ep2}

	// Nil request should fall back to random
	selected := lb.Select(endpoints, nil)
	assert.NotNil(t, selected)
}

func TestConversationHashBalancer_EmptyMessages(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyConversationHash)

	ep1 := createTestEndpoint("provider1", "model1", 1)
	ep2 := createTestEndpoint("provider2", "model1", 1)
	endpoints := []*models.Endpoint{ep1, ep2}

	req := &models.AnthropicRequest{
		Messages: []models.Message{},
	}

	// Empty messages should fall back to random
	selected := lb.Select(endpoints, req)
	assert.NotNil(t, selected)
}

func TestLeastConnectionsBalancer(t *testing.T) {
	lb := NewLoadBalancerWithStrategy(models.StrategyLeastConnections)

	ep1 := createTestEndpoint("provider1", "model1", 1)
	ep2 := createTestEndpoint("provider2", "model1", 1)
	endpoints := []*models.Endpoint{ep1, ep2}

	// Should select something (currently falls back to random)
	selected := lb.Select(endpoints, nil)
	assert.NotNil(t, selected)
}

func TestEndpointName(t *testing.T) {
	ep := createTestEndpoint("my-provider", "my-model", 1)
	name := EndpointName(ep)
	assert.Equal(t, "my-provider/my-model", name)
}

// Helper function to create test endpoints
func createTestEndpoint(providerName, modelName string, weight int) *models.Endpoint {
	return &models.Endpoint{
		Provider: &models.Provider{
			ID:      1,
			Name:    providerName,
			BaseURL: "https://api.example.com",
			APIKey:  "test-key",
			Weight:  weight,
			Enabled: true,
		},
		Model: &models.Model{
			ID:      1,
			Name:    modelName,
			Role:    models.ModelRoleDefault,
			Enabled: true,
		},
		Status: models.EndpointHealthy,
	}
}

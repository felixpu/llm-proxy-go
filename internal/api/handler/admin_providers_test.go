//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import "testing"

import "github.com/stretchr/testify/assert"

func TestNormalizeProviderAPIType_DefaultsToAnthropicMessages(t *testing.T) {
	apiType, err := normalizeProviderAPIType("")
	assert.NoError(t, err)
	assert.Equal(t, "anthropic_messages", apiType)
}

func TestNormalizeProviderAPIType_TrimsExplicitValue(t *testing.T) {
	apiType, err := normalizeProviderAPIType("  auto  ")
	assert.NoError(t, err)
	assert.Equal(t, "auto", apiType)
}

func TestNormalizeProviderAPIType_RejectsInvalidValue(t *testing.T) {
	apiType, err := normalizeProviderAPIType("responses")
	assert.Error(t, err)
	assert.Empty(t, apiType)
}

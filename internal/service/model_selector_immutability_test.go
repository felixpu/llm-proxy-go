//go:build !integration && !e2e

package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
)

// TestGlobalMapsImmutability verifies that global maps are not modified during concurrent access.
func TestGlobalMapsImmutability(t *testing.T) {
	// Capture original state using getter functions
	originalFallbackPriority := make(map[models.ModelRole][]models.ModelRole)
	for _, role := range []models.ModelRole{models.ModelRoleSimple, models.ModelRoleDefault, models.ModelRoleComplex} {
		originalFallbackPriority[role] = GetFallbackPriority(role)
	}

	originalSameRoleFallback := make(map[models.ModelRole][]models.ModelRole)
	for _, role := range []models.ModelRole{models.ModelRoleSimple, models.ModelRoleDefault, models.ModelRoleComplex} {
		originalSameRoleFallback[role] = GetSameRoleFallback(role)
	}

	// Simulate concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Read using getter functions
			_ = GetFallbackPriority(models.ModelRoleSimple)
			_ = GetFallbackPriority(models.ModelRoleDefault)
			_ = GetFallbackPriority(models.ModelRoleComplex)

			_ = GetSameRoleFallback(models.ModelRoleSimple)
			_ = GetSameRoleFallback(models.ModelRoleDefault)
			_ = GetSameRoleFallback(models.ModelRoleComplex)
		}()
	}
	wg.Wait()

	// Verify maps are unchanged
	for role, expected := range originalFallbackPriority {
		actual := GetFallbackPriority(role)
		assert.Equal(t, expected, actual, "FallbackPriority should not be modified")
	}

	for role, expected := range originalSameRoleFallback {
		actual := GetSameRoleFallback(role)
		assert.Equal(t, expected, actual, "SameRoleFallback should not be modified")
	}
}

// TestGlobalMapsAreReadOnly documents that these maps should never be modified.
// This test will fail if someone tries to modify the global maps.
func TestGlobalMapsAreReadOnly(t *testing.T) {
	// Document expected values
	expectedFallbackPriority := map[models.ModelRole][]models.ModelRole{
		models.ModelRoleSimple:  {models.ModelRoleSimple, models.ModelRoleDefault, models.ModelRoleComplex},
		models.ModelRoleDefault: {models.ModelRoleDefault, models.ModelRoleComplex},
		models.ModelRoleComplex: {models.ModelRoleComplex, models.ModelRoleDefault},
	}

	expectedSameRoleFallback := map[models.ModelRole][]models.ModelRole{
		models.ModelRoleSimple:  {models.ModelRoleSimple},
		models.ModelRoleDefault: {models.ModelRoleDefault},
		models.ModelRoleComplex: {models.ModelRoleComplex},
	}

	// Verify current values match expected
	for role, expected := range expectedFallbackPriority {
		actual := GetFallbackPriority(role)
		assert.Equal(t, expected, actual, "FallbackPriority should match expected values")
	}

	for role, expected := range expectedSameRoleFallback {
		actual := GetSameRoleFallback(role)
		assert.Equal(t, expected, actual, "SameRoleFallback should match expected values")
	}
}

// TestGetterFunctionsReturnCopies verifies that getter functions return copies, not references.
func TestGetterFunctionsReturnCopies(t *testing.T) {
	// Get a chain
	chain1 := GetFallbackPriority(models.ModelRoleSimple)
	chain2 := GetFallbackPriority(models.ModelRoleSimple)

	// Verify they are equal
	assert.Equal(t, chain1, chain2)

	// Modify chain1
	if len(chain1) > 0 {
		chain1[0] = models.ModelRoleComplex
	}

	// Verify chain2 is unchanged (proving they are separate copies)
	chain3 := GetFallbackPriority(models.ModelRoleSimple)
	assert.Equal(t, chain2, chain3, "Modifying returned slice should not affect subsequent calls")
	assert.NotEqual(t, chain1, chain3, "Modified slice should differ from original")
}

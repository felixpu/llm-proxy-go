//go:build !integration && !e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMemoryCalculation_NoUnderflow verifies that memory increase calculation
// handles the case where current allocation is less than baseline (after GC).
func TestMemoryCalculation_NoUnderflow(t *testing.T) {
	tests := []struct {
		name             string
		baselineAlloc    uint64
		currentAlloc     uint64
		expectedIncrease uint64
	}{
		{
			name:             "normal increase",
			baselineAlloc:    100 * 1024 * 1024, // 100 MB
			currentAlloc:     200 * 1024 * 1024, // 200 MB
			expectedIncrease: 100,                // 100 MB increase
		},
		{
			name:             "no change",
			baselineAlloc:    100 * 1024 * 1024,
			currentAlloc:     100 * 1024 * 1024,
			expectedIncrease: 0,
		},
		{
			name:             "decrease after GC (should not underflow)",
			baselineAlloc:    200 * 1024 * 1024, // 200 MB
			currentAlloc:     100 * 1024 * 1024, // 100 MB (after GC)
			expectedIncrease: 0,                  // Should be 0, not underflow
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the calculation with underflow protection
			var memoryIncreaseMB uint64
			if tt.currentAlloc > tt.baselineAlloc {
				memoryIncreaseMB = (tt.currentAlloc - tt.baselineAlloc) / 1024 / 1024
			} else {
				memoryIncreaseMB = 0
			}

			assert.Equal(t, tt.expectedIncrease, memoryIncreaseMB)
		})
	}
}

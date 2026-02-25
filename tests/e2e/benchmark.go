//go:build e2e
// +build e2e

package e2e_test

import (
	"encoding/json"
	"testing"
)

// BenchmarkTestCase represents a single test case for benchmarking.
type BenchmarkTestCase struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Input     any    `json:"input"`
	Expected  any    `json:"expected"`
	Tolerance float64 `json:"tolerance,omitempty"`
}

// BenchmarkTestSuite represents a collection of test cases.
type BenchmarkTestSuite struct {
	Name      string                 `json:"name"`
	Version   string                 `json:"version"`
	TestCases []BenchmarkTestCase    `json:"test_cases"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
}

// LoadTestSuite loads a test suite from JSON data.
func LoadTestSuite(data []byte) (*BenchmarkTestSuite, error) {
	var suite BenchmarkTestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

// SaveTestSuite saves a test suite to JSON data.
func SaveTestSuite(suite *BenchmarkTestSuite) ([]byte, error) {
	return json.MarshalIndent(suite, "", "  ")
}

// AssertEqual checks if two values are equal with optional tolerance for floats.
func AssertEqual(t *testing.T, expected, actual interface{}, tolerance float64) {
	t.Helper()

	// Handle float comparison with tolerance
	if expectedFloat, ok := expected.(float64); ok {
		if actualFloat, ok := actual.(float64); ok {
			if tolerance > 0 {
				diff := expectedFloat - actualFloat
				if diff < 0 {
					diff = -diff
				}
				if diff > tolerance {
					t.Errorf("expected %v, got %v (tolerance: %v)", expectedFloat, actualFloat, tolerance)
				}
				return
			}
		}
	}

	// Direct comparison
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

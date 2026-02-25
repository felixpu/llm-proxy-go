package test

import (
	"os"
	"path/filepath"
)

// TestDataExporter exports test data to JSON files.
type TestDataExporter struct {
	outputDir string
}

// NewTestDataExporter creates a new test data exporter.
func NewTestDataExporter(outputDir string) *TestDataExporter {
	return &TestDataExporter{
		outputDir: outputDir,
	}
}

// ExportTestSuite exports a test suite to a JSON file.
func (e *TestDataExporter) ExportTestSuite(suite *BenchmarkTestSuite) error {
	data, err := SaveTestSuite(suite)
	if err != nil {
		return err
	}

	filename := filepath.Join(e.outputDir, suite.Name+".json")
	return os.WriteFile(filename, data, 0o644)
}

// ImportTestSuite imports a test suite from a JSON file.
func (e *TestDataExporter) ImportTestSuite(filename string) (*BenchmarkTestSuite, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return LoadTestSuite(data)
}

// CreateConfigTestSuite creates a test suite for configuration management.
func CreateConfigTestSuite() *BenchmarkTestSuite {
	return &BenchmarkTestSuite{
		Name:    "config_management",
		Version: "1.0",
		TestCases: []BenchmarkTestCase{
			{
				ID:       "config_001",
				Name:     "Load default admin username",
				Input:    map[string]string{"key": "default_admin_username"},
				Expected: "admin",
			},
			{
				ID:       "config_002",
				Name:     "Load session expire hours",
				Input:    map[string]string{"key": "session_expire_hours"},
				Expected: float64(24),
			},
			{
				ID:       "config_003",
				Name:     "Load secret key",
				Input:    map[string]string{"key": "secret_key"},
				Expected: "change-this-to-a-random-secret-key",
			},
		},
		Metadata: map[string]any{
			"source": "Python config tests",
			"version": "1.0",
		},
	}
}

// CreateLoadBalancingTestSuite creates a test suite for load balancing.
func CreateLoadBalancingTestSuite() *BenchmarkTestSuite {
	return &BenchmarkTestSuite{
		Name:    "load_balancing",
		Version: "1.0",
		TestCases: []BenchmarkTestCase{
			{
				ID:   "lb_001",
				Name: "Round robin selection",
				Input: map[string]any{
					"strategy": "round_robin",
					"endpoints": []string{"ep1", "ep2", "ep3"},
				},
				Expected: "ep1",
			},
			{
				ID:   "lb_002",
				Name: "Weighted selection",
				Input: map[string]any{
					"strategy": "weighted",
					"weights":  []float64{1, 2, 3},
				},
				Expected: float64(2),
			},
			{
				ID:   "lb_003",
				Name: "Least connections",
				Input: map[string]any{
					"strategy": "least_connections",
					"connections": []int{5, 2, 8},
				},
				Expected: float64(1),
			},
		},
		Metadata: map[string]any{
			"source": "Python load balancer tests",
		},
	}
}

// CreateHealthCheckTestSuite creates a test suite for health checks.
func CreateHealthCheckTestSuite() *BenchmarkTestSuite {
	return &BenchmarkTestSuite{
		Name:    "health_check",
		Version: "1.0",
		TestCases: []BenchmarkTestCase{
			{
				ID:       "hc_001",
				Name:     "Healthy endpoint detection",
				Input:    map[string]string{"endpoint": "http://localhost:8000"},
				Expected: "healthy",
			},
			{
				ID:       "hc_002",
				Name:     "Unhealthy endpoint detection",
				Input:    map[string]string{"endpoint": "http://localhost:9999"},
				Expected: "unhealthy",
			},
		},
		Metadata: map[string]any{
			"source": "Python health check tests",
		},
	}
}

// CreateAuthenticationTestSuite creates a test suite for authentication.
func CreateAuthenticationTestSuite() *BenchmarkTestSuite {
	return &BenchmarkTestSuite{
		Name:    "authentication",
		Version: "1.0",
		TestCases: []BenchmarkTestCase{
			{
				ID:   "auth_001",
				Name: "Valid credentials",
				Input: map[string]string{
					"username": "admin",
					"password": "admin123",
				},
				Expected: true,
			},
			{
				ID:   "auth_002",
				Name: "Invalid credentials",
				Input: map[string]string{
					"username": "admin",
					"password": "wrongpassword",
				},
				Expected: false,
			},
		},
		Metadata: map[string]any{
			"source": "Python authentication tests",
		},
	}
}

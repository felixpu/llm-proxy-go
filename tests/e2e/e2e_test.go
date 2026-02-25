//go:build e2e
// +build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// E2ETestCase represents an end-to-end test case.
type E2ETestCase struct {
	Name           string
	Method         string
	Path           string
	Headers        map[string]string
	Body           any
	ExpectedStatus int
	ExpectedBody   any
	Setup          func() error
	Cleanup        func() error
}

// E2ETestSuite manages a suite of E2E tests.
type E2ETestSuite struct {
	server    *httptest.Server
	client    *http.Client
	testCases []*E2ETestCase
	results   []*E2ETestResult
}

// E2ETestResult represents the result of an E2E test.
type E2ETestResult struct {
	TestCase   *E2ETestCase
	StatusCode int
	Body       []byte
	Duration   time.Duration
	Error      error
	Passed     bool
}

// NewE2ETestSuite creates a new E2E test suite.
func NewE2ETestSuite(server *httptest.Server) *E2ETestSuite {
	return &E2ETestSuite{
		server:    server,
		client:    &http.Client{Timeout: 10 * time.Second},
		testCases: make([]*E2ETestCase, 0),
		results:   make([]*E2ETestResult, 0),
	}
}

// AddTestCase adds a test case to the suite.
func (suite *E2ETestSuite) AddTestCase(tc *E2ETestCase) {
	suite.testCases = append(suite.testCases, tc)
}

// Run executes all test cases in the suite.
func (suite *E2ETestSuite) Run(t *testing.T) {
	for _, tc := range suite.testCases {
		result := suite.runTestCase(tc)
		suite.results = append(suite.results, result)

		if !result.Passed {
			t.Errorf("Test %s failed: %v", tc.Name, result.Error)
		}
	}
}

// runTestCase executes a single test case.
func (suite *E2ETestSuite) runTestCase(tc *E2ETestCase) *E2ETestResult {
	result := &E2ETestResult{
		TestCase: tc,
	}

	// Setup
	if tc.Setup != nil {
		if err := tc.Setup(); err != nil {
			result.Error = fmt.Errorf("setup failed: %w", err)
			return result
		}
	}

	// Prepare request
	var body io.Reader
	if tc.Body != nil {
		bodyBytes, err := json.Marshal(tc.Body)
		if err != nil {
			result.Error = fmt.Errorf("marshal body failed: %w", err)
			return result
		}
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(tc.Method, suite.server.URL+tc.Path, body)
	if err != nil {
		result.Error = fmt.Errorf("create request failed: %w", err)
		return result
	}

	// Set headers
	for key, value := range tc.Headers {
		req.Header.Set(key, value)
	}
	if tc.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	start := time.Now()
	resp, err := suite.client.Do(req)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("request failed: %w", err)
		return result
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("read response failed: %w", err)
		return result
	}

	result.StatusCode = resp.StatusCode
	result.Body = respBody

	// Verify status code
	if resp.StatusCode != tc.ExpectedStatus {
		result.Error = fmt.Errorf("expected status %d, got %d", tc.ExpectedStatus, resp.StatusCode)
		return result
	}

	// Verify response body if expected
	if tc.ExpectedBody != nil {
		var expected, actual any
		if err := json.Unmarshal(respBody, &actual); err != nil {
			result.Error = fmt.Errorf("unmarshal response failed: %w", err)
			return result
		}

		expectedBytes, err := json.Marshal(tc.ExpectedBody)
		if err != nil {
			result.Error = fmt.Errorf("marshal expected body failed: %w", err)
			return result
		}

		if err := json.Unmarshal(expectedBytes, &expected); err != nil {
			result.Error = fmt.Errorf("unmarshal expected body failed: %w", err)
			return result
		}

		if !deepEqual(expected, actual) {
			result.Error = fmt.Errorf("response body mismatch")
			return result
		}
	}

	// Cleanup
	if tc.Cleanup != nil {
		if err := tc.Cleanup(); err != nil {
			result.Error = fmt.Errorf("cleanup failed: %w", err)
			return result
		}
	}

	result.Passed = true
	return result
}

// GetResults returns all test results.
func (suite *E2ETestSuite) GetResults() []*E2ETestResult {
	return suite.results
}

// Summary returns a summary of test results.
func (suite *E2ETestSuite) Summary() string {
	passed := 0
	failed := 0
	totalDuration := time.Duration(0)

	for _, result := range suite.results {
		if result.Passed {
			passed++
		} else {
			failed++
		}
		totalDuration += result.Duration
	}

	return fmt.Sprintf(`
E2E Test Summary:
Total: %d
Passed: %d
Failed: %d
Total Duration: %v
Average Duration: %v
`, len(suite.results), passed, failed, totalDuration, totalDuration/time.Duration(len(suite.results)))
}

// deepEqual performs a deep equality check on two values.
func deepEqual(a, b any) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return bytes.Equal(aJSON, bJSON)
}

// AuthFlow represents an authentication flow test.
type AuthFlow struct {
	Username string
	Password string
	Token    string
}

// NewAuthFlow creates a new authentication flow.
func NewAuthFlow(username, password string) *AuthFlow {
	return &AuthFlow{
		Username: username,
		Password: password,
	}
}

// LoginTestCase creates a login test case.
func (af *AuthFlow) LoginTestCase() *E2ETestCase {
	return &E2ETestCase{
		Name:           "Login",
		Method:         "POST",
		Path:           "/api/auth/login",
		Headers:        map[string]string{},
		Body:           map[string]string{"username": af.Username, "password": af.Password},
		ExpectedStatus: http.StatusOK,
	}
}

// LogoutTestCase creates a logout test case.
func (af *AuthFlow) LogoutTestCase() *E2ETestCase {
	return &E2ETestCase{
		Name:           "Logout",
		Method:         "POST",
		Path:           "/api/auth/logout",
		Headers:        map[string]string{"Authorization": "Bearer " + af.Token},
		ExpectedStatus: http.StatusOK,
	}
}

// GetMeTestCase creates a get current user test case.
func (af *AuthFlow) GetMeTestCase() *E2ETestCase {
	return &E2ETestCase{
		Name:           "Get Current User",
		Method:         "GET",
		Path:           "/api/auth/me",
		Headers:        map[string]string{"Authorization": "Bearer " + af.Token},
		ExpectedStatus: http.StatusOK,
	}
}

package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertJSONEqual compares two values as JSON, ignoring field order.
func AssertJSONEqual(t *testing.T, expected, actual any) {
	t.Helper()

	expectedJSON, err := json.Marshal(expected)
	require.NoError(t, err, "failed to marshal expected value")

	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err, "failed to marshal actual value")

	assert.JSONEq(t, string(expectedJSON), string(actualJSON))
}

// AssertHTTPStatus checks that the HTTP response has the expected status code.
func AssertHTTPStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	assert.Equal(t, expected, resp.StatusCode, "unexpected HTTP status code")
}

// AssertHTTPStatusOK checks that the HTTP response has status 200.
func AssertHTTPStatusOK(t *testing.T, resp *http.Response) {
	t.Helper()
	AssertHTTPStatus(t, resp, http.StatusOK)
}

// AssertHTTPStatusCreated checks that the HTTP response has status 201.
func AssertHTTPStatusCreated(t *testing.T, resp *http.Response) {
	t.Helper()
	AssertHTTPStatus(t, resp, http.StatusCreated)
}

// AssertHTTPStatusBadRequest checks that the HTTP response has status 400.
func AssertHTTPStatusBadRequest(t *testing.T, resp *http.Response) {
	t.Helper()
	AssertHTTPStatus(t, resp, http.StatusBadRequest)
}

// AssertHTTPStatusUnauthorized checks that the HTTP response has status 401.
func AssertHTTPStatusUnauthorized(t *testing.T, resp *http.Response) {
	t.Helper()
	AssertHTTPStatus(t, resp, http.StatusUnauthorized)
}

// AssertHTTPStatusForbidden checks that the HTTP response has status 403.
func AssertHTTPStatusForbidden(t *testing.T, resp *http.Response) {
	t.Helper()
	AssertHTTPStatus(t, resp, http.StatusForbidden)
}

// AssertHTTPStatusNotFound checks that the HTTP response has status 404.
func AssertHTTPStatusNotFound(t *testing.T, resp *http.Response) {
	t.Helper()
	AssertHTTPStatus(t, resp, http.StatusNotFound)
}

// ReadJSONResponse reads and unmarshals a JSON response body.
func ReadJSONResponse(t *testing.T, resp *http.Response, v any) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read response body")
	defer resp.Body.Close()

	err = json.Unmarshal(body, v)
	require.NoError(t, err, "failed to unmarshal response body: %s", string(body))
}

// AssertContains checks that the string contains the substring.
func AssertContains(t *testing.T, s, substr string) {
	t.Helper()
	assert.Contains(t, s, substr)
}

// AssertNotContains checks that the string does not contain the substring.
func AssertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	assert.NotContains(t, s, substr)
}

// AssertNil checks that the value is nil.
func AssertNil(t *testing.T, v any) {
	t.Helper()
	assert.Nil(t, v)
}

// AssertNotNil checks that the value is not nil.
func AssertNotNil(t *testing.T, v any) {
	t.Helper()
	assert.NotNil(t, v)
}

// AssertEqual checks that two values are equal.
func AssertEqual(t *testing.T, expected, actual any) {
	t.Helper()
	assert.Equal(t, expected, actual)
}

// AssertNotEqual checks that two values are not equal.
func AssertNotEqual(t *testing.T, expected, actual any) {
	t.Helper()
	assert.NotEqual(t, expected, actual)
}

// AssertTrue checks that the value is true.
func AssertTrue(t *testing.T, v bool) {
	t.Helper()
	assert.True(t, v)
}

// AssertFalse checks that the value is false.
func AssertFalse(t *testing.T, v bool) {
	t.Helper()
	assert.False(t, v)
}

// AssertNoError checks that the error is nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	assert.NoError(t, err)
}

// AssertError checks that the error is not nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	assert.Error(t, err)
}

// AssertErrorContains checks that the error message contains the substring.
func AssertErrorContains(t *testing.T, err error, substr string) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), substr)
}

// AssertLen checks that the slice/map/string has the expected length.
func AssertLen(t *testing.T, v any, expected int) {
	t.Helper()
	assert.Len(t, v, expected)
}

// AssertEmpty checks that the slice/map/string is empty.
func AssertEmpty(t *testing.T, v any) {
	t.Helper()
	assert.Empty(t, v)
}

// AssertNotEmpty checks that the slice/map/string is not empty.
func AssertNotEmpty(t *testing.T, v any) {
	t.Helper()
	assert.NotEmpty(t, v)
}

// RequireNoError fails the test immediately if the error is not nil.
func RequireNoError(t *testing.T, err error) {
	t.Helper()
	require.NoError(t, err)
}

// RequireError fails the test immediately if the error is nil.
func RequireError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
}

// RequireNotNil fails the test immediately if the value is nil.
func RequireNotNil(t *testing.T, v any) {
	t.Helper()
	require.NotNil(t, v)
}

// RequireEqual fails the test immediately if the values are not equal.
func RequireEqual(t *testing.T, expected, actual any) {
	t.Helper()
	require.Equal(t, expected, actual)
}

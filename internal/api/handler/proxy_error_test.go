//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/service"
)

func TestUnwrapUpstreamError(t *testing.T) {
	t.Run("unwraps wrapped upstream error", func(t *testing.T) {
		expected := &service.UpstreamError{StatusCode: 401, Body: []byte(`{"error":"invalid token"}`)}
		err := fmt.Errorf("wrapped: %w", expected)

		got := unwrapUpstreamError(err)
		require.NotNil(t, got)
		assert.Equal(t, expected.StatusCode, got.StatusCode)
		assert.Equal(t, expected.Body, got.Body)
	})

	t.Run("returns nil for non-upstream error", func(t *testing.T) {
		assert.Nil(t, unwrapUpstreamError(fmt.Errorf("boom")))
	})
}

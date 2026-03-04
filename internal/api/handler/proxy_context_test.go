//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/models"
)

func TestGetEndpointsFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing endpoints", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		eps, ok := getEndpointsFromContext(c)
		assert.False(t, ok)
		assert.Nil(t, eps)
	})

	t.Run("wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(middleware.ContextKeyEndpoints, "invalid")
		eps, ok := getEndpointsFromContext(c)
		assert.False(t, ok)
		assert.Nil(t, eps)
	})

	t.Run("valid endpoints", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		expected := []*models.Endpoint{{}}
		c.Set(middleware.ContextKeyEndpoints, expected)
		eps, ok := getEndpointsFromContext(c)
		assert.True(t, ok)
		assert.Equal(t, expected, eps)
	})
}

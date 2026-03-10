package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterNoRouteHandler_APIPathReturnsJSON404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerNoRouteHandler(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "route not found", resp["detail"])
}

func TestRegisterNoRouteHandler_SPAPathFallsBackToFrontend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerNoRouteHandler(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.NotEmpty(t, w.Body.String())
}

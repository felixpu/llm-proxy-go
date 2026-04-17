//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func TestValidateAliasRequest_ProviderScopeMustContainTargetModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testutil.NewTestDB(t)
	testutil.SeedTestData(t, db)

	h := &ModelAliasHandler{
		modelRepo:    repository.NewModelRepository(db),
		providerRepo: repository.NewProviderRepository(db),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/config/model-aliases", nil)

	validProviderID := int64(1) // anthropic-primary, associated with model #2 in seed data
	err := h.validateAliasRequest(c, "claude-sonnet-4-6", 2, &validProviderID)
	require.NoError(t, err)

	invalidProviderID := int64(3) // disabled-provider, not associated with model #2 in seed data
	err = h.validateAliasRequest(c, "claude-sonnet-4-6", 2, &invalidProviderID)
	require.Error(t, err)
	assert.Equal(t, "provider is not associated with target model", err.Error())
}

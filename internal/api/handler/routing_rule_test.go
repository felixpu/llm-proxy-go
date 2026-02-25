//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/tests/testutil"
)

func setupRoutingRuleTest(t *testing.T) (*RoutingRuleHandler, *repository.RoutingRuleRepo, int64) {
	t.Helper()
	db := testutil.NewTestDB(t)
	ruleRepo := repository.NewRoutingRuleRepository(db, testutil.NewTestLogger())
	userRepo := repository.NewUserRepository(db)

	ctx := context.Background()
	adminID, err := userRepo.Insert(ctx, &models.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hashedpassword",
		Role:         models.UserRoleAdmin,
		IsActive:     true,
	})
	require.NoError(t, err)

	handler := NewRoutingRuleHandler(ruleRepo, testutil.NewTestLogger())
	return handler, ruleRepo, adminID
}

func seedTestRules(t *testing.T, repo *repository.RoutingRuleRepo) {
	t.Helper()
	ctx := context.Background()

	rules := []*models.RoutingRule{
		{Name: "builtin_complex", Keywords: []string{"架构", "设计"}, TaskType: "complex", Priority: 100, IsBuiltin: true, Enabled: true},
		{Name: "custom_simple", Keywords: []string{"列出"}, TaskType: "simple", Priority: 80, IsBuiltin: false, Enabled: true},
		{Name: "disabled_rule", Keywords: []string{"禁用"}, TaskType: "default", Priority: 50, IsBuiltin: false, Enabled: false},
	}
	for _, r := range rules {
		_, err := repo.AddRule(ctx, r)
		require.NoError(t, err)
	}
}

func TestRoutingRuleHandler_ListRules_Success(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	seedTestRules(t, repo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.ListRules(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	rules := resp["rules"].([]any)
	assert.GreaterOrEqual(t, len(rules), 3)
}

func TestRoutingRuleHandler_ListRules_EnabledOnly(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	seedTestRules(t, repo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules?enabled_only=true", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.ListRules(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	rules := resp["rules"].([]any)
	for _, r := range rules {
		rule := r.(map[string]any)
		assert.True(t, rule["enabled"].(bool), "should only return enabled rules")
	}
}

func TestRoutingRuleHandler_GetRule_Success(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	ctx := context.Background()

	id, err := repo.AddRule(ctx, &models.RoutingRule{
		Name:     "test_rule",
		Keywords: []string{"测试"},
		TaskType: "default",
		Priority: 50,
		Enabled:  true,
	})
	require.NoError(t, err)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules/1", nil)
	c.Params = []gin.Param{{Key: "rule_id", Value: fmt.Sprintf("%d", id)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.GetRule(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "test_rule", resp["name"])
}

func TestRoutingRuleHandler_GetRule_NotFound(t *testing.T) {
	handler, _, adminID := setupRoutingRuleTest(t)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules/999", nil)
	c.Params = append(c.Params, gin.Param{Key: "rule_id", Value: fmt.Sprintf("%d", int64(999))})
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.GetRule(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRoutingRuleHandler_CreateRule_Success(t *testing.T) {
	handler, _, adminID := setupRoutingRuleTest(t)

	body := `{"name":"new_rule","keywords":["测试","关键词"],"task_type":"complex","priority":90,"enabled":true}`
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/config/routing/rules", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.CreateRule(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp["id"])
	assert.Equal(t, "Routing rule created", resp["message"])
}

func TestRoutingRuleHandler_CreateRule_InvalidJSON(t *testing.T) {
	handler, _, adminID := setupRoutingRuleTest(t)

	body := `{"name":"","task_type":""}` // missing required fields
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/config/routing/rules", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.CreateRule(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRoutingRuleHandler_UpdateRule_Success(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	ctx := context.Background()

	id, err := repo.AddRule(ctx, &models.RoutingRule{
		Name:     "update_me",
		Keywords: []string{"原始"},
		TaskType: "default",
		Priority: 50,
		Enabled:  true,
	})
	require.NoError(t, err)

	body := `{"name":"updated_name","priority":200}`
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("PUT", "/api/config/routing/rules/1", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = []gin.Param{{Key: "rule_id", Value: fmt.Sprintf("%d", id)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.UpdateRule(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "Routing rule updated", resp["message"])
}

func TestRoutingRuleHandler_UpdateRule_NotFound(t *testing.T) {
	handler, _, adminID := setupRoutingRuleTest(t)

	body := `{"name":"ghost"}`
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("PUT", "/api/config/routing/rules/999", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = []gin.Param{{Key: "rule_id", Value: "999"}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.UpdateRule(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRoutingRuleHandler_DeleteRule_Success(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	ctx := context.Background()

	id, err := repo.AddRule(ctx, &models.RoutingRule{
		Name:     "delete_me",
		Keywords: []string{"删除"},
		TaskType: "default",
		Priority: 50,
		Enabled:  true,
	})
	require.NoError(t, err)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", "/api/config/routing/rules/1", nil)
	c.Params = []gin.Param{{Key: "rule_id", Value: fmt.Sprintf("%d", id)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.DeleteRule(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify deletion
	rule, err := repo.GetRule(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, rule)
}

func TestRoutingRuleHandler_DeleteRule_BuiltinProtected(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	ctx := context.Background()

	id, err := repo.AddRule(ctx, &models.RoutingRule{
		Name:      "builtin_rule",
		Keywords:  []string{"内置"},
		TaskType:  "complex",
		Priority:  100,
		IsBuiltin: true,
		Enabled:   true,
	})
	require.NoError(t, err)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("DELETE", "/api/config/routing/rules/1", nil)
	c.Params = []gin.Param{{Key: "rule_id", Value: fmt.Sprintf("%d", id)}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.DeleteRule(c)

	// Builtin rules should not be deletable
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRoutingRuleHandler_TestMessage_Success(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	seedTestRules(t, repo)

	body := `{"message":"帮我设计一个微服务架构"}`
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/config/routing/rules/test", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.TestMessage(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["final_task_type"])
}

func TestRoutingRuleHandler_TestMessage_EmptyMessage(t *testing.T) {
	handler, _, adminID := setupRoutingRuleTest(t)

	body := `{"message":""}`
	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("POST", "/api/config/routing/rules/test", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.TestMessage(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRoutingRuleHandler_GetStats_Success(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	seedTestRules(t, repo)

	// Generate some hit counts
	ctx := context.Background()
	_ = repo.IncrementHitCount(ctx, 1)
	_ = repo.IncrementHitCount(ctx, 1)
	_ = repo.IncrementHitCount(ctx, 2)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules/stats", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.GetStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Greater(t, resp["total_requests"].(float64), float64(0))
	assert.NotNil(t, resp["rule_hits"])
}

func TestRoutingRuleHandler_ListBuiltinRules(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	seedTestRules(t, repo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules/builtin", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.ListBuiltinRules(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	rules := resp["rules"].([]any)
	for _, r := range rules {
		rule := r.(map[string]any)
		assert.True(t, rule["is_builtin"].(bool), "should only return builtin rules")
	}
}

func TestRoutingRuleHandler_ListCustomRules(t *testing.T) {
	handler, repo, adminID := setupRoutingRuleTest(t)
	seedTestRules(t, repo)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules/custom", nil)
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.ListCustomRules(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	rules := resp["rules"].([]any)
	for _, r := range rules {
		rule := r.(map[string]any)
		assert.False(t, rule["is_builtin"].(bool), "should only return custom rules")
	}
}

func TestRoutingRuleHandler_InvalidRuleID(t *testing.T) {
	handler, _, adminID := setupRoutingRuleTest(t)

	c, w := testutil.NewTestContext()
	c.Request = httptest.NewRequest("GET", "/api/config/routing/rules/abc", nil)
	c.Params = []gin.Param{{Key: "rule_id", Value: "abc"}}
	c.Set("current_user", &service.CurrentUser{
		UserID:   adminID,
		Username: "admin",
		Role:     string(models.UserRoleAdmin),
	})

	handler.GetRule(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

//go:build !integration && !e2e
// +build !integration,!e2e

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/tests/testutil"
	"go.uber.org/zap"
)

func TestRoutingRuleRepository_ListRules(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	tests := []struct {
		name        string
		enabledOnly bool
		wantMin     int
	}{
		{"all rules", false, 3},
		{"enabled only", true, 2}, // rule3 is disabled
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := repo.ListRules(ctx, tt.enabledOnly)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(rules), tt.wantMin)

			if tt.enabledOnly {
				for _, r := range rules {
					assert.True(t, r.Enabled, "expected only enabled rules")
				}
			}
		})
	}
}

func TestRoutingRuleRepository_ListRules_SortedByPriority(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	rules, err := repo.ListRules(ctx, true)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rules), 2)

	// Verify sorted by priority DESC
	for i := 1; i < len(rules); i++ {
		assert.GreaterOrEqual(t, rules[i-1].Priority, rules[i].Priority,
			"rules should be sorted by priority descending")
	}
}

func TestRoutingRuleRepository_GetRule(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	tests := []struct {
		name    string
		id      int64
		wantNil bool
	}{
		{"existing rule", 1, false},
		{"non-existing rule", 999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := repo.GetRule(ctx, tt.id)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, rule)
			} else {
				assert.NotNil(t, rule)
				assert.Equal(t, tt.id, rule.ID)
			}
		})
	}
}

func TestRoutingRuleRepository_AddRule(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	tests := []struct {
		name    string
		rule    *models.RoutingRule
		wantErr bool
	}{
		{
			name: "rule with keywords",
			rule: &models.RoutingRule{
				Name:        "test_keywords",
				Description: "Test rule with keywords",
				Keywords:    []string{"测试", "关键词"},
				TaskType:    "complex",
				Priority:    100,
				Enabled:     true,
			},
			wantErr: false,
		},
		{
			name: "rule with pattern",
			rule: &models.RoutingRule{
				Name:     "test_pattern",
				Pattern:  `(?i)test.*pattern`,
				TaskType: "simple",
				Priority: 80,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "rule with condition",
			rule: &models.RoutingRule{
				Name:      "test_condition",
				Condition: `len(message) > 100`,
				TaskType:  "default",
				Priority:  60,
				Enabled:   true,
			},
			wantErr: false,
		},
		{
			name: "builtin rule",
			rule: &models.RoutingRule{
				Name:      "builtin_test",
				Keywords:  []string{"内置"},
				TaskType:  "complex",
				Priority:  200,
				IsBuiltin: true,
				Enabled:   true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := repo.AddRule(ctx, tt.rule)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Greater(t, id, int64(0))

				// Verify insertion
				found, err := repo.GetRule(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, tt.rule.Name, found.Name)
				assert.Equal(t, tt.rule.TaskType, found.TaskType)
				assert.Equal(t, tt.rule.Priority, found.Priority)
				assert.Equal(t, tt.rule.IsBuiltin, found.IsBuiltin)
				assert.ElementsMatch(t, tt.rule.Keywords, found.Keywords)
			}
		})
	}
}

func TestRoutingRuleRepository_UpdateRule(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	tests := []struct {
		name    string
		id      int64
		updates map[string]any
		verify  func(t *testing.T, r *models.RoutingRule)
	}{
		{
			name: "update name",
			id:   1,
			updates: map[string]any{
				"name": "updated_name",
			},
			verify: func(t *testing.T, r *models.RoutingRule) {
				assert.Equal(t, "updated_name", r.Name)
			},
		},
		{
			name: "update enabled to false",
			id:   1,
			updates: map[string]any{
				"enabled": false,
			},
			verify: func(t *testing.T, r *models.RoutingRule) {
				assert.False(t, r.Enabled)
			},
		},
		{
			name: "update priority",
			id:   2,
			updates: map[string]any{
				"priority": 150,
			},
			verify: func(t *testing.T, r *models.RoutingRule) {
				assert.Equal(t, 150, r.Priority)
			},
		},
		{
			name: "update task_type",
			id:   1,
			updates: map[string]any{
				"task_type": "simple",
			},
			verify: func(t *testing.T, r *models.RoutingRule) {
				assert.Equal(t, "simple", r.TaskType)
			},
		},
		{
			name:    "empty updates",
			id:      1,
			updates: map[string]any{},
			verify:  func(t *testing.T, r *models.RoutingRule) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpdateRule(ctx, tt.id, tt.updates)
			require.NoError(t, err)

			if len(tt.updates) > 0 {
				rule, err := repo.GetRule(ctx, tt.id)
				require.NoError(t, err)
				tt.verify(t, rule)
			}
		})
	}
}

func TestRoutingRuleRepository_DeleteRule(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	// Delete rule
	err := repo.DeleteRule(ctx, 3)
	require.NoError(t, err)

	// Verify deletion
	rule, err := repo.GetRule(ctx, 3)
	require.NoError(t, err)
	assert.Nil(t, rule)
}

func TestRoutingRuleRepository_IncrementHitCount(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	// Get initial hit count
	rule, err := repo.GetRule(ctx, 1)
	require.NoError(t, err)
	initialCount := rule.HitCount

	// Increment
	err = repo.IncrementHitCount(ctx, 1)
	require.NoError(t, err)

	// Verify
	rule, err = repo.GetRule(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, initialCount+1, rule.HitCount)
}

func TestRoutingRuleRepository_GetStats(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	// Increment some hit counts
	_ = repo.IncrementHitCount(ctx, 1)
	_ = repo.IncrementHitCount(ctx, 1)
	_ = repo.IncrementHitCount(ctx, 2)

	stats, err := repo.GetStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Greater(t, stats.TotalRequests, int64(0))
}

func TestRoutingRuleRepository_ListBuiltinRules(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	rules, err := repo.ListBuiltinRules(ctx)
	require.NoError(t, err)

	for _, r := range rules {
		assert.True(t, r.IsBuiltin, "expected only builtin rules")
	}
}

func TestRoutingRuleRepository_ListCustomRules(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewRoutingRuleRepository(db, zap.NewNop())
	ctx := context.Background()

	seedRoutingRules(t, db)

	rules, err := repo.ListCustomRules(ctx)
	require.NoError(t, err)

	for _, r := range rules {
		assert.False(t, r.IsBuiltin, "expected only custom rules")
	}
}

// seedRoutingRules inserts test routing rules.
func seedRoutingRules(t *testing.T, db *sql.DB) {
	t.Helper()

	queries := []string{
		`INSERT INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled, hit_count)
		 VALUES ('test_complex', 'Complex task keywords', '["架构","设计"]', '', '', 'complex', 100, 1, 1, 0)`,
		`INSERT INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled, hit_count)
		 VALUES ('test_simple', 'Simple task pattern', '[]', '^列出.*文件', '', 'simple', 80, 0, 1, 0)`,
		`INSERT INTO routing_rules (name, description, keywords, pattern, condition, task_type, priority, is_builtin, enabled, hit_count)
		 VALUES ('test_disabled', 'Disabled rule', '["禁用"]', '', '', 'default', 50, 0, 0, 0)`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
}

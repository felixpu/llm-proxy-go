package service

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

// ClassifyResult holds the outcome of rule-based classification.
type ClassifyResult struct {
	TaskType string
	Rule     *models.RoutingRule
	Matches  []*models.RuleHit
	Reason   string
}

// RoutingClassifier performs rule-based request classification.
// Rules are evaluated by priority (highest first); the first match wins.
type RoutingClassifier struct {
	rules           []*models.RoutingRule // sorted by priority desc
	compiledPatterns map[int64]*regexp.Regexp
	condParser      *ConditionParser
}

// NewRoutingClassifier creates a classifier with builtin + custom rules.
// Custom rules are merged with builtins; DB rules with the same name override
// hardcoded builtins. Disabled rules are excluded.
func NewRoutingClassifier(customRules []*models.RoutingRule) *RoutingClassifier {
	// Build name -> rule map; builtins first, then DB rules override by name
	ruleByName := make(map[string]*models.RoutingRule, len(builtinRules)+len(customRules))

	for i := range builtinRules {
		r := builtinRules[i] // copy
		ruleByName[r.Name] = &r
	}

	for _, r := range customRules {
		if r != nil {
			ruleByName[r.Name] = r // override same-name builtin
		}
	}

	// Collect into slice
	allRules := make([]*models.RoutingRule, 0, len(ruleByName))
	for _, r := range ruleByName {
		allRules = append(allRules, r)
	}

	// Filter enabled, sort by priority desc
	enabled := make([]*models.RoutingRule, 0, len(allRules))
	for _, r := range allRules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sort.Slice(enabled, func(i, j int) bool {
		return enabled[i].Priority > enabled[j].Priority
	})

	// Pre-compile regex patterns
	compiled := make(map[int64]*regexp.Regexp)
	for _, r := range enabled {
		if r.Pattern != "" {
			if re, err := regexp.Compile(r.Pattern); err == nil {
				compiled[r.ID] = re
			}
		}
	}

	return &RoutingClassifier{
		rules:            enabled,
		compiledPatterns: compiled,
		condParser:       NewConditionParser(),
	}
}

// Classify evaluates all rules against the message and returns the highest-priority match.
func (c *RoutingClassifier) Classify(message string) *ClassifyResult {
	if message == "" {
		return &ClassifyResult{
			TaskType: string(models.ModelRoleDefault),
			Reason:   "empty message",
		}
	}

	var allHits []*models.RuleHit
	var bestRule *models.RoutingRule

	for _, rule := range c.rules {
		matched, reason := c.matchRule(rule, message)
		if !matched {
			continue
		}

		hit := &models.RuleHit{
			RuleID:   rule.ID,
			Name:     rule.Name,
			Priority: rule.Priority,
			TaskType: rule.TaskType,
			Reason:   reason,
		}
		allHits = append(allHits, hit)

		if bestRule == nil {
			bestRule = rule
		}
	}

	if bestRule == nil {
		return &ClassifyResult{
			TaskType: string(models.ModelRoleDefault),
			Matches:  allHits,
			Reason:   "no rule matched, using default",
		}
	}

	return &ClassifyResult{
		TaskType: bestRule.TaskType,
		Rule:     bestRule,
		Matches:  allHits,
		Reason:   buildMatchReason(bestRule, allHits),
	}
}

// TestMessage is like Classify but always populates all matches for debugging.
func (c *RoutingClassifier) TestMessage(message string) *ClassifyResult {
	return c.Classify(message)
}

// matchRule checks if a single rule matches the message.
// Returns (matched, reason).
func (c *RoutingClassifier) matchRule(rule *models.RoutingRule, message string) (bool, string) {
	// Check keywords (any match)
	if len(rule.Keywords) > 0 {
		for _, kw := range rule.Keywords {
			if strings.Contains(message, kw) {
				// If there's also a condition, check it
				if rule.Condition != "" {
					ok, _ := c.condParser.Evaluate(rule.Condition, message)
					if !ok {
						return false, ""
					}
				}
				return true, "keyword: " + kw
			}
		}
		// Keywords defined but none matched
		if rule.Pattern == "" && rule.Condition == "" {
			return false, ""
		}
	}

	// Check pattern (regex)
	if rule.Pattern != "" {
		re := c.compiledPatterns[rule.ID]
		if re != nil && re.MatchString(message) {
			// If there's also a condition, check it
			if rule.Condition != "" {
				ok, _ := c.condParser.Evaluate(rule.Condition, message)
				if !ok {
					return false, ""
				}
			}
			return true, "pattern: " + rule.Pattern
		}
		// Pattern defined but didn't match
		if len(rule.Keywords) == 0 && rule.Condition == "" {
			return false, ""
		}
	}

	// Check condition only (no keywords or pattern)
	if rule.Condition != "" && len(rule.Keywords) == 0 && rule.Pattern == "" {
		ok, _ := c.condParser.Evaluate(rule.Condition, message)
		if ok {
			return true, "condition: " + rule.Condition
		}
	}

	return false, ""
}

// buildMatchReason constructs a human-readable reason string.
func buildMatchReason(rule *models.RoutingRule, hits []*models.RuleHit) string {
	if len(hits) == 0 {
		return "no matches"
	}
	reason := "matched rule: " + rule.Name
	if len(hits) > 1 {
		reason += " (" + strconv.Itoa(len(hits)) + " total matches)"
	}
	return reason
}

// builtinRules defines the default routing rules.
// IDs use negative values to avoid collision with DB auto-increment IDs.
var builtinRules = []models.RoutingRule{
	// Complex rules
	{
		ID:        -1,
		Name:      "architecture_keywords",
		Keywords:  []string{"架构", "设计", "重构", "优化", "规划", "创建项目", "微服务"},
		TaskType:  "complex",
		Priority:  100,
		IsBuiltin: true,
		Enabled:   true,
	},
	{
		ID:        -2,
		Name:      "long_message",
		Condition: "len(message) > 3000",
		TaskType:  "complex",
		Priority:  80,
		IsBuiltin: true,
		Enabled:   true,
	},
	{
		ID:        -3,
		Name:      "multi_file_operation",
		Pattern:   `(?i)(多个文件|批量|所有.*文件|整个.*目录)`,
		TaskType:  "complex",
		Priority:  90,
		IsBuiltin: true,
		Enabled:   true,
	},

	// Simple rules
	{
		ID:        -4,
		Name:      "simple_operations",
		Keywords:  []string{"列出", "查看", "翻译", "转换", "格式化", "显示"},
		Condition: `len(message) < 200 AND NOT contains(message, "分析")`,
		TaskType:  "simple",
		Priority:  100,
		IsBuiltin: true,
		Enabled:   true,
	},
	{
		ID:        -5,
		Name:      "file_listing",
		Pattern:   `^(ls|列出|显示|查看).*(文件|目录|文件夹)`,
		TaskType:  "simple",
		Priority:  90,
		IsBuiltin: true,
		Enabled:   true,
	},

	// Default rules
	{
		ID:        -6,
		Name:      "code_with_analysis",
		Condition: `has_code_block(message) AND (contains(message, "分析") OR contains(message, "解释"))`,
		TaskType:  "default",
		Priority:  70,
		IsBuiltin: true,
		Enabled:   true,
	},
}

package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")
var jsonObjectRe = regexp.MustCompile(`\{[^{}]*"task_type"\s*:\s*"[^"]+?"[^{}]*\}`)

// LLMRouter performs intelligent routing by calling an LLM to infer task type.
type LLMRouter struct {
	configRepo    *repository.RoutingConfigRepository
	modelRepo     *repository.RoutingModelRepository
	embeddingRepo *repository.EmbeddingCacheRepository
	routingCache  *RoutingCache
	embeddingSvc  *EmbeddingService
	ruleRepo      *repository.RoutingRuleRepo
	logger        *zap.Logger
	client        *http.Client
}

// NewLLMRouter creates a new LLMRouter.
func NewLLMRouter(
	db *sql.DB,
	embeddingSvc *EmbeddingService,
	logger *zap.Logger,
) *LLMRouter {
	return &LLMRouter{
		configRepo:    repository.NewRoutingConfigRepository(db, logger),
		modelRepo:     repository.NewRoutingModelRepository(db, logger),
		embeddingRepo: repository.NewEmbeddingCacheRepository(db, logger),
		routingCache:  NewRoutingCache(10000, logger),
		embeddingSvc:  embeddingSvc,
		ruleRepo:      repository.NewRoutingRuleRepository(db, logger),
		logger:        logger,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// InferTaskType infers the task type for a request first using rule-based routing,
// then falling back to LLM routing if configured.
// Returns (task_type, decision, error).
// On any failure, returns (ModelRoleDefault, nil, nil) as safe fallback.
func (r *LLMRouter) InferTaskType(ctx context.Context, req *models.AnthropicRequest) (models.ModelRole, *models.RoutingDecision, error) {
	// Step 1: Get routing configuration
	cfg, err := r.configRepo.GetConfig(ctx)
	if err != nil {
		r.logger.Warn("failed to get routing config", zap.Error(err))
		return models.ModelRoleDefault, nil, nil
	}

	// Step 2: Extract content from request
	systemContent := extractSystemContent(req)
	userMessage := extractLastUserMessage(req)
	if userMessage == "" {
		r.logger.Debug("no user message found, using default role")
		return models.ModelRoleDefault, nil, nil
	}

	// Step 3: Rule-based routing (if enabled)
	if cfg.RuleBasedRoutingEnabled {
		taskType, decision, fallback := r.classifyWithRules(ctx, cfg, userMessage)
		if !fallback {
			// Rule matched - return immediately
			return taskType, decision, nil
		}
		// No rule matched, proceed to fallback strategy
	}

	// If rule-based routing is disabled or no rules matched, check if LLM routing is enabled
	if !cfg.Enabled {
		// LLM routing disabled - use fallback strategy
		taskType, decision, shouldUseLLM := r.handleFallbackStrategy(ctx, cfg, nil)
		if shouldUseLLM {
			// FallbackLLM requested but LLM is disabled, return default with decision
			return models.ModelRoleDefault, &models.RoutingDecision{
				TaskType:  models.ModelRoleDefault,
				Reason:    "fallback: LLM routing disabled, using default",
				CacheType: "rule",
			}, nil
		}
		return taskType, decision, nil
	}

	// Step 4: L1 memory cache lookup
	cacheTTL := cfg.CacheTTLSeconds
	cacheKey := GetCacheKey(systemContent, userMessage)
	if cfg.CacheEnabled {
		if taskType, hit := r.routingCache.Get(cacheKey, cacheTTL); hit {
			decision := &models.RoutingDecision{
				TaskType:  taskType,
				FromCache: true,
				CacheType: "L1",
			}
			return taskType, decision, nil
		}
	}

	// Step 5: L2 persistent cache lookup (exact match)
	if cfg.CacheEnabled {
		entry, err := r.embeddingRepo.GetExactMatch(ctx, cacheKey, cacheTTL)
		if err != nil {
			r.logger.Warn("L2 cache lookup failed", zap.Error(err))
		} else if entry != nil {
			taskType := parseModelRole(entry.TaskType)
			// Promote to L1
			r.routingCache.Set(cacheKey, taskType)
			// Update hit count async
			go func() { _ = r.embeddingRepo.UpdateHitCountByHash(context.Background(), cacheKey) }()

			decision := &models.RoutingDecision{
				TaskType:  taskType,
				Reason:    entry.Reason,
				FromCache: true,
				CacheType: "L2",
			}
			return taskType, decision, nil
		}
	}

	// Step 6: Call routing LLM model with retry
	taskType, decision := r.callRoutingWithRetry(ctx, cfg, systemContent, userMessage)

	// Step 7: Save to caches
	if decision != nil && cfg.CacheEnabled {
		r.routingCache.Set(cacheKey, taskType)

		contentPreview := userMessage
		if len(contentPreview) > 200 {
			contentPreview = contentPreview[:200]
		}
		_ = r.embeddingRepo.SaveCache(ctx, cacheKey, contentPreview, nil, string(taskType), decision.Reason)
	}

	return taskType, decision, nil
}

// classifyWithRules runs rule-based classification.
// Returns (taskType, decision, fallback) where fallback=true means no rule matched.
func (r *LLMRouter) classifyWithRules(ctx context.Context, cfg *models.RoutingConfig, message string) (models.ModelRole, *models.RoutingDecision, bool) {
	customRules, err := r.ruleRepo.ListRules(ctx, true)
	if err != nil {
		r.logger.Warn("failed to load custom rules, using builtins only", zap.Error(err))
		customRules = nil
	}

	classifier := NewRoutingClassifier(customRules)
	result := classifier.Classify(message)

	// Increment hit count for matched rule async
	if result.Rule != nil && result.Rule.ID > 0 {
		go func() { _ = r.ruleRepo.IncrementHitCount(context.Background(), result.Rule.ID) }()
	}

	taskType := parseModelRole(result.TaskType)
	decision := &models.RoutingDecision{
		TaskType:  taskType,
		Reason:    result.Reason,
		FromCache: false,
		CacheType: "rule",
	}

	// If no rule matched (fallback reason), delegate to fallback strategy
	if result.Rule == nil {
		return r.handleFallbackStrategy(ctx, cfg, decision)
	}

	return taskType, decision, false
}

// handleFallbackStrategy applies the configured fallback when no rule matches.
// Returns (taskType, decision, fallback=false) — always resolves.
func (r *LLMRouter) handleFallbackStrategy(_ context.Context, cfg *models.RoutingConfig, _ *models.RoutingDecision) (models.ModelRole, *models.RoutingDecision, bool) {
	switch cfg.RuleFallbackStrategy {
	case models.FallbackLLM:
		// Signal caller to proceed with LLM routing
		return models.ModelRoleDefault, nil, true
	case models.FallbackUserChoice:
		taskType := parseModelRole(cfg.RuleFallbackTaskType)
		return taskType, &models.RoutingDecision{
			TaskType:  taskType,
			Reason:    "fallback: user-configured task type",
			CacheType: "rule",
		}, false
	default: // FallbackDefault
		return models.ModelRoleDefault, &models.RoutingDecision{
			TaskType:  models.ModelRoleDefault,
			Reason:    "fallback: no rule matched, using default",
			CacheType: "rule",
		}, false
	}
}

// callRoutingWithRetry calls the routing LLM with retry and fallback logic.
func (r *LLMRouter) callRoutingWithRetry(
	ctx context.Context,
	cfg *models.RoutingConfig,
	systemContent, userMessage string,
) (models.ModelRole, *models.RoutingDecision) {
	if cfg.PrimaryModelID == nil {
		r.logger.Warn("no primary routing model configured")
		return models.ModelRoleDefault, nil
	}

	currentModelID := *cfg.PrimaryModelID
	maxAttempts := cfg.RetryCount + 1

	for attempt := range maxAttempts {
		modelCfg, err := r.modelRepo.GetModelWithProvider(ctx, currentModelID)
		if err != nil || modelCfg == nil {
			r.logger.Warn("failed to get routing model",
				zap.Int64("model_id", currentModelID),
				zap.Error(err))

			// Try fallback
			if cfg.FallbackModelID != nil && *cfg.FallbackModelID != currentModelID {
				currentModelID = *cfg.FallbackModelID
				continue
			}
			return models.ModelRoleDefault, nil
		}

		decision, err := r.callRoutingModel(ctx, systemContent, userMessage, modelCfg, cfg)
		if err != nil {
			r.logger.Warn("routing model call failed",
				zap.Int("attempt", attempt+1),
				zap.Int("max_attempts", maxAttempts),
				zap.String("model", modelCfg.ModelName),
				zap.Error(err))

			// Try fallback on failure
			if cfg.FallbackModelID != nil && *cfg.FallbackModelID != currentModelID {
				currentModelID = *cfg.FallbackModelID
				continue
			}
			continue
		}

		decision.ModelUsed = modelCfg.ModelName
		return decision.TaskType, decision
	}

	r.logger.Warn("all routing attempts failed, using default")
	return models.ModelRoleDefault, nil
}

// callRoutingModel calls a single routing model via OpenAI-compatible chat API.
func (r *LLMRouter) callRoutingModel(
	ctx context.Context,
	systemContent, userMessage string,
	modelCfg *models.RoutingModelWithProvider,
	routingCfg *models.RoutingConfig,
) (*models.RoutingDecision, error) {
	userPrompt := BuildRoutingPrompt(systemContent, userMessage)

	reqBody := map[string]any{
		"model":       modelCfg.ModelName,
		"max_tokens":  routingCfg.MaxTokens,
		"temperature": routingCfg.Temperature,
		"messages": []map[string]string{
			{"role": "system", "content": RoutingSystemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal routing request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", modelCfg.BaseURL)
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(routingCfg.TimeoutSeconds)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create routing request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+modelCfg.APIKey)

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("routing API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read routing response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routing API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse OpenAI-compatible response
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("decode routing response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty routing response")
	}

	content := chatResp.Choices[0].Message.Content
	return parseRoutingDecision(content)
}

// parseRoutingDecision extracts a RoutingDecision from LLM response text.
func parseRoutingDecision(text string) (*models.RoutingDecision, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in routing response: %s", truncate(text, 200))
	}

	var result struct {
		TaskType string `json:"task_type"`
		Reason   string `json:"reason"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse routing JSON: %w", err)
	}

	taskType := parseModelRole(result.TaskType)

	return &models.RoutingDecision{
		TaskType:  taskType,
		Reason:    result.Reason,
		FromCache: false,
	}, nil
}

// extractJSON extracts JSON from text, supporting markdown code blocks.
func extractJSON(text string) string {
	// Try markdown code block first
	if matches := jsonBlockRe.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try direct JSON parse
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		return text
	}

	// Try regex extraction
	if match := jsonObjectRe.FindString(text); match != "" {
		return match
	}

	return ""
}

// extractSystemContent extracts system content from the request.
func extractSystemContent(req *models.AnthropicRequest) string {
	if req.System == nil || req.System.IsEmpty() {
		return ""
	}
	return req.System.String()
}

// extractLastUserMessage extracts the last user message text from the request.
func extractLastUserMessage(req *models.AnthropicRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}

	// Iterate from the end to find the last user message
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role != "user" {
			continue
		}

		// Content can be a string or array of content parts
		parts := msg.Content.GetParts()
		if len(parts) == 0 {
			continue
		}

		var textParts []string
		for _, part := range parts {
			if part.Type == "text" && part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}

		if len(textParts) > 0 {
			raw := strings.Join(textParts, "\n")
			return stripSystemInjections(raw)
		}
	}

	return ""
}

// systemInjectionRe matches system-injected XML tags from Claude Code clients.
var systemInjectionRe = regexp.MustCompile(`(?s)<(?:system-reminder|command-name|command-message|command-args|local-command-caveat|local-command-stdout)>.*?</(?:system-reminder|command-name|command-message|command-args|local-command-caveat|local-command-stdout)>`)

// stripSystemInjections removes system-injected content from user messages
// so that routing decisions are based on actual user intent only.
func stripSystemInjections(text string) string {
	cleaned := systemInjectionRe.ReplaceAllString(text, "")
	return strings.TrimSpace(cleaned)
}

// parseModelRole converts a string to ModelRole with fallback to default.
func parseModelRole(s string) models.ModelRole {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "simple":
		return models.ModelRoleSimple
	case "complex":
		return models.ModelRoleComplex
	case "default":
		return models.ModelRoleDefault
	default:
		return models.ModelRoleDefault
	}
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

type routingConfigProvider interface {
	GetConfig(ctx context.Context) (*models.RoutingConfig, error)
}

type routingModelProvider interface {
	GetModelWithProvider(ctx context.Context, id int64) (*models.RoutingModelWithProvider, error)
}

type routingEmbeddingCache interface {
	GetExactMatch(ctx context.Context, contentHash string, ttlSeconds int) (*repository.EmbeddingCacheEntry, error)
	UpdateHitCountByHash(ctx context.Context, contentHash string) error
	SaveCache(ctx context.Context, contentHash, contentPreview string, embedding []float64, taskType, reason string) error
}

// LLMRouterDeps defines dependencies for creating an LLMRouter.
type LLMRouterDeps struct {
	ConfigRepo    routingConfigProvider
	ModelRepo     routingModelProvider
	EmbeddingRepo routingEmbeddingCache
	RoutingCache  *RoutingCache
	EmbeddingSvc  *EmbeddingService
	RuleRepo      repository.RoutingRuleRepository
	Logger        *zap.Logger
	HTTPClient    *http.Client
}

// LLMRouter performs intelligent routing by calling an LLM to infer task type.
type LLMRouter struct {
	configRepo    routingConfigProvider
	modelRepo     routingModelProvider
	decisionCache routingDecisionCache
	routingCache  *RoutingCache // retained for compatibility with existing tests
	embeddingSvc  *EmbeddingService
	ruleRepo      repository.RoutingRuleRepository
	logger        *zap.Logger
	client        *http.Client
}

// NewLLMRouter creates a new LLMRouter.
func NewLLMRouter(
	db *sql.DB,
	embeddingSvc *EmbeddingService,
	logger *zap.Logger,
) *LLMRouter {
	return NewLLMRouterWithDeps(LLMRouterDeps{
		ConfigRepo:    repository.NewRoutingConfigRepository(db, logger),
		ModelRepo:     repository.NewRoutingModelRepository(db, logger),
		EmbeddingRepo: repository.NewEmbeddingCacheRepository(db, logger),
		RoutingCache:  NewRoutingCache(DefaultRoutingCacheSize, logger),
		EmbeddingSvc:  embeddingSvc,
		RuleRepo:      repository.NewRoutingRuleRepository(db, logger),
		Logger:        logger,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	})
}

// NewLLMRouterWithDeps creates a new LLMRouter with injected dependencies.
func NewLLMRouterWithDeps(deps LLMRouterDeps) *LLMRouter {
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	cache := deps.RoutingCache
	if cache == nil {
		cache = NewRoutingCache(DefaultRoutingCacheSize, logger)
	}
	client := deps.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return &LLMRouter{
		configRepo:    deps.ConfigRepo,
		modelRepo:     deps.ModelRepo,
		decisionCache: newHybridRoutingDecisionCache(cache, deps.EmbeddingRepo, logger),
		routingCache:  cache,
		embeddingSvc:  deps.EmbeddingSvc,
		ruleRepo:      deps.RuleRepo,
		logger:        logger,
		client:        client,
	}
}

// InferTaskType infers the task type for a request first using rule-based routing,
// then falling back to LLM routing if configured.
// Returns (task_type, decision, error).
// On any failure, returns (ModelRoleDefault, nil, nil) as safe fallback.
func (r *LLMRouter) InferTaskType(ctx context.Context, req *models.AnthropicRequest) (models.ModelRole, *models.RoutingDecision, error) {
	// Step 1: Get routing configuration
	ctx, cfg, err := r.loadRoutingConfig(ctx)
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
	cacheKey := GetCacheKey(systemContent, userMessage)
	if taskType, decision, hit := r.decisionCache.Lookup(ctx, cacheKey, cfg.CacheTTLSeconds); hit {
		return taskType, decision, nil
	}

	// Step 6: Call routing LLM model with retry
	taskType, decision := r.callRoutingWithRetry(ctx, cfg, systemContent, userMessage)

	// Step 7: Save to caches
	r.persistDecisionCaches(ctx, cfg, cacheKey, userMessage, taskType, decision)

	return taskType, decision, nil
}

func (r *LLMRouter) loadRoutingConfig(ctx context.Context) (context.Context, *models.RoutingConfig, error) {
	return GetOrLoadRoutingConfig(ctx, r.configRepo)
}

func (r *LLMRouter) persistDecisionCaches(
	ctx context.Context,
	cfg *models.RoutingConfig,
	cacheKey string,
	userMessage string,
	taskType models.ModelRole,
	decision *models.RoutingDecision,
) {
	if decision == nil || !cfg.CacheEnabled {
		return
	}
	_ = r.decisionCache.Store(ctx, cacheKey, userMessage, taskType, decision.Reason)
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

	// Increment hit count for matched rule async with timeout
	if result.Rule != nil && result.Rule.ID > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultAsyncRepoTimeout)
			defer cancel()
			if err := r.ruleRepo.IncrementHitCount(ctx, result.Rule.ID); err != nil {
				r.logger.Warn("failed to increment rule hit count",
					zap.Int64("rule_id", result.Rule.ID),
					zap.Error(err))
			}
		}()
	}

	taskType := parseModelRole(result.TaskType)
	decision := &models.RoutingDecision{
		TaskType:   taskType,
		Reason:     result.Reason,
		FromCache:  false,
		CacheType:  "rule",
		AllMatches: result.Matches,
	}

	// If no rule matched (fallback reason), delegate to fallback strategy
	if result.Rule == nil {
		return r.handleFallbackStrategy(ctx, cfg, decision)
	}

	// Carry matched rule info in the decision
	decision.MatchedRule = result.Rule

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

// callRoutingModel calls a single routing model via the appropriate API adapter.
func (r *LLMRouter) callRoutingModel(
	ctx context.Context,
	systemContent, userMessage string,
	modelCfg *models.RoutingModelWithProvider,
	routingCfg *models.RoutingConfig,
) (*models.RoutingDecision, error) {
	userPrompt := BuildRoutingPrompt(systemContent, userMessage)

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(routingCfg.TimeoutSeconds)*time.Second)
	defer cancel()

	content, err := CallLLMModel(timeoutCtx, LLMCallParams{
		ModelCfg: modelCfg,
		Messages: []Message{
			{Role: "system", Content: RoutingSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: RequestOptions{
			Model:       modelCfg.ModelName,
			MaxTokens:   routingCfg.MaxTokens,
			Temperature: routingCfg.Temperature,
			Stream:      false,
		},
		Client:     r.client,
		Logger:     r.logger,
		LogContext: "routing",
	})
	if err != nil {
		return nil, err
	}

	return parseRoutingDecision(content)
}

// parseRoutingDecision extracts a RoutingDecision from LLM response text.
func parseRoutingDecision(text string) (*models.RoutingDecision, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in routing response: %s", truncate(text, DefaultContentPreviewMaxChars))
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
// Delegates to ExtractRoutingMessage (defined in message_extractor.go) to ensure
// the routing classifier and the analysis pipeline see identical text.
func extractLastUserMessage(req *models.AnthropicRequest) string {
	return ExtractRoutingMessage(req)
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

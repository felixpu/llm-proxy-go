// Package models defines the domain models for the LLM proxy service.
package models

import "time"

// ModelRole represents the role of a model.
type ModelRole string

const (
	ModelRoleSimple  ModelRole = "simple"
	ModelRoleDefault ModelRole = "default"
	ModelRoleComplex ModelRole = "complex"
)

// LoadBalanceStrategy represents a load balancing strategy.
type LoadBalanceStrategy string

const (
	StrategyRoundRobin        LoadBalanceStrategy = "round_robin"
	StrategyWeighted          LoadBalanceStrategy = "weighted"
	StrategyLeastConnections  LoadBalanceStrategy = "least_connections"
	StrategyConversationHash  LoadBalanceStrategy = "conversation_hash"
)

// EndpointStatus represents the health status of an endpoint.
type EndpointStatus string

const (
	EndpointHealthy   EndpointStatus = "healthy"
	EndpointUnhealthy EndpointStatus = "unhealthy"
	EndpointUnknown   EndpointStatus = "unknown"
)

// UserRole represents a user's role.
type UserRole string

const (
	UserRoleAdmin UserRole = "admin"
	UserRoleUser  UserRole = "user"
)

// Model represents a configured AI model.
type Model struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Role              ModelRole `json:"role"`
	CostPerMtokInput  float64   `json:"cost_per_mtok_input"`
	CostPerMtokOutput float64   `json:"cost_per_mtok_output"`
	BillingMultiplier float64   `json:"billing_multiplier"`
	SupportsThinking  bool      `json:"supports_thinking"`
	Enabled           bool      `json:"enabled"`
	Weight            int       `json:"weight"`
	CreatedAt         time.Time `json:"created_at"`
}

// Provider represents an API provider (e.g., Anthropic, OpenAI).
type Provider struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	BaseURL       string    `json:"base_url"`
	APIKey        string    `json:"-"` // Never serialize API key
	Weight        int       `json:"weight"`
	MaxConcurrent int       `json:"max_concurrent"`
	Enabled       bool      `json:"enabled"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Endpoint represents a resolved endpoint (provider + model).
type Endpoint struct {
	Provider *Provider
	Model    *Model
	Status   EndpointStatus
}

// User represents a system user.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // Never serialize
	Role         UserRole  `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// APIKey represents an API key for authentication.
type APIKey struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	KeyHash    string     `json:"-"`
	KeyFull    string     `json:"key_full,omitempty"`
	KeyPrefix  string     `json:"key_prefix"`
	Name       string     `json:"name"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// RequestLogEntry represents a request log entry for insertion.
type RequestLogEntry struct {
	RequestID    string
	UserID       int64
	APIKeyID     *int64
	ModelName    string
	EndpointName string
	TaskType     string
	InputTokens  int
	OutputTokens int
	LatencyMs    float64
	Cost         float64
	StatusCode   *int
	Success      bool
	Stream       bool

	// Routing decision fields
	MessagePreview  string     // Truncated to 200 chars for display
	RequestContent  string     // Full request content (optional)
	ResponseContent string     // Full response content (optional)
	RoutingMethod   string     // rule/cache_l1/cache_l2/llm/fallback
	RoutingReason   string     // Routing decision reason
	MatchedRuleID   *int64     // Matched rule ID
	MatchedRuleName string     // Matched rule name
	AllMatches      []*RuleHit // All matched rules
	IsInaccurate    bool       // Marked as inaccurate
}

// RequestLog represents a request log record from the database.
type RequestLog struct {
	ID           int64      `json:"id"`
	RequestID    string     `json:"request_id"`
	UserID       int64      `json:"user_id"`
	Username     string     `json:"username"`
	APIKeyID     *int64     `json:"api_key_id,omitempty"`
	ModelName    string     `json:"model_name"`
	EndpointName string     `json:"endpoint_name"`
	TaskType     string     `json:"task_type"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	LatencyMs    float64    `json:"latency_ms"`
	Cost         float64    `json:"cost"`
	StatusCode   *int       `json:"status_code,omitempty"`
	Success      bool       `json:"success"`
	Stream       bool       `json:"stream"`
	CreatedAt    time.Time  `json:"created_at"`

	// Routing decision fields
	MessagePreview  string     `json:"message_preview,omitempty"`
	RequestContent  string     `json:"request_content,omitempty"`
	ResponseContent string     `json:"response_content,omitempty"`
	RoutingMethod   string     `json:"routing_method,omitempty"`
	RoutingReason   string     `json:"routing_reason,omitempty"`
	MatchedRuleID   *int64     `json:"matched_rule_id,omitempty"`
	MatchedRuleName string     `json:"matched_rule_name,omitempty"`
	AllMatches      []*RuleHit `json:"all_matches,omitempty"`
	IsInaccurate    bool       `json:"is_inaccurate"`
}

// RoutingConfig represents the LLM routing configuration (single row, id=1).
type RoutingConfig struct {
	Enabled              bool    `json:"enabled"`
	PrimaryModelID       *int64  `json:"primary_model_id"`
	FallbackModelID      *int64  `json:"fallback_model_id"`
	TimeoutSeconds       int     `json:"timeout_seconds"`
	CacheEnabled         bool    `json:"cache_enabled"`
	CacheTTLSeconds      int     `json:"cache_ttl_seconds"`
	CacheTTLL3Seconds    int     `json:"cache_ttl_l3_seconds"`
	MaxTokens            int     `json:"max_tokens"`
	Temperature          float64 `json:"temperature"`
	RetryCount           int     `json:"retry_count"`
	SemanticCacheEnabled bool    `json:"semantic_cache_enabled"`
	EmbeddingModelID     *int64  `json:"embedding_model_id"`
	SimilarityThreshold  float64 `json:"similarity_threshold"`
	LocalEmbeddingModel  string  `json:"local_embedding_model"`
	ForceSmartRouting    bool    `json:"force_smart_routing"`

	// Rule-based routing fields
	RuleBasedRoutingEnabled bool             `json:"rule_based_routing_enabled"`
	RuleFallbackStrategy    FallbackStrategy `json:"rule_fallback_strategy"`
	RuleFallbackTaskType    string           `json:"rule_fallback_task_type"`
	RuleFallbackModelID     *int64           `json:"rule_fallback_model_id"`

	// Logging fields
	LogFullContent bool `json:"log_full_content"`
}

// DefaultRoutingConfig returns the default routing configuration.
func DefaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		Enabled:              false,
		TimeoutSeconds:       5,
		CacheEnabled:         true,
		CacheTTLSeconds:      300,
		CacheTTLL3Seconds:    604800,
		MaxTokens:            100,
		Temperature:          0.0,
		RetryCount:           2,
		SemanticCacheEnabled: true,
		SimilarityThreshold:  0.82,
		LocalEmbeddingModel:  "paraphrase-multilingual-MiniLM-L12-v2",
		ForceSmartRouting:    false,

		RuleBasedRoutingEnabled: true,
		RuleFallbackStrategy:    FallbackDefault,
		RuleFallbackTaskType:    "default",

		LogFullContent: true,
	}
}

// RoutingModel represents a routing model configuration.
type RoutingModel struct {
	ID                int64     `json:"id"`
	ProviderID        int64     `json:"provider_id"`
	ModelName         string    `json:"model_name"`
	Enabled           bool      `json:"enabled"`
	Priority          int       `json:"priority"`
	CostPerMtokInput  float64   `json:"cost_per_mtok_input"`
	CostPerMtokOutput float64   `json:"cost_per_mtok_output"`
	BillingMultiplier float64   `json:"billing_multiplier"`
	Description       string    `json:"description,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RoutingModelWithProvider includes provider details for API calls.
type RoutingModelWithProvider struct {
	RoutingModel
	BaseURL string `json:"base_url"`
	APIKey  string `json:"-"`
}

// EmbeddingModel represents an embedding model configuration.
type EmbeddingModel struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Dimension         int       `json:"dimension"`
	Description       string    `json:"description,omitempty"`
	FastembedSupported bool     `json:"fastembed_supported"`
	FastembedName     string    `json:"fastembed_name,omitempty"`
	IsBuiltin         bool      `json:"is_builtin"`
	Enabled           bool      `json:"enabled"`
	SortOrder         int       `json:"sort_order"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RoutingDecision represents the result of an LLM routing decision.
type RoutingDecision struct {
	TaskType  ModelRole `json:"task_type"`
	Reason    string    `json:"reason"`
	FromCache bool      `json:"from_cache"`
	CacheType string    `json:"cache_type,omitempty"` // "L1", "L2", "L3", ""
	ModelUsed string    `json:"model_used,omitempty"`
}

// FallbackStrategy defines the behavior when no routing rule matches.
type FallbackStrategy string

const (
	FallbackDefault    FallbackStrategy = "default"    // Use default model
	FallbackLLM        FallbackStrategy = "llm"        // Call LLM to decide
	FallbackUserChoice FallbackStrategy = "user"       // Use user-specified value
)

// RoutingRule represents a routing rule for rule-based classification.
type RoutingRule struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Keywords    []string  `json:"keywords"`
	Pattern     string    `json:"pattern"`
	Condition   string    `json:"condition"`
	TaskType    string    `json:"task_type"`
	Priority    int       `json:"priority"`
	IsBuiltin   bool      `json:"is_builtin"`
	Enabled     bool      `json:"enabled"`
	HitCount    int64     `json:"hit_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RuleMatchResult represents the result of a rule match evaluation.
type RuleMatchResult struct {
	Rule     *RoutingRule `json:"matched_rule"`
	Matches  []*RuleHit  `json:"all_matches"`
	TaskType string      `json:"final_task_type"`
	Reason   string      `json:"match_reason"`
}

// RuleHit represents a single rule hit during evaluation.
type RuleHit struct {
	RuleID   int64  `json:"rule_id"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	TaskType string `json:"task_type"`
	Reason   string `json:"reason"`
}

// FallbackInfo records model fallback information.
type FallbackInfo struct {
	OriginalRole   ModelRole `json:"original_role"`
	OriginalModel  string    `json:"original_model,omitempty"`
	FallbackRole   ModelRole `json:"fallback_role"`
	FallbackModel  string    `json:"fallback_model"`
	FallbackReason string    `json:"fallback_reason"`
	FallbackChain  []string  `json:"fallback_chain,omitempty"`
}

// RuleStats represents routing rule statistics.
type RuleStats struct {
	TotalRequests    int64              `json:"total_requests"`
	RuleHits         map[string]HitStat `json:"rule_hits"`
	UnmatchedSamples []UnmatchedSample  `json:"unmatched_samples"`
}

// HitStat represents hit statistics for a single rule.
type HitStat struct {
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// UnmatchedSample represents a request that didn't match any rule.
type UnmatchedSample struct {
	Message string `json:"message"`
	Count   int64  `json:"count"`
}

// RuleGenerateRequest represents a request to generate rules using AI.
type RuleGenerateRequest struct {
	SampleMessages   []string `json:"sample_messages"`
	ExpectedTaskType string   `json:"expected_task_type"`
	ModelID          *int64   `json:"model_id,omitempty"`
}

// SuggestedRule represents an AI-suggested routing rule.
type SuggestedRule struct {
	Name        string   `json:"name"`
	Keywords    []string `json:"keywords,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Condition   string   `json:"condition,omitempty"`
	TaskType    string   `json:"task_type"`
	Confidence  float64  `json:"confidence"`
	Explanation string   `json:"explanation"`
}

// RuleExport represents the export format for routing rules.
type RuleExport struct {
	Version    string         `json:"version"`
	ExportedAt time.Time     `json:"exported_at"`
	Rules      []RoutingRule  `json:"rules"`
}

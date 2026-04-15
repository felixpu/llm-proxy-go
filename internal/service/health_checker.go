package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// ErrorType classifies the type of error for circuit breaker decisions.
type ErrorType string

const (
	ErrorTypeUnknown   ErrorType = "unknown"
	ErrorTypeTemporary ErrorType = "temporary"
	ErrorTypePermanent ErrorType = "permanent"
	ErrorTypeAuth      ErrorType = "auth"
	ErrorTypeRateLimit ErrorType = "rate_limit"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// RequestResult encapsulates all information about a proxy request result.
type RequestResult struct {
	EndpointName string
	Success      bool
	LatencyMs    float64
	StatusCode   int    // 0 if network error
	ResponseBody []byte // nil if not available
	Err          error  // nil if success
}

// ErrorRecord stores information about a single error.
type ErrorRecord struct {
	Timestamp  time.Time
	ErrorType  ErrorType
	StatusCode int
	Message    string
}

// EndpointState tracks the health and connection state of an endpoint.
// Note: No internal mutex - all access protected by HealthChecker.mu
type EndpointState struct {
	Name               string
	Status             models.EndpointStatus
	CurrentConnections int
	TotalRequests      int
	TotalErrors        int
	LastCheckTime      *time.Time
	LastError          string
	AvgResponseTimeMs  float64
	totalResponseMs    float64 // internal accumulator

	// Circuit breaker fields
	CircuitState         CircuitState
	ConsecutiveFailures  int
	ConsecutivePermanent int
	LastFailureTime      *time.Time
	CircuitOpenedAt      *time.Time
	HalfOpenSuccesses    int
	HalfOpenFailures     int
	RecentErrors         []ErrorRecord // pre-allocated, limited to ErrorWindowSize
}

// NewEndpointState creates a new endpoint state with safe defaults.
func NewEndpointState(name string) *EndpointState {
	return &EndpointState{
		Name:         name,
		Status:       models.EndpointHealthy,
		CircuitState: CircuitClosed,
		RecentErrors: make([]ErrorRecord, 0, 20), // pre-allocate capacity
	}
}

// EndpointStateSnapshot is a copy-safe snapshot of EndpointState (no mutex).
type EndpointStateSnapshot struct {
	Name               string                `json:"name"`
	Status             models.EndpointStatus `json:"status"`
	CircuitState       CircuitState          `json:"circuit_state"`
	CurrentConnections int                   `json:"current_connections"`
	TotalRequests      int                   `json:"total_requests"`
	TotalErrors        int                   `json:"total_errors"`
	LastCheckTime      *time.Time            `json:"last_check_time,omitempty"`
	LastError          string                `json:"last_error,omitempty"`
	AvgResponseTimeMs  float64               `json:"avg_response_time_ms"`
}

// snapshot creates a copy-safe snapshot of the state.
func (s *EndpointState) snapshot() EndpointStateSnapshot {
	return EndpointStateSnapshot{
		Name:               s.Name,
		Status:             s.Status,
		CircuitState:       s.CircuitState,
		CurrentConnections: s.CurrentConnections,
		TotalRequests:      s.TotalRequests,
		TotalErrors:        s.TotalErrors,
		LastCheckTime:      s.LastCheckTime,
		LastError:          s.LastError,
		AvgResponseTimeMs:  s.AvgResponseTimeMs,
	}
}

// HealthChecker periodically checks endpoint health and tracks connection state.
type HealthChecker struct {
	cfg    config.HealthCheckConfig
	client *http.Client
	logger *zap.Logger

	mu        sync.RWMutex
	states    map[string]*EndpointState
	endpoints []*models.Endpoint

	cancel context.CancelFunc
	done   chan struct{}

	checking int32
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(cfg config.HealthCheckConfig, logger *zap.Logger) *HealthChecker {
	if logger == nil {
		logger = zap.NewNop()
	}
	cfg = sanitizeHealthCheckConfig(cfg, logger)

	return &HealthChecker{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		logger: logger,
		states: make(map[string]*EndpointState),
	}
}

func sanitizeHealthCheckConfig(cfg config.HealthCheckConfig, logger *zap.Logger) config.HealthCheckConfig {
	if cfg.IntervalSeconds <= 0 {
		logger.Warn("invalid health check interval, fallback to default",
			zap.Int("configured", cfg.IntervalSeconds),
			zap.Int("default", 60),
		)
		cfg.IntervalSeconds = 60
	}
	if cfg.TimeoutSeconds <= 0 {
		logger.Warn("invalid health check timeout, fallback to default",
			zap.Int("configured", cfg.TimeoutSeconds),
			zap.Int("default", 10),
		)
		cfg.TimeoutSeconds = 10
	}
	if cfg.CircuitBreaker.ConsecutiveFailures <= 0 {
		cfg.CircuitBreaker.ConsecutiveFailures = 5
	}
	if cfg.CircuitBreaker.PermanentErrorThreshold <= 0 {
		cfg.CircuitBreaker.PermanentErrorThreshold = 3
	}
	if cfg.CircuitBreaker.CooldownSeconds <= 0 {
		cfg.CircuitBreaker.CooldownSeconds = 60
	}
	if cfg.CircuitBreaker.HalfOpenMaxRequests <= 0 {
		cfg.CircuitBreaker.HalfOpenMaxRequests = 3
	}
	if !cfg.Enabled && cfg.CircuitBreaker.Enabled {
		logger.Info("health check disabled, circuit breaker disabled automatically")
		cfg.CircuitBreaker.Enabled = false
	}
	return cfg
}

func endpointStateName(ep *models.Endpoint) (string, bool) {
	if ep == nil || ep.Provider == nil || ep.Model == nil {
		return "", false
	}
	if strings.TrimSpace(ep.Provider.Name) == "" || strings.TrimSpace(ep.Model.Name) == "" {
		return "", false
	}
	return fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name), true
}

func probeConcurrencyLimit(total int) int {
	if total <= 0 {
		return 1
	}
	const maxConcurrency = 16
	if total < maxConcurrency {
		return total
	}
	return maxConcurrency
}

// Start begins periodic health checking.
func (hc *HealthChecker) Start(endpoints []*models.Endpoint) {
	// Save endpoints reference for CheckNow().
	hc.mu.Lock()
	hc.endpoints = endpoints
	hc.mu.Unlock()

	if !hc.cfg.Enabled {
		// When health checker is disabled, mark all endpoints as healthy
		// so they are usable by the proxy.
		hc.mu.Lock()
		for _, ep := range endpoints {
			name, ok := endpointStateName(ep)
			if !ok {
				hc.logger.Warn("skip invalid endpoint during health checker initialization")
				continue
			}
			state := NewEndpointState(name)
			state.Status = models.EndpointHealthy
			hc.states[name] = state
		}
		hc.mu.Unlock()
		hc.logger.Info("health checker disabled, all endpoints marked healthy")
		return
	}

	// Initialize states for all endpoints.
	hc.mu.Lock()
	for _, ep := range endpoints {
		name, ok := endpointStateName(ep)
		if !ok {
			hc.logger.Warn("skip invalid endpoint during health checker initialization")
			continue
		}
		state := NewEndpointState(name)
		state.Status = models.EndpointUnknown
		hc.states[name] = state
	}

	// Already started: keep existing loop and only refresh endpoints/states.
	if hc.cancel != nil {
		hc.mu.Unlock()
		hc.logger.Debug("health checker already running; refreshed endpoints")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	hc.cancel = cancel
	hc.done = done
	hc.mu.Unlock()

	go hc.loop(ctx, endpoints, done)
	hc.logger.Info("health checker started",
		zap.Int("endpoints", len(endpoints)),
		zap.Int("interval_seconds", hc.cfg.IntervalSeconds),
	)
}

// Stop halts the health checker.
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	cancel := hc.cancel
	done := hc.done
	hc.cancel = nil
	hc.done = nil
	hc.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}

func (hc *HealthChecker) loop(ctx context.Context, _ []*models.Endpoint, done chan struct{}) {
	defer close(done)

	// Run an initial check immediately.
	hc.mu.RLock()
	eps := hc.endpoints
	hc.mu.RUnlock()
	hc.checkAll(ctx, eps)

	ticker := time.NewTicker(time.Duration(hc.cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.mu.RLock()
			eps := hc.endpoints
			hc.mu.RUnlock()
			hc.checkAll(ctx, eps)
		}
	}
}

func (hc *HealthChecker) checkAll(ctx context.Context, endpoints []*models.Endpoint) {
	if len(endpoints) == 0 {
		return
	}
	if !atomic.CompareAndSwapInt32(&hc.checking, 0, 1) {
		hc.logger.Debug("health check already in progress, skipping")
		return
	}
	defer atomic.StoreInt32(&hc.checking, 0)

	sem := make(chan struct{}, probeConcurrencyLimit(len(endpoints)))
	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(ep *models.Endpoint) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			hc.checkEndpoint(ctx, ep)
		}(ep)
	}
	wg.Wait()
}

func (hc *HealthChecker) checkEndpoint(ctx context.Context, ep *models.Endpoint) {
	name, ok := endpointStateName(ep)
	if !ok {
		hc.logger.Warn("skip invalid endpoint during health probe")
		return
	}
	status, errMsg := probeEndpointStatus(ctx, hc.client, ep)
	hc.updateState(name, status, errMsg)
}

func (hc *HealthChecker) updateState(name string, status models.EndpointStatus, errMsg string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state, ok := hc.states[name]
	if !ok {
		return
	}
	now := time.Now()
	state.Status = status
	state.LastCheckTime = &now
	state.LastError = errMsg
}

// IsHealthy returns whether the named endpoint is healthy.
func (hc *HealthChecker) IsHealthy(name string) bool {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state, ok := hc.states[name]
	if !ok {
		// If state doesn't exist, assume healthy (backward compatibility)
		return true
	}

	// Check if circuit should transition to half-open
	if state.CircuitState == CircuitOpen && shouldTransitionToHalfOpen(state, hc.cfg.CircuitBreaker) {
		state.CircuitState = CircuitHalfOpen
		state.HalfOpenSuccesses = 0
		state.HalfOpenFailures = 0
		hc.logger.Info("Circuit breaker transitioned to half-open",
			zap.String("endpoint", name),
		)
	}

	// Check if request should be allowed based on circuit state
	return shouldAllowRequest(state, hc.cfg.CircuitBreaker)
}

// GetHealthyEndpoints returns endpoints that are currently healthy and allowed by circuit breaker.
// Uses write lock because half-open transitions may mutate state.
func (hc *HealthChecker) GetHealthyEndpoints(endpoints []*models.Endpoint) []*models.Endpoint {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	var result []*models.Endpoint
	for _, ep := range endpoints {
		name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
		state, ok := hc.states[name]
		if !ok {
			// Unknown endpoint — allow through (backward compatibility)
			result = append(result, ep)
			continue
		}

		// Check health status
		if state.Status != models.EndpointHealthy {
			continue
		}

		// Check circuit breaker: transition open→half-open if cooldown passed
		if state.CircuitState == CircuitOpen && shouldTransitionToHalfOpen(state, hc.cfg.CircuitBreaker) {
			state.CircuitState = CircuitHalfOpen
			state.HalfOpenSuccesses = 0
			state.HalfOpenFailures = 0
			hc.logger.Info("Circuit breaker transitioned to half-open",
				zap.String("endpoint", name),
			)
		}

		// Allow request only if circuit breaker permits
		if shouldAllowRequest(state, hc.cfg.CircuitBreaker) {
			result = append(result, ep)
		}
	}
	return result
}

// IncrementConnections increments the active connection count.
func (hc *HealthChecker) IncrementConnections(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	state, ok := hc.states[name]
	if !ok {
		return
	}
	state.CurrentConnections++
}

// DecrementConnections decrements the active connection count.
func (hc *HealthChecker) DecrementConnections(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	state, ok := hc.states[name]
	if !ok {
		return
	}
	if state.CurrentConnections > 0 {
		state.CurrentConnections--
	}
}

// UpdateRequestStats records a completed request's outcome.
//
// Deprecated: Use UpdateRequestStatsV2 which includes circuit breaker logic.
// Retained for backward compatibility in tests. New code should use UpdateRequestStatsV2.
func (hc *HealthChecker) UpdateRequestStats(name string, success bool, latencyMs float64) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state, ok := hc.states[name]
	if !ok {
		return
	}

	state.TotalRequests++
	if !success {
		state.TotalErrors++
	}
	state.totalResponseMs += latencyMs
	if state.TotalRequests > 0 {
		state.AvgResponseTimeMs = state.totalResponseMs / float64(state.TotalRequests)
	}
}

// UpdateRequestStatsV2 records a completed request's outcome with circuit breaker logic.
func (hc *HealthChecker) UpdateRequestStatsV2(result RequestResult) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state, ok := hc.states[result.EndpointName]
	if !ok {
		return
	}

	// Update basic stats
	state.TotalRequests++
	if !result.Success {
		state.TotalErrors++
	}
	state.totalResponseMs += result.LatencyMs
	if state.TotalRequests > 0 {
		state.AvgResponseTimeMs = state.totalResponseMs / float64(state.TotalRequests)
	}

	applyRequestResultToCircuitState(state, result, hc.cfg.CircuitBreaker, hc.logger)
}

// GetState returns a snapshot of the named endpoint's state.
func (hc *HealthChecker) GetState(name string) *EndpointStateSnapshot {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	state, ok := hc.states[name]
	if !ok {
		return nil
	}
	// Return a copy-safe snapshot to avoid data races.
	snapshot := state.snapshot()
	return &snapshot
}

// GetAllStates returns a snapshot of all endpoint states (copy-safe).
func (hc *HealthChecker) GetAllStates() map[string]EndpointStateSnapshot {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	result := make(map[string]EndpointStateSnapshot, len(hc.states))
	for k, v := range hc.states {
		result[k] = v.snapshot()
	}
	return result
}

// UpdateState updates the state of an endpoint (for testing).
func (hc *HealthChecker) UpdateState(name string, status models.EndpointStatus, errMsg string) {
	hc.updateState(name, status, errMsg)
}

// UpdateEndpoints atomically replaces the endpoint list and reconciles state map.
// New endpoints get an initial state; removed endpoints are pruned; existing ones keep stats.
func (hc *HealthChecker) UpdateEndpoints(endpoints []*models.Endpoint) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.endpoints = endpoints

	// Build set of current endpoint names.
	active := make(map[string]struct{}, len(endpoints))
	for _, ep := range endpoints {
		name, ok := endpointStateName(ep)
		if !ok {
			hc.logger.Warn("skip invalid endpoint during endpoint update")
			continue
		}
		active[name] = struct{}{}
		if _, exists := hc.states[name]; !exists {
			// New endpoint — initialize state.
			state := NewEndpointState(name)
			if !hc.cfg.Enabled {
				state.Status = models.EndpointHealthy
			} else {
				state.Status = models.EndpointUnknown
			}
			hc.states[name] = state
		}
	}

	// Remove stale entries.
	for name := range hc.states {
		if _, ok := active[name]; !ok {
			delete(hc.states, name)
		}
	}
}

// CheckNow triggers an immediate health check of all endpoints.
func (hc *HealthChecker) CheckNow() {
	hc.mu.RLock()
	endpoints := hc.endpoints
	hc.mu.RUnlock()
	if endpoints != nil {
		go hc.checkAll(context.Background(), endpoints)
	}
}

// GetConfig returns a copy of the current health checker configuration.
func (hc *HealthChecker) GetConfig() config.HealthCheckConfig {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.cfg
}

// ApplyConfig applies a new health checker configuration at runtime.
// It restarts the checker loop so interval/timeout changes take effect immediately.
func (hc *HealthChecker) ApplyConfig(cfg config.HealthCheckConfig) {
	cfg = sanitizeHealthCheckConfig(cfg, hc.logger)

	// Stop existing loop first to avoid overlap and ensure ticker is recreated.
	hc.Stop()

	hc.mu.Lock()
	hc.cfg = cfg
	if hc.client == nil {
		hc.client = &http.Client{}
	}
	hc.client.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	endpoints := hc.endpoints
	hc.mu.Unlock()

	hc.Start(endpoints)
	hc.logger.Info("health checker config applied",
		zap.Bool("enabled", cfg.Enabled),
		zap.Int("interval_seconds", cfg.IntervalSeconds),
		zap.Int("timeout_seconds", cfg.TimeoutSeconds),
		zap.Bool("circuit_breaker_enabled", cfg.CircuitBreaker.Enabled),
	)
}

// classifyHTTPError classifies an HTTP error based on status code and response body.
func classifyHTTPError(statusCode int, responseBody []byte) ErrorType {
	// Success codes
	if statusCode >= 200 && statusCode < 300 {
		return ErrorTypeUnknown
	}

	// Authentication errors
	if statusCode == 401 || statusCode == 403 {
		return ErrorTypeAuth
	}

	// Rate limiting
	if statusCode == 429 {
		return ErrorTypeRateLimit
	}

	// Permanent errors (client errors that won't be fixed by retry)
	if statusCode == 404 {
		return ErrorTypePermanent
	}

	// 400 and 422 - check response body for specific error patterns
	if statusCode == 400 || statusCode == 422 {
		if containsModelError(responseBody) {
			return ErrorTypePermanent
		}
		// 400 is typically a client error (permanent).
		// Only 422 (validation) is treated as potentially temporary
		// since it may be caused by transient payload issues.
		if statusCode == 400 {
			if containsOverloadedError(responseBody) {
				return ErrorTypeTemporary
			}
			return ErrorTypePermanent
		}
		return ErrorTypeTemporary
	}

	// Temporary errors (server errors, timeouts, etc.)
	if statusCode == 408 || statusCode == 502 || statusCode == 503 || statusCode == 504 || statusCode >= 500 {
		return ErrorTypeTemporary
	}

	return ErrorTypeUnknown
}

// containsModelError checks if the response body contains model-related error messages.
// Parses JSON error structure first, then falls back to keyword matching.
func containsModelError(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	// Try to parse structured JSON error response first
	errorMessage := extractErrorField(body)

	keywords := []string{
		"model not found",
		"model does not exist",
		"invalid model",
		"unsupported model",
		"model is not available",
	}

	for _, keyword := range keywords {
		if strings.Contains(errorMessage, keyword) {
			return true
		}
	}

	return false
}

// containsOverloadedError checks if the response indicates a temporary overload condition.
func containsOverloadedError(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	errorMessage := extractErrorField(body)
	overloadKeywords := []string{
		"overloaded",
		"temporarily unavailable",
		"try again",
		"capacity",
	}

	for _, keyword := range overloadKeywords {
		if strings.Contains(errorMessage, keyword) {
			return true
		}
	}

	return false
}

// extractErrorField attempts to parse a JSON error response and extract
// the error message from standard API error structures. Returns a lowercased
// string for case-insensitive matching. Falls back to full body if parsing fails.
func extractErrorField(body []byte) string {
	var structured struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &structured); err == nil && structured.Error.Message != "" {
		return strings.ToLower(structured.Error.Message)
	}

	// Fallback: try flat error string
	var flat struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.Error != "" {
		return strings.ToLower(flat.Error)
	}

	// Last resort: full body (limited to reduce noise)
	s := string(body)
	if len(s) > 500 {
		s = s[:500]
	}
	return strings.ToLower(s)
}

// classifyError classifies a general error (non-HTTP).
func classifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	errStr := strings.ToLower(err.Error())

	// Temporary network errors
	temporaryKeywords := []string{
		"timeout",
		"connection refused",
		"no such host",
	}

	for _, keyword := range temporaryKeywords {
		if strings.Contains(errStr, keyword) {
			return ErrorTypeTemporary
		}
	}

	return ErrorTypeUnknown
}

// extractErrorMessage extracts a human-readable error message from error and response body.
func extractErrorMessage(err error, responseBody []byte) string {
	if err != nil {
		return err.Error()
	}

	if len(responseBody) > 0 {
		msg := string(responseBody)
		if len(msg) > 500 {
			return msg[:500] + "..."
		}
		return msg
	}

	return "unknown error"
}

// shouldTransitionToOpen checks if the circuit breaker should transition to open state.
// Returns (shouldOpen, reason).
func shouldTransitionToOpen(state *EndpointState, cfg config.CircuitBreakerConfig) (bool, string) {
	if !cfg.Enabled {
		return false, ""
	}

	// Only transition from closed state
	if state.CircuitState != CircuitClosed {
		return false, ""
	}

	// Check consecutive permanent errors
	if state.ConsecutivePermanent >= cfg.PermanentErrorThreshold {
		return true, "consecutive permanent errors"
	}

	// Check consecutive failures
	if state.ConsecutiveFailures >= cfg.ConsecutiveFailures {
		return true, "consecutive failures"
	}

	return false, ""
}

// shouldTransitionToHalfOpen checks if the circuit breaker should transition to half-open state.
func shouldTransitionToHalfOpen(state *EndpointState, cfg config.CircuitBreakerConfig) bool {
	if !cfg.Enabled {
		return false
	}

	// Only transition from open state
	if state.CircuitState != CircuitOpen {
		return false
	}

	// Check if cooldown period has passed
	if state.CircuitOpenedAt == nil {
		return false
	}

	cooldownPeriod := time.Duration(cfg.CooldownSeconds) * time.Second
	return time.Since(*state.CircuitOpenedAt) >= cooldownPeriod
}

// shouldAllowRequest checks if a request should be allowed based on circuit state.
func shouldAllowRequest(state *EndpointState, cfg config.CircuitBreakerConfig) bool {
	if !cfg.Enabled {
		return true
	}

	switch state.CircuitState {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return false
	case CircuitHalfOpen:
		// Allow up to configured max test requests in half-open state
		totalAttempts := state.HalfOpenSuccesses + state.HalfOpenFailures
		return totalAttempts < cfg.HalfOpenMaxRequests
	default:
		return false
	}
}

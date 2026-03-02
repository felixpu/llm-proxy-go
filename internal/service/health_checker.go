package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
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
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(cfg config.HealthCheckConfig, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		logger: logger,
		states: make(map[string]*EndpointState),
		done:   make(chan struct{}),
	}
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
			name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
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
		name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
		state := NewEndpointState(name)
		state.Status = models.EndpointUnknown
		hc.states[name] = state
	}
	hc.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	hc.cancel = cancel

	go hc.loop(ctx, endpoints)
	hc.logger.Info("health checker started",
		zap.Int("endpoints", len(endpoints)),
		zap.Int("interval_seconds", hc.cfg.IntervalSeconds),
	)
}

// Stop halts the health checker.
func (hc *HealthChecker) Stop() {
	if hc.cancel != nil {
		hc.cancel()
		<-hc.done
	}
}

func (hc *HealthChecker) loop(ctx context.Context, _ []*models.Endpoint) {
	defer close(hc.done)

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
	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(ep *models.Endpoint) {
			defer wg.Done()
			hc.checkEndpoint(ctx, ep)
		}(ep)
	}
	wg.Wait()
}

func (hc *HealthChecker) checkEndpoint(ctx context.Context, ep *models.Endpoint) {
	name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.Provider.BaseURL, nil)
	if err != nil {
		hc.updateState(name, models.EndpointUnhealthy, err.Error())
		return
	}
	req.Header.Set("x-api-key", ep.Provider.APIKey)

	resp, err := hc.client.Do(req)
	if err != nil {
		hc.updateState(name, models.EndpointUnhealthy, err.Error())
		return
	}
	defer resp.Body.Close()

	// 401 = invalid key, 403 = quota/permission, <400 = healthy, >=400 = unhealthy
	var status models.EndpointStatus
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		status = models.EndpointUnhealthy
	case resp.StatusCode == http.StatusForbidden:
		status = models.EndpointUnhealthy
	case resp.StatusCode < 400:
		status = models.EndpointHealthy
	default:
		status = models.EndpointUnhealthy
	}
	hc.updateState(name, status, "")
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

// GetHealthyEndpoints returns endpoints that are currently healthy.
func (hc *HealthChecker) GetHealthyEndpoints(endpoints []*models.Endpoint) []*models.Endpoint {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	var result []*models.Endpoint
	for _, ep := range endpoints {
		name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
		state, ok := hc.states[name]
		if ok && state.Status == models.EndpointHealthy {
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

	// Handle circuit breaker logic based on current state
	switch state.CircuitState {
	case CircuitClosed:
		if result.Success {
			// Reset failure counters on success
			state.ConsecutiveFailures = 0
			state.ConsecutivePermanent = 0
		} else {
			// Increment failure counters
			state.ConsecutiveFailures++
			now := time.Now()
			state.LastFailureTime = &now

			// Classify error type
			var errorType ErrorType
			if result.Err != nil {
				errorType = classifyError(result.Err)
			} else {
				errorType = classifyHTTPError(result.StatusCode, result.ResponseBody)
			}

			if errorType == ErrorTypePermanent {
				state.ConsecutivePermanent++
			}

			// Record error
			errorRecord := ErrorRecord{
				Timestamp:  now,
				ErrorType:  errorType,
				StatusCode: result.StatusCode,
				Message:    extractErrorMessage(result.Err, result.ResponseBody),
			}
			state.RecentErrors = append(state.RecentErrors, errorRecord)
			if len(state.RecentErrors) > 20 {
				// Keep only last 20 errors
				state.RecentErrors = state.RecentErrors[len(state.RecentErrors)-20:]
			}

			// Check if should transition to open
			if shouldOpen, reason := shouldTransitionToOpen(state, hc.cfg.CircuitBreaker); shouldOpen {
				state.CircuitState = CircuitOpen
				now := time.Now()
				state.CircuitOpenedAt = &now
				state.LastError = reason
				state.Status = models.EndpointUnhealthy
				hc.logger.Warn("Circuit breaker opened",
					zap.String("endpoint", result.EndpointName),
					zap.String("reason", reason),
					zap.Int("consecutive_failures", state.ConsecutiveFailures),
					zap.Int("consecutive_permanent", state.ConsecutivePermanent),
				)
			}
		}

	case CircuitHalfOpen:
		if result.Success {
			state.HalfOpenSuccesses++
			state.ConsecutiveFailures = 0
			state.ConsecutivePermanent = 0

			// If we have enough successful test requests, close the circuit
			if state.HalfOpenSuccesses >= hc.cfg.CircuitBreaker.HalfOpenMaxRequests {
				state.CircuitState = CircuitClosed
				state.HalfOpenSuccesses = 0
				state.HalfOpenFailures = 0
				state.Status = models.EndpointHealthy
				hc.logger.Info("Circuit breaker closed after successful recovery",
					zap.String("endpoint", result.EndpointName),
				)
			}
		} else {
			state.HalfOpenFailures++

			// If any test request fails, reopen the circuit
			state.CircuitState = CircuitOpen
			now := time.Now()
			state.CircuitOpenedAt = &now
			state.HalfOpenSuccesses = 0
			state.HalfOpenFailures = 0
			state.Status = models.EndpointUnhealthy
			hc.logger.Warn("Circuit breaker reopened after failed test request",
				zap.String("endpoint", result.EndpointName),
			)
		}

	case CircuitOpen:
		// In open state, we don't process requests (they should be blocked)
		// But if somehow a request comes through, just log it
		hc.logger.Debug("Request recorded in open circuit state",
			zap.String("endpoint", result.EndpointName),
			zap.Bool("success", result.Success),
		)
	}
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
		name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
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

	// 400 and 422 - check if it's a model error
	if statusCode == 400 || statusCode == 422 {
		if containsModelError(responseBody) {
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
func containsModelError(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	keywords := []string{
		"model not found",
		"model does not exist",
		"invalid model",
		"unsupported model",
		"model is not available",
	}

	for _, keyword := range keywords {
		if strings.Contains(bodyLower, keyword) {
			return true
		}
	}

	return false
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

package service

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// EndpointState tracks the health and connection state of an endpoint.
type EndpointState struct {
	Name              string
	Status            models.EndpointStatus
	CurrentConnections int
	TotalRequests     int
	TotalErrors       int
	LastCheckTime     *time.Time
	LastError         string
	AvgResponseTimeMs float64

	mu              sync.Mutex
	totalResponseMs float64
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
			hc.states[name] = &EndpointState{
				Name:   name,
				Status: models.EndpointHealthy,
			}
		}
		hc.mu.Unlock()
		hc.logger.Info("health checker disabled, all endpoints marked healthy")
		return
	}

	// Initialize states for all endpoints.
	hc.mu.Lock()
	for _, ep := range endpoints {
		name := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
		hc.states[name] = &EndpointState{
			Name:   name,
			Status: models.EndpointUnknown,
		}
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
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	state, ok := hc.states[name]
	if !ok {
		return false
	}
	return state.Status == models.EndpointHealthy
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
	hc.mu.RLock()
	state, ok := hc.states[name]
	hc.mu.RUnlock()
	if !ok {
		return
	}
	state.mu.Lock()
	state.CurrentConnections++
	state.mu.Unlock()
}

// DecrementConnections decrements the active connection count.
func (hc *HealthChecker) DecrementConnections(name string) {
	hc.mu.RLock()
	state, ok := hc.states[name]
	hc.mu.RUnlock()
	if !ok {
		return
	}
	state.mu.Lock()
	if state.CurrentConnections > 0 {
		state.CurrentConnections--
	}
	state.mu.Unlock()
}

// UpdateRequestStats records a completed request's outcome.
func (hc *HealthChecker) UpdateRequestStats(name string, success bool, latencyMs float64) {
	hc.mu.RLock()
	state, ok := hc.states[name]
	hc.mu.RUnlock()
	if !ok {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()

	state.TotalRequests++
	if !success {
		state.TotalErrors++
	}
	state.totalResponseMs += latencyMs
	if state.TotalRequests > 0 {
		state.AvgResponseTimeMs = state.totalResponseMs / float64(state.TotalRequests)
	}
}

// GetState returns a snapshot of the named endpoint's state.
func (hc *HealthChecker) GetState(name string) *EndpointStateSnapshot {
	hc.mu.RLock()
	state, ok := hc.states[name]
	hc.mu.RUnlock()
	if !ok {
		return nil
	}
	// Return a copy-safe snapshot to avoid data races.
	state.mu.Lock()
	defer state.mu.Unlock()
	snapshot := state.snapshot()
	return &snapshot
}

// GetAllStates returns a snapshot of all endpoint states (copy-safe).
func (hc *HealthChecker) GetAllStates() map[string]EndpointStateSnapshot {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	result := make(map[string]EndpointStateSnapshot, len(hc.states))
	for k, v := range hc.states {
		v.mu.Lock()
		result[k] = v.snapshot()
		v.mu.Unlock()
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
			status := models.EndpointUnknown
			if !hc.cfg.Enabled {
				status = models.EndpointHealthy
			}
			hc.states[name] = &EndpointState{
				Name:   name,
				Status: status,
			}
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

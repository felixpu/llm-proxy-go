package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
)

// Thread-safe random source for load balancing.
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))
var rngMu sync.Mutex

func secureRandIntn(n int) int {
	rngMu.Lock()
	defer rngMu.Unlock()
	return rng.Intn(n)
}

// Balancer selects an endpoint from a list of healthy endpoints.
type Balancer interface {
	Select(endpoints []*models.Endpoint, request *models.AnthropicRequest) *models.Endpoint
}

// LoadBalancer dynamically selects strategy from database and delegates endpoint selection.
type LoadBalancer struct {
	configRepo *repository.SystemConfigRepository

	// Strategy cache to avoid DB query on every request
	mu             sync.RWMutex
	cachedStrategy models.LoadBalanceStrategy
	cacheTime      time.Time
	cacheTTL       time.Duration

	// Stateful balancers (need to persist across strategy changes)
	roundRobin *roundRobinBalancer
}

// NewLoadBalancer creates a LoadBalancer that dynamically reads strategy from database.
func NewLoadBalancer(configRepo *repository.SystemConfigRepository) *LoadBalancer {
	return &LoadBalancer{
		configRepo:     configRepo,
		cacheTTL:       5 * time.Second,
		cachedStrategy: models.StrategyWeighted, // default fallback
		roundRobin:     &roundRobinBalancer{indices: make(map[string]int)},
	}
}

// NewLoadBalancerWithStrategy creates a LoadBalancer with a fixed strategy (for testing).
func NewLoadBalancerWithStrategy(strategy models.LoadBalanceStrategy) *LoadBalancer {
	return &LoadBalancer{
		configRepo:     nil,
		cacheTTL:       0, // no cache refresh needed
		cachedStrategy: strategy,
		cacheTime:      time.Now().Add(24 * time.Hour), // never expire
		roundRobin:     &roundRobinBalancer{indices: make(map[string]int)},
	}
}

// getStrategy returns the current strategy, using cache to reduce DB queries.
func (lb *LoadBalancer) getStrategy() models.LoadBalanceStrategy {
	lb.mu.RLock()
	if time.Since(lb.cacheTime) < lb.cacheTTL {
		strategy := lb.cachedStrategy
		lb.mu.RUnlock()
		return strategy
	}
	lb.mu.RUnlock()

	// Cache expired, fetch from DB
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Since(lb.cacheTime) < lb.cacheTTL {
		return lb.cachedStrategy
	}

	if lb.configRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		cfg, err := lb.configRepo.GetLoadBalanceConfig(ctx)
		if err == nil {
			if strategy, ok := cfg["strategy"].(string); ok && strategy != "" {
				lb.cachedStrategy = models.LoadBalanceStrategy(strategy)
			}
		}
	}
	lb.cacheTime = time.Now()
	return lb.cachedStrategy
}

// Select picks an endpoint using the dynamically configured strategy.
func (lb *LoadBalancer) Select(endpoints []*models.Endpoint, req *models.AnthropicRequest) *models.Endpoint {
	if len(endpoints) == 0 {
		return nil
	}
	if len(endpoints) == 1 {
		return endpoints[0]
	}

	strategy := lb.getStrategy()
	switch strategy {
	case models.StrategyRoundRobin:
		return lb.roundRobin.Select(endpoints, req)
	case models.StrategyLeastConnections:
		return selectLeastConnections(endpoints)
	case models.StrategyConversationHash:
		return selectConversationHash(endpoints, req)
	default:
		return selectWeighted(endpoints)
	}
}

// --- Weighted Random ---

func selectWeighted(endpoints []*models.Endpoint) *models.Endpoint {
	totalWeight := 0
	for _, ep := range endpoints {
		totalWeight += ep.Provider.Weight
	}
	if totalWeight == 0 {
		return endpoints[secureRandIntn(len(endpoints))]
	}

	r := secureRandIntn(totalWeight)
	cumulative := 0
	for _, ep := range endpoints {
		cumulative += ep.Provider.Weight
		if r < cumulative {
			return ep
		}
	}
	return endpoints[len(endpoints)-1]
}

// --- Round Robin ---

type roundRobinBalancer struct {
	mu      sync.Mutex
	indices map[string]int
}

func (b *roundRobinBalancer) Select(endpoints []*models.Endpoint, _ *models.AnthropicRequest) *models.Endpoint {
	key := endpoints[0].Model.Name
	b.mu.Lock()
	idx := b.indices[key]
	b.indices[key] = (idx + 1) % len(endpoints)
	b.mu.Unlock()
	return endpoints[idx%len(endpoints)]
}

// --- Least Connections ---

func selectLeastConnections(endpoints []*models.Endpoint) *models.Endpoint {
	// Placeholder: without live connection counts, fall back to random.
	// In production, integrate with HealthChecker.GetState().
	return endpoints[secureRandIntn(len(endpoints))]
}

// --- Conversation Hash ---

func selectConversationHash(endpoints []*models.Endpoint, req *models.AnthropicRequest) *models.Endpoint {
	if req == nil || len(req.Messages) == 0 {
		return endpoints[secureRandIntn(len(endpoints))]
	}

	first := req.Messages[0]
	content := first.Role + ":"
	for _, part := range first.Content.GetParts() {
		if part.Text != "" {
			text := part.Text
			if len(text) > 200 {
				text = text[:200]
			}
			content += text
			break
		}
	}

	hash := sha256.Sum256([]byte(content))
	hashVal := binary.BigEndian.Uint64(hash[:8])
	idx := hashVal % uint64(len(endpoints))
	return endpoints[idx]
}

// EndpointName returns a display name for an endpoint.
func EndpointName(ep *models.Endpoint) string {
	return fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
}

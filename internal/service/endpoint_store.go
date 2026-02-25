package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// EndpointStore provides thread-safe, centralized endpoint management.
// All consumers read endpoints dynamically via GetEndpoints().
type EndpointStore struct {
	mu            sync.RWMutex
	endpoints     []*models.Endpoint
	modelRepo     *repository.SQLModelRepository
	providerRepo  *repository.SQLProviderRepository
	healthChecker *HealthChecker
	logger        *zap.Logger
}

// NewEndpointStore creates a new EndpointStore.
func NewEndpointStore(
	modelRepo *repository.SQLModelRepository,
	providerRepo *repository.SQLProviderRepository,
	logger *zap.Logger,
) *EndpointStore {
	return &EndpointStore{
		modelRepo:    modelRepo,
		providerRepo: providerRepo,
		logger:       logger,
	}
}

// SetHealthChecker injects the HealthChecker reference (breaks circular init).
func (s *EndpointStore) SetHealthChecker(hc *HealthChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthChecker = hc
}

// Load performs the initial endpoint load from the database.
func (s *EndpointStore) Load(ctx context.Context) error {
	endpoints, err := s.loadFromDB(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.endpoints = endpoints
	s.mu.Unlock()
	s.logger.Info("endpoints loaded", zap.Int("count", len(endpoints)))
	return nil
}

// Reload re-reads endpoints from the database and atomically replaces the slice.
func (s *EndpointStore) Reload(ctx context.Context) error {
	endpoints, err := s.loadFromDB(ctx)
	if err != nil {
		s.logger.Error("failed to reload endpoints", zap.Error(err))
		return err
	}
	s.mu.Lock()
	s.endpoints = endpoints
	s.mu.Unlock()
	s.logger.Info("endpoints reloaded", zap.Int("count", len(endpoints)))
	return nil
}

// ReloadAndNotify reloads endpoints and notifies the HealthChecker.
func (s *EndpointStore) ReloadAndNotify(ctx context.Context) {
	if err := s.Reload(ctx); err != nil {
		return
	}
	s.mu.RLock()
	hc := s.healthChecker
	eps := s.endpoints
	s.mu.RUnlock()
	if hc != nil {
		hc.UpdateEndpoints(eps)
	}
}

// GetEndpoints returns the current endpoint snapshot (zero-copy).
func (s *EndpointStore) GetEndpoints() []*models.Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpoints
}

func (s *EndpointStore) loadFromDB(ctx context.Context) ([]*models.Endpoint, error) {
	enabledModels, err := s.modelRepo.FindAllEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("find enabled models: %w", err)
	}
	var endpoints []*models.Endpoint
	for _, m := range enabledModels {
		providers, err := s.providerRepo.FindByModelID(ctx, m.ID)
		if err != nil {
			s.logger.Warn("failed to load providers for model",
				zap.Int64("model_id", m.ID),
				zap.String("model_name", m.Name),
				zap.Error(err))
			continue
		}
		for _, p := range providers {
			endpoints = append(endpoints, &models.Endpoint{
				Provider: p,
				Model:    m,
				Status:   models.EndpointUnknown,
			})
		}
	}
	return endpoints, nil
}

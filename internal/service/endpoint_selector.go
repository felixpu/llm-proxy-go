package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// EndpointSelectionResult holds the result of endpoint selection.
type EndpointSelectionResult struct {
	Endpoint        *models.Endpoint
	Model           *models.Model
	TaskType        models.ModelRole
	RoutingMethod   string
	FallbackInfo    *models.FallbackInfo
	RoutingDecision *models.RoutingDecision
	RuleMatchResult *ClassifyResult
	ShadowRouting   *ShadowRoutingResult
	shadowResultCh  <-chan *ShadowRoutingResult
}

// ResolveShadowRouting performs a non-blocking poll for async shadow results.
// It never waits; if no async result is ready yet, the current snapshot is returned.
func (r *EndpointSelectionResult) ResolveShadowRouting() *ShadowRoutingResult {
	if r == nil {
		return nil
	}
	if r.ShadowRouting != nil {
		return r.ShadowRouting
	}
	if r.shadowResultCh == nil {
		return nil
	}

	select {
	case shadow, ok := <-r.shadowResultCh:
		if ok && shadow != nil {
			r.ShadowRouting = shadow
		}
		r.shadowResultCh = nil
	default:
	}
	return r.ShadowRouting
}

// EndpointSelector integrates routing decision and endpoint selection.
type EndpointSelector struct {
	modelSelector     *ModelSelector
	healthChecker     *HealthChecker
	loadBalancer      *LoadBalancer
	llmRouter         *LLMRouter
	routingConfigRepo RoutingConfigProvider
	modelAliasRepo    modelAliasResolver
	logger            *zap.Logger

	shadowRandMu sync.Mutex
	shadowRand   *rand.Rand

	shadowLimiterMu sync.Mutex
	shadowWindowSec int64
	shadowCount     int
}

type modelAliasResolver interface {
	FindByAliasName(ctx context.Context, aliasName string) ([]*models.ModelAlias, error)
}

// NewEndpointSelector creates an EndpointSelector.
func NewEndpointSelector(
	ms *ModelSelector,
	hc *HealthChecker,
	lb *LoadBalancer,
	lr *LLMRouter,
	rcr RoutingConfigProvider,
	mar modelAliasResolver,
	logger *zap.Logger,
) *EndpointSelector {
	return &EndpointSelector{
		modelSelector:     ms,
		healthChecker:     hc,
		loadBalancer:      lb,
		llmRouter:         lr,
		routingConfigRepo: rcr,
		modelAliasRepo:    mar,
		logger:            logger,
		shadowRand:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectEndpoint selects an endpoint for the request.
// Priority (aligned with Python route_request):
// 1. ForceSmartRouting=true → smart routing
// 2. req.Model == "auto" → smart routing
// 3. req.Model exists and enabled → use specified model
// 4. req.Model disabled → same-role fallback
// 5. req.Model not found → default role fallback
// 6. No model specified → default role fallback
func (s *EndpointSelector) SelectEndpoint(
	ctx context.Context,
	req *models.AnthropicRequest,
	endpoints []*models.Endpoint,
) (*EndpointSelectionResult, error) {
	// Get routing config
	ctx, cfg, err := GetOrLoadRoutingConfig(ctx, s.routingConfigRepo)
	if err != nil {
		s.logger.Warn("failed to load routing config, using defaults",
			zap.Error(err))
		// Continue with nil cfg, which will use default behavior
	}

	// Read cross-role fallback setting
	crossRoleFallback := false
	if cfg != nil {
		crossRoleFallback = cfg.CrossRoleFallbackEnabled
	}

	// 1. Force smart routing
	if cfg != nil && cfg.ForceSmartRouting {
		s.logger.Debug("force smart routing enabled")
		return s.doSmartRouting(ctx, req, endpoints, crossRoleFallback)
	}

	// 2. User specified "auto"
	if strings.EqualFold(req.Model, "auto") {
		s.logger.Debug("auto model requested, using smart routing")
		return s.doSmartRouting(ctx, req, endpoints, crossRoleFallback)
	}

	// 3. User specified a concrete model
	if req.Model != "" {
		if model := s.findModelByName(req.Model, endpoints); model != nil {
			if model.Enabled && s.modelSelector.HasHealthyEndpoints(model, endpoints) {
				ep := s.selectEndpointForModel(model, endpoints, req)
				if ep != nil {
					result := &EndpointSelectionResult{
						Endpoint:      ep,
						Model:         model,
						TaskType:      model.Role,
						RoutingMethod: models.RoutingMethodDirect,
					}
					s.attachShadowRouting(ctx, cfg, req, endpoints, crossRoleFallback, result)
					return result, nil
				}
			}
			// Disabled model or no healthy endpoints for this model -> fallback
			fallbackModel, fallbackInfo, err := s.modelSelector.FindAvailableModelWithFallback(
				model.Role, model, endpoints, crossRoleFallback)
			if err != nil {
				return nil, fmt.Errorf("no available endpoint for model %s: %w", req.Model, err)
			}
			ep := s.selectEndpointForModel(fallbackModel, endpoints, req)
			if ep == nil {
				return nil, fmt.Errorf("no endpoint selected for fallback model %s", fallbackModel.Name)
			}
			result := &EndpointSelectionResult{
				Endpoint:      ep,
				Model:         fallbackModel,
				TaskType:      fallbackModel.Role,
				RoutingMethod: models.RoutingMethodDirect,
				FallbackInfo:  fallbackInfo,
			}
			s.attachShadowRouting(ctx, cfg, req, endpoints, crossRoleFallback, result)
			return result, nil
		}

		aliasEndpoint, resolveErr := s.resolveAliasEndpoint(ctx, req.Model, endpoints, req)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if aliasEndpoint != nil {
			result := &EndpointSelectionResult{
				Endpoint:      aliasEndpoint,
				Model:         aliasEndpoint.Model,
				TaskType:      aliasEndpoint.Model.Role,
				RoutingMethod: models.RoutingMethodDirect,
			}
			s.attachShadowRouting(ctx, cfg, req, endpoints, crossRoleFallback, result)
			return result, nil
		}

		// 5. Model not found -> return error, require admin to configure the exact model
		s.logger.Error("requested model not configured",
			zap.String("requested_model", req.Model))
		return nil, fmt.Errorf("model %q is not configured, please add it in the admin panel", req.Model)
	}

	// 6. No model specified → default role fallback
	return s.selectWithFallback(models.ModelRoleDefault, nil, endpoints, crossRoleFallback)
}

// doSmartRouting performs smart routing via LLMRouter, then selects an endpoint for the inferred role.
func (s *EndpointSelector) doSmartRouting(
	ctx context.Context,
	req *models.AnthropicRequest,
	endpoints []*models.Endpoint,
	crossRoleFallback bool,
) (*EndpointSelectionResult, error) {
	if s.llmRouter == nil {
		s.logger.Warn("smart routing requested but LLMRouter is nil, falling back to default")
		return s.selectWithFallback(models.ModelRoleDefault, nil, endpoints, crossRoleFallback)
	}

	taskType, decision, err := s.llmRouter.InferTaskType(ctx, req)
	if err != nil {
		s.logger.Warn("smart routing inference failed, falling back to default", zap.Error(err))
		return s.selectWithFallback(models.ModelRoleDefault, nil, endpoints, crossRoleFallback)
	}

	result, selErr := s.selectWithFallback(taskType, nil, endpoints, crossRoleFallback)
	if selErr != nil {
		return nil, selErr
	}
	result.RoutingMethod = deriveRoutingMethod(decision)
	result.RoutingDecision = decision
	return result, nil
}

// selectWithFallback selects an endpoint using model fallback chain.
func (s *EndpointSelector) selectWithFallback(
	role models.ModelRole,
	originalModel *models.Model,
	endpoints []*models.Endpoint,
	crossRoleFallback bool,
) (*EndpointSelectionResult, error) {
	model, fallbackInfo, err := s.modelSelector.FindAvailableModelWithFallback(role, originalModel, endpoints, crossRoleFallback)
	if err != nil {
		return nil, err
	}
	ep := s.selectEndpointForModel(model, endpoints, nil)
	if ep == nil {
		return nil, fmt.Errorf("no endpoint selected for model %s", model.Name)
	}
	return &EndpointSelectionResult{
		Endpoint:      ep,
		Model:         model,
		TaskType:      model.Role,
		RoutingMethod: models.RoutingMethodFallback,
		FallbackInfo:  fallbackInfo,
	}, nil
}

func (s *EndpointSelector) attachShadowRouting(
	ctx context.Context,
	cfg *models.RoutingConfig,
	req *models.AnthropicRequest,
	endpoints []*models.Endpoint,
	crossRoleFallback bool,
	result *EndpointSelectionResult,
) {
	if result == nil || req == nil || cfg == nil || !cfg.ShadowRoutingEnabled || s.llmRouter == nil {
		return
	}
	if !s.allowShadowRouting(cfg) {
		return
	}

	shadowCh := make(chan *ShadowRoutingResult, 1)
	result.shadowResultCh = shadowCh

	go s.runShadowRouting(ctx, cfg, req, endpoints, crossRoleFallback, shadowCh)
}

func (s *EndpointSelector) runShadowRouting(
	ctx context.Context,
	cfg *models.RoutingConfig,
	req *models.AnthropicRequest,
	endpoints []*models.Endpoint,
	crossRoleFallback bool,
	shadowCh chan<- *ShadowRoutingResult,
) {
	defer close(shadowCh)

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > DefaultShadowRoutingTimeout {
		timeout = DefaultShadowRoutingTimeout
	}

	shadowCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shadow, err := s.doSmartRouting(shadowCtx, req, endpoints, crossRoleFallback)
	if err != nil || shadow == nil {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			s.logger.Debug("shadow routing skipped after smart routing error", zap.Error(err))
		}
		return
	}

	result := &ShadowRoutingResult{
		TaskType:      shadow.TaskType,
		RoutingMethod: shadow.RoutingMethod,
		Model:         shadow.Model,
		Decision:      shadow.RoutingDecision,
	}

	select {
	case shadowCh <- result:
	default:
	}
}

func (s *EndpointSelector) allowShadowRouting(cfg *models.RoutingConfig) bool {
	if cfg == nil || !cfg.ShadowRoutingEnabled {
		return false
	}
	if cfg.ShadowSampleRate <= 0 {
		return false
	}
	s.shadowRandMu.Lock()
	sampled := s.shadowRand.Float64() < cfg.ShadowSampleRate
	s.shadowRandMu.Unlock()
	if cfg.ShadowSampleRate < 1 && !sampled {
		return false
	}

	if cfg.ShadowMaxQPS <= 0 {
		return false
	}

	nowSec := time.Now().Unix()
	s.shadowLimiterMu.Lock()
	defer s.shadowLimiterMu.Unlock()

	if s.shadowWindowSec != nowSec {
		s.shadowWindowSec = nowSec
		s.shadowCount = 0
	}
	if s.shadowCount >= cfg.ShadowMaxQPS {
		return false
	}
	s.shadowCount++
	return true
}

// selectEndpointForModel selects a healthy endpoint for the given model using load balancer.
func (s *EndpointSelector) selectEndpointForModel(
	model *models.Model,
	endpoints []*models.Endpoint,
	req *models.AnthropicRequest,
) *models.Endpoint {
	candidates := s.getEndpointsForModel(model, endpoints)
	if len(candidates) == 0 {
		return nil
	}
	return s.loadBalancer.Select(candidates, req)
}

func (s *EndpointSelector) resolveAliasEndpoint(
	ctx context.Context,
	requestedName string,
	endpoints []*models.Endpoint,
	req *models.AnthropicRequest,
) (*models.Endpoint, error) {
	if s.modelAliasRepo == nil {
		return nil, nil
	}

	aliases, err := s.modelAliasRepo.FindByAliasName(ctx, requestedName)
	if err != nil {
		return nil, fmt.Errorf("resolve model alias for %q: %w", requestedName, err)
	}
	if len(aliases) == 0 {
		return nil, nil
	}

	candidateEndpoints := make([]*models.Endpoint, 0, len(aliases))
	endpointSeen := make(map[string]bool)
	for _, alias := range aliases {
		for _, ep := range endpoints {
			if ep.Model == nil || ep.Provider == nil {
				continue
			}
			if ep.Model.ID != alias.TargetModelID {
				continue
			}
			if alias.ProviderID != nil && ep.Provider.ID != *alias.ProviderID {
				continue
			}
			if !ep.Model.Enabled || !s.healthChecker.IsHealthy(EndpointName(ep)) {
				continue
			}
			key := fmt.Sprintf("%d/%d", ep.Provider.ID, ep.Model.ID)
			if endpointSeen[key] {
				continue
			}
			endpointSeen[key] = true
			candidateEndpoints = append(candidateEndpoints, ep)
		}
	}
	if len(candidateEndpoints) == 0 {
		return nil, fmt.Errorf("model alias %q has no healthy mapped endpoints", requestedName)
	}

	seenModel := make(map[int64]bool, len(candidateEndpoints))
	candidateModels := make([]*models.Model, 0, len(candidateEndpoints))
	for _, ep := range candidateEndpoints {
		if !seenModel[ep.Model.ID] {
			seenModel[ep.Model.ID] = true
			candidateModels = append(candidateModels, ep.Model)
		}
	}
	selectedModel := s.modelSelector.SelectModelByWeight(candidateModels)
	if selectedModel == nil {
		return nil, fmt.Errorf("model alias %q has no selectable target model", requestedName)
	}

	modelEndpoints := make([]*models.Endpoint, 0, len(candidateEndpoints))
	for _, ep := range candidateEndpoints {
		if ep.Model.ID == selectedModel.ID {
			modelEndpoints = append(modelEndpoints, ep)
		}
	}
	selectedEndpoint := s.loadBalancer.Select(modelEndpoints, req)
	if selectedEndpoint == nil {
		return nil, fmt.Errorf("model alias %q selected target model %q but no endpoint available", requestedName, selectedModel.Name)
	}

	s.logger.Debug("resolved model alias",
		zap.String("requested_model", requestedName),
		zap.String("resolved_model", selectedModel.Name),
		zap.String("resolved_provider", selectedEndpoint.Provider.Name))
	return selectedEndpoint, nil
}

// findModelByName finds a model by exact name (case-insensitive) from the endpoint list.
// Returns nil if no exact match is found. Administrators must configure the exact model name.
func (s *EndpointSelector) findModelByName(name string, endpoints []*models.Endpoint) *models.Model {
	for _, ep := range endpoints {
		if strings.EqualFold(ep.Model.Name, name) {
			return ep.Model
		}
	}
	return nil
}

func (s *EndpointSelector) findModelByID(id int64, endpoints []*models.Endpoint) *models.Model {
	for _, ep := range endpoints {
		if ep.Model.ID == id {
			return ep.Model
		}
	}
	return nil
}

// getEndpointsForModel returns healthy endpoints for the specified model.
func (s *EndpointSelector) getEndpointsForModel(model *models.Model, endpoints []*models.Endpoint) []*models.Endpoint {
	var result []*models.Endpoint
	for _, ep := range endpoints {
		if ep.Model.ID == model.ID && s.healthChecker.IsHealthy(EndpointName(ep)) {
			result = append(result, ep)
		}
	}
	return result
}

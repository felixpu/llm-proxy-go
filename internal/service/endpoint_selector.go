package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// EndpointSelectionResult holds the result of endpoint selection.
type EndpointSelectionResult struct {
	Endpoint        *models.Endpoint
	Model           *models.Model
	TaskType        models.ModelRole
	FallbackInfo    *models.FallbackInfo
	RoutingDecision *models.RoutingDecision
	RuleMatchResult *ClassifyResult
}

// EndpointSelector integrates routing decision and endpoint selection.
type EndpointSelector struct {
	modelSelector     *ModelSelector
	healthChecker     *HealthChecker
	loadBalancer      *LoadBalancer
	llmRouter         *LLMRouter
	routingConfigRepo *repository.RoutingConfigRepository
	logger            *zap.Logger
}

// NewEndpointSelector creates an EndpointSelector.
func NewEndpointSelector(
	ms *ModelSelector,
	hc *HealthChecker,
	lb *LoadBalancer,
	lr *LLMRouter,
	rcr *repository.RoutingConfigRepository,
	logger *zap.Logger,
) *EndpointSelector {
	return &EndpointSelector{
		modelSelector:     ms,
		healthChecker:     hc,
		loadBalancer:      lb,
		llmRouter:         lr,
		routingConfigRepo: rcr,
		logger:            logger,
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
	cfg, _ := s.routingConfigRepo.GetConfig(ctx)

	// 1. Force smart routing
	if cfg != nil && cfg.ForceSmartRouting {
		s.logger.Debug("force smart routing enabled")
		return s.doSmartRouting(ctx, req, endpoints)
	}

	// 2. User specified "auto"
	if strings.EqualFold(req.Model, "auto") {
		s.logger.Debug("auto model requested, using smart routing")
		return s.doSmartRouting(ctx, req, endpoints)
	}

	// 3. User specified a concrete model
	if req.Model != "" {
		model := s.findModelByName(req.Model, endpoints)
		if model != nil && model.Enabled {
			if s.modelSelector.HasHealthyEndpoints(model, endpoints) {
				ep := s.selectEndpointForModel(model, endpoints, req)
				if ep != nil {
					return &EndpointSelectionResult{
						Endpoint: ep,
						Model:    model,
						TaskType: model.Role,
					}, nil
				}
			}
			// No healthy endpoints for this model → fallback within same role
			fallbackModel, fallbackInfo, err := s.modelSelector.FindAvailableModelWithFallback(
				model.Role, model, endpoints)
			if err != nil {
				return nil, fmt.Errorf("no available endpoint for model %s: %w", req.Model, err)
			}
			ep := s.selectEndpointForModel(fallbackModel, endpoints, req)
			if ep == nil {
				return nil, fmt.Errorf("no endpoint selected for fallback model %s", fallbackModel.Name)
			}
			return &EndpointSelectionResult{
				Endpoint:     ep,
				Model:        fallbackModel,
				TaskType:     fallbackModel.Role,
				FallbackInfo: fallbackInfo,
			}, nil
		}

		// 4/5. Model disabled or not found → return error, require admin to configure the exact model
		s.logger.Error("requested model not configured",
			zap.String("requested_model", req.Model))
		return nil, fmt.Errorf("model %q is not configured, please add it in the admin panel", req.Model)
	}

	// 6. No model specified → default role fallback
	return s.selectWithFallback(models.ModelRoleDefault, nil, endpoints)
}

// doSmartRouting performs smart routing via LLMRouter, then selects an endpoint for the inferred role.
func (s *EndpointSelector) doSmartRouting(
	ctx context.Context,
	req *models.AnthropicRequest,
	endpoints []*models.Endpoint,
) (*EndpointSelectionResult, error) {
	if s.llmRouter == nil {
		s.logger.Warn("smart routing requested but LLMRouter is nil, falling back to default")
		return s.selectWithFallback(models.ModelRoleDefault, nil, endpoints)
	}

	taskType, decision, err := s.llmRouter.InferTaskType(ctx, req)
	if err != nil {
		s.logger.Warn("smart routing inference failed, falling back to default", zap.Error(err))
		return s.selectWithFallback(models.ModelRoleDefault, nil, endpoints)
	}

	// Get rule match result if rule-based routing was used
	var ruleResult *ClassifyResult
	if decision != nil && decision.CacheType == "rule" {
		userMessage := extractLastUserMessage(req)
		if userMessage != "" {
			classifier := NewRoutingClassifier(nil)
			ruleResult = classifier.Classify(userMessage)
		}
	}

	result, selErr := s.selectWithFallback(taskType, nil, endpoints)
	if selErr != nil {
		return nil, selErr
	}
	result.RoutingDecision = decision
	result.RuleMatchResult = ruleResult
	return result, nil
}

// selectWithFallback selects an endpoint using model fallback chain.
func (s *EndpointSelector) selectWithFallback(
	role models.ModelRole,
	originalModel *models.Model,
	endpoints []*models.Endpoint,
) (*EndpointSelectionResult, error) {
	model, fallbackInfo, err := s.modelSelector.FindAvailableModelWithFallback(role, originalModel, endpoints)
	if err != nil {
		return nil, err
	}
	ep := s.selectEndpointForModel(model, endpoints, nil)
	if ep == nil {
		return nil, fmt.Errorf("no endpoint selected for model %s", model.Name)
	}
	return &EndpointSelectionResult{
		Endpoint:     ep,
		Model:        model,
		TaskType:     model.Role,
		FallbackInfo: fallbackInfo,
	}, nil
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

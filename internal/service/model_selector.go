package service

import (
	"fmt"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// FallbackPriority defines role fallback order (aligned with Python FALLBACK_PRIORITY).
var FallbackPriority = map[models.ModelRole][]models.ModelRole{
	models.ModelRoleSimple:  {models.ModelRoleSimple, models.ModelRoleDefault, models.ModelRoleComplex},
	models.ModelRoleDefault: {models.ModelRoleDefault, models.ModelRoleComplex},
	models.ModelRoleComplex: {models.ModelRoleComplex, models.ModelRoleDefault},
}

// ModelSelector handles model selection with weight-based picking and cross-role fallback.
type ModelSelector struct {
	healthChecker *HealthChecker
	logger        *zap.Logger
}

// NewModelSelector creates a ModelSelector.
func NewModelSelector(hc *HealthChecker, logger *zap.Logger) *ModelSelector {
	return &ModelSelector{
		healthChecker: hc,
		logger:        logger,
	}
}

// SelectModelByWeight picks the model with the highest weight (deterministic selection).
// When weights are equal, returns the first model (stable ordering).
func (s *ModelSelector) SelectModelByWeight(modelList []*models.Model) *models.Model {
	if len(modelList) == 0 {
		return nil
	}
	if len(modelList) == 1 {
		return modelList[0]
	}

	// Select the model with the highest weight (deterministic)
	// When weights are equal, select the first one (stable sort semantics)
	var best *models.Model
	for _, m := range modelList {
		if m.Weight <= 0 {
			continue
		}
		if best == nil || m.Weight > best.Weight {
			best = m
		}
	}

	// If all models have weight 0, return the first one (deterministic fallback)
	if best == nil {
		return modelList[0]
	}
	return best
}

// GetModelsForRole extracts unique models with the specified role from endpoints.
func (s *ModelSelector) GetModelsForRole(role models.ModelRole, endpoints []*models.Endpoint) []*models.Model {
	seen := make(map[int64]bool)
	var result []*models.Model
	for _, ep := range endpoints {
		if ep.Model.Role == role && ep.Model.Enabled && !seen[ep.Model.ID] {
			seen[ep.Model.ID] = true
			result = append(result, ep.Model)
		}
	}
	return result
}

// GetHealthyModelsForRole returns models of the specified role that have healthy endpoints.
func (s *ModelSelector) GetHealthyModelsForRole(role models.ModelRole, endpoints []*models.Endpoint) []*models.Model {
	modelList := s.GetModelsForRole(role, endpoints)
	var healthy []*models.Model
	for _, m := range modelList {
		if s.HasHealthyEndpoints(m, endpoints) {
			healthy = append(healthy, m)
		}
	}
	return healthy
}

// HasHealthyEndpoints checks if the model has at least one healthy endpoint.
func (s *ModelSelector) HasHealthyEndpoints(model *models.Model, endpoints []*models.Endpoint) bool {
	for _, ep := range endpoints {
		if ep.Model.ID == model.ID {
			epName := EndpointName(ep)
			if s.healthChecker.IsHealthy(epName) {
				return true
			}
		}
	}
	return false
}

// FindAvailableModelWithFallback finds an available model with cross-role fallback.
// Returns (model, fallbackInfo, error).
func (s *ModelSelector) FindAvailableModelWithFallback(
	originalRole models.ModelRole,
	originalModel *models.Model,
	endpoints []*models.Endpoint,
) (*models.Model, *models.FallbackInfo, error) {
	fallbackChain := FallbackPriority[originalRole]
	if len(fallbackChain) == 0 {
		fallbackChain = []models.ModelRole{originalRole, models.ModelRoleDefault}
	}

	var triedRoles []string
	for _, role := range fallbackChain {
		triedRoles = append(triedRoles, string(role))
		healthyModels := s.GetHealthyModelsForRole(role, endpoints)
		if len(healthyModels) == 0 {
			continue
		}

		selected := s.SelectModelByWeight(healthyModels)
		if selected == nil {
			continue
		}

		// Build fallback info
		var fallbackInfo *models.FallbackInfo
		if role != originalRole || (originalModel != nil && selected.ID != originalModel.ID) {
			originalModelName := ""
			if originalModel != nil {
				originalModelName = originalModel.Name
			}
			fallbackInfo = &models.FallbackInfo{
				OriginalRole:   originalRole,
				OriginalModel:  originalModelName,
				FallbackRole:   role,
				FallbackModel:  selected.Name,
				FallbackReason: fmt.Sprintf("fallback from %s to %s", originalRole, role),
				FallbackChain:  triedRoles,
			}
		}

		s.logger.Debug("model selected",
			zap.String("original_role", string(originalRole)),
			zap.String("selected_role", string(role)),
			zap.String("selected_model", selected.Name),
			zap.Bool("is_fallback", fallbackInfo != nil))

		return selected, fallbackInfo, nil
	}

	return nil, nil, fmt.Errorf("no available model for role %s (tried: %v)", originalRole, triedRoles)
}

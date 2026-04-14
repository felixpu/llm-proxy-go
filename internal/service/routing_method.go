package service

import (
	"fmt"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

// ShadowRoutingResult captures what smart routing would have selected without
// affecting the actual request path.
type ShadowRoutingResult struct {
	TaskType      models.ModelRole
	RoutingMethod string
	Model         *models.Model
	Decision      *models.RoutingDecision
}

// DeriveRoutingMethod converts a routing decision into an externally visible
// routing method label for admin/debug surfaces.
func DeriveRoutingMethod(decision *models.RoutingDecision) string {
	return deriveRoutingMethod(decision)
}

func deriveRoutingMethod(decision *models.RoutingDecision) string {
	if decision == nil {
		return models.RoutingMethodFallback
	}
	if decision.FromCache {
		switch decision.CacheType {
		case "L1":
			return models.RoutingMethodCacheL1
		case "L2":
			return models.RoutingMethodCacheL2
		case "L3":
			return models.RoutingMethodCacheL3
		default:
			return models.RoutingMethodCacheL1
		}
	}
	switch decision.CacheType {
	case "rule":
		return models.RoutingMethodRule
	default:
		if decision.ModelUsed != "" {
			return models.RoutingMethodLLM
		}
		return models.RoutingMethodFallback
	}
}

func describeRouting(meta *ProxyMetadata) string {
	if meta == nil {
		return ""
	}

	actualReason := strings.TrimSpace(meta.RoutingReason)
	if actualReason == "" {
		switch meta.RoutingMethod {
		case models.RoutingMethodDirect:
			actualReason = fmt.Sprintf("direct: client requested concrete model %q", meta.RequestedModel)
		case models.RoutingMethodFallback:
			actualReason = "fallback: no routing decision returned"
		}
	}

	if meta.ShadowRouting == nil {
		return actualReason
	}

	shadow := meta.ShadowRouting
	shadowReason := ""
	if shadow.Decision != nil {
		shadowReason = strings.TrimSpace(shadow.Decision.Reason)
	}
	if shadowReason == "" {
		shadowReason = "shadow routing produced no explicit reason"
	}

	shadowModel := ""
	if shadow.Model != nil {
		shadowModel = shadow.Model.Name
	}

	shadowSummary := fmt.Sprintf(
		"shadow: method=%s task_type=%s model=%s reason=%s",
		shadow.RoutingMethod,
		shadow.TaskType,
		shadowModel,
		shadowReason,
	)
	if actualReason == "" {
		return shadowSummary
	}
	return actualReason + "; " + shadowSummary
}

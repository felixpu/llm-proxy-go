package handler

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Endpoints map[string]string `json:"endpoints"`
}

// StatusResponse represents the system status response.
type StatusResponse struct {
	UptimeSeconds int64               `json:"uptime_seconds"`
	TotalRequests int64               `json:"total_requests"`
	TotalErrors   int64               `json:"total_errors"`
	Models        []ModelInfo         `json:"models"`
	Endpoints     []EndpointStateInfo `json:"endpoints"`
}

// ModelInfo represents model information in status response.
type ModelInfo struct {
	Name             string    `json:"name"`
	Role             string    `json:"role"`
	EndpointsTotal   int       `json:"endpoints_total"`
	EndpointsHealthy int       `json:"endpoints_healthy"`
	CreatedAt        time.Time `json:"created_at"`
}

// EndpointStateInfo represents endpoint state information.
type EndpointStateInfo struct {
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	TotalRequests     int64   `json:"total_requests"`
	TotalErrors       int64   `json:"total_errors"`
	CurrentConns      int     `json:"current_connections"`
	AvgResponseTimeMs float64 `json:"avg_response_time_ms"`
	LastCheckTime     string  `json:"last_check_time,omitempty"`
}
// RoutingDebugResponse represents routing debug information.
type RoutingDebugResponse struct {
	DefaultRole   string      `json:"default_role"`
	RoutingMethod string      `json:"routing_method"`
	Models        []ModelInfo `json:"models"`
}

// RoutingTestRequest represents a routing test request.
type RoutingTestRequest struct {
	Messages []map[string]interface{} `json:"messages"`
	Model    string                   `json:"model,omitempty"`
}

// RoutingTestResponse represents a routing test response.
type RoutingTestResponse struct {
	InferredTaskType string `json:"inferred_task_type"`
	Reasoning        string `json:"reasoning"`
	CacheHit         bool   `json:"cache_hit"`
	CacheType        string `json:"cache_type,omitempty"`
	SelectedRole     string `json:"selected_role"`
	SelectedModel    string `json:"selected_model"`
	RoutingMethod    string `json:"routing_method"`
}

var startTime = time.Now()

// StatusHandler handles system status API endpoints.
type StatusHandler struct {
	healthChecker *service.HealthChecker
	modelRepo     *repository.SQLModelRepository
	logRepo       repository.RequestLogRepository
	llmRouter     *service.LLMRouter
	endpointStore *service.EndpointStore
}

// NewStatusHandler creates a new StatusHandler.
func NewStatusHandler(
	hc *service.HealthChecker,
	modelRepo *repository.SQLModelRepository,
	logRepo repository.RequestLogRepository,
	llmRouter *service.LLMRouter,
	endpointStore *service.EndpointStore,
) *StatusHandler {
	return &StatusHandler{
		healthChecker: hc,
		modelRepo:     modelRepo,
		logRepo:       logRepo,
		llmRouter:     llmRouter,
		endpointStore: endpointStore,
	}
}
// GetSystemStatus returns detailed system status.
func (h *StatusHandler) GetSystemStatus(c *gin.Context) {
	states := h.healthChecker.GetAllStates()

	// Query historical stats from database
	var dbStats map[string]*repository.EndpointModelStats
	if h.logRepo != nil {
		var err error
		dbStats, err = h.logRepo.GetEndpointModelStats(c.Request.Context())
		if err != nil {
			dbStats = nil // Fall back to memory-only on error
		}
	}

	var totalReqs, totalErrs int64
	epInfos := make([]EndpointStateInfo, 0, len(states))
	for name, s := range states {
		var lastCheck string
		if s.LastCheckTime != nil {
			lastCheck = s.LastCheckTime.Format(time.RFC3339)
		}

		epInfo := EndpointStateInfo{
			Name:          name,
			Status:        string(s.Status),
			CurrentConns:  s.CurrentConnections,
			LastCheckTime: lastCheck,
		}

		// Use DB stats for historical data, memory for real-time
		if db, ok := dbStats[name]; ok {
			epInfo.TotalRequests = db.TotalRequests
			epInfo.TotalErrors = db.TotalErrors
			epInfo.AvgResponseTimeMs = db.AvgLatencyMs
		} else {
			epInfo.TotalRequests = int64(s.TotalRequests)
			epInfo.TotalErrors = int64(s.TotalErrors)
			epInfo.AvgResponseTimeMs = s.AvgResponseTimeMs
		}

		totalReqs += epInfo.TotalRequests
		totalErrs += epInfo.TotalErrors
		epInfos = append(epInfos, epInfo)
	}

	// Build model info from endpoints
	modelMap := make(map[string]*ModelInfo)
	for _, ep := range h.endpointStore.GetEndpoints() {
		name := ep.Model.Name
		mi, ok := modelMap[name]
		if !ok {
			mi = &ModelInfo{Name: name, Role: string(ep.Model.Role), CreatedAt: ep.Model.CreatedAt}
			modelMap[name] = mi
		}
		mi.EndpointsTotal++
		epName := fmt.Sprintf("%s/%s", ep.Provider.Name, ep.Model.Name)
		if s, exists := states[epName]; exists && s.Status == models.EndpointHealthy {
			mi.EndpointsHealthy++
		}
	}
	modelInfos := make([]ModelInfo, 0, len(modelMap))
	for _, mi := range modelMap {
		modelInfos = append(modelInfos, *mi)
	}

	// Sort models by creation time ascending, then by name for stable ordering
	sort.Slice(modelInfos, func(i, j int) bool {
		if modelInfos[i].CreatedAt.Equal(modelInfos[j].CreatedAt) {
			return modelInfos[i].Name < modelInfos[j].Name
		}
		return modelInfos[i].CreatedAt.Before(modelInfos[j].CreatedAt)
	})

	// Sort endpoints by name for stable ordering
	sort.Slice(epInfos, func(i, j int) bool {
		return epInfos[i].Name < epInfos[j].Name
	})

	c.JSON(http.StatusOK, StatusResponse{
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
		TotalRequests: totalReqs,
		TotalErrors:   totalErrs,
		Models:        modelInfos,
		Endpoints:     epInfos,
	})
}

// GetRoutingDebug returns routing configuration and rules.
func (h *StatusHandler) GetRoutingDebug(c *gin.Context) {
	modelList, err := h.modelRepo.FindAllEnabled(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	infos := make([]ModelInfo, 0, len(modelList))
	for _, m := range modelList {
		infos = append(infos, ModelInfo{
			Name:      m.Name,
			Role:      string(m.Role),
			CreatedAt: m.CreatedAt,
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CreatedAt.Before(infos[j].CreatedAt)
	})
	c.JSON(http.StatusOK, RoutingDebugResponse{
		DefaultRole:   string(models.ModelRoleDefault),
		RoutingMethod: "llm",
		Models:        infos,
	})
}

// TestRouting tests which model a request would be routed to.
func (h *StatusHandler) TestRouting(c *gin.Context) {
	var req RoutingTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Build AnthropicRequest from test request
	messages := make([]models.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		messages = append(messages, models.Message{
			Role:    role,
			Content: models.MessageContent{Text: content},
		})
	}
	anthropicReq := &models.AnthropicRequest{
		Model:    req.Model,
		Messages: messages,
	}

	// Call LLM router
	taskType, decision, err := h.llmRouter.InferTaskType(c.Request.Context(), anthropicReq)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp := RoutingTestResponse{
		InferredTaskType: string(taskType),
		SelectedRole:     string(taskType),
		RoutingMethod:    "llm",
	}

	if decision != nil {
		resp.Reasoning = decision.Reason
		resp.CacheHit = decision.FromCache
		resp.CacheType = decision.CacheType
		resp.SelectedModel = decision.ModelUsed
	} else {
		resp.Reasoning = "default routing (no routing decision returned)"
	}

	// Find a matching model name for the inferred role
	if resp.SelectedModel == "" {
		for _, ep := range h.endpointStore.GetEndpoints() {
			if ep.Model.Role == taskType {
				resp.SelectedModel = ep.Model.Name
				break
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// TriggerHealthCheck immediately executes a health check.
func (h *StatusHandler) TriggerHealthCheck(c *gin.Context) {
	h.healthChecker.CheckNow()
	c.JSON(http.StatusOK, gin.H{
		"checked_at": time.Now().Format(time.RFC3339),
		"message":    "Health check triggered",
	})
}

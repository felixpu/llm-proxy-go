package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

// ProxyHandler handles proxy requests.
type ProxyHandler struct {
	proxyService     *service.ProxyService
	authService      *service.AuthService
	endpointSelector *service.EndpointSelector
	configProvider   service.RoutingConfigProvider
	contentPolicy    *ContentLoggingPolicy
	logger           *zap.Logger
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(
	ps *service.ProxyService,
	as *service.AuthService,
	es *service.EndpointSelector,
	configProvider service.RoutingConfigProvider,
	logger *zap.Logger,
) *ProxyHandler {
	return &ProxyHandler{
		proxyService:     ps,
		authService:      as,
		endpointSelector: es,
		configProvider:   configProvider,
		contentPolicy:    NewContentLoggingPolicy(configProvider, logger),
		logger:           logger,
	}
}

// Messages handles POST /v1/messages.
func (h *ProxyHandler) Messages(c *gin.Context) {
	// Extract API key from header.
	apiKey := extractAPIKey(c)
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "authentication_error",
				"message": "Missing API key",
			},
		})
		return
	}

	// Validate API key.
	user, err := h.authService.ValidateAPIKey(c.Request.Context(), apiKey)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "authentication_error",
				"message": err.Error(),
			},
		})
		return
	}

	h.logger.Debug("authenticated user", zap.String("username", user.Username))

	// Parse request body.
	var req models.AnthropicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid request body",
			zap.String("error", err.Error()),
			zap.String("ip", c.ClientIP()))
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Invalid request body: " + err.Error(),
			},
		})
		return
	}

	// Validate request.
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "model is required",
			},
		})
		return
	}

	eps, ok := getEndpointsFromContext(c)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": "No endpoints configured",
			},
		})
		return
	}

	// Preload request-scoped routing config once for selector/router/logging path.
	ctx, _, _ := service.GetOrLoadRoutingConfig(c.Request.Context(), h.configProvider)
	c.Request = c.Request.WithContext(ctx)

	// Check if streaming is requested
	if req.Stream {
		h.handleStreamRequest(c, &req, eps, user)
		return
	}

	// Non-streaming request
	h.handleNonStreamRequest(c, &req, eps, user)
}

func getEndpointsFromContext(c *gin.Context) ([]*models.Endpoint, bool) {
	value, ok := c.Get(middleware.ContextKeyEndpoints)
	if !ok || value == nil {
		return nil, false
	}
	eps, ok := value.([]*models.Endpoint)
	if !ok {
		return nil, false
	}
	return eps, true
}

func unwrapUpstreamError(err error) *service.UpstreamError {
	var upstreamErr *service.UpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr
	}
	return nil
}

// handleNonStreamRequest handles non-streaming proxy requests.
func (h *ProxyHandler) handleNonStreamRequest(c *gin.Context, req *models.AnthropicRequest, eps []*models.Endpoint, user *service.CurrentUser) {
	ctx := c.Request.Context()

	// Use EndpointSelector to select endpoint
	selection, err := h.endpointSelector.SelectEndpoint(ctx, req, eps)
	if err != nil {
		h.logger.Error("endpoint selection failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": err.Error(),
			},
		})
		return
	}

	resp, meta, err := h.proxyService.ProxyRequest(ctx, req, c.Request.Header, c.Request.URL.RawQuery, selection, eps)
	if err != nil {
		if ue := unwrapUpstreamError(err); ue != nil {
			// Save error request log with proper RequestID
			if meta == nil {
				meta = &service.ProxyMetadata{
					RequestID: uuid.New().String(),
				}
			}
			meta.StatusCode = ue.StatusCode
			meta.Success = false
			meta.SelectedModel = selection.Model.Name
			meta.SelectedEndpoint = selection.Endpoint.Provider.Name
			meta.InferredTaskType = string(selection.TaskType)
			meta.RoutingMethod = selection.RoutingMethod
			meta.RoutingDecision = selection.RoutingDecision
			meta.ShadowRouting = selection.ResolveShadowRouting()
			meta.RuleMatchResult = selection.RuleMatchResult
			attachModelTrace(meta, req, selection)
			h.attachContent(ctx, meta, req, nil)
			// Save upstream error response body (always, regardless of LogFullContent)
			meta.ResponseContent = string(ue.Body)
			h.proxyService.SaveRequestLog(ctx, meta, user.UserID, user.APIKeyID)

			c.Data(ue.StatusCode, "application/json", ue.Body)
			return
		}
		h.logger.Error("proxy request failed", zap.Error(err))

		// Save error request log for non-upstream errors
		if meta == nil {
			meta = &service.ProxyMetadata{
				RequestID: uuid.New().String(),
			}
		}
		meta.StatusCode = http.StatusBadGateway
		meta.Success = false
		meta.SelectedModel = selection.Model.Name
		meta.SelectedEndpoint = selection.Endpoint.Provider.Name
		meta.InferredTaskType = string(selection.TaskType)
		meta.RoutingMethod = selection.RoutingMethod
		meta.RoutingDecision = selection.RoutingDecision
		meta.ShadowRouting = selection.ResolveShadowRouting()
		meta.RuleMatchResult = selection.RuleMatchResult
		attachModelTrace(meta, req, selection)
		h.attachContent(ctx, meta, req, nil)
		// Save error message as response content
		meta.ResponseContent = err.Error()
		h.proxyService.SaveRequestLog(ctx, meta, user.UserID, user.APIKeyID)

		c.JSON(http.StatusBadGateway, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": err.Error(),
			},
		})
		return
	}

	// Attach routing decision to metadata
	meta.StatusCode = http.StatusOK
	meta.Success = true
	meta.RoutingMethod = selection.RoutingMethod
	meta.RoutingDecision = selection.RoutingDecision
	meta.ShadowRouting = selection.ResolveShadowRouting()
	meta.RuleMatchResult = selection.RuleMatchResult
	meta.InferredTaskType = string(selection.TaskType)
	attachModelTrace(meta, req, selection)

	// Attach full content if configured
	h.attachContent(ctx, meta, req, resp)

	// Save request log
	h.proxyService.SaveRequestLog(ctx, meta, user.UserID, user.APIKeyID)

	// Set proxy metadata headers.
	setProxyHeaders(c, meta)
	c.JSON(http.StatusOK, resp)
}

// handleStreamRequest handles SSE streaming proxy requests.
func (h *ProxyHandler) handleStreamRequest(c *gin.Context, req *models.AnthropicRequest, eps []*models.Endpoint, user *service.CurrentUser) {
	ctx := c.Request.Context()

	// Use EndpointSelector to select endpoint
	selection, err := h.endpointSelector.SelectEndpoint(ctx, req, eps)
	if err != nil {
		h.logger.Error("endpoint selection failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": err.Error(),
			},
		})
		return
	}

	chunkChan, meta, err := h.proxyService.ProxyStreamRequest(ctx, req, c.Request.Header, c.Request.URL.RawQuery, selection, eps)
	if err != nil {
		if ue := unwrapUpstreamError(err); ue != nil {
			// Save error request log with proper RequestID
			if meta == nil {
				meta = &service.ProxyMetadata{
					RequestID: uuid.New().String(),
				}
			}
			meta.StatusCode = ue.StatusCode
			meta.Success = false
			meta.Stream = true
			meta.SelectedModel = selection.Model.Name
			meta.SelectedEndpoint = selection.Endpoint.Provider.Name
			meta.InferredTaskType = string(selection.TaskType)
			meta.RoutingMethod = selection.RoutingMethod
			meta.RoutingDecision = selection.RoutingDecision
			meta.ShadowRouting = selection.ResolveShadowRouting()
			meta.RuleMatchResult = selection.RuleMatchResult
			attachModelTrace(meta, req, selection)
			h.attachStreamContent(ctx, meta, req)
			// Save upstream error response body (always, regardless of LogFullContent)
			meta.ResponseContent = string(ue.Body)
			h.proxyService.SaveRequestLog(ctx, meta, user.UserID, user.APIKeyID)

			c.Data(ue.StatusCode, "application/json", ue.Body)
			return
		}
		h.logger.Error("proxy stream request failed", zap.Error(err))

		// Save error request log for non-upstream errors
		if meta == nil {
			meta = &service.ProxyMetadata{
				RequestID: uuid.New().String(),
			}
		}
		meta.StatusCode = http.StatusBadGateway
		meta.Success = false
		meta.Stream = true
		meta.SelectedModel = selection.Model.Name
		meta.SelectedEndpoint = selection.Endpoint.Provider.Name
		meta.InferredTaskType = string(selection.TaskType)
		meta.RoutingMethod = selection.RoutingMethod
		meta.RoutingDecision = selection.RoutingDecision
		meta.ShadowRouting = selection.ResolveShadowRouting()
		meta.RuleMatchResult = selection.RuleMatchResult
		attachModelTrace(meta, req, selection)
		h.attachStreamContent(ctx, meta, req)
		// Save error message as response content
		meta.ResponseContent = err.Error()
		h.proxyService.SaveRequestLog(ctx, meta, user.UserID, user.APIKeyID)

		c.JSON(http.StatusBadGateway, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": err.Error(),
			},
		})
		return
	}

	// Attach routing decision to initial metadata (will propagate to final chunk)
	meta.RoutingMethod = selection.RoutingMethod
	meta.RoutingDecision = selection.RoutingDecision
	meta.ShadowRouting = selection.ResolveShadowRouting()
	meta.RuleMatchResult = selection.RuleMatchResult
	meta.FallbackInfo = selection.FallbackInfo
	meta.InferredTaskType = string(selection.TaskType)
	attachModelTrace(meta, req, selection)

	// Attach request content if configured
	h.attachStreamContent(ctx, meta, req)

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Set initial proxy metadata headers
	c.Header("X-Proxy-Request-Id", meta.RequestID)
	c.Header("X-Proxy-Model", url.QueryEscape(meta.SelectedModel))
	c.Header("X-Proxy-Endpoint", url.QueryEscape(meta.SelectedEndpoint))
	c.Header("X-Proxy-Task-Type", meta.InferredTaskType)
	if meta.RoutingMethod != "" {
		c.Header("X-Proxy-Routing-Method", meta.RoutingMethod)
	}
	if meta.ShadowRouting != nil {
		c.Header("X-Proxy-Shadow-Task-Type", string(meta.ShadowRouting.TaskType))
		c.Header("X-Proxy-Shadow-Method", meta.ShadowRouting.RoutingMethod)
		if meta.ShadowRouting.Model != nil {
			c.Header("X-Proxy-Shadow-Model", url.QueryEscape(meta.ShadowRouting.Model.Name))
		}
	}
	c.Header("X-Proxy-Stream", "true")

	// Flush headers immediately
	c.Writer.Flush()

	// Stream chunks to client
	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			h.logger.Debug("client disconnected during stream",
				zap.String("request_id", meta.RequestID))
			return
		case chunk, ok := <-chunkChan:
			if !ok {
				// Channel closed
				return
			}

			if chunk.Err != nil {
				h.logger.Error("stream error",
					zap.String("request_id", meta.RequestID),
					zap.Error(chunk.Err))
				if shadow := selection.ResolveShadowRouting(); shadow != nil {
					meta.ShadowRouting = shadow
				}
				if chunk.Meta != nil {
					chunk.Meta.RoutingMethod = meta.RoutingMethod
					chunk.Meta.RoutingDecision = meta.RoutingDecision
					chunk.Meta.ShadowRouting = meta.ShadowRouting
					chunk.Meta.RuleMatchResult = meta.RuleMatchResult
					chunk.Meta.RequestContent = meta.RequestContent
					attachModelTrace(chunk.Meta, req, selection)
					h.proxyService.SaveRequestLog(c.Request.Context(), chunk.Meta, user.UserID, user.APIKeyID)
				}
				return
			}

			if chunk.Done {
				// Final chunk with metadata
				if shadow := selection.ResolveShadowRouting(); shadow != nil {
					meta.ShadowRouting = shadow
				}
				if chunk.Meta != nil {
					// Propagate routing fields set by handler
					chunk.Meta.RoutingMethod = meta.RoutingMethod
					chunk.Meta.RoutingDecision = meta.RoutingDecision
					chunk.Meta.ShadowRouting = meta.ShadowRouting
					chunk.Meta.RuleMatchResult = meta.RuleMatchResult
					chunk.Meta.RequestContent = meta.RequestContent
					attachModelTrace(chunk.Meta, req, selection)
					// Save request log
					h.proxyService.SaveRequestLog(c.Request.Context(), chunk.Meta, user.UserID, user.APIKeyID)

					h.logger.Debug("stream completed",
						zap.String("request_id", chunk.Meta.RequestID),
						zap.Int("input_tokens", chunk.Meta.InputTokens),
						zap.Int("output_tokens", chunk.Meta.OutputTokens),
						zap.Float64("cost", chunk.Meta.Cost),
						zap.Float64("latency_ms", chunk.Meta.LatencyMs))
				}
				return
			}

			// Write chunk to response
			if len(chunk.Data) > 0 {
				_, err := c.Writer.Write(chunk.Data)
				if err != nil {
					h.logger.Error("failed to write chunk",
						zap.String("request_id", meta.RequestID),
						zap.Error(err))
					return
				}
				c.Writer.Flush()
			}
		}
	}
}

// setProxyHeaders sets the proxy metadata headers on the response.
func setProxyHeaders(c *gin.Context, meta *service.ProxyMetadata) {
	c.Header("X-Proxy-Request-Id", meta.RequestID)
	c.Header("X-Proxy-Model", url.QueryEscape(meta.SelectedModel))
	c.Header("X-Proxy-Endpoint", url.QueryEscape(meta.SelectedEndpoint))
	c.Header("X-Proxy-Task-Type", meta.InferredTaskType)
	if meta.RoutingMethod != "" {
		c.Header("X-Proxy-Routing-Method", meta.RoutingMethod)
	}
	if meta.ShadowRouting != nil {
		c.Header("X-Proxy-Shadow-Task-Type", string(meta.ShadowRouting.TaskType))
		c.Header("X-Proxy-Shadow-Method", meta.ShadowRouting.RoutingMethod)
		if meta.ShadowRouting.Model != nil {
			c.Header("X-Proxy-Shadow-Model", url.QueryEscape(meta.ShadowRouting.Model.Name))
		}
	}
	c.Header("X-Proxy-Latency-Ms", strconv.FormatInt(int64(meta.LatencyMs), 10))
	c.Header("X-Proxy-Cost", strconv.FormatFloat(meta.Cost, 'f', -1, 64))
	c.Header("X-Proxy-Input-Tokens", strconv.Itoa(meta.InputTokens))
	c.Header("X-Proxy-Output-Tokens", strconv.Itoa(meta.OutputTokens))
}

func attachModelTrace(meta *service.ProxyMetadata, req *models.AnthropicRequest, selection *service.EndpointSelectionResult) {
	if meta == nil || req == nil {
		return
	}
	meta.RequestedModel = req.Model
	if selection != nil && selection.Model != nil {
		meta.ResolvedModel = selection.Model.Name
	}
}

// extractAPIKey extracts the API key from x-api-key header or Authorization bearer.
func extractAPIKey(c *gin.Context) string {
	if key := c.GetHeader("x-api-key"); key != "" {
		return key
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		if strings.HasPrefix(token, "sk-proxy-") {
			return token
		}
	}
	return ""
}

// attachContent attaches full request/response content to metadata if configured.
func (h *ProxyHandler) attachContent(ctx context.Context, meta *service.ProxyMetadata, req *models.AnthropicRequest, resp *models.AnthropicResponse) {
	if h.contentPolicy == nil || !h.contentPolicy.ShouldLogFullContent(ctx) {
		return
	}

	// Serialize request content
	if reqBytes, err := json.Marshal(req); err == nil {
		meta.RequestContent = string(reqBytes)
	}

	// Serialize response content
	if resp != nil {
		if respBytes, err := json.Marshal(resp); err == nil {
			meta.ResponseContent = string(respBytes)
		}
	}
}

// attachStreamContent attaches request content to stream metadata if configured.
// Response content is not available for streaming requests.
func (h *ProxyHandler) attachStreamContent(ctx context.Context, meta *service.ProxyMetadata, req *models.AnthropicRequest) {
	if h.contentPolicy == nil || !h.contentPolicy.ShouldLogFullContent(ctx) {
		return
	}

	if reqBytes, err := json.Marshal(req); err == nil {
		meta.RequestContent = string(reqBytes)
	}
}

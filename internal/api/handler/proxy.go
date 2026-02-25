package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

// ProxyHandler handles proxy requests.
type ProxyHandler struct {
	proxyService      *service.ProxyService
	authService       *service.AuthService
	endpointSelector  *service.EndpointSelector
	routingConfigRepo *repository.RoutingConfigRepository
	logger            *zap.Logger
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(
	ps *service.ProxyService,
	as *service.AuthService,
	es *service.EndpointSelector,
	rcr *repository.RoutingConfigRepository,
	logger *zap.Logger,
) *ProxyHandler {
	return &ProxyHandler{
		proxyService:      ps,
		authService:       as,
		endpointSelector:  es,
		routingConfigRepo: rcr,
		logger:            logger,
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

	// Get endpoints from context
	endpoints, ok := c.Get("endpoints")
	if !ok || endpoints == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "api_error",
				"message": "No endpoints configured",
			},
		})
		return
	}

	eps := endpoints.([]*models.Endpoint)

	// Check if streaming is requested
	if req.Stream {
		h.handleStreamRequest(c, &req, eps, user)
		return
	}

	// Non-streaming request
	h.handleNonStreamRequest(c, &req, eps, user)
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

	resp, meta, err := h.proxyService.ProxyRequest(ctx, req, c.Request.Header, selection, eps)
	if err != nil {
		if ue, ok := err.(*service.UpstreamError); ok {
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
			meta.RoutingDecision = selection.RoutingDecision
			meta.RuleMatchResult = selection.RuleMatchResult
			h.attachContent(ctx, meta, req, nil)
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
		meta.RoutingDecision = selection.RoutingDecision
		meta.RuleMatchResult = selection.RuleMatchResult
		h.attachContent(ctx, meta, req, nil)
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
	meta.RoutingDecision = selection.RoutingDecision
	meta.RuleMatchResult = selection.RuleMatchResult
	meta.InferredTaskType = string(selection.TaskType)

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

	chunkChan, meta, err := h.proxyService.ProxyStreamRequest(ctx, req, c.Request.Header, selection, eps)
	if err != nil {
		if ue, ok := err.(*service.UpstreamError); ok {
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
			meta.RoutingDecision = selection.RoutingDecision
			meta.RuleMatchResult = selection.RuleMatchResult
			h.attachStreamContent(ctx, meta, req)
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
		meta.RoutingDecision = selection.RoutingDecision
		meta.RuleMatchResult = selection.RuleMatchResult
		h.attachStreamContent(ctx, meta, req)
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
	meta.RoutingDecision = selection.RoutingDecision
	meta.RuleMatchResult = selection.RuleMatchResult
	meta.FallbackInfo = selection.FallbackInfo
	meta.InferredTaskType = string(selection.TaskType)

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
				return
			}

			if chunk.Done {
				// Final chunk with metadata
				if chunk.Meta != nil {
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
	c.Header("X-Proxy-Latency-Ms", strconv.FormatInt(int64(meta.LatencyMs), 10))
	c.Header("X-Proxy-Cost", strconv.FormatFloat(meta.Cost, 'f', -1, 64))
	c.Header("X-Proxy-Input-Tokens", strconv.Itoa(meta.InputTokens))
	c.Header("X-Proxy-Output-Tokens", strconv.Itoa(meta.OutputTokens))
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
	if h.routingConfigRepo == nil {
		return
	}

	cfg, err := h.routingConfigRepo.GetConfig(ctx)
	if err != nil {
		h.logger.Warn("failed to get routing config for content logging", zap.Error(err))
		return
	}

	if !cfg.LogFullContent {
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
	if h.routingConfigRepo == nil {
		return
	}

	cfg, err := h.routingConfigRepo.GetConfig(ctx)
	if err != nil {
		h.logger.Warn("failed to get routing config for content logging", zap.Error(err))
		return
	}

	if !cfg.LogFullContent {
		return
	}

	if reqBytes, err := json.Marshal(req); err == nil {
		meta.RequestContent = string(reqBytes)
	}
}

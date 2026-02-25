package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// ProxyMetadata contains metadata about a proxied request.
type ProxyMetadata struct {
	RequestID        string
	SelectedModel    string
	SelectedEndpoint string
	InferredTaskType string
	LatencyMs        float64
	Cost             float64
	InputTokens      int
	OutputTokens     int
	Stream           bool
	StatusCode       int
	Success          bool

	// Routing decision info
	RoutingDecision *models.RoutingDecision
	RuleMatchResult *ClassifyResult
	FallbackInfo    *models.FallbackInfo
	RequestContent  string // Full request content
	ResponseContent string // Full response content
}

// StreamChunk represents a chunk of SSE stream data.
type StreamChunk struct {
	Data []byte
	Err  error
	Done bool
	Meta *ProxyMetadata // Only set on final chunk
}

// ProxyService forwards requests to upstream LLM providers.
type ProxyService struct {
	healthChecker *HealthChecker
	loadBalancer  *LoadBalancer
	logRepo       repository.RequestLogRepository
	logger        *zap.Logger
	client        *http.Client
	streamClient  *http.Client // Separate client for streaming with longer timeout
}

// NewProxyService creates a new ProxyService.
func NewProxyService(
	hc *HealthChecker,
	lb *LoadBalancer,
	logRepo repository.RequestLogRepository,
	logger *zap.Logger,
) *ProxyService {
	return &ProxyService{
		healthChecker: hc,
		loadBalancer:  lb,
		logRepo:       logRepo,
		logger:        logger,
		client: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		streamClient: &http.Client{
			Timeout: 0, // No timeout for streaming
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ProxyRequest forwards a non-streaming request with endpoint retry support.
func (s *ProxyService) ProxyRequest(
	ctx context.Context,
	req *models.AnthropicRequest,
	originalHeaders http.Header,
	selection *EndpointSelectionResult,
	endpoints []*models.Endpoint,
) (*models.AnthropicResponse, *ProxyMetadata, error) {
	start := time.Now()
	requestID := uuid.New().String()

	if selection == nil || selection.Endpoint == nil {
		return nil, nil, fmt.Errorf("no endpoint selected")
	}

	const maxRetries = 3
	triedEndpoints := make(map[string]bool)
	ep := selection.Endpoint

	for attempt := 0; attempt < maxRetries; attempt++ {
		epName := EndpointName(ep)
		triedEndpoints[epName] = true

		resp, meta, err := s.proxyToEndpoint(ctx, req, originalHeaders, ep, requestID, start)
		if err == nil {
			meta.FallbackInfo = selection.FallbackInfo
			return resp, meta, nil
		}

		// Check if it's a client error (4xx) - don't retry
		if ue, ok := err.(*UpstreamError); ok && ue.StatusCode < 500 {
			return nil, nil, err
		}

		s.logger.Warn("endpoint request failed, trying alternative",
			zap.Int("attempt", attempt+1),
			zap.String("endpoint", epName),
			zap.Error(err))

		// Select alternative endpoint
		ep = s.selectAlternativeEndpoint(selection.Model, endpoints, triedEndpoints)
		if ep == nil {
			return nil, nil, fmt.Errorf("all endpoints failed for model %s: %w", selection.Model.Name, err)
		}
	}

	return nil, nil, fmt.Errorf("max retries exceeded for model %s", selection.Model.Name)
}

// proxyToEndpoint sends a request to a single endpoint.
func (s *ProxyService) proxyToEndpoint(
	ctx context.Context,
	req *models.AnthropicRequest,
	originalHeaders http.Header,
	ep *models.Endpoint,
	requestID string,
	start time.Time,
) (*models.AnthropicResponse, *ProxyMetadata, error) {
	epName := EndpointName(ep)
	s.healthChecker.IncrementConnections(epName)
	defer s.healthChecker.DecrementConnections(epName)

	// Create a copy of the request and replace model name with the selected endpoint's model
	proxyReq := *req
	proxyReq.Model = ep.Model.Name
	body, err := json.Marshal(&proxyReq)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	upstreamURL := fmt.Sprintf("%s/v1/messages", ep.Provider.BaseURL)
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create upstream request: %w", err)
	}

	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("x-api-key", ep.Provider.APIKey)
	upReq.Header.Set("anthropic-version", headerOrDefault(originalHeaders, "Anthropic-Version", "2023-06-01"))
	copyAnthropicHeaders(originalHeaders, upReq.Header)

	resp, err := s.client.Do(upReq)
	if err != nil {
		s.healthChecker.UpdateRequestStats(epName, false, msSince(start))
		return nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	latencyMs := msSince(start)
	success := resp.StatusCode < 500

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.healthChecker.UpdateRequestStats(epName, false, latencyMs)
		return nil, nil, fmt.Errorf("read upstream response: %w", err)
	}

	s.healthChecker.UpdateRequestStats(epName, success, latencyMs)

	if resp.StatusCode >= 400 {
		return nil, nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	var anthropicResp models.AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, nil, fmt.Errorf("decode upstream response: %w", err)
	}

	meta := &ProxyMetadata{
		RequestID:        requestID,
		SelectedModel:    ep.Model.Name,
		SelectedEndpoint: ep.Provider.Name,
		InferredTaskType: string(ep.Model.Role),
		LatencyMs:        latencyMs,
		InputTokens:      anthropicResp.Usage.InputTokens,
		OutputTokens:     anthropicResp.Usage.OutputTokens,
		Cost:             calculateCost(ep.Model, anthropicResp.Usage),
	}

	return &anthropicResp, meta, nil
}

// selectAlternativeEndpoint selects an alternative healthy endpoint for the model.
func (s *ProxyService) selectAlternativeEndpoint(
	model *models.Model,
	endpoints []*models.Endpoint,
	excludeNames map[string]bool,
) *models.Endpoint {
	var candidates []*models.Endpoint
	for _, ep := range endpoints {
		if ep.Model.ID == model.ID {
			epName := EndpointName(ep)
			if !excludeNames[epName] && s.healthChecker.IsHealthy(epName) {
				candidates = append(candidates, ep)
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	return s.loadBalancer.Select(candidates, nil)
}

// UpstreamError represents an error response from the upstream provider.
type UpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream returned status %d", e.StatusCode)
}

// --- Helper functions ---

func headerOrDefault(h http.Header, key, def string) string {
	if v := h.Get(key); v != "" {
		return v
	}
	return def
}

func copyAnthropicHeaders(src, dst http.Header) {
	for k, vv := range src {
		if len(k) > 10 && k[:10] == "Anthropic-" && k != "Anthropic-Version" {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func msSince(start time.Time) float64 {
	return float64(time.Since(start).Milliseconds())
}

func calculateCost(model *models.Model, usage models.Usage) float64 {
	inputCost := float64(usage.InputTokens) / 1_000_000 * model.CostPerMtokInput
	outputCost := float64(usage.OutputTokens) / 1_000_000 * model.CostPerMtokOutput * model.BillingMultiplier
	return inputCost + outputCost
}

func calculateCostFromTokens(model *models.Model, inputTokens, outputTokens int) float64 {
	inputCost := float64(inputTokens) / 1_000_000 * model.CostPerMtokInput
	outputCost := float64(outputTokens) / 1_000_000 * model.CostPerMtokOutput * model.BillingMultiplier
	return inputCost + outputCost
}

// SaveRequestLog persists a request log entry to the database.
func (s *ProxyService) SaveRequestLog(ctx context.Context, meta *ProxyMetadata, userID int64, apiKeyID *int64) {
	if s.logRepo == nil || meta == nil {
		return
	}
	statusCode := meta.StatusCode
	entry := &models.RequestLogEntry{
		RequestID:    meta.RequestID,
		UserID:       userID,
		APIKeyID:     apiKeyID,
		ModelName:    meta.SelectedModel,
		EndpointName: meta.SelectedEndpoint,
		TaskType:     meta.InferredTaskType,
		InputTokens:  meta.InputTokens,
		OutputTokens: meta.OutputTokens,
		LatencyMs:    meta.LatencyMs,
		Cost:         meta.Cost,
		StatusCode:   &statusCode,
		Success:      meta.Success,
		Stream:       meta.Stream,
		RequestContent:  meta.RequestContent,
		ResponseContent: meta.ResponseContent,
	}

	// Populate routing decision fields
	if meta.RoutingDecision != nil {
		d := meta.RoutingDecision
		entry.RoutingReason = d.Reason
		entry.RoutingMethod = routingMethodFromDecision(d)
	}

	// Populate rule match fields
	if meta.RuleMatchResult != nil {
		r := meta.RuleMatchResult
		if r.Rule != nil {
			entry.MatchedRuleID = &r.Rule.ID
			entry.MatchedRuleName = r.Rule.Name
		}
		entry.AllMatches = r.Matches
	}

	// Generate message preview from request content
	if meta.RequestContent != "" {
		entry.MessagePreview = truncateStr(meta.RequestContent, 200)
	}

	if _, err := s.logRepo.Insert(ctx, entry); err != nil {
		s.logger.Error("failed to save request log",
			zap.String("request_id", meta.RequestID),
			zap.Error(err))
	}
}

// routingMethodFromDecision derives the routing_method string from a RoutingDecision.
func routingMethodFromDecision(d *models.RoutingDecision) string {
	if d.FromCache {
		switch d.CacheType {
		case "L1":
			return "cache_l1"
		case "L2":
			return "cache_l2"
		case "L3":
			return "cache_l3"
		default:
			return "cache_l1"
		}
	}
	switch d.CacheType {
	case "rule":
		return "rule"
	default:
		if d.ModelUsed != "" {
			return "llm"
		}
		return "fallback"
	}
}

// truncateStr truncates a string to maxLen runes.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// ProxyStreamRequest forwards a streaming request with endpoint selection support.
func (s *ProxyService) ProxyStreamRequest(
	ctx context.Context,
	req *models.AnthropicRequest,
	originalHeaders http.Header,
	selection *EndpointSelectionResult,
	endpoints []*models.Endpoint,
) (<-chan StreamChunk, *ProxyMetadata, error) {
	start := time.Now()
	requestID := uuid.New().String()

	if selection == nil || selection.Endpoint == nil {
		return nil, nil, fmt.Errorf("no endpoint selected")
	}

	ep := selection.Endpoint
	epName := EndpointName(ep)
	s.healthChecker.IncrementConnections(epName)

	// Ensure stream is enabled in request and use the selected endpoint's model name
	streamReq := *req
	streamReq.Model = ep.Model.Name
	streamReq.Stream = true

	body, err := json.Marshal(&streamReq)
	if err != nil {
		s.healthChecker.DecrementConnections(epName)
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	upstreamURL := fmt.Sprintf("%s/v1/messages", ep.Provider.BaseURL)
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		s.healthChecker.DecrementConnections(epName)
		return nil, nil, fmt.Errorf("create upstream request: %w", err)
	}

	// Set headers
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	upReq.Header.Set("x-api-key", ep.Provider.APIKey)
	upReq.Header.Set("anthropic-version", headerOrDefault(originalHeaders, "Anthropic-Version", "2023-06-01"))
	copyAnthropicHeaders(originalHeaders, upReq.Header)

	resp, err := s.streamClient.Do(upReq)
	if err != nil {
		s.healthChecker.DecrementConnections(epName)
		s.healthChecker.UpdateRequestStats(epName, false, msSince(start))
		return nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}

	// Check for error response
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		s.healthChecker.DecrementConnections(epName)
		respBody, _ := io.ReadAll(resp.Body)
		s.healthChecker.UpdateRequestStats(epName, false, msSince(start))
		return nil, nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	// Initial metadata (will be updated with final values)
	meta := &ProxyMetadata{
		RequestID:        requestID,
		SelectedModel:    ep.Model.Name,
		SelectedEndpoint: ep.Provider.Name,
		InferredTaskType: string(ep.Model.Role),
		Stream:           true,
		StatusCode:       resp.StatusCode,
		Success:          true,
	}

	// Create channel for streaming chunks
	chunkChan := make(chan StreamChunk, 100)

	// Start goroutine to read stream
	go s.readSSEStream(ctx, resp, ep, epName, start, meta, chunkChan)

	return chunkChan, meta, nil
}

// readSSEStream reads SSE events from the response and sends chunks to the channel.
func (s *ProxyService) readSSEStream(
	ctx context.Context,
	resp *http.Response,
	ep *models.Endpoint,
	epName string,
	start time.Time,
	meta *ProxyMetadata,
	chunkChan chan<- StreamChunk,
) {
	defer close(chunkChan)
	defer resp.Body.Close()
	defer s.healthChecker.DecrementConnections(epName)

	var inputTokens, outputTokens int
	reader := bufio.NewReader(resp.Body)

	for {
		select {
		case <-ctx.Done():
			chunkChan <- StreamChunk{Err: ctx.Err(), Done: true}
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				// EOF may carry remaining data — send it before finishing
				if len(line) > 0 {
					chunkChan <- StreamChunk{Data: line}
					s.parseSSEUsage(line, &inputTokens, &outputTokens)
				}
				break
			}
			s.logger.Error("error reading stream", zap.Error(err))
			chunkChan <- StreamChunk{Err: err, Done: true}
			s.healthChecker.UpdateRequestStats(epName, false, msSince(start))
			return
		}

		// Send raw chunk to client
		if len(line) > 0 {
			chunkChan <- StreamChunk{Data: line}
		}

		// Parse SSE event for token counting
		s.parseSSEUsage(line, &inputTokens, &outputTokens)
	}

	// Calculate final metrics
	latencyMs := msSince(start)
	cost := calculateCostFromTokens(ep.Model, inputTokens, outputTokens)

	// Update metadata with final values
	meta.LatencyMs = latencyMs
	meta.InputTokens = inputTokens
	meta.OutputTokens = outputTokens
	meta.Cost = cost

	// Send final chunk with metadata
	chunkChan <- StreamChunk{Done: true, Meta: meta}

	// Update health stats
	s.healthChecker.UpdateRequestStats(epName, true, latencyMs)

	s.logger.Debug("stream completed",
		zap.String("request_id", meta.RequestID),
		zap.Int("input_tokens", inputTokens),
		zap.Int("output_tokens", outputTokens),
		zap.Float64("cost", cost),
		zap.Float64("latency_ms", latencyMs))
}

// parseSSEUsage extracts token usage from an SSE data line.
func (s *ProxyService) parseSSEUsage(line []byte, inputTokens, outputTokens *int) {
	lineStr := string(line)
	if !strings.HasPrefix(lineStr, "data: ") {
		return
	}
	dataStr := strings.TrimSpace(strings.TrimPrefix(lineStr, "data: "))
	if dataStr == "" || dataStr == "[DONE]" {
		return
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
		return
	}
	usage, ok := event["usage"].(map[string]any)
	if !ok {
		return
	}
	if it, ok := usage["input_tokens"].(float64); ok {
		*inputTokens = int(it)
	}
	if ot, ok := usage["output_tokens"].(float64); ok {
		*outputTokens = int(ot)
	}
}

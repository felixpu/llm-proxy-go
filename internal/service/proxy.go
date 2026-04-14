package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	RequestedModel   string
	ResolvedModel    string
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

	// Cache statistics (Anthropic Prompt Caching)
	CacheCreationInputTokens int
	CacheReadInputTokens     int

	// Routing decision info
	RoutingMethod   string
	RoutingReason   string
	RoutingDecision *models.RoutingDecision
	ShadowRouting   *ShadowRoutingResult
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

const maxEndpointRetries = 3

// ProxyService forwards requests to upstream LLM providers.
type ProxyService struct {
	healthChecker *HealthChecker
	loadBalancer  *LoadBalancer
	logRepo       repository.RequestLogWriteRepository
	logger        *zap.Logger
	client        *http.Client
	streamClient  *http.Client // Separate client for streaming with longer timeout
	asyncPool     *AsyncWorkerPool
}

// NewProxyService creates a new ProxyService.
func NewProxyService(
	hc *HealthChecker,
	lb *LoadBalancer,
	logRepo repository.RequestLogWriteRepository,
	logger *zap.Logger,
	asyncPool *AsyncWorkerPool,
) *ProxyService {
	return &ProxyService{
		healthChecker: hc,
		loadBalancer:  lb,
		logRepo:       logRepo,
		logger:        logger,
		asyncPool:     asyncPool,
		client: &http.Client{
			Timeout: DefaultProxyHTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		streamClient: &http.Client{
			Timeout: 0, // No overall timeout for streaming — body reads are unbounded
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       90 * time.Second,
				ResponseHeaderTimeout: DefaultProxyHTTPTimeout, // Timeout for initial response headers
			},
		},
	}
}

// ProxyRequest forwards a non-streaming request with endpoint retry support.
func (s *ProxyService) ProxyRequest(
	ctx context.Context,
	req *models.AnthropicRequest,
	originalHeaders http.Header,
	originalQuery string,
	selection *EndpointSelectionResult,
	endpoints []*models.Endpoint,
) (*models.AnthropicResponse, *ProxyMetadata, error) {
	requestID := uuid.New().String()

	if selection == nil || selection.Endpoint == nil {
		return nil, nil, fmt.Errorf("no endpoint selected")
	}

	triedEndpoints := make(map[string]bool)
	ep := selection.Endpoint

	for attempt := 0; attempt < maxEndpointRetries; attempt++ {
		attemptStart := time.Now()
		epName := EndpointName(ep)
		triedEndpoints[epName] = true

		resp, meta, err := s.proxyToEndpoint(ctx, req, originalHeaders, originalQuery, ep, requestID, attemptStart)
		if err == nil {
			meta.FallbackInfo = selection.FallbackInfo
			return resp, meta, nil
		}

		// Check if the error is non-retryable (e.g. 400, 404, 422)
		var ue *UpstreamError
		if errors.As(err, &ue) && !isRetryableStatusCode(ue.StatusCode) {
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
	originalQuery string,
	ep *models.Endpoint,
	requestID string,
	start time.Time,
) (*models.AnthropicResponse, *ProxyMetadata, error) {
	epName := EndpointName(ep)
	s.healthChecker.IncrementConnections(epName)
	defer s.healthChecker.DecrementConnections(epName)

	apiType, err := resolveProxyAPIType(ctx, ep)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve api type: %w", err)
	}

	body, err := buildProxyUpstreamBody(req, ep.Model.Name, apiType)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	adapter := GetAdapter(apiType)

	upstreamURL := buildUpstreamURL(ep.Provider.BaseURL, adapter.GetEndpoint(), originalQuery)
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create upstream request: %w", err)
	}

	upReq.Header.Set("Content-Type", "application/json")
	adapter.SetAuthHeaders(upReq, ep.Provider.APIKey)

	// For Anthropic APIs, also set version header if present
	if apiType == APITypeAnthropicMessages || apiType == APITypeAnthropicResponses {
		upReq.Header.Set("anthropic-version", headerOrDefault(originalHeaders, "Anthropic-Version", DefaultAnthropicVersion))
		copyAnthropicHeaders(originalHeaders, upReq.Header)
	}

	// Forward client User-Agent if present
	if ua := originalHeaders.Get("User-Agent"); ua != "" {
		upReq.Header.Set("User-Agent", ua)
	}
	// Apply provider-level custom headers (highest priority)
	applyCustomHeaders(ep.Provider.CustomHeaders, upReq.Header)

	resp, err := s.client.Do(upReq)
	if err != nil {
		s.healthChecker.UpdateRequestStatsV2(RequestResult{
			EndpointName: epName,
			Success:      false,
			LatencyMs:    msSince(start),
			Err:          err,
		})
		return nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	latencyMs := msSince(start)
	success := resp.StatusCode < 400

	readLimit := int64(DefaultProxyResponseMaxBytes)
	if resp.StatusCode >= 400 {
		readLimit = int64(DefaultProxyErrorResponseMaxBytes)
	}
	respBody, truncated, err := readBodyWithLimit(resp.Body, readLimit)
	if err != nil {
		s.healthChecker.UpdateRequestStatsV2(RequestResult{
			EndpointName: epName,
			Success:      false,
			LatencyMs:    latencyMs,
			StatusCode:   resp.StatusCode,
			Err:          err,
		})
		return nil, nil, fmt.Errorf("read upstream response: %w", err)
	}
	if truncated && resp.StatusCode < 400 {
		truncateErr := fmt.Errorf("upstream response exceeds %d bytes limit", DefaultProxyResponseMaxBytes)
		s.healthChecker.UpdateRequestStatsV2(RequestResult{
			EndpointName: epName,
			Success:      false,
			LatencyMs:    latencyMs,
			StatusCode:   resp.StatusCode,
			ResponseBody: respBody,
			Err:          truncateErr,
		})
		return nil, nil, truncateErr
	}
	if truncated && resp.StatusCode >= 400 {
		s.logger.Warn("upstream error response truncated",
			zap.String("endpoint", epName),
			zap.Int("status_code", resp.StatusCode),
			zap.Int64("limit_bytes", readLimit))
	}

	s.healthChecker.UpdateRequestStatsV2(RequestResult{
		EndpointName: epName,
		Success:      success,
		LatencyMs:    latencyMs,
		StatusCode:   resp.StatusCode,
		ResponseBody: respBody,
	})

	if resp.StatusCode >= 400 {
		return nil, nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	anthropicResp, err := parseProxyUpstreamResponse(respBody, apiType)
	if err != nil {
		return nil, nil, fmt.Errorf("decode upstream response: %w", err)
	}

	meta := &ProxyMetadata{
		RequestID:                requestID,
		SelectedModel:            ep.Model.Name,
		SelectedEndpoint:         ep.Provider.Name,
		InferredTaskType:         string(ep.Model.Role),
		LatencyMs:                latencyMs,
		InputTokens:              anthropicResp.Usage.InputTokens,
		OutputTokens:             anthropicResp.Usage.OutputTokens,
		CacheCreationInputTokens: anthropicResp.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     anthropicResp.Usage.CacheReadInputTokens,
		Cost:                     calculateCost(ep.Model, anthropicResp.Usage),
	}

	return anthropicResp, meta, nil
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

// isRetryableStatusCode determines if a status code should trigger endpoint retry.
// Retryable: 401 (invalid key), 402 (insufficient balance), 403 (quota/permission),
// 408 (timeout), 429 (rate limit), >=500 (server errors).
// Non-retryable: 400 (bad request), 404 (not found), 413 (too large), 422 (validation).
func isRetryableStatusCode(code int) bool {
	switch code {
	case 401, 402, 403, 408, 429:
		return true
	default:
		return code >= 500
	}
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
		lower := strings.ToLower(k)
		// Forward Anthropic-* headers (except Anthropic-Version which is set explicitly)
		if strings.HasPrefix(lower, "anthropic-") && lower != "anthropic-version" {
			for _, v := range vv {
				dst.Add(k, v)
			}
			continue
		}
		// Forward Claude Code / Stainless client identification headers
		if lower == "x-app" || strings.HasPrefix(lower, "x-stainless-") ||
			strings.HasPrefix(lower, "x-claude-") || strings.HasPrefix(lower, "x-client-") {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func buildUpstreamURL(baseURL, endpointPath, rawQuery string) string {
	joined := strings.TrimRight(baseURL, "/") + endpointPath
	if rawQuery == "" {
		return joined
	}

	u, err := url.Parse(joined)
	if err != nil {
		return joined
	}
	u.RawQuery = rawQuery
	return u.String()
}

// applyCustomHeaders applies provider-level custom headers to the request.
// Custom headers have the highest priority and override any previously set headers.
func applyCustomHeaders(custom map[string]string, dst http.Header) {
	for k, v := range custom {
		dst.Set(k, v)
	}
}

func msSince(start time.Time) float64 {
	return float64(time.Since(start).Milliseconds())
}

func calculateCost(model *models.Model, usage models.Usage) float64 {
	// Normal input tokens (excluding cache read and cache creation tokens)
	normalInputTokens := usage.InputTokens - usage.CacheReadInputTokens - usage.CacheCreationInputTokens
	if normalInputTokens < 0 {
		normalInputTokens = 0
	}
	inputCost := float64(normalInputTokens) / 1_000_000 * model.CostPerMtokInput

	// Cache read tokens cost 10% of normal input token price
	cacheReadCost := float64(usage.CacheReadInputTokens) / 1_000_000 * model.CostPerMtokInput * 0.1

	// Cache creation tokens cost 125% of normal input token price
	cacheCreationCost := float64(usage.CacheCreationInputTokens) / 1_000_000 * model.CostPerMtokInput * 1.25

	// Output tokens
	outputCost := float64(usage.OutputTokens) / 1_000_000 * model.CostPerMtokOutput * model.BillingMultiplier

	return inputCost + cacheReadCost + cacheCreationCost + outputCost
}

func calculateCostFromTokens(model *models.Model, inputTokens, outputTokens int) float64 {
	inputCost := float64(inputTokens) / 1_000_000 * model.CostPerMtokInput
	outputCost := float64(outputTokens) / 1_000_000 * model.CostPerMtokOutput * model.BillingMultiplier
	return inputCost + outputCost
}

// calculateCostFromTokensWithCache calculates cost considering cache tokens.
func calculateCostFromTokensWithCache(model *models.Model, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) float64 {
	normalInputTokens := inputTokens - cacheReadTokens - cacheCreationTokens
	if normalInputTokens < 0 {
		normalInputTokens = 0
	}
	inputCost := float64(normalInputTokens) / 1_000_000 * model.CostPerMtokInput
	cacheReadCost := float64(cacheReadTokens) / 1_000_000 * model.CostPerMtokInput * 0.1
	cacheCreationCost := float64(cacheCreationTokens) / 1_000_000 * model.CostPerMtokInput * 1.25
	outputCost := float64(outputTokens) / 1_000_000 * model.CostPerMtokOutput * model.BillingMultiplier
	return inputCost + cacheReadCost + cacheCreationCost + outputCost
}

// SaveRequestLog persists a request log entry to the database asynchronously.
// Uses a detached context because the request context may already be cancelled.
func (s *ProxyService) SaveRequestLog(ctx context.Context, meta *ProxyMetadata, userID int64, apiKeyID *int64) {
	if s.logRepo == nil || meta == nil {
		return
	}
	statusCode := meta.StatusCode
	entry := &models.RequestLogEntry{
		RequestID:                meta.RequestID,
		UserID:                   userID,
		APIKeyID:                 apiKeyID,
		ModelName:                meta.SelectedModel,
		RequestedModel:           meta.RequestedModel,
		ResolvedModel:            meta.ResolvedModel,
		EndpointName:             meta.SelectedEndpoint,
		TaskType:                 meta.InferredTaskType,
		InputTokens:              meta.InputTokens,
		OutputTokens:             meta.OutputTokens,
		CacheCreationInputTokens: meta.CacheCreationInputTokens,
		CacheReadInputTokens:     meta.CacheReadInputTokens,
		LatencyMs:                meta.LatencyMs,
		Cost:                     meta.Cost,
		StatusCode:               &statusCode,
		Success:                  meta.Success,
		Stream:                   meta.Stream,
		RequestContent:           meta.RequestContent,
		ResponseContent:          meta.ResponseContent,
	}

	// Populate routing decision fields
	if meta.RoutingDecision != nil {
		d := meta.RoutingDecision
		entry.RoutingReason = d.Reason
		entry.RoutingMethod = deriveRoutingMethod(d)

		// Populate rule match fields from RoutingDecision
		if d.MatchedRule != nil {
			entry.MatchedRuleID = &d.MatchedRule.ID
			entry.MatchedRuleName = d.MatchedRule.Name
		}
		if len(d.AllMatches) > 0 {
			entry.AllMatches = d.AllMatches
		}
	}

	// Legacy: populate from RuleMatchResult if RoutingDecision didn't have rule info
	if entry.MatchedRuleName == "" && meta.RuleMatchResult != nil {
		r := meta.RuleMatchResult
		if r.Rule != nil {
			entry.MatchedRuleID = &r.Rule.ID
			entry.MatchedRuleName = r.Rule.Name
		}
		if len(r.Matches) > 0 {
			entry.AllMatches = r.Matches
		}
	}

	if entry.RoutingMethod == "" {
		entry.RoutingMethod = meta.RoutingMethod
	}
	if entry.RoutingReason == "" || meta.ShadowRouting != nil {
		entry.RoutingReason = describeRouting(meta)
	}

	// Generate message preview from request content
	if meta.RequestContent != "" {
		entry.MessagePreview = truncateStr(meta.RequestContent, DefaultContentPreviewMaxChars)
	}

	if ok := s.asyncPool.Submit(func() {
		saveCtx, cancel := context.WithTimeout(context.Background(), DefaultAsyncRepoTimeout)
		defer cancel()
		if _, err := s.logRepo.Insert(saveCtx, entry); err != nil {
			s.logger.Error("failed to save request log",
				zap.String("request_id", meta.RequestID),
				zap.Error(err))
		}
	}); !ok {
		s.logger.Warn("dropped request log write",
			zap.String("request_id", meta.RequestID))
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

// ProxyStreamRequest forwards a streaming request with endpoint retry support.
// Retries happen only during the connection phase (before any SSE data is sent to the client).
func (s *ProxyService) ProxyStreamRequest(
	ctx context.Context,
	req *models.AnthropicRequest,
	originalHeaders http.Header,
	originalQuery string,
	selection *EndpointSelectionResult,
	endpoints []*models.Endpoint,
) (<-chan StreamChunk, *ProxyMetadata, error) {
	requestID := uuid.New().String()

	if selection == nil || selection.Endpoint == nil {
		return nil, nil, fmt.Errorf("no endpoint selected")
	}

	triedEndpoints := make(map[string]bool)
	ep := selection.Endpoint

	for attempt := 0; attempt < maxEndpointRetries; attempt++ {
		attemptStart := time.Now()
		epName := EndpointName(ep)
		triedEndpoints[epName] = true

		apiType, err := resolveProxyAPIType(ctx, ep)
		if err != nil {
			s.logger.Warn("failed to resolve stream api type",
				zap.Int("attempt", attempt+1),
				zap.String("endpoint", epName),
				zap.Error(err))

			ep = s.selectAlternativeEndpoint(selection.Model, endpoints, triedEndpoints)
			if ep == nil {
				return nil, nil, fmt.Errorf("all endpoints failed for model %s: %w", selection.Model.Name, err)
			}
			continue
		}

		resp, err := s.connectStreamEndpoint(ctx, req, originalHeaders, originalQuery, ep, apiType, attemptStart)
		if err != nil {
			// Check if the error is non-retryable
			var ue *UpstreamError
			if errors.As(err, &ue) && !isRetryableStatusCode(ue.StatusCode) {
				return nil, nil, err
			}

			s.logger.Warn("stream endpoint failed, trying alternative",
				zap.Int("attempt", attempt+1),
				zap.String("endpoint", epName),
				zap.Error(err))

			ep = s.selectAlternativeEndpoint(selection.Model, endpoints, triedEndpoints)
			if ep == nil {
				return nil, nil, fmt.Errorf("all endpoints failed for model %s: %w", selection.Model.Name, err)
			}
			continue
		}

		// Connection succeeded — track it and start streaming
		s.healthChecker.IncrementConnections(epName)

		meta := &ProxyMetadata{
			RequestID:        requestID,
			SelectedModel:    ep.Model.Name,
			SelectedEndpoint: ep.Provider.Name,
			InferredTaskType: string(ep.Model.Role),
			Stream:           true,
			StatusCode:       resp.StatusCode,
			Success:          true,
			FallbackInfo:     selection.FallbackInfo,
		}

		chunkChan := make(chan StreamChunk, 100)
		// Return a copy so the caller cannot race with the goroutine
		// that populates streaming fields (LatencyMs, InputTokens, etc.).
		returnMeta := *meta
		go s.readSSEStream(ctx, resp, ep, epName, apiType, attemptStart, meta, chunkChan)
		return chunkChan, &returnMeta, nil
	}

	return nil, nil, fmt.Errorf("max retries exceeded for model %s", selection.Model.Name)
}

// connectStreamEndpoint establishes a streaming connection to a single endpoint.
// Returns the HTTP response on success, or an error (including UpstreamError for 4xx/5xx).
func (s *ProxyService) connectStreamEndpoint(
	ctx context.Context,
	req *models.AnthropicRequest,
	originalHeaders http.Header,
	originalQuery string,
	ep *models.Endpoint,
	apiType APIType,
	start time.Time,
) (*http.Response, error) {
	epName := EndpointName(ep)
	adapter := GetAdapter(apiType)

	streamReq := *req
	streamReq.Stream = true
	body, err := buildProxyUpstreamBody(&streamReq, ep.Model.Name, apiType)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	upstreamURL := buildUpstreamURL(ep.Provider.BaseURL, adapter.GetEndpoint(), originalQuery)
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	adapter.SetAuthHeaders(upReq, ep.Provider.APIKey)

	// For Anthropic APIs, also set version header if present
	if apiType == APITypeAnthropicMessages || apiType == APITypeAnthropicResponses {
		upReq.Header.Set("anthropic-version", headerOrDefault(originalHeaders, "Anthropic-Version", DefaultAnthropicVersion))
		copyAnthropicHeaders(originalHeaders, upReq.Header)
	}

	if ua := originalHeaders.Get("User-Agent"); ua != "" {
		upReq.Header.Set("User-Agent", ua)
	}
	applyCustomHeaders(ep.Provider.CustomHeaders, upReq.Header)

	resp, err := s.streamClient.Do(upReq)
	if err != nil {
		s.healthChecker.UpdateRequestStatsV2(RequestResult{
			EndpointName: epName,
			Success:      false,
			LatencyMs:    msSince(start),
			Err:          err,
		})
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		respBody, truncated, readErr := readBodyWithLimit(resp.Body, int64(DefaultProxyErrorResponseMaxBytes))
		resp.Body.Close()
		if truncated {
			s.logger.Warn("stream error response truncated",
				zap.String("endpoint", epName),
				zap.Int("status_code", resp.StatusCode),
				zap.Int("limit_bytes", DefaultProxyErrorResponseMaxBytes))
		}
		s.healthChecker.UpdateRequestStatsV2(RequestResult{
			EndpointName: epName,
			Success:      false,
			LatencyMs:    msSince(start),
			StatusCode:   resp.StatusCode,
			ResponseBody: respBody,
			Err:          readErr,
		})
		if readErr != nil {
			return nil, fmt.Errorf("read upstream error response (status %d): %w", resp.StatusCode, readErr)
		}
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	return resp, nil
}

// readSSEStream reads SSE events from the response and sends chunks to the channel.
func (s *ProxyService) readSSEStream(
	ctx context.Context,
	resp *http.Response,
	ep *models.Endpoint,
	epName string,
	apiType APIType,
	start time.Time,
	meta *ProxyMetadata,
	chunkChan chan<- StreamChunk,
) {
	defer close(chunkChan)
	defer resp.Body.Close()
	defer s.healthChecker.DecrementConnections(epName)

	var inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int
	var firstByteTime time.Time
	reader := bufio.NewReader(resp.Body)
	transformer := newSSETransformer(apiType, ep.Model.Name)

	for {
		select {
		case <-ctx.Done():
			latencyMs := streamLatency(firstByteTime, start)
			s.healthChecker.UpdateRequestStatsV2(RequestResult{
				EndpointName: epName,
				Success:      false,
				LatencyMs:    latencyMs,
				Err:          ctx.Err(),
			})
			finalMeta := buildStreamMeta(meta, ep, false, latencyMs, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
			sendStreamChunk(ctx, chunkChan, StreamChunk{Err: ctx.Err(), Done: true, Meta: &finalMeta})
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				// EOF may carry remaining data — send it before finishing
				if len(line) > 0 {
					if err := s.emitTransformedSSE(ctx, line, transformer, &firstByteTime, &inputTokens, &outputTokens, &cacheReadTokens, &cacheCreationTokens, chunkChan); err != nil {
						s.logger.Error("error transforming final stream chunk", zap.Error(err))
						latencyMs := streamLatency(firstByteTime, start)
						s.healthChecker.UpdateRequestStatsV2(RequestResult{
							EndpointName: epName,
							Success:      false,
							LatencyMs:    latencyMs,
							Err:          err,
						})
						finalMeta := buildStreamMeta(meta, ep, false, latencyMs, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
						sendStreamChunk(ctx, chunkChan, StreamChunk{Err: err, Done: true, Meta: &finalMeta})
						return
					}
				}
				if err := s.emitSSEFinalize(ctx, transformer, &firstByteTime, &inputTokens, &outputTokens, &cacheReadTokens, &cacheCreationTokens, chunkChan); err != nil {
					s.logger.Error("error finalizing transformed stream", zap.Error(err))
					latencyMs := streamLatency(firstByteTime, start)
					s.healthChecker.UpdateRequestStatsV2(RequestResult{
						EndpointName: epName,
						Success:      false,
						LatencyMs:    latencyMs,
						Err:          err,
					})
					finalMeta := buildStreamMeta(meta, ep, false, latencyMs, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
					sendStreamChunk(ctx, chunkChan, StreamChunk{Err: err, Done: true, Meta: &finalMeta})
					return
				}
				break
			}
			s.logger.Error("error reading stream", zap.Error(err))
			latencyMs := streamLatency(firstByteTime, start)
			s.healthChecker.UpdateRequestStatsV2(RequestResult{
				EndpointName: epName,
				Success:      false,
				LatencyMs:    latencyMs,
				Err:          err,
			})
			finalMeta := buildStreamMeta(meta, ep, false, latencyMs, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
			sendStreamChunk(ctx, chunkChan, StreamChunk{Err: err, Done: true, Meta: &finalMeta})
			return
		}

		if err := s.emitTransformedSSE(ctx, line, transformer, &firstByteTime, &inputTokens, &outputTokens, &cacheReadTokens, &cacheCreationTokens, chunkChan); err != nil {
			s.logger.Error("error transforming stream", zap.Error(err))
			latencyMs := streamLatency(firstByteTime, start)
			s.healthChecker.UpdateRequestStatsV2(RequestResult{
				EndpointName: epName,
				Success:      false,
				LatencyMs:    latencyMs,
				Err:          err,
			})
			finalMeta := buildStreamMeta(meta, ep, false, latencyMs, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
			sendStreamChunk(ctx, chunkChan, StreamChunk{Err: err, Done: true, Meta: &finalMeta})
			return
		}
	}

	// Calculate final metrics using TTFB
	latencyMs := streamLatency(firstByteTime, start)
	finalMeta := buildStreamMeta(meta, ep, true, latencyMs, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)

	// Send final chunk with completed metadata
	if !sendStreamChunk(ctx, chunkChan, StreamChunk{Done: true, Meta: &finalMeta}) {
		return
	}

	// Update health stats
	s.healthChecker.UpdateRequestStatsV2(RequestResult{
		EndpointName: epName,
		Success:      true,
		LatencyMs:    latencyMs,
		StatusCode:   200,
	})

	s.logger.Debug("stream completed",
		zap.String("request_id", meta.RequestID),
		zap.Int("input_tokens", inputTokens),
		zap.Int("output_tokens", outputTokens),
		zap.Float64("cost", finalMeta.Cost),
		zap.Float64("latency_ms", latencyMs))
}

func (s *ProxyService) emitTransformedSSE(
	ctx context.Context,
	line []byte,
	transformer sseTransformer,
	firstByteTime *time.Time,
	inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens *int,
	chunkChan chan<- StreamChunk,
) error {
	chunks, err := transformer.Transform(line)
	if err != nil {
		return err
	}
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		if firstByteTime.IsZero() {
			*firstByteTime = time.Now()
		}
		if !sendStreamChunk(ctx, chunkChan, StreamChunk{Data: chunk}) {
			return ctx.Err()
		}
		s.parseSSEUsage(chunk, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
	}
	return nil
}

func (s *ProxyService) emitSSEFinalize(
	ctx context.Context,
	transformer sseTransformer,
	firstByteTime *time.Time,
	inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens *int,
	chunkChan chan<- StreamChunk,
) error {
	chunks, err := transformer.Finalize()
	if err != nil {
		return err
	}
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		if firstByteTime.IsZero() {
			*firstByteTime = time.Now()
		}
		if !sendStreamChunk(ctx, chunkChan, StreamChunk{Data: chunk}) {
			return ctx.Err()
		}
		s.parseSSEUsage(chunk, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
	}
	return nil
}

// parseSSEUsage extracts token usage from an SSE data line.
func (s *ProxyService) parseSSEUsage(line []byte, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens *int) {
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
	if crt, ok := usage["cache_read_input_tokens"].(float64); ok {
		*cacheReadTokens = int(crt)
	}
	if cct, ok := usage["cache_creation_input_tokens"].(float64); ok {
		*cacheCreationTokens = int(cct)
	}
}

// streamLatency returns TTFB if available, otherwise falls back to time since start.
func streamLatency(firstByteTime, start time.Time) float64 {
	if !firstByteTime.IsZero() {
		return float64(firstByteTime.Sub(start).Milliseconds())
	}
	return msSince(start)
}

func sendStreamChunk(ctx context.Context, ch chan<- StreamChunk, chunk StreamChunk) bool {
	// Fast path: if a receiver/buffer slot is immediately available, send even if
	// context cancellation races in the same tick.
	select {
	case ch <- chunk:
		return true
	default:
	}

	if ctx.Err() != nil {
		return false
	}

	select {
	case ch <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}

// buildStreamMeta creates a copy of metadata with final streaming values.
func buildStreamMeta(meta *ProxyMetadata, ep *models.Endpoint, success bool, latencyMs float64, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int) ProxyMetadata {
	finalMeta := *meta
	finalMeta.LatencyMs = latencyMs
	finalMeta.InputTokens = inputTokens
	finalMeta.OutputTokens = outputTokens
	finalMeta.CacheReadInputTokens = cacheReadTokens
	finalMeta.CacheCreationInputTokens = cacheCreationTokens
	finalMeta.Cost = calculateCostFromTokensWithCache(ep.Model, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens)
	finalMeta.Success = success
	return finalMeta
}

func readBodyWithLimit(body io.Reader, limit int64) ([]byte, bool, error) {
	if limit <= 0 {
		data, err := io.ReadAll(body)
		return data, false, err
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

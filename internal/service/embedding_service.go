package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/user/llm-proxy-go/internal/repository"
	"go.uber.org/zap"
)

// EmbeddingService provides text embedding with remote API fallback.
// Layer 1: Check config (semantic_cache_enabled)
// Layer 2: Remote API call (embedding_model_id)
// Layer 3: Disabled (return nil) — local models not supported in Go version
type EmbeddingService struct {
	configRepo *repository.RoutingConfigRepository
	modelRepo  *repository.EmbeddingModelRepository
	logger     *zap.Logger
	client     *http.Client
}

// NewEmbeddingService creates a new EmbeddingService.
func NewEmbeddingService(
	configRepo *repository.RoutingConfigRepository,
	modelRepo *repository.EmbeddingModelRepository,
	logger *zap.Logger,
) *EmbeddingService {
	return &EmbeddingService{
		configRepo: configRepo,
		modelRepo:  modelRepo,
		logger:     logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// embeddingAPIRequest is the request body for OpenAI-compatible embedding API.
type embeddingAPIRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embeddingAPIResponse is the response from OpenAI-compatible embedding API.
type embeddingAPIResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// GetEmbedding returns the embedding vector for the given text.
// Returns nil if embedding is unavailable (disabled or all layers failed).
func (es *EmbeddingService) GetEmbedding(ctx context.Context, text string) ([]float64, error) {
	// Layer 1: Check config
	cfg, err := es.configRepo.GetConfig(ctx)
	if err != nil {
		es.logger.Warn("failed to get routing config for embedding", zap.Error(err))
		return nil, nil
	}

	if !cfg.SemanticCacheEnabled {
		es.logger.Debug("semantic cache disabled, skipping embedding")
		return nil, nil
	}

	// Layer 2: Remote API call
	if cfg.EmbeddingModelID != nil {
		embedding, err := es.getEmbeddingRemote(ctx, *cfg.EmbeddingModelID, text)
		if err != nil {
			es.logger.Warn("remote embedding failed, falling back",
				zap.Error(err),
				zap.Int64("model_id", *cfg.EmbeddingModelID))
		} else if embedding != nil {
			return embedding, nil
		}
	}

	// Layer 3: Local embedding not supported in Go version
	// Python version uses SentenceTransformers/FastEmbed, which requires Python runtime.
	// For Go, we rely on remote API only. If remote fails, semantic cache is unavailable.
	es.logger.Debug("no embedding available, semantic cache disabled for this request")
	return nil, nil
}

// getEmbeddingRemote calls a remote OpenAI-compatible embedding API.
func (es *EmbeddingService) getEmbeddingRemote(ctx context.Context, modelID int64, text string) ([]float64, error) {
	// Look up the routing model with provider info
	// We reuse RoutingModelRepository since embedding models reference providers
	// But embedding_model_id points to embedding_models table, not routing_models.
	// We need to find the provider config from the routing config.
	cfg, err := es.configRepo.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// Get the embedding model name from embedding_models table
	models, err := es.modelRepo.ListModels(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list embedding models: %w", err)
	}

	var modelName string
	for _, m := range models {
		if m.ID == modelID {
			modelName = m.Name
			break
		}
	}
	if modelName == "" {
		return nil, fmt.Errorf("embedding model not found: %d", modelID)
	}

	// For remote embedding, we need a provider with base_url and api_key.
	// Use the primary routing model's provider as the embedding endpoint.
	if cfg.PrimaryModelID == nil {
		return nil, fmt.Errorf("no primary model configured for embedding API")
	}

	// This is a simplified approach — in production, embedding_model_id
	// should directly reference a provider endpoint for embedding.
	// For now, we log a warning and return nil.
	es.logger.Debug("remote embedding requested",
		zap.String("model", modelName),
		zap.Int64("model_id", modelID))

	return nil, fmt.Errorf("remote embedding not yet configured")
}

// CallEmbeddingAPI calls an OpenAI-compatible embedding API directly.
// This is the low-level method used when base_url and api_key are known.
func (es *EmbeddingService) CallEmbeddingAPI(ctx context.Context, baseURL, apiKey, modelName, text string) ([]float64, error) {
	reqBody := embeddingAPIRequest{
		Model: modelName,
		Input: text,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	// Try /v1/embeddings first, fall back to /embeddings
	urls := []string{
		fmt.Sprintf("%s/v1/embeddings", baseURL),
		fmt.Sprintf("%s/embeddings", baseURL),
	}

	var lastErr error
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := es.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("embedding API returned status %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		var apiResp embeddingAPIResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			lastErr = fmt.Errorf("decode embedding response: %w", err)
			continue
		}

		if len(apiResp.Data) == 0 || len(apiResp.Data[0].Embedding) == 0 {
			lastErr = fmt.Errorf("empty embedding response")
			continue
		}

		return apiResp.Data[0].Embedding, nil
	}

	return nil, fmt.Errorf("all embedding API endpoints failed: %w", lastErr)
}

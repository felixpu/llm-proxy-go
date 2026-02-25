package testutil

import (
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// SampleModel returns a sample model for the given role.
func SampleModel(role models.ModelRole) *models.Model {
	switch role {
	case models.ModelRoleSimple:
		return &models.Model{
			ID:                1,
			Name:              "claude-3-haiku",
			Role:              models.ModelRoleSimple,
			CostPerMtokInput:  0.25,
			CostPerMtokOutput: 1.25,
			BillingMultiplier: 1.0,
			SupportsThinking:  false,
			Enabled:           true,
			Weight:            100,
			CreatedAt:         time.Now().UTC(),
		}
	case models.ModelRoleComplex:
		return &models.Model{
			ID:                3,
			Name:              "claude-opus-4",
			Role:              models.ModelRoleComplex,
			CostPerMtokInput:  15.0,
			CostPerMtokOutput: 75.0,
			BillingMultiplier: 1.0,
			SupportsThinking:  true,
			Enabled:           true,
			Weight:            100,
			CreatedAt:         time.Now().UTC(),
		}
	default: // ModelRoleDefault
		return &models.Model{
			ID:                2,
			Name:              "claude-sonnet-4",
			Role:              models.ModelRoleDefault,
			CostPerMtokInput:  3.0,
			CostPerMtokOutput: 15.0,
			BillingMultiplier: 1.0,
			SupportsThinking:  false,
			Enabled:           true,
			Weight:            100,
			CreatedAt:         time.Now().UTC(),
		}
	}
}

// SampleProvider returns a sample provider.
func SampleProvider() *models.Provider {
	now := time.Now().UTC()
	return &models.Provider{
		ID:            1,
		Name:          "anthropic-primary",
		BaseURL:       "https://api.anthropic.com",
		APIKey:        "sk-ant-test-key-1",
		Weight:        2,
		MaxConcurrent: 10,
		Enabled:       true,
		Description:   "Primary Anthropic Provider",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// SampleProviderDisabled returns a disabled provider.
func SampleProviderDisabled() *models.Provider {
	now := time.Now().UTC()
	return &models.Provider{
		ID:            3,
		Name:          "disabled-provider",
		BaseURL:       "https://disabled.example.com",
		APIKey:        "sk-disabled",
		Weight:        1,
		MaxConcurrent: 5,
		Enabled:       false,
		Description:   "Disabled Provider",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// SampleUser returns a sample user with the given role.
func SampleUser(role models.UserRole) *models.User {
	now := time.Now().UTC()
	switch role {
	case models.UserRoleAdmin:
		return &models.User{
			ID:           1,
			Username:     "admin",
			PasswordHash: "$2a$10$hashedpassword1",
			Role:         models.UserRoleAdmin,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
	default: // UserRoleUser
		return &models.User{
			ID:           2,
			Username:     "testuser",
			PasswordHash: "$2a$10$hashedpassword2",
			Role:         models.UserRoleUser,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
	}
}

// SampleUserInactive returns an inactive user.
func SampleUserInactive() *models.User {
	now := time.Now().UTC()
	return &models.User{
		ID:           3,
		Username:     "inactive",
		PasswordHash: "$2a$10$hashedpassword3",
		Role:         models.UserRoleUser,
		IsActive:     false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// SampleAPIKey returns a sample API key.
func SampleAPIKey(userID int64) *models.APIKey {
	now := time.Now().UTC()
	return &models.APIKey{
		ID:        1,
		UserID:    userID,
		KeyHash:   "hash_test_key_1",
		KeyFull:   "sk-test-full-key-12345",
		KeyPrefix: "sk-test",
		Name:      "Test API Key",
		IsActive:  true,
		CreatedAt: now,
	}
}

// SampleAPIKeyExpired returns an expired API key.
func SampleAPIKeyExpired(userID int64) *models.APIKey {
	now := time.Now().UTC()
	expired := now.Add(-24 * time.Hour)
	return &models.APIKey{
		ID:        2,
		UserID:    userID,
		KeyHash:   "hash_expired_key",
		KeyFull:   "sk-expired-key-12345",
		KeyPrefix: "sk-exp",
		Name:      "Expired Key",
		IsActive:  true,
		CreatedAt: now.Add(-48 * time.Hour),
		ExpiresAt: &expired,
	}
}

// SampleAPIKeyRevoked returns a revoked API key.
func SampleAPIKeyRevoked(userID int64) *models.APIKey {
	now := time.Now().UTC()
	return &models.APIKey{
		ID:        3,
		UserID:    userID,
		KeyHash:   "hash_revoked_key",
		KeyFull:   "sk-revoked-key-12345",
		KeyPrefix: "sk-rev",
		Name:      "Revoked Key",
		IsActive:  false,
		CreatedAt: now,
	}
}

// SampleRoutingModel returns a sample routing model.
func SampleRoutingModel(providerID int64) *models.RoutingModel {
	now := time.Now().UTC()
	return &models.RoutingModel{
		ID:                1,
		ProviderID:        providerID,
		ModelName:         "claude-sonnet-4",
		Enabled:           true,
		Priority:          10,
		CostPerMtokInput:  3.0,
		CostPerMtokOutput: 15.0,
		BillingMultiplier: 1.0,
		Description:       "Default routing model",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

// SampleRoutingConfig returns a sample routing configuration.
func SampleRoutingConfig() *models.RoutingConfig {
	primaryID := int64(1)
	return &models.RoutingConfig{
		Enabled:              true,
		PrimaryModelID:       &primaryID,
		TimeoutSeconds:       30,
		CacheEnabled:         true,
		CacheTTLSeconds:      300,
		CacheTTLL3Seconds:    604800,
		MaxTokens:            1024,
		Temperature:          0.0,
		RetryCount:           2,
		SemanticCacheEnabled: true,
		SimilarityThreshold:  0.82,
		LocalEmbeddingModel:  "paraphrase-multilingual-MiniLM-L12-v2",
		ForceSmartRouting:    false,
	}
}

// SampleEmbeddingModel returns a sample embedding model.
func SampleEmbeddingModel() *models.EmbeddingModel {
	now := time.Now().UTC()
	return &models.EmbeddingModel{
		ID:                 1,
		Name:               "paraphrase-multilingual-MiniLM-L12-v2",
		Dimension:          384,
		Description:        "Multilingual model",
		FastembedSupported: true,
		FastembedName:      "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2",
		IsBuiltin:          true,
		Enabled:            true,
		SortOrder:          0,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// SampleRequestLogEntry returns a sample request log entry for insertion.
func SampleRequestLogEntry(userID int64) *models.RequestLogEntry {
	return &models.RequestLogEntry{
		RequestID:    "req_test_12345",
		UserID:       userID,
		ModelName:    "claude-sonnet-4",
		EndpointName: "anthropic-primary",
		TaskType:     "default",
		InputTokens:  100,
		OutputTokens: 50,
		LatencyMs:    150.5,
		Cost:         0.001,
		Success:      true,
		Stream:       false,
	}
}

// SimpleRequest returns a simple task request body.
func SimpleRequest() map[string]any {
	return map[string]any{
		"model":      "claude-sonnet-4",
		"max_tokens": 1024,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": "read file config.yaml",
			},
		},
	}
}

// ComplexRequest returns a complex task request body.
func ComplexRequest() map[string]any {
	return map[string]any{
		"model":      "claude-sonnet-4",
		"max_tokens": 4096,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": "Design a microservices architecture for a high-traffic e-commerce platform with real-time inventory management, distributed caching, and event-driven communication between services.",
			},
		},
	}
}

// DefaultRequest returns a default task request body.
func DefaultRequest() map[string]any {
	return map[string]any{
		"model":      "claude-sonnet-4",
		"max_tokens": 2048,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": "Explain how to implement a binary search tree in Go with insert and search operations.",
			},
		},
	}
}

// StreamRequest returns a streaming request body.
func StreamRequest() map[string]any {
	return map[string]any{
		"model":      "claude-sonnet-4",
		"max_tokens": 1024,
		"stream":     true,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": "Write a short poem about coding.",
			},
		},
	}
}

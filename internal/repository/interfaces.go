// Package repository defines data access interfaces and implementations.
package repository

import (
	"context"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

// ModelRepository provides access to model data.
type ModelRepository interface {
	FindByID(ctx context.Context, id int64) (*models.Model, error)
	FindByName(ctx context.Context, name string) (*models.Model, error)
	FindByRole(ctx context.Context, role models.ModelRole) ([]*models.Model, error)
	FindAllEnabled(ctx context.Context) ([]*models.Model, error)
	FindAll(ctx context.Context) ([]*models.Model, error)
	Insert(ctx context.Context, m *models.Model) (int64, error)
	Update(ctx context.Context, id int64, updates map[string]any) error
	Delete(ctx context.Context, id int64) error
}

// ProviderRepository provides access to provider data.
type ProviderRepository interface {
	FindByID(ctx context.Context, id int64) (*models.Provider, error)
	FindByModelID(ctx context.Context, modelID int64) ([]*models.Provider, error)
	FindAllEnabled(ctx context.Context) ([]*models.Provider, error)
	FindAll(ctx context.Context) ([]*models.Provider, error)
	Insert(ctx context.Context, p *models.Provider, modelIDs []int64) (int64, error)
	Update(ctx context.Context, id int64, updates map[string]any, modelIDs []int64) error
	Delete(ctx context.Context, id int64) error
	GetModelIDsForProvider(ctx context.Context, providerID int64) ([]int64, error)
}

// APIKeyRepository provides access to API key data.
type APIKeyRepository interface {
	FindByKeyHash(ctx context.Context, keyHash string) (*models.APIKey, error)
	FindByID(ctx context.Context, id int64) (*models.APIKey, error)
	FindByUserID(ctx context.Context, userID int64) ([]*models.APIKey, error)
	FindAll(ctx context.Context) ([]*models.APIKey, error)
	Insert(ctx context.Context, key *models.APIKey) (int64, error)
	UpdateLastUsed(ctx context.Context, id int64) error
	Revoke(ctx context.Context, id int64, userID *int64) error
	SetActive(ctx context.Context, id int64, userID *int64, active bool) error
	Delete(ctx context.Context, id int64, userID *int64) error
	CleanupExpired(ctx context.Context) (int64, error)
}

// UserRepository provides access to user data.
type UserRepository interface {
	FindByID(ctx context.Context, id int64) (*models.User, error)
	FindByUsername(ctx context.Context, username string) (*models.User, error)
	FindByUsernameWithHash(ctx context.Context, username string) (*models.User, error)
	FindAll(ctx context.Context, offset, limit int) ([]*models.User, int64, error)
	Insert(ctx context.Context, user *models.User) (int64, error)
	Update(ctx context.Context, user *models.User) error
	UpdatePassword(ctx context.Context, userID int64, passwordHash string) error
	Delete(ctx context.Context, id int64) error
	CountByRole(ctx context.Context, role models.UserRole) (int64, error)
}

// RoutingRuleRepository provides access to routing rule data.
type RoutingRuleRepository interface {
	ListRules(ctx context.Context, enabledOnly bool) ([]*models.RoutingRule, error)
	GetRule(ctx context.Context, id int64) (*models.RoutingRule, error)
	AddRule(ctx context.Context, rule *models.RoutingRule) (int64, error)
	UpdateRule(ctx context.Context, id int64, updates map[string]any) error
	DeleteRule(ctx context.Context, id int64) error
	IncrementHitCount(ctx context.Context, id int64) error
	GetStats(ctx context.Context) (*models.RuleStats, error)
	ListBuiltinRules(ctx context.Context) ([]*models.RoutingRule, error)
	ListCustomRules(ctx context.Context) ([]*models.RoutingRule, error)
}

// RequestLogRepository provides access to request log data.
type RequestLogRepository interface {
	Insert(ctx context.Context, entry *models.RequestLogEntry) (int64, error)
	GetByID(ctx context.Context, id int64) (*models.RequestLog, error)
	List(ctx context.Context, limit, offset int, userID *int64, modelName, endpointName *string, startTime, endTime *time.Time, success *bool) ([]*models.RequestLog, int64, error)
	GetStatistics(ctx context.Context, startTime, endTime *time.Time, userID *int64, modelName, endpointName *string, success *bool) (*LogStatistics, error)
	Count(ctx context.Context, modelName, endpointName *string, startTime, endTime *time.Time) (int64, error)
	Delete(ctx context.Context, modelName, endpointName *string, startTime, endTime *time.Time) (int64, error)
	MarkInaccurate(ctx context.Context, id int64, inaccurate bool) error
	// GetRoutingAggregation returns routing method/rule counts via SQL aggregation.
	GetRoutingAggregation(ctx context.Context, startTime, endTime *time.Time) (*RoutingAggregation, error)
	// ListInaccurate returns inaccurate logs with pagination (SQL-level filtering).
	ListInaccurate(ctx context.Context, limit, offset int) ([]*models.RequestLog, int64, error)
	// ListForAnalysis returns logs with request_content for routing analysis.
	ListForAnalysis(ctx context.Context, startTime, endTime *time.Time, maxResults int) ([]*models.RequestLog, error)
	// GetEndpointModelStats returns historical stats grouped by endpoint_name/model_name.
	GetEndpointModelStats(ctx context.Context) (map[string]*EndpointModelStats, error)
}

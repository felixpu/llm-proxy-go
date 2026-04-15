package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/api/handler"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

func registerCacheRoutes(group *gin.RouterGroup, cacheHandler *handler.CacheHandler) {
	group.GET("/stats", cacheHandler.GetStats)
	group.GET("/stats/timeseries", cacheHandler.GetTimeseries)
	group.GET("/entries", cacheHandler.GetEntries)
	group.POST("/clear", cacheHandler.Clear)
	group.POST("/stats/reset", cacheHandler.ResetStats)
}

// Server wraps the HTTP server and dependencies.
type Server struct {
	router *gin.Engine
	logger *zap.Logger
}

type routingConfigRepository interface {
	GetConfig(ctx context.Context) (*models.RoutingConfig, error)
	UpdateConfigPatch(ctx context.Context, patch repository.RoutingConfigPatch) error
}

type modelRepository interface {
	FindByID(ctx context.Context, id int64) (*models.Model, error)
	FindByName(ctx context.Context, name string) (*models.Model, error)
	FindByRole(ctx context.Context, role models.ModelRole) ([]*models.Model, error)
	FindAllEnabled(ctx context.Context) ([]*models.Model, error)
	FindAll(ctx context.Context) ([]*models.Model, error)
	Insert(ctx context.Context, m *models.Model) (int64, error)
	UpdatePatch(ctx context.Context, id int64, patch repository.ModelPatch) error
	Delete(ctx context.Context, id int64) error
}

type providerRepository interface {
	FindByID(ctx context.Context, id int64) (*models.Provider, error)
	FindByModelID(ctx context.Context, modelID int64) ([]*models.Provider, error)
	FindAllEnabled(ctx context.Context) ([]*models.Provider, error)
	FindAll(ctx context.Context) ([]*models.Provider, error)
	Insert(ctx context.Context, p *models.Provider, modelIDs []int64) (int64, error)
	UpdatePatch(ctx context.Context, id int64, patch repository.ProviderPatch, modelIDs []int64) error
	Delete(ctx context.Context, id int64) error
	GetModelIDsForProvider(ctx context.Context, providerID int64) ([]int64, error)
}

type modelAliasRepository interface {
	FindByID(ctx context.Context, id int64) (*models.ModelAlias, error)
	FindByAliasName(ctx context.Context, aliasName string) (*models.ModelAlias, error)
	FindAll(ctx context.Context) ([]*models.ModelAlias, error)
	Insert(ctx context.Context, alias *models.ModelAlias) (int64, error)
	UpdatePatch(ctx context.Context, id int64, patch repository.ModelAliasPatch) error
	Delete(ctx context.Context, id int64) error
}

type routingModelRepository interface {
	ListModels(ctx context.Context, providerID *int64) ([]*models.RoutingModel, error)
	GetModel(ctx context.Context, id int64) (*models.RoutingModel, error)
	AddModel(ctx context.Context, m *models.RoutingModel) (int64, error)
	UpdateModelPatch(ctx context.Context, id int64, patch repository.RoutingModelPatch) error
	DeleteModel(ctx context.Context, id int64) error
}

type embeddingModelRepository interface {
	ListModels(ctx context.Context, enabledOnly bool) ([]*models.EmbeddingModel, error)
	GetModelByName(ctx context.Context, name string) (*models.EmbeddingModel, error)
	AddModel(ctx context.Context, m *models.EmbeddingModel) (int64, error)
	UpdateModelPatch(ctx context.Context, id int64, patch repository.EmbeddingModelPatch) error
	DeleteModel(ctx context.Context, id int64) error
}

type embeddingCacheRepository interface {
	Count(ctx context.Context) (int64, error)
	GetStats(ctx context.Context) (map[string]interface{}, error)
	GetTopEntries(ctx context.Context, sortBy string, limit int) ([]*repository.EmbeddingCacheEntry, error)
	DeleteAll(ctx context.Context) (int64, error)
}

type systemConfigRepository interface {
	GetRoutingConfig(ctx context.Context) (map[string]any, error)
	UpdateRoutingConfigPatch(ctx context.Context, patch repository.SystemRoutingConfigPatch) error
	GetLoadBalanceConfig(ctx context.Context) (map[string]any, error)
	UpdateLoadBalanceConfigPatch(ctx context.Context, patch repository.SystemLoadBalanceConfigPatch) error
	GetHealthCheckConfig(ctx context.Context) (map[string]any, error)
	UpdateHealthCheckConfigPatch(ctx context.Context, patch repository.SystemHealthCheckConfigPatch) error
	GetUIConfig(ctx context.Context) (map[string]any, error)
	UpdateUIConfigPatch(ctx context.Context, patch repository.SystemUIConfigPatch) error
}

type analysisReportRepository interface {
	List(ctx context.Context, limit, offset int) ([]*models.AnalysisReport, int, error)
	GetByID(ctx context.Context, id int64) (*models.AnalysisReport, error)
	Delete(ctx context.Context, id int64) error
}

type routingAnalysisLogRepository interface {
	repository.RequestLogQueryRepository
	repository.RequestLogAnalyticsRepository
	repository.RequestLogWriteRepository
}

// ServerDeps holds all dependencies for the API server.
type ServerDeps struct {
	ProxyService       *service.ProxyService
	AuthService        *service.AuthService
	HealthChecker      *service.HealthChecker
	RoutingCache       *service.RoutingCache
	LLMRouter          *service.LLMRouter
	RoutingAnalyzer    *service.RoutingAnalyzer
	UserRepo           repository.UserRepository
	KeyRepo            repository.APIKeyRepository
	LogWriteRepo       repository.RequestLogWriteRepository
	LogQueryRepo       repository.RequestLogQueryRepository
	LogAnalyticsRepo   repository.RequestLogAnalyticsRepository
	LogRoutingRepo     routingAnalysisLogRepository
	EmbeddingRepo      embeddingModelRepository
	ModelRepo          modelRepository
	ModelAliasRepo     modelAliasRepository
	ProviderRepo       providerRepository
	RoutingModelRepo   routingModelRepository
	RoutingConfigRepo  routingConfigRepository
	RoutingRuleRepo    repository.RoutingRuleRepository
	EmbeddingCacheRepo embeddingCacheRepository
	SystemConfigRepo   systemConfigRepository
	AnalysisReportRepo analysisReportRepository
	EndpointStore      *service.EndpointStore
	RateLimit          *middleware.RateLimitConfig
	DB                 *sql.DB
	Logger             *zap.Logger
}

// NewServer creates a new API server with all routes configured.
func NewServer(deps ServerDeps) *Server {
	logger := deps.Logger
	authService := deps.AuthService

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	registerGlobalMiddleware(r, deps, logger)
	registerPublicRoutes(r, deps.HealthChecker)

	endpointSelector := newEndpointSelector(deps, logger)
	registerProxyRoutes(r, deps, authService, endpointSelector, logger)
	registerAuthRoutes(r, authService, logger)
	registerUserRoutes(r, deps, authService)
	registerAPIKeyRoutes(r, deps, authService)

	routingAnalysisHandler := registerLogAndAnalysisRoutes(r, deps, authService, logger)
	if deps.RoutingAnalyzer != nil {
		routingAnalysisHandler.SetAnalyzer(deps.RoutingAnalyzer, deps.AnalysisReportRepo)
	}

	registerSystemLogRoutes(r, authService)
	registerStatusRoutes(r, deps, authService, endpointSelector)
	registerConfigRoutes(r, deps, authService, logger)
	registerCacheCompatibilityRoutes(r, deps, authService)
	registerNoRouteHandler(r)

	return &Server{
		router: r,
		logger: logger,
	}
}

func registerGlobalMiddleware(r *gin.Engine, deps ServerDeps, logger *zap.Logger) {
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.RateLimit(deps.RateLimit))
	r.Use(middleware.CSRF(nil))
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyEndpoints, deps.EndpointStore.GetEndpoints())
		c.Next()
	})
}

func registerPublicRoutes(r *gin.Engine, healthChecker *service.HealthChecker) {
	r.GET("/api/docs/openapi.yaml", handler.ServeOpenAPISpec)
	healthHandler := handler.NewHealthHandler(healthChecker)
	r.GET("/api/health", healthHandler.Health)
}

func newEndpointSelector(deps ServerDeps, logger *zap.Logger) *service.EndpointSelector {
	modelSelector := service.NewModelSelector(deps.HealthChecker, logger)
	loadBalancer := service.NewLoadBalancer(deps.SystemConfigRepo)
	loadBalancer.SetStateReader(deps.HealthChecker)
	return service.NewEndpointSelector(
		modelSelector,
		deps.HealthChecker,
		loadBalancer,
		deps.LLMRouter,
		deps.RoutingConfigRepo,
		deps.ModelAliasRepo,
		logger,
	)
}

func registerProxyRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService, endpointSelector *service.EndpointSelector, logger *zap.Logger) {
	proxyHandler := handler.NewProxyHandler(deps.ProxyService, authService, endpointSelector, deps.RoutingConfigRepo, logger)
	v1 := r.Group("/v1")
	{
		v1.POST("/messages", proxyHandler.Messages)
	}
}

func registerAuthRoutes(r *gin.Engine, authService *service.AuthService, logger *zap.Logger) {
	authHandler := handler.NewAuthHandler(authService, logger)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/logout", authHandler.Logout)
		authGroup.GET("/me", middleware.RequireAuth(authService), authHandler.GetMe)
		authGroup.POST("/refresh", middleware.RequireAuth(authService), authHandler.Refresh)
	}
}

func registerUserRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService) {
	userHandler := handler.NewUserHandler(deps.UserRepo, authService)
	userGroup := r.Group("/api/users")
	userGroup.Use(middleware.RequireAuth(authService))
	{
		userGroup.GET("/me", userHandler.GetCurrentUser)
		userGroup.POST("/change-password", userHandler.ChangePassword)
		adminGroup := userGroup.Group("")
		adminGroup.Use(middleware.RequireAdmin())
		{
			adminGroup.GET("", userHandler.ListUsers)
			adminGroup.GET("/:id", userHandler.GetUser)
			adminGroup.POST("", userHandler.CreateUser)
			adminGroup.PATCH("/:id", userHandler.UpdateUser)
			adminGroup.DELETE("/:id", userHandler.DeleteUser)
			adminGroup.POST("/:id/password", userHandler.AdminChangePassword)
		}
	}
}

func registerAPIKeyRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService) {
	keyHandler := handler.NewAPIKeyHandler(deps.KeyRepo)
	keyGroup := r.Group("/api/keys")
	keyGroup.Use(middleware.RequireAuth(authService))
	{
		keyGroup.GET("", keyHandler.ListAPIKeys)
		keyGroup.POST("", keyHandler.CreateAPIKey)
		keyGroup.GET("/:id", keyHandler.GetAPIKey)
		keyGroup.POST("/:id/revoke", keyHandler.RevokeAPIKey)
		keyGroup.POST("/:id/toggle", keyHandler.ToggleAPIKey)
		keyGroup.DELETE("/:id", keyHandler.DeleteAPIKey)
	}
}

func registerLogAndAnalysisRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService, logger *zap.Logger) *handler.RoutingAnalysisHandler {
	logsHandler := handler.NewLogsHandler(deps.LogQueryRepo, logger)
	routingAnalysisHandler := handler.NewRoutingAnalysisHandler(deps.LogRoutingRepo, deps.RoutingRuleRepo, logger)
	logsGroup := r.Group("/api/logs")
	logsGroup.Use(middleware.RequireAuth(authService))
	logsGroup.Use(middleware.RequireAdmin())
	{
		logsGroup.GET("", logsHandler.GetRequestLogs)
		logsGroup.DELETE("", logsHandler.DeleteRequestLogs)
		logsGroup.GET("/stats", logsHandler.GetLogStats)
		logsGroup.GET("/:id", routingAnalysisHandler.GetLogDetail)
		logsGroup.POST("/:id/mark-inaccurate", routingAnalysisHandler.MarkLogInaccurate)
	}

	// Routing analysis endpoints (admin only).
	routingAnalysisGroup := r.Group("/api/routing/analysis")
	routingAnalysisGroup.Use(middleware.RequireAuth(authService))
	routingAnalysisGroup.Use(middleware.RequireAdmin())
	{
		routingAnalysisGroup.GET("/stats", routingAnalysisHandler.GetRoutingStats)
		routingAnalysisGroup.GET("/inaccurate", routingAnalysisHandler.GetInaccurateLogs)
		routingAnalysisGroup.GET("/export", routingAnalysisHandler.ExportRoutingData)
		routingAnalysisGroup.POST("/analyze", routingAnalysisHandler.StartAnalysis)
		routingAnalysisGroup.GET("/task/:task_id", routingAnalysisHandler.GetAnalysisTask)
		routingAnalysisGroup.GET("/reports", routingAnalysisHandler.ListAnalysisReports)
		routingAnalysisGroup.GET("/reports/:id", routingAnalysisHandler.GetAnalysisReport)
		routingAnalysisGroup.DELETE("/reports/:id", routingAnalysisHandler.DeleteAnalysisReport)
	}
	return routingAnalysisHandler
}

func registerSystemLogRoutes(r *gin.Engine, authService *service.AuthService) {
	systemLogsGroup := r.Group("/api/system-logs")
	systemLogsGroup.Use(middleware.RequireAuth(authService))
	{
		systemLogsGroup.GET("", handler.GetSystemLogEntries)
		systemLogsGroup.GET("/stream", handler.StreamSystemLogs)
		adminSystemLogsGroup := systemLogsGroup.Group("")
		adminSystemLogsGroup.Use(middleware.RequireAdmin())
		{
			adminSystemLogsGroup.POST("/clear", handler.ClearSystemLogEntries)
		}
	}
}

func registerStatusRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService, endpointSelector *service.EndpointSelector) {
	statusHandler := handler.NewStatusHandler(deps.HealthChecker, deps.ModelRepo, deps.LogAnalyticsRepo, deps.LLMRouter, deps.EndpointStore, endpointSelector)
	statusGroup := r.Group("/api")
	statusGroup.Use(middleware.RequireAuth(authService))
	{
		statusGroup.GET("/status", statusHandler.GetSystemStatus)
		statusGroup.GET("/routing/debug", statusHandler.GetRoutingDebug)
		statusGroup.POST("/routing/test", statusHandler.TestRouting)
		adminStatusGroup := statusGroup.Group("")
		adminStatusGroup.Use(middleware.RequireAdmin())
		{
			adminStatusGroup.POST("/health/check-now", statusHandler.TriggerHealthCheck)
		}
	}
}

func registerConfigRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService, logger *zap.Logger) {
	configHandler := handler.NewConfigHandler(deps.SystemConfigRepo, deps.HealthChecker)
	routingHandler := handler.NewRoutingHandler(deps.RoutingModelRepo, deps.RoutingConfigRepo)
	modelHandler := handler.NewModelHandler(deps.ModelRepo, deps.EndpointStore)
	modelAliasHandler := handler.NewModelAliasHandler(deps.ModelAliasRepo, deps.ModelRepo)
	providerHandler := handler.NewProviderHandler(deps.ProviderRepo, deps.ModelRepo, service.NewModelDetector(logger), deps.EndpointStore)
	configGroup := r.Group("/api/config")
	configGroup.Use(middleware.RequireAuth(authService))
	configGroup.Use(middleware.RequireAdmin())
	{
		// System config (routing/load-balance/health-check/ui)
		configGroup.GET("/routing", configHandler.GetRoutingConfig)
		configGroup.PUT("/routing", configHandler.UpdateRoutingConfig)
		configGroup.GET("/load-balance", configHandler.GetLoadBalanceConfig)
		configGroup.PUT("/load-balance", configHandler.UpdateLoadBalanceConfig)
		configGroup.GET("/health-check", configHandler.GetHealthCheckConfig)
		configGroup.PUT("/health-check", configHandler.UpdateHealthCheckConfig)
		configGroup.GET("/ui", configHandler.GetUIConfig)
		configGroup.PUT("/ui", configHandler.UpdateUIConfig)

		// Config reload / migrate / legacy
		configGroup.POST("/reload", handler.ReloadConfig)
		configGroup.POST("/migrate", handler.MigrateConfig)
		configGroup.GET("/endpoints", handler.ListEndpoints)
		configGroup.POST("/endpoints", handler.CreateEndpoint)
		configGroup.DELETE("/endpoints/:endpoint_id", handler.DeleteEndpoint)

		// Backup / restore
		backupHandler := handler.NewBackupHandler(deps.DB, deps.EndpointStore)
		configGroup.GET("/backup/export", backupHandler.Export)
		configGroup.POST("/backup/import", backupHandler.Import)

		// Model management
		configGroup.GET("/models", modelHandler.ListModels)
		configGroup.GET("/models/:model_id", modelHandler.GetModel)
		configGroup.POST("/models", modelHandler.CreateModel)
		configGroup.PUT("/models/:model_id", modelHandler.UpdateModel)
		configGroup.DELETE("/models/:model_id", modelHandler.DeleteModel)

		// Model alias management
		configGroup.GET("/model-aliases", modelAliasHandler.ListModelAliases)
		configGroup.GET("/model-aliases/:alias_id", modelAliasHandler.GetModelAlias)
		configGroup.POST("/model-aliases", modelAliasHandler.CreateModelAlias)
		configGroup.PUT("/model-aliases/:alias_id", modelAliasHandler.UpdateModelAlias)
		configGroup.DELETE("/model-aliases/:alias_id", modelAliasHandler.DeleteModelAlias)

		// Provider management
		configGroup.GET("/providers", providerHandler.ListProviders)
		configGroup.GET("/providers/:provider_id", providerHandler.GetProvider)
		configGroup.POST("/providers", providerHandler.CreateProvider)
		configGroup.PUT("/providers/:provider_id", providerHandler.UpdateProvider)
		configGroup.DELETE("/providers/:provider_id", providerHandler.DeleteProvider)
		configGroup.GET("/providers/:provider_id/models", providerHandler.GetProviderModels)
		configGroup.POST("/detect-models", providerHandler.DetectModels)

		// Routing model management
		configGroup.GET("/routing/models", routingHandler.ListRoutingModels)
		configGroup.GET("/routing/models/:model_id", routingHandler.GetRoutingModel)
		configGroup.POST("/routing/models", routingHandler.CreateRoutingModel)
		configGroup.PUT("/routing/models/:model_id", routingHandler.UpdateRoutingModel)
		configGroup.DELETE("/routing/models/:model_id", routingHandler.DeleteRoutingModel)
		configGroup.GET("/routing/llm-config", routingHandler.GetLLMRoutingConfig)
		configGroup.PUT("/routing/llm-config", routingHandler.UpdateLLMRoutingConfig)

		// Routing rule management
		ruleHandler := handler.NewRoutingRuleHandler(deps.RoutingRuleRepo, logger)
		configGroup.GET("/routing/rules", ruleHandler.ListRules)
		configGroup.GET("/routing/rules/builtin", ruleHandler.ListBuiltinRules)
		configGroup.GET("/routing/rules/custom", ruleHandler.ListCustomRules)
		configGroup.GET("/routing/rules/stats", ruleHandler.GetStats)
		configGroup.POST("/routing/rules/test", ruleHandler.TestMessage)
		configGroup.GET("/routing/rules/:rule_id", ruleHandler.GetRule)
		configGroup.POST("/routing/rules", ruleHandler.CreateRule)
		configGroup.PUT("/routing/rules/:rule_id", ruleHandler.UpdateRule)
		configGroup.DELETE("/routing/rules/:rule_id", ruleHandler.DeleteRule)

		// Embedding model management
		embeddingHandler := handler.NewEmbeddingHandler(deps.EmbeddingRepo)
		configGroup.GET("/embedding/models", embeddingHandler.ListModels)
		configGroup.POST("/embedding/models", embeddingHandler.CreateModel)
		configGroup.PUT("/embedding/models/:model_id", embeddingHandler.UpdateModel)
		configGroup.DELETE("/embedding/models/:model_id", embeddingHandler.DeleteModel)
		configGroup.GET("/embedding/local-models", embeddingHandler.ListLocalModels)
		configGroup.GET("/embedding/local-models/:model_name/status", embeddingHandler.GetModelStatus)
		configGroup.POST("/embedding/local-models/:model_name/download", embeddingHandler.DownloadModel)
		configGroup.DELETE("/embedding/local-models/:model_name", embeddingHandler.DeleteLocalModel)

		// Cache monitoring
		cacheHandler := handler.NewCacheHandler(deps.RoutingCache, deps.EmbeddingCacheRepo)
		registerCacheRoutes(configGroup.Group("/cache"), cacheHandler)
	}
}

func registerCacheCompatibilityRoutes(r *gin.Engine, deps ServerDeps, authService *service.AuthService) {
	cacheGroup := r.Group("/api/cache")
	cacheGroup.Use(middleware.RequireAuth(authService))
	cacheGroup.Use(middleware.RequireAdmin())
	{
		cacheHandler := handler.NewCacheHandler(deps.RoutingCache, deps.EmbeddingCacheRepo)
		registerCacheRoutes(cacheGroup, cacheHandler)
	}
}

func registerNoRouteHandler(r *gin.Engine) {
	serveFrontend := handler.ServeFrontend()

	r.NoRoute(func(c *gin.Context) {
		if isAPIPath(c.Request.URL.Path) {
			c.JSON(http.StatusNotFound, gin.H{"detail": "route not found"})
			return
		}

		serveFrontend(c)
	})
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Run starts the HTTP server.
func (s *Server) Run(addr string) error {
	s.logger.Info("starting server", zap.String("addr", addr))
	return s.router.Run(addr)
}

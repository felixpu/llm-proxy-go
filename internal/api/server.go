package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/api/handler"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

// Server wraps the HTTP server and dependencies.
type Server struct {
	router *gin.Engine
	logger *zap.Logger
}

// ServerDeps holds all dependencies for the API server.
type ServerDeps struct {
	ProxyService     *service.ProxyService
	AuthService      *service.AuthService
	HealthChecker    *service.HealthChecker
	RoutingCache     *service.RoutingCache
	LLMRouter        *service.LLMRouter
	RoutingAnalyzer  *service.RoutingAnalyzer
	UserRepo         repository.UserRepository
	KeyRepo          repository.APIKeyRepository
	LogRepo          repository.RequestLogRepository
	EmbeddingRepo    *repository.EmbeddingModelRepository
	ModelRepo        *repository.SQLModelRepository
	ProviderRepo     *repository.SQLProviderRepository
	RoutingModelRepo *repository.RoutingModelRepository
	RoutingConfigRepo *repository.RoutingConfigRepository
	RoutingRuleRepo   *repository.RoutingRuleRepo
	EmbeddingCacheRepo *repository.EmbeddingCacheRepository
	SystemConfigRepo *repository.SystemConfigRepository
	AnalysisReportRepo *repository.AnalysisReportRepository
	EndpointStore    *service.EndpointStore
	RateLimit        *middleware.RateLimitConfig
	DB               *sql.DB
	Logger           *zap.Logger
}

// NewServer creates a new API server with all routes configured.
func NewServer(deps ServerDeps) *Server {
	logger := deps.Logger
	authService := deps.AuthService

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware.
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.RateLimit(deps.RateLimit))
	r.Use(middleware.CSRF(nil))

	// Inject endpoints into context for proxy handler (dynamic per-request).
	r.Use(func(c *gin.Context) {
		c.Set("endpoints", deps.EndpointStore.GetEndpoints())
		c.Next()
	})

	// OpenAPI spec (no auth, public documentation).
	r.GET("/api/docs/openapi.yaml", handler.ServeOpenAPISpec)

	// Health check (no auth).
	healthHandler := handler.NewHealthHandler(deps.HealthChecker)
	r.GET("/api/health", healthHandler.Health)

	// Create ModelSelector and EndpointSelector
	modelSelector := service.NewModelSelector(deps.HealthChecker, logger)
	loadBalancer := service.NewLoadBalancer(deps.SystemConfigRepo)
	endpointSelector := service.NewEndpointSelector(
		modelSelector,
		deps.HealthChecker,
		loadBalancer,
		deps.LLMRouter,
		deps.RoutingConfigRepo,
		logger,
	)

	// Proxy endpoint (API key auth).
	proxyHandler := handler.NewProxyHandler(deps.ProxyService, authService, endpointSelector, deps.RoutingConfigRepo, logger)
	v1 := r.Group("/v1")
	{
		v1.POST("/messages", proxyHandler.Messages)
	}

	// Auth endpoints.
	authHandler := handler.NewAuthHandler(authService, logger)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/logout", authHandler.Logout)
		authGroup.GET("/me", middleware.RequireAuth(authService), authHandler.GetMe)
		authGroup.POST("/refresh", middleware.RequireAuth(authService), authHandler.Refresh)
	}

	// User management endpoints.
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

	// API Key management endpoints.
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

	// Logs endpoints (admin only).
	logsHandler := handler.NewLogsHandler(deps.LogRepo, logger)
	routingAnalysisHandler := handler.NewRoutingAnalysisHandler(deps.LogRepo, deps.RoutingRuleRepo, logger)
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
	}

	// Set analyzer on handler after route registration.
	if deps.RoutingAnalyzer != nil {
		routingAnalysisHandler.SetAnalyzer(deps.RoutingAnalyzer, deps.AnalysisReportRepo)
	}

	// System logs endpoints.
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

	// Admin status endpoints.
	statusHandler := handler.NewStatusHandler(deps.HealthChecker, deps.ModelRepo, deps.LogRepo, deps.LLMRouter, deps.EndpointStore)
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

	// Admin config endpoints (admin only).
	configHandler := handler.NewConfigHandler(deps.SystemConfigRepo)
	routingHandler := handler.NewRoutingHandler(deps.RoutingModelRepo, deps.RoutingConfigRepo)
	modelHandler := handler.NewModelHandler(deps.ModelRepo, deps.EndpointStore)
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
		configGroup.GET("/cache/stats", cacheHandler.GetStats)
		configGroup.GET("/cache/stats/timeseries", cacheHandler.GetTimeseries)
		configGroup.GET("/cache/entries", cacheHandler.GetEntries)
		configGroup.POST("/cache/clear", cacheHandler.Clear)
		configGroup.POST("/cache/stats/reset", cacheHandler.ResetStats)
	}

	// Cache monitoring routes (frontend uses /api/cache/ path).
	cacheGroup := r.Group("/api/cache")
	cacheGroup.Use(middleware.RequireAuth(authService))
	cacheGroup.Use(middleware.RequireAdmin())
	{
		cachePublicHandler := handler.NewCacheHandler(deps.RoutingCache, deps.EmbeddingCacheRepo)
		cacheGroup.GET("/stats", cachePublicHandler.GetStats)
		cacheGroup.GET("/stats/timeseries", cachePublicHandler.GetTimeseries)
		cacheGroup.GET("/entries", cachePublicHandler.GetEntries)
		cacheGroup.POST("/clear", cachePublicHandler.Clear)
		cacheGroup.POST("/stats/reset", cachePublicHandler.ResetStats)
	}

	// SPA frontend: all unmatched routes serve index.html.
	r.NoRoute(handler.ServeFrontend())

	return &Server{
		router: r,
		logger: logger,
	}
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

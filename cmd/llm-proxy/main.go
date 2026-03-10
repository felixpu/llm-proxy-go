package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/user/llm-proxy-go/internal/api"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/database"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/internal/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type appRepositories struct {
	modelRepo          *repository.SQLModelRepository
	modelAliasRepo     *repository.SQLModelAliasRepository
	providerRepo       *repository.SQLProviderRepository
	keyRepo            repository.APIKeyRepository
	userRepo           repository.UserRepository
	logWriteRepo       repository.RequestLogWriteRepository
	logQueryRepo       repository.RequestLogQueryRepository
	logAnalyticsRepo   repository.RequestLogAnalyticsRepository
	logRoutingRepo     repository.RequestLogRepository
	embeddingRepo      *repository.EmbeddingModelRepository
	routingModelRepo   *repository.RoutingModelRepository
	routingConfigRepo  *repository.RoutingConfigRepository
	embeddingCacheRepo *repository.EmbeddingCacheRepository
	routingRuleRepo    *repository.RoutingRuleRepo
	systemConfigRepo   *repository.SystemConfigRepository
	sessionRepo        *repository.SessionRepository
	analysisReportRepo *repository.AnalysisReportRepository
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println(version.Info())
			os.Exit(0)
		case "--init":
			if err := runInit(); err != nil {
				log.Fatalf("init: %v", err)
			}
			os.Exit(0)
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		}
	}
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func printUsage() {
	fmt.Printf("LLM Proxy Go - %s\n\n", version.Short())
	fmt.Println("Usage: llm-proxy [OPTIONS]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --init         Generate .env.example configuration template")
	fmt.Println("  --version, -v  Show version information")
	fmt.Println("  --help, -h     Show this help message")
	fmt.Println()
	fmt.Println("Without options, starts the LLM proxy server.")
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Println("  Use environment variables or .env file (see .env.example)")
	fmt.Println("  Run 'llm-proxy --init' to generate configuration template")
}

func run() error {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Initialize logger.
	logDir := getLogDir()
	logger, err := newLogger(cfg.Proxy.LogLevel, logDir, cfg.LogRotation)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("starting llm-proxy",
		zap.String("version", version.Short()),
		zap.String("host", cfg.Proxy.Host),
		zap.Int("port", cfg.Proxy.Port),
	)

	db, readDB, err := initDatabases(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer db.Close()
	defer readDB.Close()

	// Run migrations.
	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	repos := initRepositories(db, readDB, logger)

	// Initialize worker coordinator for multi-worker support.
	workerCoordinator := service.NewWorkerCoordinator(db, logger)
	if err := workerCoordinator.Register(context.Background()); err != nil {
		logger.Warn("failed to register worker", zap.Error(err))
	}
	workerCoordinator.Start(context.Background())
	defer func() {
		if err := workerCoordinator.Unregister(context.Background()); err != nil {
			logger.Warn("failed to unregister worker", zap.Error(err))
		}
	}()

	logger.Info("worker coordinator initialized",
		zap.String("worker_id", workerCoordinator.WorkerID()),
		zap.Bool("is_primary", workerCoordinator.IsPrimary()))

	// Initialize endpoint store.
	endpointStore := service.NewEndpointStore(repos.modelRepo, repos.providerRepo, logger)
	if err := endpointStore.Load(context.Background()); err != nil {
		return fmt.Errorf("load endpoints: %w", err)
	}

	// Initialize services.
	healthChecker := service.NewHealthChecker(cfg.HealthCheck, logger)
	loadBalancer := service.NewLoadBalancer(repos.systemConfigRepo)
	authService := service.NewAuthService(repos.keyRepo, repos.userRepo, repos.sessionRepo, logger)
	proxyService := service.NewProxyService(healthChecker, loadBalancer, repos.logWriteRepo, logger)

	// Create default admin user if not exists.
	if err := authService.CreateDefaultAdmin(
		context.Background(),
		cfg.Security.DefaultAdmin.Username,
		cfg.Security.DefaultAdmin.Password,
	); err != nil {
		logger.Warn("failed to create default admin", zap.Error(err))
	}

	// Start health checker with current endpoints.
	healthChecker.Start(endpointStore.GetEndpoints())
	endpointStore.SetHealthChecker(healthChecker)
	defer healthChecker.Stop()

	// Initialize routing cache.
	routingCache := service.NewRoutingCache(10000, logger)

	// Initialize LLM router for intelligent routing.
	llmRouter := service.NewLLMRouter(db, nil, logger)

	// Initialize routing analyzer for rule optimization.
	routingAnalyzer := service.NewRoutingAnalyzer(repos.logAnalyticsRepo, repos.routingRuleRepo, repos.routingModelRepo, repos.analysisReportRepo, logger)

	// Create HTTP server.
	server := api.NewServer(buildServerDeps(cfg, db, logger, repos, proxyService, authService, healthChecker, routingCache, llmRouter, routingAnalyzer, endpointStore))

	// Start server in goroutine.
	addr := fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
	httpServer := newHTTPServer(addr, server)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	logger.Info("server started", zap.String("addr", addr))

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func initDatabases(dbPath string) (*sql.DB, *sql.DB, error) {
	db, err := database.New(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("init database: %w", err)
	}
	readDB, err := database.NewReadOnly(dbPath)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("init read-only database: %w", err)
	}
	return db, readDB, nil
}

func initRepositories(db, readDB *sql.DB, logger *zap.Logger) appRepositories {
	logRepoImpl := repository.NewRequestLogRepositoryImpl(db, logger, readDB)
	return appRepositories{
		modelRepo:          repository.NewModelRepository(db),
		modelAliasRepo:     repository.NewModelAliasRepository(db, readDB),
		providerRepo:       repository.NewProviderRepository(db),
		keyRepo:            repository.NewAPIKeyRepository(db, readDB),
		userRepo:           repository.NewUserRepository(db, readDB),
		logWriteRepo:       logRepoImpl,
		logQueryRepo:       logRepoImpl,
		logAnalyticsRepo:   logRepoImpl,
		logRoutingRepo:     logRepoImpl,
		embeddingRepo:      repository.NewEmbeddingModelRepository(db, logger),
		routingModelRepo:   repository.NewRoutingModelRepository(db, logger),
		routingConfigRepo:  repository.NewRoutingConfigRepository(db, logger, readDB),
		embeddingCacheRepo: repository.NewEmbeddingCacheRepository(db, logger),
		routingRuleRepo:    repository.NewRoutingRuleRepository(db, logger),
		systemConfigRepo:   repository.NewSystemConfigRepository(db, readDB),
		sessionRepo:        repository.NewSessionRepository(db, logger),
		analysisReportRepo: repository.NewAnalysisReportRepository(db, logger, readDB),
	}
}

func buildServerDeps(
	cfg *config.Config,
	db *sql.DB,
	logger *zap.Logger,
	repos appRepositories,
	proxyService *service.ProxyService,
	authService *service.AuthService,
	healthChecker *service.HealthChecker,
	routingCache *service.RoutingCache,
	llmRouter *service.LLMRouter,
	routingAnalyzer *service.RoutingAnalyzer,
	endpointStore *service.EndpointStore,
) api.ServerDeps {
	return api.ServerDeps{
		ProxyService:       proxyService,
		AuthService:        authService,
		HealthChecker:      healthChecker,
		RoutingCache:       routingCache,
		LLMRouter:          llmRouter,
		RoutingAnalyzer:    routingAnalyzer,
		UserRepo:           repos.userRepo,
		KeyRepo:            repos.keyRepo,
		LogWriteRepo:       repos.logWriteRepo,
		LogQueryRepo:       repos.logQueryRepo,
		LogAnalyticsRepo:   repos.logAnalyticsRepo,
		LogRoutingRepo:     repos.logRoutingRepo,
		EmbeddingRepo:      repos.embeddingRepo,
		ModelRepo:          repos.modelRepo,
		ModelAliasRepo:     repos.modelAliasRepo,
		ProviderRepo:       repos.providerRepo,
		RoutingModelRepo:   repos.routingModelRepo,
		RoutingConfigRepo:  repos.routingConfigRepo,
		RoutingRuleRepo:    repos.routingRuleRepo,
		EmbeddingCacheRepo: repos.embeddingCacheRepo,
		SystemConfigRepo:   repos.systemConfigRepo,
		AnalysisReportRepo: repos.analysisReportRepo,
		EndpointStore:      endpointStore,
		RateLimit: &middleware.RateLimitConfig{
			Enabled:       cfg.RateLimit.Enabled,
			MaxRequests:   cfg.RateLimit.MaxRequests,
			WindowSeconds: cfg.RateLimit.WindowSeconds,
			ExemptPaths:   middleware.DefaultRateLimitConfig().ExemptPaths,
		},
		DB:     db,
		Logger: logger,
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // streaming responses need a long write timeout
		IdleTimeout:  120 * time.Second,
	}
}

func newLogger(level string, logDir string, rotation config.LogRotationConfig) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	switch level {
	case "debug", "DEBUG":
		zapLevel = zap.DebugLevel
	case "warn", "WARN":
		zapLevel = zap.WarnLevel
	case "error", "ERROR":
		zapLevel = zap.ErrorLevel
	default:
		zapLevel = zap.InfoLevel
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir %s: %w", logDir, err)
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "llm-proxy.log"),
		MaxSize:    rotation.MaxSizeMB,
		MaxBackups: rotation.MaxBackups,
		MaxAge:     rotation.MaxAgeDays,
		Compress:   rotation.Compress,
	}

	// File core: JSON encoder for structured log parsing
	fileEncoderCfg := zap.NewProductionEncoderConfig()
	fileEncoderCfg.TimeKey = "ts"
	fileEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(fileEncoderCfg),
		zapcore.AddSync(lj),
		zapLevel,
	)

	// Console core: human-readable output to stdout/stderr
	consoleEncoderCfg := zap.NewDevelopmentEncoderConfig()
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05")
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderCfg)

	// stdout for DEBUG/INFO, stderr for WARN/ERROR+
	stdoutCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.Lock(os.Stdout),
		zap.LevelEnablerFunc(func(l zapcore.Level) bool {
			return l >= zapLevel && l < zapcore.WarnLevel
		}),
	)
	stderrCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.Lock(os.Stderr),
		zap.LevelEnablerFunc(func(l zapcore.Level) bool {
			return l >= zapLevel && l >= zapcore.WarnLevel
		}),
	)

	core := zapcore.NewTee(fileCore, stdoutCore, stderrCore)

	return zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	), nil
}

func getLogDir() string {
	if dir := os.Getenv("LLM_PROXY_LOGS_DIR"); dir != "" {
		return dir
	}
	return "logs"
}

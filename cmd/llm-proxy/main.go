package main

import (
	"context"
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

	// Initialize database.
	db, err := database.New(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer db.Close()

	// Initialize read-only database pool for query-heavy workloads (log stats, list).
	// This prevents expensive analytical queries from starving proxy auth/write operations.
	readDB, err := database.NewReadOnly(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("init read-only database: %w", err)
	}
	defer readDB.Close()

	// Run migrations.
	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Initialize repositories.
	modelRepo := repository.NewModelRepository(db)
	providerRepo := repository.NewProviderRepository(db)
	keyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	logRepo := repository.NewRequestLogRepositoryImpl(db, logger, readDB)
	embeddingRepo := repository.NewEmbeddingModelRepository(db, logger)
	routingModelRepo := repository.NewRoutingModelRepository(db, logger)
	routingConfigRepo := repository.NewRoutingConfigRepository(db, logger)
	embeddingCacheRepo := repository.NewEmbeddingCacheRepository(db, logger)
	routingRuleRepo := repository.NewRoutingRuleRepository(db, logger)
	systemConfigRepo := repository.NewSystemConfigRepository(db)

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
	endpointStore := service.NewEndpointStore(modelRepo, providerRepo, logger)
	if err := endpointStore.Load(context.Background()); err != nil {
		return fmt.Errorf("load endpoints: %w", err)
	}

	// Initialize services.
	sessionRepo := repository.NewSessionRepository(db, logger)
	healthChecker := service.NewHealthChecker(cfg.HealthCheck, logger)
	loadBalancer := service.NewLoadBalancer(systemConfigRepo)
	authService := service.NewAuthService(keyRepo, userRepo, sessionRepo, logger)
	proxyService := service.NewProxyService(healthChecker, loadBalancer, logRepo, logger)

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

	// Create HTTP server.
	server := api.NewServer(api.ServerDeps{
		ProxyService:       proxyService,
		AuthService:        authService,
		HealthChecker:      healthChecker,
		RoutingCache:       routingCache,
		LLMRouter:          llmRouter,
		UserRepo:           userRepo,
		KeyRepo:            keyRepo,
		LogRepo:            logRepo,
		EmbeddingRepo:      embeddingRepo,
		ModelRepo:          modelRepo,
		ProviderRepo:       providerRepo,
		RoutingModelRepo:   routingModelRepo,
		RoutingConfigRepo:  routingConfigRepo,
		RoutingRuleRepo:    routingRuleRepo,
		EmbeddingCacheRepo: embeddingCacheRepo,
		SystemConfigRepo:   systemConfigRepo,
		EndpointStore:      endpointStore,
		RateLimit: &middleware.RateLimitConfig{
			Enabled:       cfg.RateLimit.Enabled,
			MaxRequests:   cfg.RateLimit.MaxRequests,
			WindowSeconds: cfg.RateLimit.WindowSeconds,
		},
		DB:     db,
		Logger: logger,
	})

	// Start server in goroutine.
	addr := fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // streaming responses need a long write timeout
		IdleTimeout:  120 * time.Second,
	}

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

package config

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/user/llm-proxy-go/internal/pkg/paths"
)

// Load loads configuration with 3-tier priority:
// Environment variables > SQLite database > Default values
func Load() (*Config, error) {
	// Load .env file if exists
	loadDotEnv()

	// Start with defaults
	cfg := DefaultConfig()

	// Set database path
	cfg.Database.Path = paths.GetDBPath()

	// Try loading from SQLite
	if err := loadFromDatabase(cfg); err != nil {
		log.Printf("WARN: Failed to load config from database: %v", err)
	}

	// Apply environment variable overrides (highest priority)
	applyEnvOverrides(cfg)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// loadDotEnv loads .env file from the project root.
func loadDotEnv() {
	envFile := filepath.Join(paths.GetBasePath(), ".env")
	data, err := os.ReadFile(envFile)
	if err != nil {
		return // .env file is optional
	}

	// Simple .env parser: KEY=VALUE lines
	for _, line := range splitLines(string(data)) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if idx := indexOf(line, '='); idx > 0 {
			key := trimSpace(line[:idx])
			val := trimSpace(line[idx+1:])
			// Remove surrounding quotes
			val = trimQuotes(val)
			// Only set if not already set (env vars take precedence)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

// loadFromDatabase loads configuration from SQLite database.
func loadFromDatabase(cfg *Config) error {
	dbPath := cfg.Database.Path
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil // Database doesn't exist yet, use defaults
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Load proxy config
	row := db.QueryRow("SELECT host, port FROM proxy_config WHERE id = 1")
	var host string
	var port int
	if err := row.Scan(&host, &port); err == nil {
		cfg.Proxy.Host = host
		cfg.Proxy.Port = port
	}

	// Load health check config
	row = db.QueryRow("SELECT enabled, interval_seconds, timeout_seconds FROM health_check_config WHERE id = 1")
	var enabled int
	var interval, timeout int
	if err := row.Scan(&enabled, &interval, &timeout); err == nil {
		cfg.HealthCheck.Enabled = enabled == 1
		cfg.HealthCheck.IntervalSeconds = interval
		cfg.HealthCheck.TimeoutSeconds = timeout
	}

	// Load load balance config
	row = db.QueryRow("SELECT strategy FROM load_balance_config WHERE id = 1")
	var strategy string
	if err := row.Scan(&strategy); err == nil {
		cfg.LoadBalance.Strategy = strategy
	}

	return nil
}

// applyEnvOverrides applies environment variable overrides to config.
func applyEnvOverrides(cfg *Config) {
	// Proxy config
	cfg.Proxy.Host = getEnvStr("LLM_PROXY_HOST", cfg.Proxy.Host)
	cfg.Proxy.Port = getEnvInt("LLM_PROXY_PORT", cfg.Proxy.Port)
	cfg.Proxy.Workers = getEnvInt("LLM_PROXY_WORKERS", cfg.Proxy.Workers)
	cfg.Proxy.TimeoutKeepAlive = getEnvInt("LLM_PROXY_TIMEOUT_KEEP_ALIVE", cfg.Proxy.TimeoutKeepAlive)
	cfg.Proxy.TimeoutGracefulShutdown = getEnvIntOptional("LLM_PROXY_TIMEOUT_GRACEFUL_SHUTDOWN")
	cfg.Proxy.AccessLog = getEnvBool("LLM_PROXY_ACCESS_LOG", cfg.Proxy.AccessLog)
	cfg.Proxy.ProxyHeaders = getEnvBool("LLM_PROXY_PROXY_HEADERS", cfg.Proxy.ProxyHeaders)
	cfg.Proxy.ForwardedAllowIPs = getEnvStr("LLM_PROXY_FORWARDED_ALLOW_IPS", cfg.Proxy.ForwardedAllowIPs)
	cfg.Proxy.Reload = getEnvBool("LLM_PROXY_RELOAD", cfg.Proxy.Reload)
	cfg.Proxy.LogLevel = getEnvStr("LOG_LEVEL", cfg.Proxy.LogLevel)

	// SSL config
	cfg.Proxy.SSLKeyfile = getEnvStr("LLM_PROXY_SSL_KEYFILE", cfg.Proxy.SSLKeyfile)
	cfg.Proxy.SSLCertfile = getEnvStr("LLM_PROXY_SSL_CERTFILE", cfg.Proxy.SSLCertfile)
	cfg.Proxy.SSLKeyfilePassword = getEnvStr("LLM_PROXY_SSL_KEYFILE_PASSWORD", cfg.Proxy.SSLKeyfilePassword)

	// Security config
	cfg.Security.SecretKey = getEnvStr("LLM_PROXY_SECRET_KEY", cfg.Security.SecretKey)
	cfg.Security.SessionExpireHours = getEnvInt("LLM_PROXY_SESSION_EXPIRE_HOURS", cfg.Security.SessionExpireHours)
	cfg.Security.DefaultAdmin.Username = getEnvStr("LLM_PROXY_DEFAULT_ADMIN_USERNAME", cfg.Security.DefaultAdmin.Username)
	cfg.Security.DefaultAdmin.Password = getEnvStr("LLM_PROXY_DEFAULT_ADMIN_PASSWORD", cfg.Security.DefaultAdmin.Password)

	// Database path
	if dbPath := os.Getenv("LLM_PROXY_DB"); dbPath != "" {
		cfg.Database.Path = dbPath
	}

	// Log rotation config
	cfg.LogRotation.MaxSizeMB = getEnvInt("LLM_PROXY_LOG_MAX_SIZE_MB", cfg.LogRotation.MaxSizeMB)
	cfg.LogRotation.MaxBackups = getEnvInt("LLM_PROXY_LOG_MAX_BACKUPS", cfg.LogRotation.MaxBackups)
	cfg.LogRotation.MaxAgeDays = getEnvInt("LLM_PROXY_LOG_MAX_AGE_DAYS", cfg.LogRotation.MaxAgeDays)
	cfg.LogRotation.Compress = getEnvBool("LLM_PROXY_LOG_COMPRESS", cfg.LogRotation.Compress)

	// Rate limit config
	cfg.RateLimit.Enabled = getEnvBool("LLM_PROXY_RATE_LIMIT_ENABLED", cfg.RateLimit.Enabled)
	cfg.RateLimit.MaxRequests = getEnvInt("LLM_PROXY_RATE_LIMIT_MAX_REQUESTS", cfg.RateLimit.MaxRequests)
	cfg.RateLimit.WindowSeconds = getEnvInt("LLM_PROXY_RATE_LIMIT_WINDOW_SECONDS", cfg.RateLimit.WindowSeconds)
}

// String utility functions (avoiding external dependencies).

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

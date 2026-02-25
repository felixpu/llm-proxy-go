// Package config provides configuration management with 3-tier priority:
// Environment variables > SQLite database > Default values
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Proxy       ProxyConfig
	Security    SecurityConfig
	HealthCheck HealthCheckConfig
	LoadBalance LoadBalanceConfig
	Database    DatabaseConfig
	LogRotation LogRotationConfig
	RateLimit   RateLimitConfig
}

// LogRotationConfig holds log rotation settings powered by lumberjack.
type LogRotationConfig struct {
	MaxSizeMB  int  // Maximum size in MB before rotation
	MaxBackups int  // Maximum number of old log files to retain
	MaxAgeDays int  // Maximum number of days to retain old log files
	Compress   bool // Whether to gzip compress rotated files
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	Enabled       bool
	MaxRequests   int
	WindowSeconds int
}

// ProxyConfig holds proxy server configuration.
type ProxyConfig struct {
	Host                    string
	Port                    int
	Workers                 int
	TimeoutKeepAlive        int
	TimeoutGracefulShutdown *int
	AccessLog               bool
	ProxyHeaders            bool
	ForwardedAllowIPs       string
	Reload                  bool
	SSLKeyfile              string
	SSLCertfile             string
	SSLKeyfilePassword      string
	LogLevel                string
}

// SecurityConfig holds security-related configuration.
type SecurityConfig struct {
	SecretKey          string
	SessionExpireHours int
	DefaultAdmin       DefaultAdminConfig
}

// DefaultAdminConfig holds default admin credentials.
type DefaultAdminConfig struct {
	Username string
	Password string
}

// HealthCheckConfig holds health check configuration.
type HealthCheckConfig struct {
	Enabled         bool
	IntervalSeconds int
	TimeoutSeconds  int
}

// LoadBalanceConfig holds load balancing configuration.
type LoadBalanceConfig struct {
	Strategy string // round_robin, weighted, least_connections, conversation_hash
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Proxy: ProxyConfig{
			Host:              "0.0.0.0",
			Port:              8000,
			Workers:           1,
			TimeoutKeepAlive:  5,
			AccessLog:         true,
			ProxyHeaders:      true,
			ForwardedAllowIPs: "*",
			Reload:            false,
			LogLevel:          "INFO",
		},
		Security: SecurityConfig{
			SecretKey:          "change-this-to-a-random-secret-key",
			SessionExpireHours: 24,
			DefaultAdmin: DefaultAdminConfig{
				Username: "admin",
				Password: "admin123",
			},
		},
		HealthCheck: HealthCheckConfig{
			Enabled:         true,
			IntervalSeconds: 60,
			TimeoutSeconds:  10,
		},
		LoadBalance: LoadBalanceConfig{
			Strategy: "weighted",
		},
		Database: DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		LogRotation: LogRotationConfig{
			MaxSizeMB:  10,
			MaxBackups: 5,
			MaxAgeDays: 30,
			Compress:   true,
		},
		RateLimit: RateLimitConfig{
			Enabled:       true,
			MaxRequests:   100,
			WindowSeconds: 60,
		},
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Proxy.Port < 1 || c.Proxy.Port > 65535 {
		return &ConfigError{Field: "proxy.port", Message: "must be between 1 and 65535"}
	}
	if c.Proxy.Workers < 1 {
		return &ConfigError{Field: "proxy.workers", Message: "must be at least 1"}
	}
	if c.Proxy.Workers > 1 && c.Proxy.Reload {
		return &ConfigError{Field: "proxy", Message: "workers > 1 and reload=true are mutually exclusive"}
	}
	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config error: " + e.Field + ": " + e.Message
}

// Helper functions for environment variable parsing.

func getEnvStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvIntOptional(key string) *int {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func getEnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	lower := strings.ToLower(v)
	return lower == "true" || lower == "1" || lower == "yes" || lower == "on"
}

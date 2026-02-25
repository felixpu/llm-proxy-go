package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitConfig holds rate limiter configuration.
type RateLimitConfig struct {
	Enabled       bool
	MaxRequests   int
	WindowSeconds int
	ExemptPaths   []string
}

// DefaultRateLimitConfig returns the default rate limit configuration.
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Enabled:       true,
		MaxRequests:   100,
		WindowSeconds: 60,
		ExemptPaths: []string{
			"/api/health",
			"/api/status",
			"/static/",
			"/login",
			"/favicon.ico",
		},
	}
}

// rateLimiter implements a sliding window rate limiter.
type rateLimiter struct {
	mu          sync.Mutex
	requests    map[string][]time.Time
	maxRequests int
	window      time.Duration
}

func newRateLimiter(maxRequests int, windowSeconds int) *rateLimiter {
	return &rateLimiter{
		requests:    make(map[string][]time.Time),
		maxRequests: maxRequests,
		window:      time.Duration(windowSeconds) * time.Second,
	}
}

// isAllowed checks if a request from clientID is allowed.
// Returns (allowed, remaining, resetTimestamp).
func (rl *rateLimiter) isAllowed(clientID string) (bool, int, int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Clean expired entries
	reqs := rl.requests[clientID]
	valid := reqs[:0]
	for _, t := range reqs {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	remaining := rl.maxRequests - len(valid)
	resetTime := now.Add(rl.window).Unix()

	if len(valid) >= rl.maxRequests {
		rl.requests[clientID] = valid
		return false, 0, resetTime
	}

	valid = append(valid, now)
	rl.requests[clientID] = valid

	return true, remaining - 1, resetTime
}

// RateLimit returns a rate limiting middleware.
func RateLimit(cfg *RateLimitConfig) gin.HandlerFunc {
	if cfg == nil {
		cfg = DefaultRateLimitConfig()
	}

	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	limiter := newRateLimiter(cfg.MaxRequests, cfg.WindowSeconds)

	// Background cleanup every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			limiter.cleanup()
		}
	}()

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip exempt paths
		for _, exempt := range cfg.ExemptPaths {
			if strings.HasPrefix(path, exempt) {
				c.Next()
				return
			}
		}

		clientIP := getClientIP(c)
		allowed, remaining, resetTime := limiter.isAllowed(clientIP)

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.MaxRequests))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))

		if !allowed {
			retryAfter := resetTime - time.Now().Unix()
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"detail": "Rate limit exceeded. Please try again later.",
				"error": gin.H{
					"type":    "rate_limit_error",
					"message": "Too many requests",
				},
			})
			return
		}

		c.Next()
	}
}

// cleanup removes expired entries from the rate limiter.
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for clientID, reqs := range rl.requests {
		valid := reqs[:0]
		for _, t := range reqs {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.requests, clientID)
		} else {
			rl.requests[clientID] = valid
		}
	}
}

// getClientIP extracts the client IP, respecting reverse proxy headers.
func getClientIP(c *gin.Context) string {
	// X-Forwarded-For (first IP)
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	// X-Real-IP
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return xri
	}
	return c.ClientIP()
}

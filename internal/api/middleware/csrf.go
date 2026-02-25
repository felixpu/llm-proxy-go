package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	csrfTokenBytes  = 32
	csrfCookieName  = "csrf_token"
	csrfHeaderName  = "X-CSRF-Token"
)

// Default CSRF exempt paths (API key authenticated endpoints).
var defaultCSRFExemptPaths = []string{
	"/v1/",
	"/api/health",
	"/api/status",
	"/api/routing/debug",
	"/api/routing/test",
	"/api/health/check-now",
	"/api/cache/",
	"/api/system-logs/",
}

// CSRFConfig holds CSRF middleware configuration.
type CSRFConfig struct {
	ExemptPaths    []string
	CookieSecure   bool
	CookieSameSite http.SameSite
}

// DefaultCSRFConfig returns the default CSRF configuration.
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		ExemptPaths:    defaultCSRFExemptPaths,
		CookieSecure:   false,
		CookieSameSite: http.SameSiteLaxMode,
	}
}

// CSRF returns a CSRF protection middleware using Double Submit Cookie pattern.
func CSRF(cfg *CSRFConfig) gin.HandlerFunc {
	if cfg == nil {
		cfg = DefaultCSRFConfig()
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip static files
		if strings.HasPrefix(path, "/static/") {
			c.Next()
			return
		}

		// Get or generate CSRF token
		csrfCookie, _ := c.Cookie(csrfCookieName)
		needsNewToken := csrfCookie == ""
		if needsNewToken {
			csrfCookie = generateCSRFToken()
			// Set cookie immediately for new tokens
			c.SetSameSite(cfg.CookieSameSite)
			c.SetCookie(csrfCookieName, csrfCookie, 86400, "/", "", cfg.CookieSecure, false)
		}
		c.Set("csrf_token", csrfCookie)

		// Validate on protected methods
		method := c.Request.Method
		if isProtectedMethod(method) && !isExemptPath(path, cfg.ExemptPaths) && !hasAPIKey(c) {
			csrfHeader := c.GetHeader(csrfHeaderName)
			if !validateCSRFToken(csrfCookie, csrfHeader) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"detail": "CSRF token 验证失败",
				})
				return
			}
		}

		c.Next()

		// Refresh CSRF cookie
		needsRefresh := csrfCookie == "" ||
			(isProtectedMethod(method) && !isExemptPath(path, cfg.ExemptPaths) && !hasAPIKey(c))

		if needsRefresh {
			newToken := generateCSRFToken()
			c.SetSameSite(cfg.CookieSameSite)
			c.SetCookie(csrfCookieName, newToken, 86400, "/", "", cfg.CookieSecure, false)
		}
	}
}

func generateCSRFToken() string {
	b := make([]byte, csrfTokenBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validateCSRFToken(cookieToken, headerToken string) bool {
	if cookieToken == "" || headerToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1
}

func isProtectedMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodDelete || method == http.MethodPatch
}

func isExemptPath(path string, exemptPaths []string) bool {
	for _, exempt := range exemptPaths {
		if strings.HasPrefix(path, exempt) {
			return true
		}
	}
	return false
}

func hasAPIKey(c *gin.Context) bool {
	if c.GetHeader("x-api-key") != "" {
		return true
	}
	auth := c.GetHeader("Authorization")
	return strings.HasPrefix(auth, "Bearer sk-proxy-")
}

package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/service"
)

// GetSessionToken extracts the session token from cookie or Authorization header.
func GetSessionToken(c *gin.Context) string {
	// Try cookie first
	if token, err := c.Cookie("session_token"); err == nil && token != "" {
		return token
	}

	// Try Authorization header
	auth := c.GetHeader("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}

	return ""
}

// GetCurrentUser retrieves the authenticated user from context.
func GetCurrentUser(c *gin.Context) *service.CurrentUser {
	user, ok := c.Get("current_user")
	if !ok {
		return nil
	}
	cu, ok := user.(*service.CurrentUser)
	if !ok {
		return nil
	}
	return cu
}

// RequireAuth is a middleware that requires authentication.
func RequireAuth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := GetSessionToken(c)
		if token == "" {
			c.AbortWithStatusJSON(401, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Missing authentication token",
				},
			})
			return
		}

		user, err := authService.ValidateSession(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Invalid or expired session",
				},
			})
			return
		}

		c.Set("current_user", user)
		c.Next()
	}
}

// RequireAdmin is a middleware that requires admin role.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetCurrentUser(c)
		if user == nil || user.Role != string(models.UserRoleAdmin) {
			c.AbortWithStatusJSON(403, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "permission_error",
					"message": "Admin access required",
				},
			})
			return
		}
		c.Next()
	}
}

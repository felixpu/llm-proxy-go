package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/service"
	"go.uber.org/zap"
)

var cookieSecure = os.Getenv("LLM_PROXY_COOKIE_SECURE") == "true"

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService *service.AuthService
	logger      *zap.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(as *service.AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{authService: as, logger: logger}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	user, err := h.authService.AuthenticateUser(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "用户名或密码错误",
		})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	session, err := h.authService.CreateSession(c.Request.Context(), user.ID, ipAddress, userAgent)
	if err != nil {
		h.logger.Error("failed to create session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create session",
		})
		return
	}

	maxAge := int(session.ExpiresAt.Sub(session.CreatedAt).Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("session_token", session.Token, maxAge, "/", "", cookieSecure, true)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"user": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"role":       user.Role,
			"is_active":  user.IsActive,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
		"token":      session.Token,
		"expires_at": session.ExpiresAt,
	})
}

// Logout handles POST /api/auth/logout.
func (h *AuthHandler) Logout(c *gin.Context) {
	token := middleware.GetSessionToken(c)
	if token != "" {
		_ = h.authService.DeleteSession(c.Request.Context(), token)
	}

	c.SetCookie("session_token", "", -1, "/", "", cookieSecure, true)
	c.SetCookie("csrf_token", "", -1, "/", "", cookieSecure, false)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已登出",
	})
}

// GetMe handles GET /api/auth/me.
func (h *AuthHandler) GetMe(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "authentication_error",
				"message": "Not authenticated",
			},
		})
		return
	}
	c.JSON(http.StatusOK, user)
}

// Refresh handles POST /api/auth/refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Not authenticated",
		})
		return
	}

	// Delete old session
	oldToken := middleware.GetSessionToken(c)
	if oldToken != "" {
		_ = h.authService.DeleteSession(c.Request.Context(), oldToken)
	}

	// Create new session
	session, err := h.authService.CreateSession(
		c.Request.Context(),
		user.UserID,
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	if err != nil {
		h.logger.Error("failed to refresh session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to refresh session",
		})
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("session_token", session.Token, 86400, "/", "", cookieSecure, true)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"token":      session.Token,
		"expires_at": session.ExpiresAt.Format("2006-01-02T15:04:05Z"),
	})
}

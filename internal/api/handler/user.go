package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/internal/api/middleware"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
)

// UserHandler handles user management endpoints.
type UserHandler struct {
	userRepo    repository.UserRepository
	authService *service.AuthService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userRepo repository.UserRepository, authService *service.AuthService) *UserHandler {
	return &UserHandler{
		userRepo:    userRepo,
		authService: authService,
	}
}

// ListUsers lists all users (admin only).
// GET /api/users?offset=0&limit=50
func (h *UserHandler) ListUsers(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	users, _, err := h.userRepo.FindAll(c.Request.Context(), offset, limit)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to list users")
		return
	}

	c.JSON(http.StatusOK, users)
}

// GetUser retrieves a user by ID (admin only).
// GET /api/users/:id
func (h *UserHandler) GetUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	user, err := h.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

// CreateUser creates a new user (admin only).
// POST /api/users
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Password string `json:"password" binding:"required,min=8"`
		Role     string `json:"role" binding:"required,oneof=admin user"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Check if user already exists
	existing, _ := h.userRepo.FindByUsername(c.Request.Context(), req.Username)
	if existing != nil {
		errorResponse(c, http.StatusConflict, "Username already exists")
		return
	}

	// Hash password
	hash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	user := &models.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         models.UserRole(req.Role),
		IsActive:     true,
	}

	id, err := h.userRepo.Insert(c.Request.Context(), user)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to create user")
		return
	}

	user.ID = id
	c.JSON(http.StatusCreated, user)
}

// UpdateUser updates a user (admin only).
// PATCH /api/users/:id
func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req struct {
		Username string `json:"username" binding:"omitempty,min=3,max=50"`
		Role     string `json:"role" binding:"omitempty,oneof=admin user"`
		IsActive *bool  `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.userRepo.FindByID(c.Request.Context(), userID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Role != "" {
		user.Role = models.UserRole(req.Role)
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if err := h.userRepo.Update(c.Request.Context(), user); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to update user")
		return
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser deletes a user (admin only).
// DELETE /api/users/:id
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Prevent deleting self
	currentUser := middleware.GetCurrentUser(c)
	if currentUser != nil && currentUser.UserID == userID {
		errorResponse(c, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	if err := h.userRepo.Delete(c.Request.Context(), userID); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

// ChangePassword changes the current user's password.
// POST /api/users/change-password
func (h *UserHandler) ChangePassword(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Get user with password hash
	user, err := h.userRepo.FindByUsernameWithHash(c.Request.Context(), currentUser.Username)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	// Verify old password
	if !h.authService.VerifyPassword(req.OldPassword, user.PasswordHash) {
		errorResponse(c, http.StatusUnauthorized, "Invalid password")
		return
	}

	// Hash new password
	newHash, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	if err := h.userRepo.UpdatePassword(c.Request.Context(), currentUser.UserID, newHash); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to update password")
		return
	}

	// 更新密码后清除所有会话，强制重新登录
	if _, err := h.authService.DeleteUserSessions(c.Request.Context(), currentUser.UserID); err != nil {
		// 记录错误但不阻塞响应（密码已成功更改）
		// 用户需要重新登录时会发现旧会话已失效
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed, please login again"})
}

// GetCurrentUser returns the current authenticated user.
// GET /api/users/me
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	currentUser := middleware.GetCurrentUser(c)
	if currentUser == nil {
		errorResponse(c, http.StatusUnauthorized, "Not authenticated")
		return
	}

	user, err := h.userRepo.FindByID(c.Request.Context(), currentUser.UserID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

// AdminChangePassword allows admin to change any user's password.
// POST /api/users/:id/password
func (h *UserHandler) AdminChangePassword(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req struct {
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Verify user exists
	if _, err := h.userRepo.FindByID(c.Request.Context(), userID); err != nil {
		errorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	// Hash new password
	newHash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	if err := h.userRepo.UpdatePassword(c.Request.Context(), userID, newHash); err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to update password")
		return
	}

	// 清除目标用户所有会话
	if _, err := h.authService.DeleteUserSessions(c.Request.Context(), userID); err != nil {
		// 记录错误但不阻塞响应（密码已成功更改）
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed"})
}

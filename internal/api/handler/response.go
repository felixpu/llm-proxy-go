package handler

import "github.com/gin-gonic/gin"

// errorResponse sends a JSON error response with {detail: message} format
// matching the frontend's expected error format.
func errorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"detail": message})
}

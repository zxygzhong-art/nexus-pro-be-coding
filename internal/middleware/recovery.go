// Package middleware holds the Gin middleware chain: recovery, CORS, request id,
// principal resolution (with tenant-scoped tx), and per-route authz enforcement.
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recovery converts panics into a 500 JSON response.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal server error",
		})
	})
}

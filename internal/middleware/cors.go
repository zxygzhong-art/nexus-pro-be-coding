package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS mirrors the prototype's permissive dev CORS, extended with PUT/DELETE for
// IAM management endpoints.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Tenant-ID, X-Account-ID, X-Assumed-Role-Session-ID, X-Request-ID")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

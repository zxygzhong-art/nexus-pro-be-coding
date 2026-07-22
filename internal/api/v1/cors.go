package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	corsAllowedMethods = "GET, POST, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type, X-Request-ID, Idempotency-Key, X-Assumable-Role-Session-ID"
)

// corsMiddleware 處理 CORS middleware。
func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}
		// 無論來源是否允許，response 都會依 Origin 變動。
		c.Writer.Header().Add("Vary", "Origin")
		if _, ok := allowed[origin]; !ok {
			c.Next()
			return
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
		c.Writer.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

package middleware

import (
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/idgen"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"github.com/gin-gonic/gin"
)

// RequestID propagates an inbound X-Request-ID or generates one, exposing it on
// the response header and request context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = idgen.New("req")
		}
		c.Writer.Header().Set("X-Request-ID", id)
		c.Request = c.Request.WithContext(reqctx.WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

// Package response centralizes JSON envelopes so handlers stay consistent with
// the frontend contract: list endpoints return {"items": [...]}.
package response

import (
	"net/http"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/apperror"
	"github.com/gin-gonic/gin"
)

// Items wraps a slice in the {"items": [...]} envelope the frontend expects.
func Items(c *gin.Context, items any) {
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// OK writes a 200 with the given body.
func OK(c *gin.Context, body any) {
	c.JSON(http.StatusOK, body)
}

// Error maps an error to its HTTP status + JSON body and aborts the chain.
func Error(c *gin.Context, err error) {
	if e, ok := apperror.As(err); ok {
		c.AbortWithStatusJSON(e.Status, gin.H{"error": e.Code, "message": e.Message})
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"error":   "internal",
		"message": "internal server error",
	})
}

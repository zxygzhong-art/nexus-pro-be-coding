package middleware

import (
	"net/http"
	"strings"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/identity"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/principal"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Principal resolves the caller, opens a tenant-scoped transaction (SET LOCAL
// app.current_tenant) so RLS is enforced, validates the account, loads its group
// memberships, and stores the principal + tx on the request context. The tx is
// committed on success (status < 400) and rolled back otherwise.
func Principal(provider identity.Provider, gdb *gorm.DB, repo *repository.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := identity.Headers{
			TenantID:    c.GetHeader("X-Tenant-ID"),
			AccountID:   c.GetHeader("X-Account-ID"),
			BearerToken: bearer(c),
		}
		id, err := provider.Resolve(c.Request.Context(), h)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "could not resolve identity"})
			return
		}

		tx := gdb.WithContext(c.Request.Context()).Begin()
		if tx.Error != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "db begin failed"})
			return
		}
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", id.TenantID).Error; err != nil {
			tx.Rollback()
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "set tenant failed"})
			return
		}

		ctx := reqctx.WithTenantDB(c.Request.Context(), tx)
		c.Request = c.Request.WithContext(ctx)

		acct, err := repo.GetAccount(ctx, id.AccountID)
		if err != nil {
			tx.Rollback()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "unknown account"})
			return
		}
		if acct.Status != "active" {
			tx.Rollback()
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "account not active"})
			return
		}

		groupIDs, err := repo.GroupIDsForAccount(ctx, id.AccountID)
		if err != nil {
			tx.Rollback()
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "load memberships failed"})
			return
		}

		p := principal.Principal{
			TenantID:             id.TenantID,
			AccountID:            id.AccountID,
			Email:                acct.Email,
			Name:                 acct.DisplayName,
			GroupIDs:             groupIDs,
			AssumedRoleSessionID: c.GetHeader("X-Assumed-Role-Session-ID"),
		}
		c.Request = c.Request.WithContext(reqctx.WithPrincipal(c.Request.Context(), p))

		c.Next()

		if c.Writer.Status() >= http.StatusBadRequest {
			tx.Rollback()
			return
		}
		if err := tx.Commit().Error; err != nil {
			// Response already written; log path would go here.
			_ = err
		}
	}
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[len("bearer "):])
	}
	return ""
}

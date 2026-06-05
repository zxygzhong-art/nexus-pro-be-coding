package middleware

import (
	"net/http"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/authorizer"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/audit"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"github.com/gin-gonic/gin"
)

// Enforcer builds per-route authorization middleware.
type Enforcer struct {
	az  authorizer.Authorizer
	rec *audit.Recorder
}

// NewEnforcer constructs an Enforcer.
func NewEnforcer(az authorizer.Authorizer, rec *audit.Recorder) *Enforcer {
	return &Enforcer{az: az, rec: rec}
}

// Require returns middleware that checks the given permission (application,
// resource, action), audits the decision, and aborts with 403 on denial.
func (e *Enforcer) Require(applicationCode, resourceType, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		req := authz.Request{ApplicationCode: applicationCode, ResourceType: resourceType, Action: action}
		dec, err := e.az.Check(ctx, req)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "authz check failed"})
			return
		}

		p, _ := reqctx.Principal(ctx)
		decisionStr := "deny"
		if dec.Allowed {
			decisionStr = "allow"
		}
		if dec.RequiresApproval {
			decisionStr = "approval_required"
		}
		boundary := ""
		if dec.PermissionBoundary != nil {
			boundary = *dec.PermissionBoundary
		}
		_ = e.rec.Record(ctx, audit.Entry{
			TenantID:             p.TenantID,
			ApplicationCode:      applicationCode,
			ActorAccountID:       p.AccountID,
			Action:               action,
			ResourceType:         resourceType,
			Decision:             decisionStr,
			MatchedPermissions:   dec.MatchedBy,
			AssumedRoleSessionID: p.AssumedRoleSessionID,
			PermissionBoundary:   boundary,
			DataScope:            dec.Scope,
			FieldPolicies:        dec.FieldPolicies,
			RequestID:            reqctx.RequestID(ctx),
		})

		if !dec.Allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":               "forbidden",
				"message":             dec.Reason,
				"missing_permissions": dec.MissingPermissions,
			})
			return
		}
		c.Next()
	}
}

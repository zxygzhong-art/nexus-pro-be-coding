package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type AuthzSnapshotCache interface {
	GetAuthzSnapshot(ctx context.Context, key string) (CheckResult, bool, error)
	SetAuthzSnapshot(ctx context.Context, key string, result CheckResult, ttl time.Duration) error
	InvalidateTenant(ctx context.Context, tenantID string) error
}

func (c *Service) authzSnapshotKey(ctx RequestContext, account Account, req CheckRequest, version int64) string {
	payload, _ := json.Marshal(map[string]any{
		"tenant_id":                 ctx.TenantID,
		"account_id":                account.ID,
		"assumed_role_id":           firstNonEmpty(ctx.AssumedRoleSessionID, account.ActiveAssumableRoleID),
		"permission_version":        version,
		"application_code":          req.ApplicationCode,
		"resource_type":             req.ResourceType,
		"resource_id":               req.ResourceID,
		"resource":                  req.Resource,
		"action":                    req.Action,
		"target":                    req.Target,
		"target_employee_id":        req.TargetEmployeeID,
		"context":                   req.Context,
		"approval_confirmation_set": ctx.ApprovalConfirmed,
	})
	sum := sha1.Sum(payload)
	return fmt.Sprintf("authz:snapshot:%s:%s", ctx.TenantID, hex.EncodeToString(sum[:]))
}

func (c *Service) shouldUseAuthzSnapshot(ctx RequestContext) bool {
	return ctx.AssumedRoleSessionID == ""
}

func (c *Service) getAuthzSnapshot(ctx context.Context, key string) (CheckResult, bool) {
	if c.authzSnapshot == nil {
		return CheckResult{}, false
	}
	result, ok, err := c.authzSnapshot.GetAuthzSnapshot(ctx, key)
	if err != nil || !ok {
		return CheckResult{}, false
	}
	return result, true
}

func (c *Service) setAuthzSnapshot(ctx context.Context, key string, result CheckResult) {
	if c.authzSnapshot == nil {
		return
	}
	_ = c.authzSnapshot.SetAuthzSnapshot(ctx, key, result, 5*time.Minute)
}

func (c *Service) invalidateAuthzSnapshots(ctx context.Context, tenantID string) {
	if c.authzSnapshot == nil {
		return
	}
	_ = c.authzSnapshot.InvalidateTenant(ctx, tenantID)
}

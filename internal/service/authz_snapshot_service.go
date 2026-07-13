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

// authzSnapshotKey 處理授權快照 key 的服務流程。
func (c *Service) authzSnapshotKey(ctx RequestContext, account Account, req CheckRequest, version int64) string {
	payload, _ := json.Marshal(map[string]any{
		"tenant_id":               ctx.TenantID,
		"account_id":              account.ID,
		"assumed_role_session_id": ctx.AssumedRoleSessionID,
		"permission_version":      version,
		"application_code":        req.ApplicationCode,
		"resource_type":           req.ResourceType,
		"resource_id":             req.ResourceID,
		"resource":                req.Resource,
		"action":                  req.Action,
		"target":                  req.Target,
		"target_employee_id":      req.TargetEmployeeID,
		"route_method":            req.RouteMethod,
		"route_path":              req.RoutePath,
		"context":                 req.Context,
	})
	sum := sha1.Sum(payload)
	return fmt.Sprintf("authz:snapshot:%s:%s", ctx.TenantID, hex.EncodeToString(sum[:]))
}

// shouldUseAuthzSnapshot 處理 should use 授權快照的服務流程。
func (c *Service) shouldUseAuthzSnapshot(ctx RequestContext) bool {
	return ctx.AssumedRoleSessionID == ""
}

// getAuthzSnapshot 取得授權快照的服務流程。
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

// setAuthzSnapshot 處理集合授權快照的服務流程。
func (c *Service) setAuthzSnapshot(ctx context.Context, key string, result CheckResult) {
	if c.authzSnapshot == nil {
		return
	}
	_ = c.authzSnapshot.SetAuthzSnapshot(ctx, key, result, 5*time.Minute)
}

// invalidateAuthzSnapshots 處理 invalidate 授權 snapshots 的服務流程。
func (c *Service) invalidateAuthzSnapshots(ctx context.Context, tenantID string) {
	if c.authzSnapshot == nil {
		return
	}
	_ = c.authzSnapshot.InvalidateTenant(ctx, tenantID)
}

// requireServiceAuthz 處理 require 服務授權的服務流程。
func (c *Service) requireServiceAuthz(ctx RequestContext, app ApplicationCode, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	account, decision, _, err := c.Authorize(ctx, CheckRequest{
		ApplicationCode: app,
		ResourceType:    resource,
		ResourceID:      resourceID,
		Action:          action,
	}, AuditTarget{})
	return account, decision, err
}

// requireIAMAuthz 處理 require IAM 授權的服務流程。
func (c IAMService) requireIAMAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppIAM, resource, action, resourceID)
}

// requireWorkflowAuthz 處理 require 流程授權的服務流程。
func (c WorkflowService) requireWorkflowAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppWorkflow, resource, action, resourceID)
}

// requireAgentAuthz 處理 require agent 授權的服務流程。
func (c AgentService) requireAgentAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppAgent, resource, action, resourceID)
}

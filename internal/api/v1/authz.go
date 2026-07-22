package v1

import (
	"net/http"

	"nexus-pro-api/internal/domain"
)

// authorize 授權目前流程。
func (a *API) authorize(ctx domain.RequestContext, r *http.Request, routePath, resource, action string, authz routeAuthz) error {
	req := domain.CheckRequest{Resource: resource, Action: domain.Action(action), RouteMethod: r.Method, RoutePath: routePath}
	if authz.resourceIDParam != "" {
		req.ResourceID = r.PathValue(authz.resourceIDParam)
	}
	if authz.targetEmployeeIDParam != "" {
		req.TargetEmployeeID = r.PathValue(authz.targetEmployeeIDParam)
		if req.ResourceID == "" {
			req.ResourceID = req.TargetEmployeeID
		}
	}
	var result domain.CheckResult
	var err error
	if authz.currentAccessProjection {
		result, err = a.authz.CheckCurrentAccessProjection(ctx, req)
	} else {
		result, err = a.authz.Check(ctx, req)
	}
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ForbiddenReason(apiAuthzReasonCode(result), result.Reason)
	}
	if authz.requireTenantWide && !apiTenantWideScope(result.EffectiveScope, result.Scope) {
		result.Allowed = false
		result.Reason = "workspace management requires tenant-wide data scope"
		_ = a.authz.AuditDecision(ctx, req, result)
		return domain.ForbiddenReason("data_scope_denied", result.Reason)
	}
	return nil
}

// apiTenantWideScope 判斷授權決策是否可進入租戶管理面。
func apiTenantWideScope(effective, fallback domain.Scope) bool {
	scope := effective
	if scope == "" {
		scope = fallback
	}
	switch scope {
	case "", domain.ScopeAll, domain.ScopeTenant, domain.ScopeSystem:
		return true
	default:
		return false
	}
}

// apiAuthzReasonCode 處理 API 授權 reason 碼。
func apiAuthzReasonCode(result domain.CheckResult) string {
	switch result.Reason {
	case "missing permission":
		switch result.Action {
		case domain.ActionRead:
			return "menu_denied"
		case domain.ActionCreate, domain.ActionUpdate, domain.ActionDelete, domain.ActionExport, domain.ActionImport, domain.ActionInvite, domain.ActionApprove, domain.ActionUpdateStatus, domain.ActionStatusTransition:
			return "button_denied"
		default:
			return "permission_missing"
		}
	case "data scope denied":
		return "data_scope_denied"
	default:
		return "permission_missing"
	}
}

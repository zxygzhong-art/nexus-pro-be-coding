package service

import (
	"go.opentelemetry.io/otel/attribute"
	"strings"
)

// AuthzService 定義授權服務的資料結構。
type AuthzService struct {
	*Service
}

// Authz 處理授權的服務流程。
func (c *Service) Authz() AuthzService {
	return AuthzService{Service: c}
}

// Check 檢查對應的服務流程。
func (c AuthzService) Check(ctx RequestContext, req CheckRequest) (result CheckResult, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.check", authzSpanAttributes(req)...)
	defer func() {
		setAuthzSpanResult(span, result)
		finishServiceSpan(span, err)
	}()
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	result, err = c.evaluateAuthz(ctx, account, req)
	if err == nil && c.shouldAuditRouteAuthzCheck(ctx, result) {
		_ = c.auditAuthzTarget(ctx, AuditTarget{}.fromRequest(req), result)
	}
	return result, err
}

// BatchCheck 處理批次 check 的服務流程。
func (c AuthzService) BatchCheck(ctx RequestContext, req BatchCheckRequest) (result BatchCheckResult, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.batch_check")
	defer func() {
		span.SetAttributes(attribute.Int("authz.batch_size", len(req.Checks)))
		finishServiceSpan(span, err)
	}()
	results := make([]CheckResult, 0, len(req.Checks))
	for _, item := range req.Checks {
		itemResult, err := c.Check(ctx, item)
		if err != nil {
			return BatchCheckResult{}, err
		}
		results = append(results, itemResult)
	}
	return BatchCheckResult{Results: results}, nil
}

// Explain 解釋單筆授權決策。
func (c AuthzService) Explain(ctx RequestContext, req CheckRequest) (result AuthzExplainResponse, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.explain", authzSpanAttributes(req)...)
	defer func() {
		setAuthzSpanResult(span, result.Decision)
		finishServiceSpan(span, err)
	}()
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return AuthzExplainResponse{}, err
	}
	req = normalizeCheckRequest(req)
	grants, setIDs, assumedRole, boundary, err := c.collectAuthzGrants(ctx, account)
	if err != nil {
		return AuthzExplainResponse{}, err
	}
	trace := &authzTrace{}
	decision, err := c.evaluateAuthzDecision(ctx, account, req, grants, setIDs, assumedRole, boundary, trace)
	if err != nil {
		return AuthzExplainResponse{}, err
	}
	result = trace.response(decision)
	if err := c.auditAuthzGovernance(ctx, "iam.authz.explain", "normal", account.ID, req, nil, decision, nil); err != nil {
		return AuthzExplainResponse{}, err
	}
	return result, nil
}

// Simulate 模擬套用授權覆蓋後的決策差異。
func (c AuthzService) Simulate(ctx RequestContext, req AuthzSimulationRequest) (result AuthzSimulationResponse, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.simulate", authzSpanAttributes(req.Check)...)
	defer func() {
		setAuthzSpanResult(span, result.After)
		finishServiceSpan(span, err)
	}()
	targetCtx, account, err := c.resolveSimulationAccount(ctx, req.AccountID)
	if err != nil {
		return AuthzSimulationResponse{}, err
	}
	check := normalizeCheckRequest(req.Check)
	baseGrants, baseSetIDs, baseAssumedRole, baseBoundary, err := c.collectAuthzGrants(targetCtx, account)
	if err != nil {
		return AuthzSimulationResponse{}, err
	}
	before, err := c.evaluateAuthzDecision(targetCtx, account, check, baseGrants, baseSetIDs, baseAssumedRole, baseBoundary, nil)
	if err != nil {
		return AuthzSimulationResponse{}, err
	}
	opts, err := simulationOverridesToGrantOptions(req.Overrides)
	if err != nil {
		return AuthzSimulationResponse{}, err
	}
	grants, setIDs, assumedRole, boundary, err := c.collectAuthzGrantsWithOptions(targetCtx, account, opts)
	if err != nil {
		return AuthzSimulationResponse{}, err
	}
	after, err := c.evaluateAuthzDecision(targetCtx, account, check, grants, setIDs, assumedRole, boundary, nil)
	if err != nil {
		return AuthzSimulationResponse{}, err
	}
	diff := authzSimulationDiff(before, after)
	result = AuthzSimulationResponse{Before: before, After: after, Diff: diff}
	if err := c.auditAuthzGovernance(ctx, "iam.authz.simulate", "high", account.ID, check, &req.Overrides, after, diff); err != nil {
		return AuthzSimulationResponse{}, err
	}
	return result, nil
}

// shouldAuditRouteAuthzCheck 處理 should 稽核路由授權 check 的服務流程。
func (c AuthzService) shouldAuditRouteAuthzCheck(ctx RequestContext, result CheckResult) bool {
	return !result.Allowed
}

// AuditDecision 處理稽核決策的服務流程。
func (c AuthzService) AuditDecision(ctx RequestContext, req CheckRequest, result CheckResult) error {
	return c.auditAuthzTarget(ctx, AuditTarget{}.fromRequest(req), result)
}

func (c AuthzService) resolveSimulationAccount(ctx RequestContext, accountID string) (RequestContext, Account, error) {
	targetCtx := ctx
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		targetCtx.AccountID = accountID
		if accountID != ctx.AccountID {
			targetCtx.AssumedRoleSessionID = ""
			targetCtx.AssumedRoleID = ""
		}
	}
	account, _, err := c.resolveAccount(targetCtx)
	if err != nil {
		return RequestContext{}, Account{}, err
	}
	return targetCtx, account, nil
}

func (c AuthzService) auditAuthzGovernance(ctx RequestContext, action, severity, targetAccountID string, req CheckRequest, overrides *AuthzSimulationOverrides, decision CheckResult, diff any) error {
	details := map[string]any{
		"target_account_id": targetAccountID,
		"check":             authzCheckAuditSummary(req),
	}
	if overrides != nil {
		details["overrides"] = authzOverridesAuditSummary(*overrides)
	}
	if diff != nil {
		details["diff"] = diff
	}
	return c.audit(ctx, action, "authz", targetAccountID, severity, auditDecisionDetails(ctx, decision, details))
}

func authzCheckAuditSummary(req CheckRequest) map[string]any {
	req = normalizeCheckRequest(req)
	return map[string]any{
		"application_code":   req.ApplicationCode,
		"resource_type":      req.ResourceType,
		"resource":           req.Resource,
		"resource_id":        req.ResourceID,
		"action":             req.Action,
		"target":             req.Target,
		"target_employee_id": req.TargetEmployeeID,
		"route_method":       req.RouteMethod,
		"route_path":         req.RoutePath,
	}
}

func authzOverridesAuditSummary(overrides AuthzSimulationOverrides) map[string]any {
	return map[string]any{
		"add_user_groups":             uniqueStrings(overrides.AddUserGroups),
		"remove_user_groups":          uniqueStrings(overrides.RemoveUserGroups),
		"add_permission_sets":         uniqueStrings(overrides.AddPermissionSets),
		"remove_permission_sets":      uniqueStrings(overrides.RemovePermissionSets),
		"assume_role_id":              strings.TrimSpace(overrides.AssumeRoleID),
		"permission_set_change_count": len(overrides.PermissionSetChanges),
	}
}

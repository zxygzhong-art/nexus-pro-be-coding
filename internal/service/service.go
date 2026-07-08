package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

// Service 定義服務的資料結構。
type Service struct {
	store                 repository.Store
	now                   func() time.Time
	logger                *slog.Logger
	authzSnapshot         AuthzSnapshotCache
	relationships         RelationshipChecker
	openFGAScopeChecks    bool
	objectStore           ObjectStore
	ehrmsClient           EHRMSClient
	identityProvisioner   IdentityProvisioner
	formApprovalWorkflows FormApprovalWorkflowClient
}

// Options 定義選項的資料結構。
type Options struct {
	Logger                *slog.Logger
	Now                   func() time.Time
	AuthzSnapshot         AuthzSnapshotCache
	Relationships         RelationshipChecker
	OpenFGAScopeChecks    bool
	ObjectStore           ObjectStore
	EHRMSClient           EHRMSClient
	IdentityProvisioner   IdentityProvisioner
	FormApprovalWorkflows FormApprovalWorkflowClient
}

// RelationshipChecker 定義關係 checker 的行為契約。
type RelationshipChecker interface {
	CheckRelationship(ctx context.Context, check domain.RelationshipCheck) (bool, error)
}

// EHRMSClient 定義 eHRMS client 的行為契約。
type EHRMSClient interface {
	ListEmployees(context.Context) ([]domain.EHRMSEmployeeRecord, error)
}

// IdentityProvisioner 定義身分 provisioner 的行為契約。
type IdentityProvisioner interface {
	EnsureUser(context.Context, domain.IdentityProvisioningInput) (domain.ProvisionedIdentity, error)
}

// FormApprovalWorkflowClient defines the Temporal operations used by workflow service.
type FormApprovalWorkflowClient interface {
	StartFormApprovalWorkflow(context.Context, domain.FormApprovalWorkflowStart) error
	SignalFormApprovalWorkflow(context.Context, domain.FormApprovalWorkflowSignal) error
}

// New 建立 服務 的主要物件。
func New(store repository.Store, options ...Options) *Service {
	cfg := Options{}
	if len(options) > 0 {
		cfg = options[0]
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	return &Service{
		store:                 store,
		now:                   now,
		logger:                logger,
		authzSnapshot:         cfg.AuthzSnapshot,
		relationships:         cfg.Relationships,
		openFGAScopeChecks:    cfg.OpenFGAScopeChecks,
		objectStore:           firstObjectStore(cfg.ObjectStore),
		ehrmsClient:           cfg.EHRMSClient,
		identityProvisioner:   cfg.IdentityProvisioner,
		formApprovalWorkflows: cfg.FormApprovalWorkflows,
	}
}

// Store 處理儲存層的服務流程。
func (c *Service) Store() repository.Store {
	return c.store
}

// withTenantTransaction 附加租戶 transaction 的服務流程。
func (c *Service) withTenantTransaction(ctx RequestContext, fn func(*Service) error) error {
	return repository.WithinTenantTransaction(goContext(ctx), c.store, ctx.TenantID, func(store repository.Store) error {
		next := *c
		next.store = store
		return fn(&next)
	})
}

// goContext 處理 go context。
func goContext(ctx RequestContext) context.Context {
	if ctx.Context != nil {
		return ctx.Context
	}
	return context.Background()
}

// Now 處理 now 的服務流程。
func (c *Service) Now() time.Time {
	return c.now().UTC()
}

// logInfo 處理 log info 的服務流程。
func (c *Service) logInfo(ctx RequestContext, message string, args ...any) {
	c.loggerFor(ctx).InfoContext(goContext(ctx), message, args...)
}

// logWarn 處理 log warn 的服務流程。
func (c *Service) logWarn(ctx RequestContext, message string, args ...any) {
	c.loggerFor(ctx).WarnContext(goContext(ctx), message, args...)
}

// loggerFor 處理 logger for 的服務流程。
func (c *Service) loggerFor(ctx RequestContext) *slog.Logger {
	logger := slog.Default()
	if c != nil && c.logger != nil {
		logger = c.logger
	}
	attrs := []any{"component", "service"}
	if ctx.TenantID != "" {
		attrs = append(attrs, "tenant_id", ctx.TenantID)
	}
	if ctx.AccountID != "" {
		attrs = append(attrs, "account_id", ctx.AccountID)
	}
	if ctx.RequestID != "" {
		attrs = append(attrs, "request_id", ctx.RequestID)
	}
	if ctx.TraceID != "" {
		attrs = append(attrs, "trace_id", ctx.TraceID)
	}
	if ctx.AssumedRoleID != "" {
		attrs = append(attrs, "assumed_role_id", ctx.AssumedRoleID)
	}
	if ctx.AssumedRoleSessionID != "" {
		attrs = append(attrs, "assumed_role_session_id", ctx.AssumedRoleSessionID)
	}
	return logger.With(attrs...)
}

// resolveAccount 解析帳號的服務流程。
func (c *Service) resolveAccount(ctx RequestContext) (Account, Tenant, error) {
	if strings.TrimSpace(ctx.TenantID) == "" {
		return Account{}, Tenant{}, BadRequest("tenant id is required")
	}
	if strings.TrimSpace(ctx.AccountID) == "" {
		return Account{}, Tenant{}, BadRequest("account id is required")
	}
	tenant, ok, err := c.store.GetTenant(goContext(ctx), ctx.TenantID)
	if err != nil {
		return Account{}, Tenant{}, err
	}
	if !ok {
		return Account{}, Tenant{}, NotFound("tenant", ctx.TenantID)
	}
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return Account{}, Tenant{}, err
	}
	if !ok {
		return Account{}, Tenant{}, NotFound("account", ctx.AccountID)
	}
	if account.Status == string(AccountStatusDisabled) || account.Status == string(AccountStatusPendingInvite) {
		return Account{}, Tenant{}, domain.UnauthorizedReason("account_inactive", "account is not active")
	}
	return account, tenant, nil
}

// AuditTarget 定義稽核 target 的資料結構。
type AuditTarget struct {
	Event    string
	Resource string
	Target   string
}

// AuthzAudit 定義授權稽核的資料結構。
type AuthzAudit struct {
	service  *Service
	target   AuditTarget
	decision CheckResult
}

// Authorize 授權對應的服務流程。
func (c *Service) Authorize(ctx RequestContext, req CheckRequest, audit AuditTarget) (account Account, decision CheckResult, done AuthzAudit, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.authorize", authzSpanAttributes(req)...)
	defer func() {
		setAuthzSpanResult(span, decision)
		finishServiceSpan(span, err)
	}()
	account, _, err = c.resolveAccount(ctx)
	if err != nil {
		return Account{}, CheckResult{}, AuthzAudit{}, err
	}
	decision, err = c.evaluateAuthz(ctx, account, req)
	if err != nil {
		return Account{}, CheckResult{}, AuthzAudit{}, err
	}
	audit = audit.fromRequest(req)
	done = AuthzAudit{service: c, target: audit, decision: decision}
	if !decision.Allowed {
		_ = c.auditAuthzTarget(ctx, audit, decision)
		c.logWarn(ctx, "authorization denied",
			"application_code", req.ApplicationCode,
			"resource_type", req.ResourceType,
			"resource_id", req.ResourceID,
			"action", req.Action,
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
			"matched_by", decision.MatchedBy,
		)
		return Account{}, decision, AuthzAudit{}, forbiddenAuthz(decision)
	}
	if decision.RequiresApproval {
		if err := c.confirmApproval(ctx, req); err == nil {
			return account, decision, done, nil
		} else if ctx.ApprovalInstanceID != "" {
			_ = c.auditAuthzTarget(ctx, audit, decision)
			return Account{}, decision, AuthzAudit{}, err
		}
		_ = c.auditAuthzTarget(ctx, audit, decision)
		c.logWarn(ctx, "authorization requires approval",
			"application_code", req.ApplicationCode,
			"resource_type", req.ResourceType,
			"resource_id", req.ResourceID,
			"action", req.Action,
			"risk_level", decision.RiskLevel,
			"approval_type", decision.ApprovalType,
			"approval_reason", decision.ApprovalReason,
		)
		return Account{}, decision, AuthzAudit{}, domain.ForbiddenReason("approval_required", "high-risk action requires approval")
	}
	return account, decision, done, nil
}

// ValidateApprovalInstance 驗證核准實例的服務流程。
func (c *Service) ValidateApprovalInstance(ctx RequestContext, req CheckRequest) error {
	return c.validateApprovalInstance(ctx, normalizeCheckRequest(req))
}

// confirmApproval 確認核准的服務流程。
func (c *Service) confirmApproval(ctx RequestContext, req CheckRequest) error {
	if ctx.ApprovalInstanceID != "" {
		return c.ValidateApprovalInstance(ctx, req)
	}
	if ctx.ApprovalConfirmed {
		return nil
	}
	return domain.ForbiddenReason("approval_required", "high-risk action requires approval")
}

// validateApprovalInstance 驗證核准實例的服務流程。
func (c *Service) validateApprovalInstance(ctx RequestContext, req CheckRequest) error {
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, ctx.ApprovalInstanceID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ForbiddenReason("approval_required", "approval instance not found")
	}
	if instance.ApplicantAccountID != ctx.AccountID {
		return domain.ForbiddenReason("approval_required", "approval instance does not belong to current account")
	}
	if !strings.EqualFold(instance.Status, "approved") {
		return domain.ForbiddenReason("approval_required", "approval instance is not approved")
	}
	payload := instance.Payload
	if !approvalPayloadMatches(payload, "application_code", string(req.ApplicationCode)) {
		return domain.ForbiddenReason("approval_required", "approval application_code does not match request")
	}
	resourceMatches := approvalPayloadMatches(payload, "resource_type", string(req.ResourceType))
	if req.Resource != "" {
		resourceMatches = resourceMatches || approvalPayloadMatches(payload, "resource", req.Resource)
	}
	if !resourceMatches {
		return domain.ForbiddenReason("approval_required", "approval resource does not match request")
	}
	if !approvalPayloadMatches(payload, "action", string(req.Action)) {
		return domain.ForbiddenReason("approval_required", "approval action does not match request")
	}
	target := utils.FirstNonEmpty(req.ResourceID, req.Target, req.TargetEmployeeID)
	if target != "" && !approvalPayloadMatchesAny(payload, target, "target", "resource_id", "employee_id", "session_id") {
		return domain.ForbiddenReason("approval_required", "approval target does not match request")
	}
	if filters, ok := req.Context["filters"]; ok {
		payloadFilters, hasFilters := payload["filters"]
		if !hasFilters || !approvalPayloadValueMatches(payloadFilters, filters) {
			return domain.ForbiddenReason("approval_required", "approval filters do not match request")
		}
	}
	return nil
}

// approvalPayloadMatches 處理核准 payload matches。
func approvalPayloadMatches(payload map[string]any, key, expected string) bool {
	if strings.TrimSpace(expected) == "" {
		return true
	}
	value, ok := payload[key]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(value)), strings.TrimSpace(expected))
}

// approvalPayloadMatchesAny 處理核准 payload matches any。
func approvalPayloadMatchesAny(payload map[string]any, expected string, keys ...string) bool {
	for _, key := range keys {
		if value, ok := payload[key]; ok && strings.EqualFold(strings.TrimSpace(stringFromAny(value)), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}

// approvalPayloadValueMatches 處理核准 payload value matches。
func approvalPayloadValueMatches(actual, expected any) bool {
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	if actualErr == nil && expectedErr == nil {
		return string(actualJSON) == string(expectedJSON)
	}
	return reflect.DeepEqual(actual, expected)
}

// stringFromAny 處理字串 來源 any。
func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case Action:
		return string(v)
	case ApplicationCode:
		return string(v)
	case ResourceType:
		return string(v)
	default:
		return ""
	}
}

// Commit 提交目前流程。
func (a AuthzAudit) Commit(ctx RequestContext) error {
	return a.CommitWith(ctx, a.service)
}

// CommitWith 提交 with。
func (a AuthzAudit) CommitWith(ctx RequestContext, service *Service) error {
	if service == nil {
		return nil
	}
	return service.auditAuthzTarget(ctx, a.target, a.decision)
}

// fromRequest 轉換請求。
func (a AuditTarget) fromRequest(req CheckRequest) AuditTarget {
	req = normalizeCheckRequest(req)
	if a.Event == "" {
		a.Event = req.AuditEvent()
	}
	if a.Resource == "" {
		a.Resource = string(req.ResourceType)
	}
	if a.Target == "" {
		a.Target = req.ResourceID
	}
	return a
}

// auditAuthzTarget 處理稽核授權 target 的服務流程。
func (c *Service) auditAuthzTarget(ctx RequestContext, audit AuditTarget, decision CheckResult) error {
	if audit.Event == "" {
		return nil
	}
	if !shouldAuditAuthzDecision(decision) {
		return nil
	}
	return c.auditAuthzDecision(ctx, audit.Event, audit.Resource, audit.Target, decision)
}

// shouldAuditAuthzDecision 判斷授權決策是否需要寫入操作稽核。
func shouldAuditAuthzDecision(decision CheckResult) bool {
	if !decision.Allowed {
		return true
	}
	if decision.RequiresApproval {
		return true
	}
	return decision.Action != ActionRead
}

// resolveAccess 解析 access 的服務流程。
func (c *Service) resolveAccess(ctx RequestContext, account Account) ([]Permission, []PermissionSet, []UserGroup, error) {
	permissionSetIDs := map[string]struct{}{}
	for _, id := range account.DirectPermissionSetIDs {
		permissionSetIDs[id] = struct{}{}
	}
	assignments, err := c.store.ListPermissionSetAssignmentsForPrincipal(goContext(ctx), ctx.TenantID, "account", account.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, assignment := range assignments {
		if assignment.Effect == "allow" {
			permissionSetIDs[assignment.PermissionSetID] = struct{}{}
		}
	}

	groups, err := c.activeUserGroupsForAccount(ctx, account)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, group := range groups {
		for _, id := range group.PermissionSetIDs {
			permissionSetIDs[id] = struct{}{}
		}
		assignments, err := c.store.ListPermissionSetAssignmentsForPrincipal(goContext(ctx), ctx.TenantID, "user_group", group.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, assignment := range assignments {
			if assignment.Effect == "allow" {
				permissionSetIDs[assignment.PermissionSetID] = struct{}{}
			}
		}
	}

	role, _, err := c.activeAssumableRole(ctx, account)
	if err != nil {
		return nil, nil, nil, err
	}
	if role != nil {
		for _, id := range role.PermissionSetIDs {
			permissionSetIDs[id] = struct{}{}
		}
		assignments, err := c.store.ListPermissionSetAssignmentsForPrincipal(goContext(ctx), ctx.TenantID, "assumable_role", role.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, assignment := range assignments {
			if assignment.Effect == "allow" {
				permissionSetIDs[assignment.PermissionSetID] = struct{}{}
			}
		}
	}

	setIDs := make([]string, 0, len(permissionSetIDs))
	for id := range permissionSetIDs {
		setIDs = append(setIDs, id)
	}
	sort.Strings(setIDs)

	permissionSets := make([]PermissionSet, 0, len(setIDs))
	for _, id := range setIDs {
		set, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return nil, nil, nil, err
		}
		if !ok {
			continue
		}
		permissionSets = append(permissionSets, set)
	}
	grants, _, assumed, boundary, err := c.collectAuthzGrants(ctx, account)
	if err != nil {
		return nil, nil, nil, err
	}
	permissions := effectiveAccessPermissionsFromGrants(grants, boundary, assumed != nil)

	return permissions, permissionSets, groups, nil
}

// activeUserGroupsForAccount 以成員關係表解析帳號目前有效的使用者群組。
func (c *Service) activeUserGroupsForAccount(ctx RequestContext, account Account) ([]UserGroup, error) {
	at := c.Now()
	memberships, err := c.store.ListActiveGroupMembershipsForAccount(goContext(ctx), ctx.TenantID, account.ID, at)
	if err != nil {
		return nil, err
	}
	groups := make([]UserGroup, 0, len(memberships)+len(account.UserGroupIDs))
	seen := map[string]struct{}{}
	addGroup := func(groupID string) error {
		if _, ok := seen[groupID]; ok {
			return nil
		}
		group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, groupID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		seen[groupID] = struct{}{}
		groups = append(groups, group)
		return nil
	}
	for _, membership := range memberships {
		if err := addGroup(membership.UserGroupID); err != nil {
			return nil, err
		}
	}
	for _, groupID := range account.UserGroupIDs {
		if _, ok := seen[groupID]; ok {
			continue
		}
		membership, ok, err := c.store.GetGroupMembership(goContext(ctx), ctx.TenantID, groupID, account.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			if groupMembershipActiveAt(membership, at) {
				if err := addGroup(groupID); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := addGroup(groupID); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

// groupMembershipActiveAt 判斷群組成員關係在指定時間是否有效。
func groupMembershipActiveAt(membership GroupMembership, at time.Time) bool {
	if !membership.ValidFrom.IsZero() && membership.ValidFrom.After(at) {
		return false
	}
	return membership.ValidUntil == nil || !membership.ValidUntil.Before(at)
}

// audit 處理稽核的服務流程。
func (c *Service) audit(ctx RequestContext, action, resource, target, severity string, details map[string]any) error {
	details = auditDetailsWithContext(ctx, details)
	result := auditResultFromDetails(details)
	traceID := ctx.TraceID
	if traceID == "" {
		traceID = ctx.RequestID
	}
	return c.store.AppendAuditLog(goContext(ctx), AuditLog{
		ID:             utils.NewID("aud"),
		TenantID:       ctx.TenantID,
		ActorAccountID: ctx.AccountID,
		Action:         action,
		Resource:       resource,
		Target:         target,
		Result:         result,
		Severity:       severity,
		TraceID:        traceID,
		Details:        details,
		CreatedAt:      c.Now(),
	})
}

// auditResultFromDetails 處理稽核結果 來源 details。
func auditResultFromDetails(details map[string]any) string {
	if result := strings.TrimSpace(stringFromAny(details["result"])); result != "" {
		return result
	}
	if reasonCode := strings.TrimSpace(stringFromAny(details["reason_code"])); reasonCode == "approval_required" {
		return "approval_required"
	}
	if allowed, ok := details["authz_decision"].(bool); ok {
		if !allowed {
			return "denied"
		}
		if requiresApproval, ok := details["requires_approval"].(bool); ok && requiresApproval {
			if strings.TrimSpace(stringFromAny(details["approval_method"])) == "" {
				return "approval_required"
			}
		}
		return "allowed"
	}
	return "success"
}

// forbiddenAuthz 處理禁止授權。
func forbiddenAuthz(decision CheckResult) error {
	return domain.ForbiddenReason(authzReasonCode(decision), decision.Reason)
}

// forbiddenDataScope 處理禁止資料範圍。
func forbiddenDataScope(message string) error {
	return domain.ForbiddenReason("data_scope_denied", message)
}

// authzReasonCode 處理授權 reason 碼。
func authzReasonCode(decision CheckResult) string {
	if decision.Reason == "approval_required" {
		return "approval_required"
	}
	switch decision.Reason {
	case "missing permission":
		switch decision.Action {
		case ActionRead:
			return "menu_denied"
		case ActionCreate, ActionUpdate, ActionDelete, ActionExport, ActionImport, ActionInvite, ActionApprove, ActionUpdateStatus, ActionStatusTransition:
			return "button_denied"
		default:
			return "permission_missing"
		}
	case "relationship denied", "explicit deny":
		return "permission_missing"
	default:
		return "permission_missing"
	}
}

// auditDetailsWithContext 處理稽核 details with context。
func auditDetailsWithContext(ctx RequestContext, details map[string]any) map[string]any {
	out := utils.CopyStringMap(details)
	if out == nil {
		out = map[string]any{}
	}
	if ctx.RequestID != "" {
		out["request_id"] = ctx.RequestID
	}
	traceID := ctx.TraceID
	if traceID == "" {
		traceID = ctx.RequestID
	}
	if traceID != "" {
		out["trace_id"] = traceID
	}
	if ctx.AccountID != "" {
		out["actor_account_id"] = ctx.AccountID
	}
	if ctx.TenantID != "" {
		out["tenant_id"] = ctx.TenantID
	}
	if ctx.RouteApplicationCode != "" {
		out["application_code"] = ctx.RouteApplicationCode
	}
	if ctx.RouteResourceType != "" {
		out["resource_type"] = ctx.RouteResourceType
	}
	if ctx.RouteAction != "" {
		out["route_action"] = ctx.RouteAction
	}
	if ctx.RoutePath != "" {
		out["route_path"] = ctx.RoutePath
	}
	if ctx.AssumedRoleSessionID != "" {
		out["assumed_role_session_id"] = ctx.AssumedRoleSessionID
	}
	if ctx.ApprovalInstanceID != "" {
		out["approval_method"] = "workflow_approval"
		out["approval_instance_id"] = ctx.ApprovalInstanceID
	} else if ctx.ApprovalConfirmed {
		out["approval_method"] = "header_confirmation"
	}
	return out
}

// auditDecisionDetails 處理稽核決策 details。
func auditDecisionDetails(ctx RequestContext, decision CheckResult, details map[string]any) map[string]any {
	out := auditDetailsWithContext(ctx, details)
	out["authz_decision"] = decision.Allowed
	if decision.ApplicationCode != "" {
		out["application_code"] = decision.ApplicationCode
	}
	if decision.ResourceType != "" {
		out["resource_type"] = decision.ResourceType
	}
	if _, ok := out["reason"]; ok {
		out["authz_reason"] = decision.Reason
	} else {
		out["reason"] = decision.Reason
	}
	out["reason_code"] = authzReasonCode(decision)
	out["matched_permissions"] = decision.MatchedPermissions
	out["matched_sources"] = decision.MatchedBy
	out["permission_boundary"] = decision.PermissionBoundary
	out["data_scope"] = decision.Scope
	out["field_policies"] = decision.FieldPolicies
	out["requires_approval"] = decision.RequiresApproval
	out["risk_level"] = decision.RiskLevel
	out["approval_type"] = decision.ApprovalType
	out["approval_reason"] = decision.ApprovalReason
	return out
}

// permissionMatches 處理權限 matches。
func permissionMatches(perm Permission, req CheckRequest, account Account) bool {
	perm = normalizePermission(perm)
	req = normalizeCheckRequest(req)
	if !wildcardMatch(req.Resource, perm.Resource) {
		return false
	}
	if !wildcardMatch(string(req.Action), string(perm.Action)) {
		return false
	}
	if perm.Target != "" && perm.Target != "*" && !strings.HasPrefix(perm.Target, "rebac:") {
		target := utils.FirstNonEmpty(req.Target, req.TargetEmployeeID, req.ResourceID)
		if target != perm.Target {
			return false
		}
	}
	if perm.Scope == "" || perm.Scope == "all" {
		return true
	}
	switch perm.Scope {
	case "self", "own":
		if req.Scope != "" && !sameOwnScope(req.Scope, perm.Scope) {
			return false
		}
		if account.EmployeeID == "" {
			return false
		}
		if req.TargetEmployeeID != "" && req.TargetEmployeeID != account.EmployeeID {
			return false
		}
		if req.Target != "" && req.Target != account.EmployeeID {
			return false
		}
		return true
	default:
		return req.Scope == "" || perm.Scope == req.Scope
	}
}

// sameOwnScope 處理 same own 範圍。
func sameOwnScope(a, b Scope) bool {
	return (a == ScopeSelf || a == ScopeOwn) && (b == ScopeSelf || b == ScopeOwn)
}

// permissionLabel 處理權限 label。
func permissionLabel(p Permission) string {
	p = normalizePermission(p)
	label := p.Resource + "." + string(p.Action)
	if p.Target != "" {
		label += ":" + p.Target
	}
	if p.Scope != "" {
		label += "#" + string(p.Scope)
	}
	return label
}

// wildcardMatch 處理 wildcard match。
func wildcardMatch(value, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return strings.EqualFold(value, pattern)
}

// uniqueStrings 處理 unique 字串。
func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// capabilitiesFromPermissions 處理 capabilities 來源 權限。
func capabilitiesFromPermissions(perms []Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, permissionLabel(perm))
	}
	return out
}

func effectiveAccessPermissionsFromGrants(grants []authzGrant, boundary map[string]any, requireAssumed bool) []Permission {
	normalAllowed := map[string]struct{}{}
	assumedAllowed := map[string]struct{}{}
	denied := make([]string, 0)
	for _, grant := range grants {
		perm := normalizePermission(grant.Permission)
		key := permissionKey(perm.ApplicationCode, perm.ResourceType, perm.Action)
		if permissionEffect(grant) == "deny" {
			denied = append(denied, key)
			continue
		}
		if policyDenies(boundary, key) || !policyAllows(boundary, key) {
			continue
		}
		if grant.SourceKind == authzGrantSourceAssumed {
			assumedAllowed[key] = struct{}{}
			continue
		}
		normalAllowed[key] = struct{}{}
	}

	out := make([]Permission, 0, len(grants))
	seen := map[string]struct{}{}
	for _, grant := range grants {
		perm := normalizePermission(grant.Permission)
		key := permissionKey(perm.ApplicationCode, perm.ResourceType, perm.Action)
		if permissionEffect(grant) == "deny" {
			continue
		}
		if policyDenies(boundary, key) || !policyAllows(boundary, key) || permissionKeyDenied(key, denied) {
			continue
		}
		if requireAssumed {
			if _, ok := normalAllowed[key]; !ok {
				continue
			}
			if _, ok := assumedAllowed[key]; !ok {
				continue
			}
		}
		label := permissionLabel(perm)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, perm)
	}
	return out
}

func permissionKeyDenied(key string, denied []string) bool {
	for _, pattern := range denied {
		if permissionKeyMatches(key, pattern) {
			return true
		}
	}
	return false
}

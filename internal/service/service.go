package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

type Service struct {
	store         repository.Store
	now           func() time.Time
	logger        *slog.Logger
	authzSnapshot AuthzSnapshotCache
	relationships RelationshipChecker
	objectStore   ObjectStore
}

type Options struct {
	Logger        *slog.Logger
	AuthzSnapshot AuthzSnapshotCache
	Relationships RelationshipChecker
	ObjectStore   ObjectStore
}

func New(store repository.Store, options ...Options) *Service {
	cfg := Options{}
	if len(options) > 0 {
		cfg = options[0]
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:         store,
		now:           time.Now,
		logger:        logger,
		authzSnapshot: cfg.AuthzSnapshot,
		relationships: cfg.Relationships,
		objectStore:   firstObjectStore(cfg.ObjectStore),
	}
}

func (c *Service) Store() repository.Store {
	return c.store
}

func (c *Service) withTenantTransaction(ctx RequestContext, fn func(*Service) error) error {
	return repository.WithinTenantTransaction(goContext(ctx), c.store, ctx.TenantID, func(store repository.Store) error {
		next := *c
		next.store = store
		return fn(&next)
	})
}

func goContext(ctx RequestContext) context.Context {
	if ctx.Context != nil {
		return ctx.Context
	}
	return context.Background()
}

func (c *Service) Now() time.Time {
	return c.now().UTC()
}

func (c *Service) logInfo(ctx RequestContext, message string, args ...any) {
	c.loggerFor(ctx).InfoContext(goContext(ctx), message, args...)
}

func (c *Service) logWarn(ctx RequestContext, message string, args ...any) {
	c.loggerFor(ctx).WarnContext(goContext(ctx), message, args...)
}

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
	if ctx.AssumedRoleID != "" {
		attrs = append(attrs, "assumed_role_id", ctx.AssumedRoleID)
	}
	if ctx.AssumedRoleSessionID != "" {
		attrs = append(attrs, "assumed_role_session_id", ctx.AssumedRoleSessionID)
	}
	return logger.With(attrs...)
}

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
	return account, tenant, nil
}

type AuditTarget struct {
	Event    string
	Resource string
	Target   string
}

type AuthzAudit struct {
	service  *Service
	target   AuditTarget
	decision CheckResult
}

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
	if decision.RequiresApproval && !ctx.ApprovalConfirmed {
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

func (a AuthzAudit) Commit(ctx RequestContext) error {
	return a.CommitWith(ctx, a.service)
}

func (a AuthzAudit) CommitWith(ctx RequestContext, service *Service) error {
	if service == nil {
		return nil
	}
	return service.auditAuthzTarget(ctx, a.target, a.decision)
}

func (a AuditTarget) fromRequest(req CheckRequest) AuditTarget {
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

func (c *Service) auditAuthzTarget(ctx RequestContext, audit AuditTarget, decision CheckResult) error {
	if audit.Event == "" {
		return nil
	}
	return c.auditAuthzDecision(ctx, audit.Event, audit.Resource, audit.Target, decision)
}

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

	groups := make([]UserGroup, 0)
	for _, groupID := range account.UserGroupIDs {
		group, ok, err := c.store.GetUserGroup(goContext(ctx), ctx.TenantID, groupID)
		if err != nil {
			return nil, nil, nil, err
		}
		if !ok {
			continue
		}
		groups = append(groups, group)
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
	permissions := make([]Permission, 0)
	for _, id := range setIDs {
		set, ok, err := c.store.GetPermissionSet(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return nil, nil, nil, err
		}
		if !ok {
			continue
		}
		permissionSets = append(permissionSets, set)
		permissions = append(permissions, set.Permissions...)
	}

	return permissions, permissionSets, groups, nil
}

func (c *Service) canAssumeRole(ctx RequestContext, account Account, role AssumableRole) (bool, error) {
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode: AppIAM,
		ResourceType:    ResourceAssumableRole,
		ResourceID:      role.ID,
		Target:          role.ID,
		Action:          ActionAssume,
	})
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func (c *Service) audit(ctx RequestContext, action, resource, target, severity string, details map[string]any) error {
	details = auditDetailsWithContext(ctx, details)
	return c.store.AppendAuditLog(goContext(ctx), AuditLog{
		ID:             utils.NewID("aud"),
		TenantID:       ctx.TenantID,
		ActorAccountID: ctx.AccountID,
		Action:         action,
		Resource:       resource,
		Target:         target,
		Severity:       severity,
		TraceID:        ctx.RequestID,
		Details:        details,
		CreatedAt:      c.Now(),
	})
}

func forbiddenAuthz(decision CheckResult) error {
	return domain.ForbiddenReason(authzReasonCode(decision), decision.Reason)
}

func forbiddenDataScope(message string) error {
	return domain.ForbiddenReason("data_scope_denied", message)
}

func authzReasonCode(decision CheckResult) string {
	switch decision.Reason {
	case "missing permission":
		switch decision.Action {
		case ActionRead:
			return "menu_denied"
		case ActionCreate, ActionUpdate, ActionDelete, ActionExport, ActionImport, ActionInvite, ActionUpdateStatus, ActionStatusTransition:
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

func auditDetailsWithContext(ctx RequestContext, details map[string]any) map[string]any {
	out := utils.CopyStringMap(details)
	if out == nil {
		out = map[string]any{}
	}
	if ctx.RequestID != "" {
		out["request_id"] = ctx.RequestID
		out["trace_id"] = ctx.RequestID
	}
	if ctx.AccountID != "" {
		out["actor_account_id"] = ctx.AccountID
	}
	if ctx.TenantID != "" {
		out["tenant_id"] = ctx.TenantID
	}
	return out
}

func auditDecisionDetails(ctx RequestContext, decision CheckResult, details map[string]any) map[string]any {
	out := auditDetailsWithContext(ctx, details)
	out["authz_decision"] = decision.Allowed
	out["reason"] = decision.Reason
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

func sameOwnScope(a, b Scope) bool {
	return (a == ScopeSelf || a == ScopeOwn) && (b == ScopeSelf || b == ScopeOwn)
}

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

func wildcardMatch(value, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return strings.EqualFold(value, pattern)
}

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

func capabilitiesFromPermissions(perms []Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, permissionLabel(perm))
	}
	return out
}

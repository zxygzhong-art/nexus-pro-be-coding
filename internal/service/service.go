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

// Service is the root business facade composed from stores and platform adapters.
type Service struct {
	store         repository.Store
	now           func() time.Time
	logger        *slog.Logger
	authzSnapshot AuthzSnapshotCache
	relationships RelationshipChecker
	objectStore   ObjectStore
}

// Options configures optional runtime adapters for the service facade.
type Options struct {
	Logger        *slog.Logger
	AuthzSnapshot AuthzSnapshotCache
	Relationships RelationshipChecker
	ObjectStore   ObjectStore
}

// RelationshipChecker verifies external relationship tuples for authorization decisions.
type RelationshipChecker interface {
	CheckRelationship(ctx context.Context, check domain.RelationshipCheck) (bool, error)
}

// New builds a service facade over the supplied repository store.
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

// Store exposes the backing repository for integrations that need lower-level access.
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

// Now returns the current UTC time and gives tests a single override point.
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

// AuditTarget identifies the resource written to the audit log after authorization.
type AuditTarget struct {
	Event    string
	Resource string
	Target   string
}

// AuthzAudit is a deferred audit handle returned after successful authorization.
type AuthzAudit struct {
	service  *Service
	target   AuditTarget
	decision CheckResult
}

// Authorize resolves the account, evaluates permissions, and prepares audit recording.
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

// ValidateApprovalInstance verifies that a high-risk approval instance belongs to the caller.
func (c *Service) ValidateApprovalInstance(ctx RequestContext, req CheckRequest) error {
	return c.validateApprovalInstance(ctx, normalizeCheckRequest(req))
}

func (c *Service) confirmApproval(ctx RequestContext, req CheckRequest) error {
	if ctx.ApprovalInstanceID != "" {
		return c.ValidateApprovalInstance(ctx, req)
	}
	if ctx.ApprovalConfirmed {
		return nil
	}
	return domain.ForbiddenReason("approval_required", "high-risk action requires approval")
}

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

func approvalPayloadMatchesAny(payload map[string]any, expected string, keys ...string) bool {
	for _, key := range keys {
		if value, ok := payload[key]; ok && strings.EqualFold(strings.TrimSpace(stringFromAny(value)), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}

func approvalPayloadValueMatches(actual, expected any) bool {
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	if actualErr == nil && expectedErr == nil {
		return string(actualJSON) == string(expectedJSON)
	}
	return reflect.DeepEqual(actual, expected)
}

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

// Commit writes the prepared authorization audit record with its original service.
func (a AuthzAudit) Commit(ctx RequestContext) error {
	return a.CommitWith(ctx, a.service)
}

// CommitWith writes the prepared authorization audit record with an override service.
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
	if ctx.ApprovalInstanceID != "" {
		out["approval_method"] = "workflow_approval"
		out["approval_instance_id"] = ctx.ApprovalInstanceID
	} else if ctx.ApprovalConfirmed {
		out["approval_method"] = "header_confirmation"
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

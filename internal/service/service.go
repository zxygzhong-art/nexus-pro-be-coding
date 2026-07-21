package service

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/utils"
)

// Service 定義服務的資料結構。
type Service struct {
	store                   repository.Store
	now                     func() time.Time
	logger                  *slog.Logger
	authzSnapshot           AuthzSnapshotCache
	relationships           RelationshipChecker
	openFGAScopeChecks      bool
	agentChatRuntime        AgentChatRuntime
	liteLLMAdmin            LiteLLMAdminClient
	knowledgeEmbedder       KnowledgeEmbedder
	objectStore             ObjectStore
	ehrmsClient             EHRMSClient
	identityProvisioner     IdentityProvisioner
	identityPasswordChanger IdentityPasswordChanger
	formApprovalWorkflows   FormApprovalWorkflowClient
	credentialCipher        CredentialCipher
}

// Options 定義選項的資料結構。
type Options struct {
	Logger                  *slog.Logger
	Now                     func() time.Time
	AuthzSnapshot           AuthzSnapshotCache
	Relationships           RelationshipChecker
	OpenFGAScopeChecks      bool
	AgentChatRuntime        AgentChatRuntime
	LiteLLMAdmin            LiteLLMAdminClient
	KnowledgeEmbedder       KnowledgeEmbedder
	ObjectStore             ObjectStore
	EHRMSClient             EHRMSClient
	IdentityProvisioner     IdentityProvisioner
	IdentityPasswordChanger IdentityPasswordChanger
	FormApprovalWorkflows   FormApprovalWorkflowClient
	CredentialCipher        CredentialCipher
}

// CredentialCipher encrypts persisted secrets with contextual associated data.
type CredentialCipher interface {
	Encrypt(plaintext, associatedData []byte) (string, error)
	Decrypt(ciphertext string, associatedData []byte) ([]byte, error)
}

// LiteLLMAdminClient 定義 LiteLLM 管理與探測 client 行為契約。
type LiteLLMAdminClient interface {
	SyncModel(context.Context, domain.AgentModel) (string, error)
	DeleteModel(context.Context, string) (string, error)
	ListManagedModelIDs(context.Context) ([]string, error)
	TestModel(context.Context, domain.AgentModel) (string, error)
}

// KnowledgeEmbedder generates vectors through a stable public model alias.
type KnowledgeEmbedder interface {
	Model() string
	Embed(context.Context, []string) ([][]float32, error)
}

// RelationshipChecker 定義關係 checker 的行為契約。
type RelationshipChecker interface {
	CheckRelationship(ctx context.Context, check domain.RelationshipCheck) (bool, error)
}

// EHRMSClient 定義 eHRMS client 的行為契約。
type EHRMSClient interface {
	ListEmployees(context.Context) ([]domain.EHRMSEmployeeRecord, error)
	ListDepartments(context.Context) ([]domain.EHRMSDepartmentRecord, error)
	ListPositions(context.Context) ([]domain.EHRMSPositionRecord, error)
	ListAttendance(context.Context) ([]domain.EHRMSAttendanceRecord, error)
	ListLeaveTypes(context.Context) ([]domain.EHRMSLeaveType, error)
	ListLeaveBalances(context.Context) ([]domain.EHRMSLeaveBalanceRecord, error)
	ListLeaveDetails(context.Context) ([]domain.EHRMSLeaveDetailRecord, error)
}

// IdentityProvisioner 定義身分 provisioner 的行為契約。
type IdentityProvisioner interface {
	EnsureUser(context.Context, domain.IdentityProvisioningInput) (domain.ProvisionedIdentity, error)
}

// IdentityPasswordChanger updates one externally managed password after ownership and current-credential verification.
type IdentityPasswordChanger interface {
	ChangePassword(context.Context, domain.IdentityPasswordChangeInput) error
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
		store:                   store,
		now:                     now,
		logger:                  logger,
		authzSnapshot:           cfg.AuthzSnapshot,
		relationships:           cfg.Relationships,
		openFGAScopeChecks:      cfg.OpenFGAScopeChecks,
		agentChatRuntime:        cfg.AgentChatRuntime,
		liteLLMAdmin:            cfg.LiteLLMAdmin,
		knowledgeEmbedder:       cfg.KnowledgeEmbedder,
		objectStore:             firstObjectStore(cfg.ObjectStore),
		ehrmsClient:             cfg.EHRMSClient,
		identityProvisioner:     cfg.IdentityProvisioner,
		identityPasswordChanger: cfg.IdentityPasswordChanger,
		formApprovalWorkflows:   cfg.FormApprovalWorkflows,
		credentialCipher:        cfg.CredentialCipher,
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

// AuditTarget 定義稽覈 target 的資料結構。
type AuditTarget struct {
	Event    string
	Resource string
	Target   string
}

// AuthzAudit 定義授權稽覈的資料結構。
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
	return account, decision, done, nil
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

// auditAuthzTarget 處理稽覈授權 target 的服務流程。
func (c *Service) auditAuthzTarget(ctx RequestContext, audit AuditTarget, decision CheckResult) error {
	if audit.Event == "" {
		return nil
	}
	if !shouldAuditAuthzDecision(decision) {
		return nil
	}
	return c.auditAuthzDecision(ctx, audit.Event, audit.Resource, audit.Target, decision)
}

// shouldAuditAuthzDecision 判斷授權決策是否需要寫入操作稽覈。
func shouldAuditAuthzDecision(decision CheckResult) bool {
	if !decision.Allowed {
		return true
	}
	return decision.Action != ActionRead || decision.RiskLevel == string(domain.RiskHigh) || decision.RiskLevel == string(domain.RiskCritical)
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
	grants, setIDs, assumed, boundary, err := c.collectAuthzGrants(ctx, account)
	if err != nil {
		return nil, nil, nil, err
	}
	permissions := effectiveAccessPermissionCandidates(grants)
	permissions, err = c.projectEffectivePermissionScopes(ctx, account, permissions, grants, setIDs, assumed, boundary)
	if err != nil {
		return nil, nil, nil, err
	}

	return permissions, permissionSets, groups, nil
}

// activeUserGroupsForAccount 以成員關係表解析帳號目前有效的使用者羣組。
func (c *Service) activeUserGroupsForAccount(ctx RequestContext, account Account) ([]UserGroup, error) {
	groups, _, err := c.activeUserGroupsForAccountWithExpiries(ctx, account)
	return groups, err
}

// activeUserGroupsForAccountWithExpiries 同時回傳羣組成員關係期限，供授權快照限制 TTL。
func (c *Service) activeUserGroupsForAccountWithExpiries(ctx RequestContext, account Account) ([]UserGroup, map[string]*time.Time, error) {
	at := c.Now()
	memberships, err := c.store.ListActiveGroupMembershipsForAccount(goContext(ctx), ctx.TenantID, account.ID, at)
	if err != nil {
		return nil, nil, err
	}
	groups := make([]UserGroup, 0, len(memberships)+len(account.UserGroupIDs))
	seen := map[string]struct{}{}
	expiries := map[string]*time.Time{}
	addGroup := func(groupID string, validUntil *time.Time) error {
		if _, ok := seen[groupID]; ok {
			expiries[groupID] = earliestExpiry(expiries[groupID], validUntil)
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
		expiries[groupID] = validUntil
		groups = append(groups, group)
		return nil
	}
	for _, membership := range memberships {
		if err := addGroup(membership.UserGroupID, membership.ValidUntil); err != nil {
			return nil, nil, err
		}
	}
	return groups, expiries, nil
}

// groupMembershipActiveAt 判斷羣組成員關係在指定時間是否有效。
func groupMembershipActiveAt(membership GroupMembership, at time.Time) bool {
	if !membership.ValidFrom.IsZero() && membership.ValidFrom.After(at) {
		return false
	}
	return membership.ValidUntil == nil || at.Before(*membership.ValidUntil)
}

// audit 處理稽覈的服務流程。
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

// auditDetailAuthzDecisionKey 是稽覈 details 中記錄授權決策（bool）的鍵名，寫讀雙方共用。
const auditDetailAuthzDecisionKey = "authz_decision"

// auditResultFromDetails 處理稽覈結果 來源 details。
func auditResultFromDetails(details map[string]any) string {
	if result := strings.TrimSpace(stringFromAny(details["result"])); result != "" {
		return result
	}
	raw, present := details[auditDetailAuthzDecisionKey]
	if !present {
		return "success"
	}
	allowed, ok := raw.(bool)
	if !ok {
		// 決策鍵存在但型別異常：顯式標記 unknown，避免靜默歸類為 success。
		return "unknown"
	}
	if !allowed {
		return "denied"
	}
	return "allowed"
}

// forbiddenAuthz 處理禁止授權。
func forbiddenAuthz(decision CheckResult) error {
	return domain.ForbiddenReason(authzReasonCode(decision), decision.Reason)
}

// ForbiddenDataScope 處理禁止資料範圍。
func ForbiddenDataScope(message string) error {
	return domain.ForbiddenReason("data_scope_denied", message)
}

// authzReasonCode 處理授權 reason 碼。
func authzReasonCode(decision CheckResult) string {
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
	case "data scope denied":
		return "data_scope_denied"
	default:
		return "permission_missing"
	}
}

// auditDetailsWithContext 處理稽覈 details with context。
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
	return out
}

// auditDecisionDetails 處理稽覈決策 details。
func auditDecisionDetails(ctx RequestContext, decision CheckResult, details map[string]any) map[string]any {
	out := auditDetailsWithContext(ctx, details)
	out[auditDetailAuthzDecisionKey] = decision.Allowed
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
	out["risk_level"] = decision.RiskLevel
	return out
}

// permissionMatches 處理權限 matches。
func permissionMatches(perm Permission, req CheckRequest, account Account) bool {
	perm = normalizePermission(perm)
	req = normalizeCheckRequest(req)
	switch perm.PermissionType {
	case "", PermissionTypeAPI, PermissionTypeButton:
	case PermissionTypeMenu, PermissionTypeField, PermissionTypeScope:
		return false
	default:
		return false
	}
	if !wildcardMatch(string(req.ApplicationCode), string(perm.ApplicationCode)) ||
		!wildcardMatch(string(req.ResourceType), string(perm.ResourceType)) {
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

// effectiveAccessPermissionCandidates collects positive candidates only.
// Concrete boundary, deny, target, scope and assumed-role intersection checks
// are deliberately deferred to the authoritative projection decision pass.
func effectiveAccessPermissionCandidates(grants []authzGrant) []Permission {
	out := make([]Permission, 0, len(grants))
	seen := map[string]struct{}{}
	for _, grant := range grants {
		perm := normalizePermission(grant.Permission)
		if !permissionCanAuthorizeRequest(perm) {
			continue
		}
		if permissionEffect(grant) == "deny" {
			continue
		}
		// Boundary and explicit denies are intentionally deferred until every
		// candidate has been materialized with its target and concrete key.
		label := effectivePermissionIdentity(perm) + "|relation:" + relationshipConstraint(perm)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, perm)
	}
	for _, grant := range grants {
		perm := normalizePermission(grant.Permission)
		if perm.PermissionType != PermissionTypeMenu || permissionEffect(grant) == "deny" {
			continue
		}
		menuKey := strings.TrimSpace(perm.MenuKey)
		if menuKey == "" {
			menuKey = strings.TrimSpace(perm.Resource)
		}
		label := effectivePermissionIdentity(perm)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, perm)
	}
	return out
}

// permissionCoveredByAny uses the same wildcard resource/action semantics as
// request authorization. The candidate must be no broader than at least one
// grant on the other side of an assumed-role intersection.
func permissionCoveredByAny(candidate Permission, grants []Permission) bool {
	for _, grant := range grants {
		grant = normalizePermission(grant)
		if !wildcardMatch(string(candidate.ApplicationCode), string(grant.ApplicationCode)) ||
			!wildcardMatch(string(candidate.ResourceType), string(grant.ResourceType)) ||
			!wildcardMatch(string(candidate.Action), string(grant.Action)) {
			continue
		}
		if grant.Target != "" && grant.Target != "*" && !strings.HasPrefix(grant.Target, "rebac:") &&
			!wildcardMatch(candidate.Target, grant.Target) {
			continue
		}
		return true
	}
	return false
}

func projectionPermissionHasWildcard(permission Permission) bool {
	permission = normalizePermission(permission)
	return permission.ApplicationCode == "" || permission.ApplicationCode == "*" ||
		permission.ResourceType == "" || permission.ResourceType == "*" ||
		permission.Action == "" || permission.Action == "*"
}

// materializeProjectionPermissions turns intersected wildcard grants into the
// finite set of concrete HTTP permissions known by the route policy catalog.
// Exact custom permissions (for example agent tool targets) remain available,
// while unknown wildcard namespaces fail closed instead of being advertised as
// broader client capabilities.
func materializeProjectionPermissions(permissions []Permission) []Permission {
	catalog := make([]Permission, 0, len(permissions)+len(domain.DefaultRoutePolicies))
	catalogSeen := map[string]struct{}{}
	addCatalog := func(permission Permission) {
		permission = normalizePermission(permission)
		if !permissionCanAuthorizeRequest(permission) || projectionPermissionHasWildcard(permission) {
			return
		}
		identity := projectionRequestIdentity(permission)
		if _, exists := catalogSeen[identity]; exists {
			return
		}
		catalogSeen[identity] = struct{}{}
		catalog = append(catalog, permission)
	}
	// Prefer exact grants because they retain target/relation/menu metadata.
	for _, permission := range permissions {
		addCatalog(permission)
	}
	for _, permission := range defaultPermissions() {
		addCatalog(permission)
	}

	out := make([]Permission, 0, len(permissions))
	indexes := map[string]int{}
	addProjection := func(permission Permission) {
		permission = normalizePermission(permission)
		identity := projectionRequestIdentity(permission)
		if !permissionCanAuthorizeRequest(permission) {
			identity = "control|" + effectivePermissionIdentity(permission)
		}
		if index, exists := indexes[identity]; exists {
			current := &out[index]
			if current.MenuKey == "" && permission.MenuKey != "" {
				current.MenuKey = permission.MenuKey
			}
			if current.PermissionType == "" && permission.PermissionType != "" {
				current.PermissionType = permission.PermissionType
			}
			if riskRank(permission.RiskLevel) > riskRank(current.RiskLevel) {
				current.RiskLevel = permission.RiskLevel
			}
			return
		}
		indexes[identity] = len(out)
		out = append(out, permission)
	}

	for _, permission := range permissions {
		permission = normalizePermission(permission)
		if permissionCanAuthorizeRequest(permission) {
			if menuKey := strings.TrimSpace(permission.MenuKey); menuKey != "" {
				addProjection(Permission{
					PermissionType: PermissionTypeMenu,
					Resource:       canonicalPageMenuKey(menuKey),
					Action:         ActionRead,
					MenuKey:        canonicalPageMenuKey(menuKey),
				})
			}
		}
		if !permissionCanAuthorizeRequest(permission) || !projectionPermissionHasWildcard(permission) {
			addProjection(permission)
			continue
		}
		for _, candidate := range catalog {
			if permissionCoveredByAny(candidate, []Permission{permission}) {
				addProjection(candidate)
			}
		}
	}
	return out
}

func projectionRequestIdentity(permission Permission) string {
	permission = normalizePermission(permission)
	permissionType := permission.PermissionType
	if permissionType == "" {
		permissionType = PermissionTypeAPI
	}
	return strings.Join([]string{
		string(permissionType),
		string(permission.ApplicationCode),
		string(permission.ResourceType),
		string(permission.Action),
		permission.Target,
		relationshipConstraint(permission),
	}, "|")
}

// projectEffectivePermissionScopes narrows every projected request permission
// to the same final scope produced by authorization, including data-scope and
// boundary intersections. Control permissions are retained only when the
// narrowed request permissions still satisfy their page requirement.
func (c *Service) projectEffectivePermissionScopes(
	ctx RequestContext,
	account Account,
	permissions []Permission,
	grants []authzGrant,
	setIDs []string,
	assumed *AssumedRoleDecision,
	boundary map[string]any,
) ([]Permission, error) {
	permissions = materializeProjectionPermissions(permissions)
	scopeCache := &authzDecisionScopeCache{}
	requestPermissions := make([]Permission, 0, len(permissions))
	controlPermissions := make([]Permission, 0, len(permissions))
	seen := map[string]struct{}{}
	for _, permission := range permissions {
		permission = normalizePermission(permission)
		if !permissionCanAuthorizeRequest(permission) {
			controlPermissions = append(controlPermissions, permission)
			continue
		}
		// Relation/object grants require a concrete resource ID and therefore must
		// not be projected as tenant-wide client capabilities. Object endpoints
		// continue to use the authoritative per-object Authz.Check path.
		if relationshipConstraint(permission) != "" {
			continue
		}
		decision, err := c.evaluateAuthzDecisionWithFieldPolicies(ctx, account, CheckRequest{
			ApplicationCode: permission.ApplicationCode,
			ResourceType:    permission.ResourceType,
			Resource:        permission.Resource,
			Action:          permission.Action,
			Target:          permission.Target,
		}, grants, setIDs, assumed, boundary, nil, false, scopeCache)
		if err != nil {
			var appErr *domain.AppError
			if errors.As(err, &appErr) && appErr.ReasonCode == "data_scope_denied" {
				continue
			}
			return nil, err
		}
		if !decision.Allowed {
			continue
		}
		if decision.Scope == ScopeObject {
			continue
		}
		permission.Scope = decision.Scope
		permission.Conditions = utils.CopyStringMap(decision.Conditions)
		permission.RiskLevel = maxRiskLevel(permission.RiskLevel, decision.RiskLevel)
		identity := effectivePermissionIdentity(permission)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		requestPermissions = append(requestPermissions, permission)
	}

	out := requestPermissions
	for _, permission := range controlPermissions {
		if permission.PermissionType == PermissionTypeMenu {
			menuKey := strings.TrimSpace(permission.MenuKey)
			if menuKey == "" {
				menuKey = strings.TrimSpace(permission.Resource)
			}
			requirement, ok := menuPrimaryReadRequirement(menuKey)
			if !ok || !permissionsSatisfyMenuRequirement(requestPermissions, requirement) {
				continue
			}
		}
		identity := effectivePermissionIdentity(permission)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		out = append(out, permission)
	}
	return out, nil
}

// permissionCanAuthorizeRequest 限制只有 API、button 與舊版未標型別權限可參與 API 授權。
func permissionCanAuthorizeRequest(permission Permission) bool {
	switch permission.PermissionType {
	case "", PermissionTypeAPI, PermissionTypeButton:
		return true
	default:
		return false
	}
}

// effectivePermissionIdentity 避免 menu grant 與同名 API grant 在 /me 回應互相覆蓋。
func effectivePermissionIdentity(permission Permission) string {
	return strings.Join([]string{
		string(permission.PermissionType),
		permissionLabel(permission),
		canonicalPageMenuKey(permission.MenuKey),
	}, "|")
}

package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

const (
	openFGATypeTenant        = "tenant"
	openFGATypeOrgUnit       = "org_unit"
	openFGATypeUserGroup     = "user_group"
	openFGATypeEmployee      = "hr.employee"
	openFGATypeAssumableRole = "assumable_role"
	openFGATypeAgentTool     = "agent_tool"

	openFGARelationTenant              = "tenant"
	openFGARelationTenantMember        = "member"
	openFGARelationTenantAdmin         = "admin"
	openFGARelationTenantSecurityAdmin = "security_admin"
	openFGARelationOrgUnitParent       = "parent"
	openFGARelationOrgUnitMember       = "member"
	openFGARelationOrgUnitManager      = "manager"
	openFGARelationOrgUnitMemberTree   = "member_recursive"
	openFGARelationUserGroupMember     = "member"
	openFGARelationUserGroupManager    = "manager"
	openFGARelationEmployeeOrg         = "org"
	openFGARelationTrustedUser         = "trusted_user"
	openFGARelationTrustedGroup        = "trusted_group"
	openFGARelationApprover            = "approver"
	openFGARelationCanAssume           = "can_assume"
	openFGARelationRunner              = "runner"
	openFGARelationCanRun              = "can_run"
	openFGASubjectTypeAccount          = "account"
	openFGASubjectTypeOrgUnit          = "org_unit"
	openFGASubjectTypeTenant           = "tenant"
	openFGASubjectTypeUserGroupMember  = "user_group#member"
)

// OpenFGABackfillInput 定義 OpenFGA tuple backfill 輸入。
type OpenFGABackfillInput struct {
	TenantID string
	DryRun   bool
	Logger   *slog.Logger
}

// OpenFGABackfillResult 定義 OpenFGA tuple backfill 結果。
type OpenFGABackfillResult struct {
	TenantID      string `json:"tenant_id"`
	DesiredTuples int    `json:"desired_tuples"`
	CreatedTuples int    `json:"created_tuples"`
	SkippedTuples int    `json:"skipped_tuples"`
	OutboxEvents  int    `json:"outbox_events"`
	DryRun        bool   `json:"dry_run"`
}

// OpenFGAGrantRelationshipInput 定義手工授權 OpenFGA tuple 輸入。
type OpenFGAGrantRelationshipInput struct {
	TenantID    string
	ObjectType  string
	ObjectID    string
	Relation    string
	SubjectType string
	SubjectID   string
	DryRun      bool
	Logger      *slog.Logger
}

// OpenFGAGrantRelationshipResult 定義手工授權 OpenFGA tuple 結果。
type OpenFGAGrantRelationshipResult struct {
	TenantID     string `json:"tenant_id"`
	ObjectType   string `json:"object_type"`
	ObjectID     string `json:"object_id"`
	Relation     string `json:"relation"`
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	Created      bool   `json:"created"`
	Skipped      bool   `json:"skipped"`
	OutboxEvents int    `json:"outbox_events"`
	DryRun       bool   `json:"dry_run"`
}

// OpenFGAGrantTenantAdminInput 定義租戶管理員 tuple 手工授權輸入。
type OpenFGAGrantTenantAdminInput struct {
	TenantID  string
	AccountID string
	DryRun    bool
	Logger    *slog.Logger
}

// OpenFGAGrantAgentToolInput 定義 agent tool tuple 手工授權輸入。
type OpenFGAGrantAgentToolInput struct {
	TenantID  string
	ToolID    string
	AccountID string
	DryRun    bool
	Logger    *slog.Logger
}

// OpenFGABackfillTuples 依租戶來源資料補齊 OpenFGA relationship tuple。
func (c *Service) OpenFGABackfillTuples(ctx context.Context, input OpenFGABackfillInput) (OpenFGABackfillResult, error) {
	tenantID := strings.TrimSpace(input.TenantID)
	if tenantID == "" {
		return OpenFGABackfillResult{}, BadRequest("tenant_id is required")
	}
	if c == nil || c.store == nil {
		return OpenFGABackfillResult{}, BadRequest("service store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := input.Logger
	if logger == nil {
		logger = c.logger
	}
	if logger == nil {
		logger = slog.Default()
	}
	result := OpenFGABackfillResult{TenantID: tenantID, DryRun: input.DryRun}
	err := repository.WithinTenantTransaction(ctx, c.store, tenantID, func(store repository.Store) error {
		next := *c
		next.store = store
		tuples, err := next.desiredOpenFGATuples(ctx, tenantID)
		if err != nil {
			return err
		}
		result.DesiredTuples = len(tuples)
		for i, tuple := range tuples {
			exists, err := next.relationshipTupleExists(ctx, tuple)
			if err != nil {
				return err
			}
			if exists {
				result.SkippedTuples++
				continue
			}
			if input.DryRun {
				result.CreatedTuples++
				continue
			}
			if err := next.applyRelationshipTupleChange(RequestContext{Context: ctx, TenantID: tenantID}, domain.AuthzRelationshipTupleChange{
				Operation: domain.AuthzRelationshipTupleWrite,
				Tuple:     tuple,
			}); err != nil {
				return err
			}
			result.CreatedTuples++
			result.OutboxEvents++
			if result.CreatedTuples == 1 || result.CreatedTuples%100 == 0 || i == len(tuples)-1 {
				logger.InfoContext(ctx, "openfga backfill progress",
					"tenant_id", tenantID,
					"desired", result.DesiredTuples,
					"created", result.CreatedTuples,
					"skipped", result.SkippedTuples,
				)
			}
		}
		return nil
	})
	if err != nil {
		return OpenFGABackfillResult{}, err
	}
	logger.InfoContext(ctx, "openfga backfill completed",
		"tenant_id", result.TenantID,
		"desired", result.DesiredTuples,
		"created", result.CreatedTuples,
		"skipped", result.SkippedTuples,
		"outbox_events", result.OutboxEvents,
		"dry_run", result.DryRun,
	)
	return result, nil
}

// desiredOpenFGATuples 從來源表計算本批 OpenFGA tuples。
func (c *Service) desiredOpenFGATuples(ctx context.Context, tenantID string) ([]domain.AuthzRelationshipTuple, error) {
	tenant, ok, err := c.store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, NotFound("tenant", tenantID)
	}
	now := c.Now()
	out := make([]domain.AuthzRelationshipTuple, 0)
	add := func(objectType, objectID, relation, subjectType, subjectID string) {
		if strings.TrimSpace(objectID) == "" || strings.TrimSpace(subjectID) == "" {
			return
		}
		out = append(out, domain.AuthzRelationshipTuple{
			ID:          utils.NewID("rel"),
			TenantID:    tenant.ID,
			ObjectType:  objectType,
			ObjectID:    objectID,
			Relation:    relation,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			CreatedAt:   now,
		})
	}

	accounts, err := c.store.ListAccounts(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if accountTenantMemberActive(account) {
			add(openFGATypeTenant, tenant.ID, openFGARelationTenantMember, openFGASubjectTypeAccount, account.ID)
		}
	}

	orgUnits, err := c.store.ListOrgUnits(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	for _, unit := range orgUnits {
		add(openFGATypeOrgUnit, unit.ID, openFGARelationTenant, openFGASubjectTypeTenant, tenant.ID)
		add(openFGATypeOrgUnit, unit.ID, openFGARelationOrgUnitParent, openFGASubjectTypeOrgUnit, unit.ParentID)
	}

	employees, err := c.store.ListEmployees(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	for _, employee := range employees {
		add(openFGATypeEmployee, employee.ID, employeeOwnerRelation, openFGASubjectTypeAccount, employee.AccountID)
		add(openFGATypeEmployee, employee.ID, openFGARelationEmployeeOrg, openFGASubjectTypeOrgUnit, employee.OrgUnitID)
		if employee.OrgUnitID != "" && employee.AccountID != "" {
			add(openFGATypeOrgUnit, employee.OrgUnitID, openFGARelationOrgUnitMember, openFGASubjectTypeAccount, employee.AccountID)
		}
		managerAccountID, err := c.HR().employeeAccountID(RequestContext{Context: ctx, TenantID: tenant.ID}, employee.ManagerEmployeeID)
		if err != nil {
			return nil, err
		}
		add(openFGATypeEmployee, employee.ID, employeeManagerRelation, openFGASubjectTypeAccount, managerAccountID)
	}

	groups, err := c.store.ListUserGroups(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		for _, accountID := range group.MemberAccountIDs {
			add(openFGATypeUserGroup, group.ID, openFGARelationUserGroupMember, openFGASubjectTypeAccount, accountID)
		}
	}

	roles, err := c.store.ListAssumableRoles(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		for _, tuple := range assumableRoleRelationshipTuples(RequestContext{TenantID: tenant.ID}, role, now) {
			out = append(out, tuple)
		}
	}

	for _, toolID := range defaultAgentToolIDs() {
		add(openFGATypeAgentTool, toolID, openFGARelationTenant, openFGASubjectTypeTenant, tenant.ID)
	}

	return dedupeAndSortRelationshipTuples(out), nil
}

// OpenFGAGrantTenantAdmin 手工授予 tenant#admin。
func (c *Service) OpenFGAGrantTenantAdmin(ctx context.Context, input OpenFGAGrantTenantAdminInput) (OpenFGAGrantRelationshipResult, error) {
	return c.openFGAGrantTenantAccountRelation(ctx, input, openFGARelationTenantAdmin)
}

// OpenFGAGrantTenantSecurityAdmin 手工授予 tenant#security_admin。
func (c *Service) OpenFGAGrantTenantSecurityAdmin(ctx context.Context, input OpenFGAGrantTenantAdminInput) (OpenFGAGrantRelationshipResult, error) {
	return c.openFGAGrantTenantAccountRelation(ctx, input, openFGARelationTenantSecurityAdmin)
}

// OpenFGAGrantAgentTool 手工授予 agent_tool runner 關係。
func (c *Service) OpenFGAGrantAgentTool(ctx context.Context, input OpenFGAGrantAgentToolInput) (OpenFGAGrantRelationshipResult, error) {
	tenantID := strings.TrimSpace(input.TenantID)
	toolID := strings.TrimSpace(input.ToolID)
	accountID := strings.TrimSpace(input.AccountID)
	if toolID == "" {
		return OpenFGAGrantRelationshipResult{}, BadRequest("tool_id is required")
	}
	if err := c.validateOpenFGATenantAccount(ctx, tenantID, accountID); err != nil {
		return OpenFGAGrantRelationshipResult{}, err
	}
	return c.OpenFGAGrantRelationship(ctx, OpenFGAGrantRelationshipInput{
		TenantID:    tenantID,
		ObjectType:  openFGATypeAgentTool,
		ObjectID:    toolID,
		Relation:    openFGARelationRunner,
		SubjectType: openFGASubjectTypeAccount,
		SubjectID:   accountID,
		DryRun:      input.DryRun,
		Logger:      input.Logger,
	})
}

// OpenFGAGrantRelationship 寫入單筆手工 OpenFGA tuple，保持冪等。
func (c *Service) OpenFGAGrantRelationship(ctx context.Context, input OpenFGAGrantRelationshipInput) (OpenFGAGrantRelationshipResult, error) {
	tenantID := strings.TrimSpace(input.TenantID)
	if tenantID == "" {
		return OpenFGAGrantRelationshipResult{}, BadRequest("tenant_id is required")
	}
	if c == nil || c.store == nil {
		return OpenFGAGrantRelationshipResult{}, BadRequest("service store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := input.Logger
	if logger == nil {
		logger = c.logger
	}
	if logger == nil {
		logger = slog.Default()
	}
	result := OpenFGAGrantRelationshipResult{
		TenantID:    tenantID,
		ObjectType:  strings.TrimSpace(input.ObjectType),
		ObjectID:    strings.TrimSpace(input.ObjectID),
		Relation:    strings.TrimSpace(input.Relation),
		SubjectType: strings.TrimSpace(input.SubjectType),
		SubjectID:   strings.TrimSpace(input.SubjectID),
		DryRun:      input.DryRun,
	}
	err := repository.WithinTenantTransaction(ctx, c.store, tenantID, func(store repository.Store) error {
		next := *c
		next.store = store
		tuple := normalizeAuthzRelationshipTuple(RequestContext{Context: ctx, TenantID: tenantID}, domain.AuthzRelationshipTuple{
			TenantID:    tenantID,
			ObjectType:  result.ObjectType,
			ObjectID:    result.ObjectID,
			Relation:    result.Relation,
			SubjectType: result.SubjectType,
			SubjectID:   result.SubjectID,
		}, next.Now())
		if tuple.ObjectType == "" || tuple.ObjectID == "" || tuple.Relation == "" || tuple.SubjectType == "" || tuple.SubjectID == "" {
			return BadRequest("object_type, object_id, relation, subject_type and subject_id are required")
		}
		exists, err := next.relationshipTupleExists(ctx, tuple)
		if err != nil {
			return err
		}
		if exists {
			result.Skipped = true
			return nil
		}
		result.Created = true
		if input.DryRun {
			return nil
		}
		if err := next.applyRelationshipTupleChange(RequestContext{Context: ctx, TenantID: tenantID}, domain.AuthzRelationshipTupleChange{
			Operation: domain.AuthzRelationshipTupleWrite,
			Tuple:     tuple,
		}); err != nil {
			return err
		}
		result.OutboxEvents = 1
		return nil
	})
	if err != nil {
		return OpenFGAGrantRelationshipResult{}, err
	}
	logger.InfoContext(ctx, "openfga relationship tuple grant completed",
		"tenant_id", result.TenantID,
		"object_type", result.ObjectType,
		"object_id", result.ObjectID,
		"relation", result.Relation,
		"subject_type", result.SubjectType,
		"subject_id", result.SubjectID,
		"created", result.Created,
		"skipped", result.Skipped,
		"dry_run", result.DryRun,
	)
	return result, nil
}

func (c *Service) openFGAGrantTenantAccountRelation(ctx context.Context, input OpenFGAGrantTenantAdminInput, relation string) (OpenFGAGrantRelationshipResult, error) {
	tenantID := strings.TrimSpace(input.TenantID)
	accountID := strings.TrimSpace(input.AccountID)
	if err := c.validateOpenFGATenantAccount(ctx, tenantID, accountID); err != nil {
		return OpenFGAGrantRelationshipResult{}, err
	}
	return c.OpenFGAGrantRelationship(ctx, OpenFGAGrantRelationshipInput{
		TenantID:    tenantID,
		ObjectType:  openFGATypeTenant,
		ObjectID:    tenantID,
		Relation:    relation,
		SubjectType: openFGASubjectTypeAccount,
		SubjectID:   accountID,
		DryRun:      input.DryRun,
		Logger:      input.Logger,
	})
}

func (c *Service) validateOpenFGATenantAccount(ctx context.Context, tenantID, accountID string) error {
	tenantID = strings.TrimSpace(tenantID)
	accountID = strings.TrimSpace(accountID)
	if tenantID == "" {
		return BadRequest("tenant_id is required")
	}
	if accountID == "" {
		return BadRequest("account_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok, err := c.store.GetTenant(ctx, tenantID); err != nil {
		return err
	} else if !ok {
		return NotFound("tenant", tenantID)
	}
	if _, ok, err := c.store.GetAccount(ctx, tenantID, accountID); err != nil {
		return err
	} else if !ok {
		return NotFound("account", accountID)
	}
	return nil
}

// applyRelationshipTupleChange 寫入本地 tuple 並產生 OpenFGA outbox 事件。
func (c *Service) applyRelationshipTupleChange(ctx RequestContext, change domain.AuthzRelationshipTupleChange) error {
	tuple := normalizeAuthzRelationshipTuple(ctx, change.Tuple, c.Now())
	if tuple.ObjectType == "" || tuple.ObjectID == "" || tuple.Relation == "" || tuple.SubjectType == "" || tuple.SubjectID == "" {
		return nil
	}
	switch change.Operation {
	case domain.AuthzRelationshipTupleWrite:
		if err := c.store.UpsertAuthzRelationshipTuple(goContext(ctx), tuple); err != nil {
			return err
		}
	case domain.AuthzRelationshipTupleDelete:
		if err := c.store.DeleteAuthzRelationshipTuple(goContext(ctx), tuple); err != nil {
			return err
		}
	default:
		return BadRequest("unsupported relationship tuple operation")
	}
	return c.store.AppendOutboxEvent(goContext(ctx), domain.OutboxEvent{
		ID:            utils.NewID("outbox"),
		TenantID:      ctx.TenantID,
		EventType:     relationshipOutboxEventType(change.Operation),
		AggregateType: domain.OutboxAggregateAuthz,
		AggregateID:   tuple.ObjectID,
		Payload:       relationshipTuplePayload(change.Operation, tuple),
		Status:        "pending",
		RetryCount:    0,
		CreatedAt:     c.Now(),
	})
}

// relationshipTupleExists 判斷本地 tuple 是否已存在。
func (c *Service) relationshipTupleExists(ctx context.Context, tuple domain.AuthzRelationshipTuple) (bool, error) {
	items, err := c.store.ListAuthzRelationshipTuplesForObject(ctx, tuple.TenantID, tuple.ObjectType, tuple.ObjectID)
	if err != nil {
		return false, err
	}
	key := relationshipTupleIdentity(tuple)
	for _, item := range items {
		if relationshipTupleIdentity(item) == key {
			return true, nil
		}
	}
	return false, nil
}

// syncAccountTenantMembershipTuple 同步 tenant#member tuple。
func (c *Service) syncAccountTenantMembershipTuple(ctx RequestContext, before, after Account) error {
	changes := make([]domain.AuthzRelationshipTupleChange, 0, 2)
	if before.ID != "" && accountTenantMemberActive(before) && (after.ID == "" || !accountTenantMemberActive(after)) {
		changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleDelete, openFGATypeTenant, before.TenantID, openFGARelationTenantMember, openFGASubjectTypeAccount, before.ID, c.Now()))
	}
	if after.ID != "" && accountTenantMemberActive(after) && (before.ID == "" || !accountTenantMemberActive(before)) {
		changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleWrite, openFGATypeTenant, after.TenantID, openFGARelationTenantMember, openFGASubjectTypeAccount, after.ID, c.Now()))
	}
	for _, change := range dedupeRelationshipTupleChanges(changes) {
		if err := c.applyRelationshipTupleChange(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

// syncOrgUnitRelationshipTuples 同步 org_unit parent tuple。
func (c *Service) syncOrgUnitRelationshipTuples(ctx RequestContext, before, after OrgUnit) error {
	changes := make([]domain.AuthzRelationshipTupleChange, 0, 4)
	objectID := strings.TrimSpace(after.ID)
	if objectID == "" {
		objectID = strings.TrimSpace(before.ID)
	}
	if before.TenantID != "" && before.TenantID != after.TenantID {
		changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleDelete, openFGATypeOrgUnit, objectID, openFGARelationTenant, openFGASubjectTypeTenant, before.TenantID, c.Now()))
	}
	if after.TenantID != "" && before.TenantID != after.TenantID {
		changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleWrite, openFGATypeOrgUnit, objectID, openFGARelationTenant, openFGASubjectTypeTenant, after.TenantID, c.Now()))
	}
	if before.ParentID != "" && before.ParentID != after.ParentID {
		changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleDelete, openFGATypeOrgUnit, objectID, openFGARelationOrgUnitParent, openFGASubjectTypeOrgUnit, before.ParentID, c.Now()))
	}
	if after.ParentID != "" && before.ParentID != after.ParentID {
		changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleWrite, openFGATypeOrgUnit, objectID, openFGARelationOrgUnitParent, openFGASubjectTypeOrgUnit, after.ParentID, c.Now()))
	}
	for _, change := range dedupeRelationshipTupleChanges(changes) {
		if err := c.applyRelationshipTupleChange(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

// syncAssumableRoleRelationshipTuples 同步 assumable_role tenant/trust tuples。
func (c *Service) syncAssumableRoleRelationshipTuples(ctx RequestContext, before, after AssumableRole) error {
	now := c.Now()
	beforeTuples := assumableRoleRelationshipTuples(ctx, before, now)
	afterTuples := assumableRoleRelationshipTuples(ctx, after, now)
	beforeByKey := map[string]domain.AuthzRelationshipTuple{}
	afterByKey := map[string]domain.AuthzRelationshipTuple{}
	for _, tuple := range beforeTuples {
		beforeByKey[relationshipTupleIdentity(tuple)] = tuple
	}
	for _, tuple := range afterTuples {
		afterByKey[relationshipTupleIdentity(tuple)] = tuple
	}
	changes := make([]domain.AuthzRelationshipTupleChange, 0, len(beforeByKey)+len(afterByKey))
	for key, tuple := range beforeByKey {
		if _, ok := afterByKey[key]; !ok {
			changes = append(changes, domain.AuthzRelationshipTupleChange{Operation: domain.AuthzRelationshipTupleDelete, Tuple: tuple})
		}
	}
	for key, tuple := range afterByKey {
		if _, ok := beforeByKey[key]; !ok {
			changes = append(changes, domain.AuthzRelationshipTupleChange{Operation: domain.AuthzRelationshipTupleWrite, Tuple: tuple})
		}
	}
	for _, change := range dedupeRelationshipTupleChanges(changes) {
		if err := c.applyRelationshipTupleChange(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

func assumableRoleRelationshipTuples(ctx RequestContext, role AssumableRole, now time.Time) []domain.AuthzRelationshipTuple {
	if strings.TrimSpace(role.ID) == "" {
		return nil
	}
	tenantID := strings.TrimSpace(role.TenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(ctx.TenantID)
	}
	if tenantID == "" {
		return nil
	}
	out := make([]domain.AuthzRelationshipTuple, 0)
	add := func(relation, subjectType, subjectID string) {
		subjectID = strings.TrimSpace(subjectID)
		if subjectID == "" {
			return
		}
		out = append(out, domain.AuthzRelationshipTuple{
			ID:          utils.NewID("rel"),
			TenantID:    tenantID,
			ObjectType:  openFGATypeAssumableRole,
			ObjectID:    role.ID,
			Relation:    relation,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			CreatedAt:   now,
		})
	}
	add(openFGARelationTenant, openFGASubjectTypeTenant, tenantID)
	if !role.Trusted {
		return dedupeAndSortRelationshipTuples(out)
	}
	for _, accountID := range trustPolicyAccountIDs(role.TrustPolicy) {
		add(openFGARelationTrustedUser, openFGASubjectTypeAccount, accountID)
	}
	for _, groupID := range trustPolicyUserGroupIDs(role.TrustPolicy) {
		add(openFGARelationTrustedGroup, openFGASubjectTypeUserGroupMember, groupID)
	}
	return dedupeAndSortRelationshipTuples(out)
}

func trustPolicyAccountIDs(policy map[string]any) []string {
	return uniqueStrings(append(stringSliceFromAny(policy["accounts"]), stringSliceFromAny(policy["account_ids"])...))
}

func trustPolicyUserGroupIDs(policy map[string]any) []string {
	return uniqueStrings(append(stringSliceFromAny(policy["user_groups"]), stringSliceFromAny(policy["user_group_ids"])...))
}

// defaultAgentToolIDs derives provisioning tuples from the runtime catalog to prevent authorization drift.
func defaultAgentToolIDs() []string {
	items := agentToolCatalog()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.Value); id != "" {
			ids = append(ids, id)
		}
	}
	return uniqueStrings(ids)
}

// syncUserGroupRelationshipTuples 同步 user_group#member tuple。
func (c *Service) syncUserGroupRelationshipTuples(ctx RequestContext, before, after UserGroup) error {
	beforeMembers := stringSet(before.MemberAccountIDs)
	afterMembers := stringSet(after.MemberAccountIDs)
	changes := make([]domain.AuthzRelationshipTupleChange, 0, len(beforeMembers)+len(afterMembers))
	for accountID := range beforeMembers {
		if _, ok := afterMembers[accountID]; !ok {
			changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleDelete, openFGATypeUserGroup, before.ID, openFGARelationUserGroupMember, openFGASubjectTypeAccount, accountID, c.Now()))
		}
	}
	for accountID := range afterMembers {
		if _, ok := beforeMembers[accountID]; !ok {
			changes = append(changes, relationshipTupleChange(ctx, domain.AuthzRelationshipTupleWrite, openFGATypeUserGroup, after.ID, openFGARelationUserGroupMember, openFGASubjectTypeAccount, accountID, c.Now()))
		}
	}
	for _, change := range dedupeRelationshipTupleChanges(changes) {
		if err := c.applyRelationshipTupleChange(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

func relationshipTupleChange(ctx RequestContext, operation domain.AuthzRelationshipTupleOperation, objectType, objectID, relation, subjectType, subjectID string, now time.Time) domain.AuthzRelationshipTupleChange {
	return domain.AuthzRelationshipTupleChange{
		Operation: operation,
		Tuple: domain.AuthzRelationshipTuple{
			ID:          utils.NewID("rel"),
			TenantID:    ctx.TenantID,
			ObjectType:  objectType,
			ObjectID:    objectID,
			Relation:    relation,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			CreatedAt:   now,
		},
	}
}

func accountTenantMemberActive(account Account) bool {
	return strings.TrimSpace(account.ID) != "" &&
		strings.TrimSpace(account.TenantID) != "" &&
		account.Status != string(domain.AccountStatusDisabled)
}

func dedupeAndSortRelationshipTuples(items []domain.AuthzRelationshipTuple) []domain.AuthzRelationshipTuple {
	out := make([]domain.AuthzRelationshipTuple, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.ObjectType == "" || item.ObjectID == "" || item.Relation == "" || item.SubjectType == "" || item.SubjectID == "" {
			continue
		}
		key := relationshipTupleIdentity(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return relationshipTupleIdentity(out[i]) < relationshipTupleIdentity(out[j])
	})
	return out
}

func relationshipTupleIdentity(tuple domain.AuthzRelationshipTuple) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", tuple.ObjectType, tuple.ObjectID, tuple.Relation, tuple.SubjectType, tuple.SubjectID)
}

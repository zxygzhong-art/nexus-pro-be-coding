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
	openFGATypeTenant    = "tenant"
	openFGATypeOrgUnit   = "org_unit"
	openFGATypeUserGroup = "user_group"
	openFGATypeEmployee  = "hr.employee"

	openFGARelationTenantMember      = "member"
	openFGARelationOrgUnitParent     = "parent"
	openFGARelationOrgUnitMember     = "member"
	openFGARelationOrgUnitManager    = "manager"
	openFGARelationUserGroupMember   = "member"
	openFGARelationEmployeeOrg       = "org"
	openFGARelationOrgUnitMemberTree = "member_recursive"
	openFGASubjectTypeAccount        = "account"
	openFGASubjectTypeOrgUnit        = "org_unit"
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

	return dedupeAndSortRelationshipTuples(out), nil
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
	changes := make([]domain.AuthzRelationshipTupleChange, 0, 2)
	objectID := strings.TrimSpace(after.ID)
	if objectID == "" {
		objectID = strings.TrimSpace(before.ID)
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

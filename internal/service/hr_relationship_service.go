package service

import (
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	employeeOwnerRelation   = "owner"
	employeeManagerRelation = "manager"
)

// syncEmployeeRelationshipTuples 同步員工關係 tuple 的服務流程。
func (c HRService) syncEmployeeRelationshipTuples(ctx RequestContext, before, after Employee) error {
	changes, err := c.employeeRelationshipTupleChanges(ctx, before, after)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if err := c.applyAuthzRelationshipTupleChange(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

// employeeRelationshipTupleChanges 處理員工關係 tuple changes 的服務流程。
func (c HRService) employeeRelationshipTupleChanges(ctx RequestContext, before, after Employee) ([]domain.AuthzRelationshipTupleChange, error) {
	now := c.Now()
	objectType := openFGATypeEmployee
	objectID := strings.TrimSpace(after.ID)
	if objectID == "" {
		objectID = strings.TrimSpace(before.ID)
	}
	changes := make([]domain.AuthzRelationshipTupleChange, 0, 8)
	add := func(operation domain.AuthzRelationshipTupleOperation, tupleObjectType, tupleObjectID, relation, subjectType, subjectID string) {
		if strings.TrimSpace(subjectID) == "" || strings.TrimSpace(tupleObjectID) == "" {
			return
		}
		changes = append(changes, domain.AuthzRelationshipTupleChange{
			Operation: operation,
			Tuple: domain.AuthzRelationshipTuple{
				ID:          utils.NewID("rel"),
				TenantID:    ctx.TenantID,
				ObjectType:  tupleObjectType,
				ObjectID:    tupleObjectID,
				Relation:    relation,
				SubjectType: subjectType,
				SubjectID:   subjectID,
				CreatedAt:   now,
			},
		})
	}

	beforeAccountID := strings.TrimSpace(before.AccountID)
	afterAccountID := strings.TrimSpace(after.AccountID)
	if beforeAccountID != "" && beforeAccountID != afterAccountID {
		add(domain.AuthzRelationshipTupleDelete, objectType, objectID, employeeOwnerRelation, openFGASubjectTypeAccount, beforeAccountID)
	}
	if afterAccountID != "" {
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, afterAccountID); err != nil {
			return nil, err
		} else if ok {
			add(domain.AuthzRelationshipTupleWrite, objectType, objectID, employeeOwnerRelation, openFGASubjectTypeAccount, afterAccountID)
		}
	}

	beforeOrgUnitID := strings.TrimSpace(before.OrgUnitID)
	afterOrgUnitID := strings.TrimSpace(after.OrgUnitID)
	if beforeOrgUnitID != "" && beforeOrgUnitID != afterOrgUnitID {
		add(domain.AuthzRelationshipTupleDelete, objectType, objectID, openFGARelationEmployeeOrg, openFGASubjectTypeOrgUnit, beforeOrgUnitID)
	}
	if afterOrgUnitID != "" {
		add(domain.AuthzRelationshipTupleWrite, objectType, objectID, openFGARelationEmployeeOrg, openFGASubjectTypeOrgUnit, afterOrgUnitID)
	}
	if beforeOrgUnitID != "" && beforeAccountID != "" && (beforeOrgUnitID != afterOrgUnitID || beforeAccountID != afterAccountID) {
		add(domain.AuthzRelationshipTupleDelete, openFGATypeOrgUnit, beforeOrgUnitID, openFGARelationOrgUnitMember, openFGASubjectTypeAccount, beforeAccountID)
	}
	if afterOrgUnitID != "" && afterAccountID != "" {
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, afterAccountID); err != nil {
			return nil, err
		} else if ok {
			add(domain.AuthzRelationshipTupleWrite, openFGATypeOrgUnit, afterOrgUnitID, openFGARelationOrgUnitMember, openFGASubjectTypeAccount, afterAccountID)
		}
	}

	beforeManagerAccountID, err := c.employeeAccountID(ctx, before.ManagerEmployeeID)
	if err != nil {
		return nil, err
	}
	afterManagerAccountID, err := c.employeeAccountID(ctx, after.ManagerEmployeeID)
	if err != nil {
		return nil, err
	}
	if beforeManagerAccountID != "" && beforeManagerAccountID != afterManagerAccountID {
		add(domain.AuthzRelationshipTupleDelete, objectType, objectID, employeeManagerRelation, openFGASubjectTypeAccount, beforeManagerAccountID)
	}
	if afterManagerAccountID != "" {
		add(domain.AuthzRelationshipTupleWrite, objectType, objectID, employeeManagerRelation, openFGASubjectTypeAccount, afterManagerAccountID)
	}

	return dedupeRelationshipTupleChanges(changes), nil
}

// employeeAccountID 處理員工帳號 ID 的服務流程。
func (c HRService) employeeAccountID(ctx RequestContext, employeeID string) (string, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return "", nil
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(employee.AccountID), nil
}

// applyAuthzRelationshipTupleChange 處理 apply 授權關係 tuple change 的服務流程。
func (c HRService) applyAuthzRelationshipTupleChange(ctx RequestContext, change domain.AuthzRelationshipTupleChange) error {
	return c.Service.applyRelationshipTupleChange(ctx, change)
}

// normalizeAuthzRelationshipTuple 正規化授權關係 tuple。
func normalizeAuthzRelationshipTuple(ctx RequestContext, tuple domain.AuthzRelationshipTuple, now time.Time) domain.AuthzRelationshipTuple {
	tuple.TenantID = utils.FirstNonEmpty(strings.TrimSpace(tuple.TenantID), ctx.TenantID)
	tuple.ObjectType = strings.TrimSpace(tuple.ObjectType)
	tuple.ObjectID = strings.TrimSpace(tuple.ObjectID)
	tuple.Relation = strings.TrimSpace(tuple.Relation)
	tuple.SubjectType = strings.TrimSpace(tuple.SubjectType)
	tuple.SubjectID = strings.TrimSpace(tuple.SubjectID)
	if tuple.ID == "" {
		tuple.ID = utils.NewID("rel")
	}
	if tuple.CreatedAt.IsZero() {
		tuple.CreatedAt = now
	}
	return tuple
}

// relationshipOutboxEventType 處理關係 outbox 事件 type。
func relationshipOutboxEventType(operation domain.AuthzRelationshipTupleOperation) string {
	switch operation {
	case domain.AuthzRelationshipTupleDelete:
		return string(domain.EventOpenFGARelationshipDelete)
	default:
		return string(domain.EventOpenFGARelationshipWrite)
	}
}

// relationshipTuplePayload 處理關係 tuple payload。
func relationshipTuplePayload(operation domain.AuthzRelationshipTupleOperation, tuple domain.AuthzRelationshipTuple) (map[string]any, error) {
	return domain.OpenFGARelationshipPayload{
		Operation:   string(operation),
		ObjectType:  tuple.ObjectType,
		ObjectID:    tuple.ObjectID,
		Relation:    tuple.Relation,
		SubjectType: tuple.SubjectType,
		SubjectID:   tuple.SubjectID,
	}.Map()
}

// dedupeRelationshipTupleChanges 處理 dedupe 關係 tuple changes。
func dedupeRelationshipTupleChanges(changes []domain.AuthzRelationshipTupleChange) []domain.AuthzRelationshipTupleChange {
	out := make([]domain.AuthzRelationshipTupleChange, 0, len(changes))
	seen := map[string]struct{}{}
	for _, change := range changes {
		key := string(change.Operation) + "\x00" + change.Tuple.ObjectType + "\x00" + change.Tuple.ObjectID + "\x00" + change.Tuple.Relation + "\x00" + change.Tuple.SubjectType + "\x00" + change.Tuple.SubjectID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, change)
	}
	return out
}

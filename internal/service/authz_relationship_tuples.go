package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	employeeOwnerRelation   = "owner"
	employeeManagerRelation = "manager"
	relationshipSubjectType = "account"
)

func (c *Service) syncEmployeeRelationshipTuples(ctx RequestContext, before, after Employee) error {
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

func (c *Service) employeeRelationshipTupleChanges(ctx RequestContext, before, after Employee) ([]domain.AuthzRelationshipTupleChange, error) {
	now := c.Now()
	objectType := routeResourceName(AppHR, ResourceEmployee)
	changes := make([]domain.AuthzRelationshipTupleChange, 0, 4)
	add := func(operation domain.AuthzRelationshipTupleOperation, relation, subjectID string) {
		if strings.TrimSpace(subjectID) == "" || strings.TrimSpace(after.ID) == "" {
			return
		}
		changes = append(changes, domain.AuthzRelationshipTupleChange{
			Operation: operation,
			Tuple: domain.AuthzRelationshipTuple{
				ID:          utils.NewID("rel"),
				TenantID:    ctx.TenantID,
				ObjectType:  objectType,
				ObjectID:    after.ID,
				Relation:    relation,
				SubjectType: relationshipSubjectType,
				SubjectID:   subjectID,
				CreatedAt:   now,
			},
		})
	}

	beforeAccountID := strings.TrimSpace(before.AccountID)
	afterAccountID := strings.TrimSpace(after.AccountID)
	if beforeAccountID != "" && beforeAccountID != afterAccountID {
		add(domain.AuthzRelationshipTupleDelete, employeeOwnerRelation, beforeAccountID)
	}
	if afterAccountID != "" {
		if _, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, afterAccountID); err != nil {
			return nil, err
		} else if ok {
			add(domain.AuthzRelationshipTupleWrite, employeeOwnerRelation, afterAccountID)
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
		add(domain.AuthzRelationshipTupleDelete, employeeManagerRelation, beforeManagerAccountID)
	}
	if afterManagerAccountID != "" {
		add(domain.AuthzRelationshipTupleWrite, employeeManagerRelation, afterManagerAccountID)
	}

	return dedupeRelationshipTupleChanges(changes), nil
}

func (c *Service) employeeAccountID(ctx RequestContext, employeeID string) (string, error) {
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

func (c *Service) applyAuthzRelationshipTupleChange(ctx RequestContext, change domain.AuthzRelationshipTupleChange) error {
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
	return c.store.AppendAuthzOutboxEvent(goContext(ctx), domain.AuthzOutboxEvent{
		ID:         utils.NewID("outbox"),
		TenantID:   ctx.TenantID,
		EventType:  relationshipOutboxEventType(change.Operation),
		Payload:    relationshipTuplePayload(change.Operation, tuple),
		Status:     "pending",
		RetryCount: 0,
		CreatedAt:  c.Now(),
	})
}

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

func relationshipOutboxEventType(operation domain.AuthzRelationshipTupleOperation) string {
	switch operation {
	case domain.AuthzRelationshipTupleDelete:
		return string(domain.EventOpenFGARelationshipDelete)
	default:
		return string(domain.EventOpenFGARelationshipWrite)
	}
}

func relationshipTuplePayload(operation domain.AuthzRelationshipTupleOperation, tuple domain.AuthzRelationshipTuple) map[string]any {
	return map[string]any{
		"operation":    string(operation),
		"object_type":  tuple.ObjectType,
		"object_id":    tuple.ObjectID,
		"relation":     tuple.Relation,
		"subject_type": tuple.SubjectType,
		"subject_id":   tuple.SubjectID,
	}
}

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

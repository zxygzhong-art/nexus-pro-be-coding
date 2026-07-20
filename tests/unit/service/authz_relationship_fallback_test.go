package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestImplicitRelationshipFallbackSkipsUnmodeledRoutePairs keeps missing permissions on the normal deny path.
func TestImplicitRelationshipFallbackSkipsUnmodeledRoutePairs(t *testing.T) {
	checkerErr := errors.New("relationship checker must not be called")
	checker := &mappedRelationshipChecker{err: checkerErr}
	svc, ctx := newImplicitRelationshipFallbackFixture(t, checker)

	requests := []domain.CheckRequest{
		{Resource: "attendance.correction", ResourceID: "correction-1", Action: domain.ActionApprove},
		{Resource: "agent.definition", ResourceID: "agent-1", Action: domain.ActionUpdate},
		{Resource: "agent.run", ResourceID: "session-1", Action: domain.ActionRead},
	}
	for _, request := range requests {
		decision, err := svc.Authz().Check(ctx, request)
		if err != nil {
			t.Fatalf("unmodeled request %+v returned relationship error: %v", request, err)
		}
		if decision.Allowed || decision.Reason != "missing permission" {
			t.Fatalf("expected ordinary missing-permission denial for %+v, got %+v", request, decision)
		}
	}
	if len(checker.checks) != 0 {
		t.Fatalf("unmodeled route pairs must skip implicit relationship checks, got %+v", checker.checks)
	}
}

// TestImplicitRelationshipFallbackPreservesModeledChecksAndErrors protects supported OpenFGA behavior.
func TestImplicitRelationshipFallbackPreservesModeledChecksAndErrors(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		checker := &mappedRelationshipChecker{allowed: map[string]bool{
			relationshipCheckKey("account:acct-relationship", "read", "hr.employee:emp-1"): true,
		}}
		svc, ctx := newImplicitRelationshipFallbackFixture(t, checker)
		decision, err := svc.Authz().Check(ctx, domain.CheckRequest{
			Resource: "hr.employee", ResourceID: "emp-1", Action: domain.ActionRead,
		})
		if err != nil || !decision.Allowed {
			t.Fatalf("expected modeled relationship fallback to allow, decision=%+v err=%v", decision, err)
		}
		if len(checker.checks) != 1 || checker.checks[0].Object != "hr.employee:emp-1" || checker.checks[0].Relation != "read" {
			t.Fatalf("unexpected modeled relationship check: %+v", checker.checks)
		}
	})

	t.Run("checker_error", func(t *testing.T) {
		checkerErr := errors.New("openfga unavailable")
		checker := &mappedRelationshipChecker{err: checkerErr}
		svc, ctx := newImplicitRelationshipFallbackFixture(t, checker)
		_, err := svc.Authz().Check(ctx, domain.CheckRequest{
			Resource: "hr.employee", ResourceID: "emp-1", Action: domain.ActionRead,
		})
		if !errors.Is(err, checkerErr) {
			t.Fatalf("expected modeled checker error to propagate, got %v", err)
		}
		if len(checker.checks) != 1 {
			t.Fatalf("expected one modeled relationship check, got %+v", checker.checks)
		}
	})
}

// newImplicitRelationshipFallbackFixture creates an account with no coarse permission grants.
func newImplicitRelationshipFallbackFixture(t *testing.T, checker service.RelationshipChecker) (*service.Service, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-relationship", TenantID: "tenant-1", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return service.New(store, service.Options{Relationships: checker}), domain.RequestContext{
		TenantID: "tenant-1", AccountID: "acct-relationship",
	}
}

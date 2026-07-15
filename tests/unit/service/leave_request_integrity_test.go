package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestCreateLeaveRequestUsesPolicyHours verifies caller-provided hours never drive reservation or persistence.
func TestCreateLeaveRequestUsesPolicyHours(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, _ := newLeaveRequestIntegrityFixture(t, now)
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingHours: 16, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual",
		StartAt:   "2026-06-10T09:00:00+08:00",
		EndAt:     "2026-06-10T17:00:00+08:00",
		Hours:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Hours != 7 {
		t.Fatalf("expected policy-derived seven hours, got %+v", created)
	}
	if created.EvaluationSnapshot["hours"] != float64(7) {
		t.Fatalf("expected evaluation snapshot to persist policy hours, got %+v", created.EvaluationSnapshot)
	}
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-2026")
	if err != nil || !ok {
		t.Fatalf("leave balance lookup failed ok=%v err=%v", ok, err)
	}
	if balance.RemainingHours != 9 || balance.UsedHours != 7 {
		t.Fatalf("expected seven reserved hours, got %+v", balance)
	}
	instance, ok, err := store.GetFormInstance(context.Background(), "tenant-1", created.FormInstanceID)
	if err != nil || !ok {
		t.Fatalf("form instance lookup failed ok=%v err=%v", ok, err)
	}
	if instance.Payload["hours"] != float64(7) {
		t.Fatalf("expected form payload to persist policy hours, got %+v", instance.Payload)
	}
}

// TestLegacyLeaveReleaseResolvesTheRequestPeriod verifies only the reserved entitlement period is restored.
func TestLegacyLeaveReleaseResolvesTheRequestPeriod(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-2025", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2025-01-01", PeriodEnd: "2025-12-31", RemainingHours: 16, UpdatedAt: now},
		{ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingHours: 16, UpdatedAt: now.Add(time.Second)},
	} {
		if err := store.UpsertLeaveBalance(context.Background(), balance); err != nil {
			t.Fatal(err)
		}
	}

	created := createLegacyLeaveRequestForReview(t, store, svc, employeeCtx)
	if created.LeaveBalanceID != "lb-2026" {
		t.Fatalf("expected the 2026 balance to be reserved, got %+v", created)
	}
	created.LeaveBalanceID = ""
	if err := store.UpsertLeaveRequest(context.Background(), created); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "not approved"}); err != nil {
		t.Fatal(err)
	}
	assertLeaveBalanceHours(t, store, "lb-2025", 16, 0)
	assertLeaveBalanceHours(t, store, "lb-2026", 16, 0)

	// A replay may be rejected by the workflow state, but it must never restore the balance twice.
	_, _ = svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "replayed"})
	assertLeaveBalanceHours(t, store, "lb-2026", 16, 0)
}

// TestLegacyLeaveReleaseRejectsAmbiguousPeriod verifies review state and balances roll back together.
func TestLegacyLeaveReleaseRejectsAmbiguousPeriod(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-a", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingHours: 16, UpdatedAt: now},
		{ID: "lb-b", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2026-06-01", PeriodEnd: "2026-06-30", RemainingHours: 16, UpdatedAt: now.Add(time.Second)},
	} {
		if err := store.UpsertLeaveBalance(context.Background(), balance); err != nil {
			t.Fatal(err)
		}
	}

	created := createLegacyLeaveRequestForReview(t, store, svc, employeeCtx)
	created.LeaveBalanceID = ""
	if err := store.UpsertLeaveRequest(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	beforeA, _, _ := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-a")
	beforeB, _, _ := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-b")

	if _, err := svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "not approved"}); err == nil {
		t.Fatal("expected an ambiguous legacy balance period to block review")
	}
	afterA, _, _ := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-a")
	afterB, _, _ := store.GetLeaveBalance(context.Background(), "tenant-1", "lb-b")
	if afterA.RemainingHours != beforeA.RemainingHours || afterA.UsedHours != beforeA.UsedHours ||
		afterB.RemainingHours != beforeB.RemainingHours || afterB.UsedHours != beforeB.UsedHours {
		t.Fatalf("ambiguous release changed balances: before=(%+v,%+v) after=(%+v,%+v)", beforeA, beforeB, afterA, afterB)
	}
	request, ok, err := store.GetLeaveRequest(context.Background(), "tenant-1", created.ID)
	if err != nil || !ok || request.Status != "pending_approval" {
		t.Fatalf("failed review must retain the pending request, ok=%v err=%v request=%+v", ok, err, request)
	}
	instance, ok, err := store.GetFormInstance(context.Background(), "tenant-1", created.FormInstanceID)
	if err != nil || !ok || instance.Status != "in_review" {
		t.Fatalf("failed review must roll back form state, ok=%v err=%v instance=%+v", ok, err, instance)
	}
}

// TestLeaveReleaseDoesNotFallbackFromMissingLinkedBalance preserves the request's exact allocation identity.
func TestLeaveReleaseDoesNotFallbackFromMissingLinkedBalance(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingHours: 16, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created := createLegacyLeaveRequestForReview(t, store, svc, employeeCtx)
	created.LeaveBalanceID = "lb-missing"
	if err := store.UpsertLeaveRequest(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "not approved"}); err == nil {
		t.Fatal("expected a missing linked balance to block review without a date fallback")
	}
	assertLeaveBalanceHours(t, store, "lb-2026", 9, 7)
	request, ok, err := store.GetLeaveRequest(context.Background(), "tenant-1", created.ID)
	if err != nil || !ok || request.Status != "pending_approval" {
		t.Fatalf("failed review must retain the pending request, ok=%v err=%v request=%+v", ok, err, request)
	}
}

func newLeaveRequestIntegrityFixture(t *testing.T, now time.Time) (*memory.Store, *service.Service, domain.RequestContext, domain.RequestContext) {
	t.Helper()
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, permissionSet := range []domain.PermissionSet{
		{ID: "ps-leave", TenantID: "tenant-1", Name: "Leave Self Service", Permissions: []domain.Permission{{Resource: "attendance.leave", Action: "create", Scope: "self"}}, CreatedAt: now},
		{ID: "ps-review", TenantID: "tenant-1", Name: "Workflow Reviewer", Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		}, CreatedAt: now},
	} {
		if err := store.UpsertPermissionSet(context.Background(), permissionSet); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []domain.Account{
		{ID: "acct-employee", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-leave"}, CreatedAt: now},
		{ID: "acct-reviewer", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-review"}, CreatedAt: now},
	} {
		if err := store.UpsertAccount(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-leave", TenantID: "tenant-1", Key: "leave-request", Name: "Leave Request",
		Schema: workflowEnabledTemplateSchema("acct-reviewer"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }})
	return store, svc,
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"},
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}
}

func createLegacyLeaveRequestForReview(t *testing.T, store *memory.Store, svc *service.Service, ctx domain.RequestContext) domain.LeaveRequest {
	t.Helper()
	created, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-06-10T09:00:00+08:00", EndAt: "2026-06-10T17:00:00+08:00", Hours: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func assertLeaveBalanceHours(t *testing.T, store *memory.Store, balanceID string, remaining, used float64) {
	t.Helper()
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", balanceID)
	if err != nil || !ok {
		t.Fatalf("leave balance %s lookup failed ok=%v err=%v", balanceID, ok, err)
	}
	if balance.RemainingHours != remaining || balance.UsedHours != used {
		t.Fatalf("unexpected leave balance %s: %+v", balanceID, balance)
	}
}

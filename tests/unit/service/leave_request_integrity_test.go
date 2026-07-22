package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestCreateLeaveRequestUsesPolicyHours verifies caller-provided hours never drive reservation or persistence.
func TestCreateLeaveRequestUsesPolicyHours(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, _ := newLeaveRequestIntegrityFixture(t, now)
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingMinutes: 16 * 60, UpdatedAt: now,
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
	if created.RequestedMinutes != 7*60 {
		t.Fatalf("expected policy-derived seven hours, got %+v", created)
	}
	if created.EvaluationSnapshot["requested_minutes"] != 7*60 {
		t.Fatalf("expected evaluation snapshot to persist policy minutes, got %+v", created.EvaluationSnapshot)
	}
	balance := effectiveLeaveBalanceForTest(t, store, "lb-2026")
	if balance.RemainingMinutes != 9*60 || balance.PendingMinutes != 7*60 {
		t.Fatalf("expected seven overlay-reserved hours, got %+v", balance)
	}
	instance, ok, err := store.GetFormInstance(context.Background(), "tenant-1", created.FormInstanceID)
	if err != nil || !ok {
		t.Fatalf("form instance lookup failed ok=%v err=%v", ok, err)
	}
	if instance.Payload["hours"] != float64(7) {
		t.Fatalf("expected form payload to persist policy hours, got %+v", instance.Payload)
	}
}

// TestLeaveRequestAllocatesAcrossBuckets verifies allocations are the sole,
// minute-exact request-to-entitlement relationship.
func TestLeaveRequestAllocatesAcrossBuckets(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, _ := newLeaveRequestIntegrityFixture(t, now)
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-expiring", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodEnd: "2026-06-30", RemainingMinutes: 3 * 60, UpdatedAt: now},
		{ID: "lb-later", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodEnd: "2026-12-31", RemainingMinutes: 5 * 60, UpdatedAt: now},
	} {
		if err := store.UpsertLeaveBalance(t.Context(), balance); err != nil {
			t.Fatal(err)
		}
	}
	created, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-06-10T09:00:00+08:00", EndAt: "2026-06-10T17:00:00+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := store.ListLeaveRequestAllocationsByRequest(t.Context(), "tenant-1", created.ID)
	if err != nil || len(allocations) != 2 {
		t.Fatalf("expected two allocations, err=%v allocations=%+v", err, allocations)
	}
	reserved := map[string]int{}
	for _, allocation := range allocations {
		reserved[allocation.LeaveBalanceID] = allocation.ReservedMinutes
	}
	if reserved["lb-expiring"] != 3*60 || reserved["lb-later"] != 4*60 {
		t.Fatalf("expected FEFO 180+240 minute allocation, got %+v", allocations)
	}
	assertLeaveBalanceMinutes(t, store, "lb-expiring", 0, 3*60)
	assertLeaveBalanceMinutes(t, store, "lb-later", 60, 4*60)
}

// TestReturnedLeaveFormCanBeEditedAndResubmitted verifies the same form and leave request survive supplementation.
func TestReturnedLeaveFormCanBeEditedAndResubmitted(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	permissionSet, ok, err := store.GetPermissionSet(t.Context(), "tenant-1", "ps-leave")
	if err != nil || !ok {
		t.Fatalf("leave permission set lookup failed ok=%v err=%v", ok, err)
	}
	permissionSet.Permissions = append(permissionSet.Permissions,
		domain.Permission{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		domain.Permission{Resource: "workflow.form_instance", Action: "update", Scope: "self"},
		domain.Permission{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
	)
	if err := store.UpsertPermissionSet(t.Context(), permissionSet); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLeaveBalance(t.Context(), domain.LeaveBalance{
		ID: "lb-resubmit", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingMinutes: 16 * 60, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-06-10T09:00:00+08:00", EndAt: "2026-06-10T17:00:00+08:00",
		Reason: "original reason",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().ReturnForm(reviewerCtx, created.FormInstanceID, domain.ReturnFormInput{Reason: "please supplement"}); err != nil {
		t.Fatal(err)
	}
	assertLeaveBalanceMinutes(t, store, "lb-resubmit", 16*60, 0)

	updated, err := svc.Workflow().UpdateFormDraft(employeeCtx, created.FormInstanceID, domain.UpdateFormDraftInput{Payload: map[string]any{
		"leave_type": "annual",
		"start_at":   "2026-06-11T09:00:00+08:00",
		"end_at":     "2026-06-11T17:00:00+08:00",
		"reason":     "supplemented reason",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.WorkflowFormStatusReturned || updated.Payload["reason"] != "supplemented reason" {
		t.Fatalf("returned form update was not persisted: %+v", updated)
	}

	resubmitted, err := svc.Workflow().SubmitForm(employeeCtx, domain.SubmitFormInput{
		TemplateKey: created.FormInstanceID,
		Payload:     updated.Payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resubmitted.ID != created.FormInstanceID || resubmitted.Status != domain.WorkflowFormStatusInReview {
		t.Fatalf("expected the returned form to re-enter review in place, got %+v", resubmitted)
	}
	request, ok, err := store.GetLeaveRequestByFormInstanceID(t.Context(), "tenant-1", created.FormInstanceID)
	if err != nil || !ok {
		t.Fatalf("resubmitted leave request lookup failed ok=%v err=%v", ok, err)
	}
	if request.ID != created.ID || request.Status != "pending_approval" || request.Reason != "supplemented reason" || request.StartAt.Day() != 11 {
		t.Fatalf("expected stable linked leave projection with supplemented values, got %+v", request)
	}
	allocations, err := store.ListLeaveRequestAllocationsByRequest(t.Context(), "tenant-1", request.ID)
	if err != nil || len(allocations) != 2 || allocations[0].Cycle != 1 || allocations[1].Cycle != 2 {
		t.Fatalf("resubmission must retain immutable allocation cycles, err=%v allocations=%+v", err, allocations)
	}
	assertLeaveBalanceMinutes(t, store, "lb-resubmit", 9*60, 7*60)
	runs, err := store.ListWorkflowRunsByFormInstance(t.Context(), "tenant-1", created.FormInstanceID)
	if err != nil || len(runs) != 2 || runs[1].Version != 2 {
		t.Fatalf("expected a second workflow run for resubmission, err=%v runs=%+v", err, runs)
	}
}

// TestLeaveReleaseUsesPersistedAllocation verifies review never re-resolves a bucket from request dates.
func TestLeaveReleaseUsesPersistedAllocation(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-2025", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2025-01-01", PeriodEnd: "2025-12-31", RemainingMinutes: 16 * 60, UpdatedAt: now},
		{ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingMinutes: 16 * 60, UpdatedAt: now.Add(time.Second)},
	} {
		if err := store.UpsertLeaveBalance(context.Background(), balance); err != nil {
			t.Fatal(err)
		}
	}

	created := createLegacyLeaveRequestForReview(t, store, svc, employeeCtx)
	allocations, err := store.ListLeaveRequestAllocationsByRequest(t.Context(), "tenant-1", created.ID)
	if err != nil || len(allocations) != 1 || allocations[0].LeaveBalanceID != "lb-2026" {
		t.Fatalf("expected the request allocation to select the 2026 bucket, err=%v allocations=%+v", err, allocations)
	}

	if _, err := svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "not approved"}); err != nil {
		t.Fatal(err)
	}
	assertLeaveBalanceMinutes(t, store, "lb-2025", 16*60, 0)
	assertLeaveBalanceMinutes(t, store, "lb-2026", 16*60, 0)

	// A replay may be rejected by the workflow state, but it must never restore the balance twice.
	_, _ = svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "replayed"})
	assertLeaveBalanceMinutes(t, store, "lb-2026", 16*60, 0)
}

// TestLeaveAllocationUsesSoonestExpiringBucket verifies overlapping buckets are deterministic.
func TestLeaveAllocationUsesSoonestExpiringBucket(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-a", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingMinutes: 16 * 60, UpdatedAt: now},
		{ID: "lb-b", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", PeriodStart: "2026-06-01", PeriodEnd: "2026-06-30", RemainingMinutes: 16 * 60, UpdatedAt: now.Add(time.Second)},
	} {
		if err := store.UpsertLeaveBalance(context.Background(), balance); err != nil {
			t.Fatal(err)
		}
	}

	created := createLegacyLeaveRequestForReview(t, store, svc, employeeCtx)
	allocations, err := store.ListLeaveRequestAllocationsByRequest(t.Context(), "tenant-1", created.ID)
	if err != nil || len(allocations) != 1 || allocations[0].LeaveBalanceID != "lb-b" {
		t.Fatalf("expected soonest-expiring bucket, err=%v allocations=%+v", err, allocations)
	}
	if _, err := svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "not approved"}); err != nil {
		t.Fatal(err)
	}
	assertLeaveBalanceMinutes(t, store, "lb-a", 16*60, 0)
	assertLeaveBalanceMinutes(t, store, "lb-b", 16*60, 0)
}

// TestLeaveReleaseRejectsAllocationCoverageMismatch preserves the persisted allocation invariant.
func TestLeaveReleaseRejectsAllocationCoverageMismatch(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31", RemainingMinutes: 16 * 60, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created := createLegacyLeaveRequestForReview(t, store, svc, employeeCtx)
	created.RequestedMinutes += 60
	if err := store.UpsertLeaveRequest(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().RejectForm(reviewerCtx, created.FormInstanceID, domain.RejectFormInput{Reason: "not approved"}); err == nil {
		t.Fatal("expected incomplete allocation coverage to block review")
	}
	assertLeaveBalanceMinutes(t, store, "lb-2026", 9*60, 7*60)
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

func assertLeaveBalanceMinutes(t *testing.T, store *memory.Store, balanceID string, remaining, used int) {
	t.Helper()
	balance := effectiveLeaveBalanceForTest(t, store, balanceID)
	overlayUsed := balance.SnapshotRemainingMinutes - balance.RemainingMinutes
	if balance.RemainingMinutes != remaining || overlayUsed != used {
		t.Fatalf("unexpected effective leave balance %s: %+v", balanceID, balance)
	}
}

func effectiveLeaveBalanceForTest(t *testing.T, store *memory.Store, balanceID string) domain.LeaveBalance {
	t.Helper()
	balance, ok, err := store.GetLeaveBalance(context.Background(), "tenant-1", balanceID)
	if err != nil || !ok {
		t.Fatalf("leave balance %s lookup failed ok=%v err=%v", balanceID, ok, err)
	}
	balance.SnapshotRemainingMinutes = balance.RemainingMinutes
	entries, err := store.ListLeaveBalanceEntries(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.BalanceID != balanceID {
			continue
		}
		balance.RemainingMinutes += entry.AmountMinutes
		switch entry.EntryType {
		case "reserve", "release":
			balance.PendingMinutes -= entry.AmountMinutes
		case "local_consume", "local_refund", "external_reconcile", "external_reversal":
			balance.LocalUsedMinutes -= entry.AmountMinutes
		}
	}
	return balance
}

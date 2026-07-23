package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestCreateLeaveRequestUsesPolicyMinutesAndOneAnnualBalance(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, _ := newLeaveRequestIntegrityFixture(t, now)
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1",
		LeaveType: "annual", LeaveTypeID: "annual", EntitlementYear: 2026,
		RemainingMinutes: 16 * 60, Source: "ehrms", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-06-10T09:00:00+08:00",
		EndAt: "2026-06-10T17:00:00+08:00", Hours: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.RequestedMinutes != 7*60 {
		t.Fatalf("expected policy-derived seven hours, got %+v", created)
	}
	record, ok, err := store.GetLeaveRecord(t.Context(), "tenant-1", created.ID)
	if err != nil || !ok || record.BalanceID != "lb-2026" || record.EntitlementYear != 2026 || record.Status != "pending" {
		t.Fatalf("expected one annual leave record, ok=%v err=%v record=%+v", ok, err, record)
	}
	assertLeaveBalanceMinutes(t, store, "lb-2026", 9*60, 7*60)
}

func TestCreateLeaveRequestRejectsCrossYearInterval(t *testing.T) {
	now := time.Date(2026, 12, 31, 8, 0, 0, 0, time.UTC)
	_, svc, employeeCtx, _ := newLeaveRequestIntegrityFixture(t, now)
	_, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual",
		StartAt:   "2026-12-31T20:00:00+08:00",
		EndAt:     "2027-01-01T02:00:00+08:00",
	})
	if err == nil {
		t.Fatal("expected cross-year leave to be rejected")
	}
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "bad_request" {
		t.Fatalf("expected bad_request, got %v", err)
	}
}

func TestApprovedLeaveKeepsRecordBoundToOriginalBalance(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, employeeCtx, reviewerCtx := newLeaveRequestIntegrityFixture(t, now)
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-2025", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", LeaveTypeID: "annual", EntitlementYear: 2025, RemainingMinutes: 16 * 60, Source: "ehrms", UpdatedAt: now},
		{ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", LeaveTypeID: "annual", EntitlementYear: 2026, RemainingMinutes: 16 * 60, Source: "ehrms", UpdatedAt: now},
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
	if _, err := svc.Workflow().ApproveForm(reviewerCtx, created.FormInstanceID, domain.ApproveFormInput{}); err != nil {
		t.Fatal(err)
	}
	record, ok, err := store.GetLeaveRecord(t.Context(), "tenant-1", created.ID)
	if err != nil || !ok || record.BalanceID != "lb-2026" || record.Status != "active" {
		t.Fatalf("approved record lost its annual balance: ok=%v err=%v record=%+v", ok, err, record)
	}
	assertLeaveBalanceMinutes(t, store, "lb-2025", 16*60, 0)
	assertLeaveBalanceMinutes(t, store, "lb-2026", 9*60, 7*60)
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
		ID: "emp-1", TenantID: "tenant-1", Name: "Employee One",
		Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
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

package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestLeaveDualSourceOverlayReconcilesWithoutDoubleDeduction covers the migration
// period where the employee submits the same request in Nexus and eHRMS.
func TestLeaveDualSourceOverlayReconcilesWithoutDoubleDeduction(t *testing.T) {
	now := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	client := &fakeEHRMSClient{leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
		"emp_id": "IKM018", "year": "2026", "leave_type": "annual", "leave_code": "001",
		"unit": "hours", "quota": "140", "used": "130", "remaining": "10",
		"grant_start": "2026-02-01", "expire_date": "2027-01-31",
	}}, leaveDetails: []domain.EHRMSLeaveDetailRecord{{
		"emp_id": "IKM018", "date": "2026-07-22", "leave_type": "annual", "leave_code": "001",
		"start": "2026-07-22 09:00:00", "end": "2026-07-22 17:00:00", "hours": "7",
		"deduct_hours": "60分鐘", "leave_item": "特休假", "source": "假勤輸入",
	}}}
	if err := store.UpsertTenant(t.Context(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, permissionSet := range []domain.PermissionSet{
		{ID: "ps-employee", TenantID: "tenant-1", Name: "Leave Self", Permissions: []domain.Permission{{Resource: "attendance.leave", Action: "create", Scope: "self"}}, CreatedAt: now},
		{ID: "ps-reviewer", TenantID: "tenant-1", Name: "Leave Reviewer", Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
			{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
			{Resource: "attendance.clock", Action: "import", Scope: "all"},
		}, CreatedAt: now},
	} {
		if err := store.UpsertPermissionSet(t.Context(), permissionSet); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []domain.Account{
		{ID: "acct-employee", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-employee"}, CreatedAt: now},
		{ID: "acct-reviewer", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-reviewer"}, CreatedAt: now},
	} {
		if err := store.UpsertAccount(t.Context(), account); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertEmployee(t.Context(), domain.Employee{
		ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "IKM018", Name: "測試員工IKM018",
		Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(t.Context(), domain.FormTemplate{
		ID: "ft-leave", TenantID: "tenant-1", Key: "leave-request", Name: "Leave Request",
		Schema: workflowEnabledTemplateSchema("acct-reviewer"), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{EHRMSClient: client, Now: func() time.Time { return now }})
	employeeCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}

	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	balances, err := store.ListLeaveBalances(t.Context(), "tenant-1")
	if err != nil || len(balances) != 1 {
		t.Fatalf("expected one upstream entitlement, err=%v balances=%+v", err, balances)
	}
	balanceID := balances[0].ID

	request, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-07-22T09:00:00+08:00", EndAt: "2026-07-22T17:00:00+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().ApproveForm(reviewerCtx, request.FormInstanceID, domain.ApproveFormInput{}); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := store.GetLeaveBalance(t.Context(), "tenant-1", balanceID)
	if err != nil || !ok || raw.RemainingHours != 10 {
		t.Fatalf("Nexus approval must not mutate the eHRMS snapshot, ok=%v err=%v balance=%+v", ok, err, raw)
	}
	effective := effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.RemainingHours != 3 || effective.PendingHours != 0 || effective.LocalUsedHours != 7 {
		t.Fatalf("expected Nexus-only approval to consume seven overlay hours, got %+v", effective)
	}

	client.leaveBalances[0]["used"] = "137"
	client.leaveBalances[0]["remaining"] = "3"
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.UpstreamRemainingHours != 3 || effective.RemainingHours != 3 || effective.LocalUsedHours != 0 {
		t.Fatalf("matched eHRMS fact must neutralize the local deduction, got %+v", effective)
	}
	stored, ok, err := store.GetLeaveRequest(t.Context(), "tenant-1", request.ID)
	if err != nil || !ok || stored.ReconciliationStatus != "matched" {
		t.Fatalf("expected the Nexus request to be reconciled, ok=%v err=%v request=%+v", ok, err, stored)
	}
	leaveCase, ok, err := store.GetLeaveCaseBySource(t.Context(), "tenant-1", "nexus_request", request.ID)
	if err != nil || !ok || leaveCase.Origin != "both" {
		t.Fatalf("expected one logical case linked to both sources, ok=%v err=%v case=%+v", ok, err, leaveCase)
	}
	external, err := store.ListExternalLeaveRecords(context.Background(), "tenant-1")
	if err != nil || len(external) != 1 || external[0].GrossMinutes != 480 || external[0].DeductMinutes != 60 || external[0].NetMinutes != 420 {
		t.Fatalf("expected normalized eHRMS interval/deduction/net minutes, err=%v records=%+v", err, external)
	}

	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.RemainingHours != 3 {
		t.Fatalf("replayed sync must be idempotent, got %+v", effective)
	}
}

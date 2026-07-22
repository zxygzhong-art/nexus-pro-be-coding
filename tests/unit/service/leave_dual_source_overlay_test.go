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
	if err != nil || !ok || raw.RemainingMinutes != 10*60 {
		t.Fatalf("Nexus approval must not mutate the eHRMS snapshot, ok=%v err=%v balance=%+v", ok, err, raw)
	}
	effective := effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.RemainingMinutes != 3*60 || effective.PendingMinutes != 0 || effective.LocalUsedMinutes != 7*60 {
		t.Fatalf("expected Nexus-only approval to consume seven overlay hours, got %+v", effective)
	}

	// Seeing the detail before the authoritative balance snapshot absorbs it
	// must not reopen availability.
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.RemainingMinutes != 3*60 || effective.LocalUsedMinutes != 7*60 {
		t.Fatalf("unconfirmed upstream balance must keep the local deduction, got %+v", effective)
	}
	pending, ok, err := store.GetLeaveRequest(t.Context(), "tenant-1", request.ID)
	if err != nil || !ok || pending.ReconciliationStatus != "pending_balance_confirmation" {
		t.Fatalf("expected pending balance confirmation, ok=%v err=%v request=%+v", ok, err, pending)
	}
	entries, err := store.ListLeaveBalanceEntries(t.Context(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.BalanceID == balanceID && entry.EntryType == "external_reconcile" {
			t.Fatalf("unconfirmed upstream balance must not create external reconciliation entry: %+v", entry)
		}
	}

	client.leaveBalances[0]["used"] = "137"
	client.leaveBalances[0]["remaining"] = "3"
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.SnapshotRemainingMinutes != 3*60 || effective.RemainingMinutes != 3*60 || effective.LocalUsedMinutes != 0 {
		t.Fatalf("matched eHRMS fact must neutralize the local deduction, got %+v", effective)
	}
	stored, ok, err := store.GetLeaveRequest(t.Context(), "tenant-1", request.ID)
	if err != nil || !ok || stored.ReconciliationStatus != "matched" {
		t.Fatalf("expected the Nexus request to be reconciled, ok=%v err=%v request=%+v", ok, err, stored)
	}
	leaveCase, ok, err := store.GetLeaveCaseByLeaveRequest(t.Context(), "tenant-1", request.ID)
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
	if effective.RemainingMinutes != 3*60 {
		t.Fatalf("replayed sync must be idempotent, got %+v", effective)
	}

	// Correcting the same external identity breaks the exact match, reverses the
	// old reconciliation, and updates the eHRMS case without mutating snapshots.
	client.leaveDetails[0]["hours"] = "6"
	client.leaveDetails[0]["deduct_hours"] = "120分鐘"
	client.leaveBalances[0]["used"] = "136"
	client.leaveBalances[0]["remaining"] = "4"
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.SnapshotRemainingMinutes != 4*60 || effective.RemainingMinutes != -3*60 || effective.LocalUsedMinutes != 7*60 {
		t.Fatalf("corrected mismatch must restore the Nexus-only deduction, got %+v", effective)
	}
	correctedRecords, err := store.ListExternalLeaveRecords(t.Context(), "tenant-1")
	if err != nil || len(correctedRecords) != 1 || correctedRecords[0].NetMinutes != 6*60 {
		t.Fatalf("corrected external record was not updated in place, err=%v records=%+v", err, correctedRecords)
	}
	correctedCase, ok, err := store.GetLeaveCaseByExternalRecord(t.Context(), "tenant-1", correctedRecords[0].ID)
	if err != nil || !ok || correctedCase.NetMinutes != 6*60 || correctedCase.Origin != "ehrms" {
		t.Fatalf("corrected external case was not split and updated, ok=%v err=%v case=%+v", ok, err, correctedCase)
	}

	// Restoring the original upstream fact reuses the same identity but starts a
	// new ledger generation, so the exact match can be applied again.
	client.leaveDetails[0]["hours"] = "7"
	client.leaveDetails[0]["deduct_hours"] = "60分鐘"
	client.leaveBalances[0]["used"] = "137"
	client.leaveBalances[0]["remaining"] = "3"
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.RemainingMinutes != 3*60 || effective.LocalUsedMinutes != 0 {
		t.Fatalf("restored exact fact must reconcile in a new generation, got %+v", effective)
	}

	// A successful empty detail snapshot tombstones the upstream fact and
	// reverses only its prior reconciliation. The approved Nexus request remains
	// the active deduction while the eHRMS snapshot restores its minutes.
	client.leaveDetails = nil
	client.leaveBalances[0]["used"] = "130"
	client.leaveBalances[0]["remaining"] = "10"
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	effective = effectiveLeaveBalanceForTest(t, store, balanceID)
	if effective.SnapshotRemainingMinutes != 10*60 || effective.RemainingMinutes != 3*60 || effective.LocalUsedMinutes != 7*60 {
		t.Fatalf("tombstone must restore the Nexus-only deduction exactly once, got %+v", effective)
	}
	external, err = store.ListExternalLeaveRecords(context.Background(), "tenant-1")
	if err != nil || len(external) != 1 || external[0].DeletedAt == nil || external[0].Status != "cancelled" {
		t.Fatalf("missing upstream fact was not tombstoned, err=%v records=%+v", err, external)
	}
	localCase, ok, err := store.GetLeaveCaseByLeaveRequest(t.Context(), "tenant-1", request.ID)
	if err != nil || !ok || localCase.Origin != "nexus" || localCase.Status != "active" {
		t.Fatalf("local case must remain active after upstream deletion, ok=%v err=%v case=%+v", ok, err, localCase)
	}
	externalCase, ok, err := store.GetLeaveCaseByExternalRecord(t.Context(), "tenant-1", external[0].ID)
	if err != nil || !ok || externalCase.Origin != "ehrms" || externalCase.Status != "cancelled" {
		t.Fatalf("external case must retain a cancelled tombstone, ok=%v err=%v case=%+v", ok, err, externalCase)
	}
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	if replayed := effectiveLeaveBalanceForTest(t, store, balanceID); replayed.RemainingMinutes != 3*60 {
		t.Fatalf("replayed tombstone must be idempotent, got %+v", replayed)
	}
}

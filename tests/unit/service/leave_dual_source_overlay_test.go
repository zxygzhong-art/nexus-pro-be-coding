package service_test

import (
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestLeaveRecordsMatchNexusAndEHRMSOneToOne(t *testing.T) {
	now := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	client := &fakeEHRMSClient{
		leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
			"emp_id": "IKM018", "year": "2026", "leave_type": "annual", "leave_code": "001",
			"unit": "hours", "quota": "140", "used": "130", "remaining": "10",
		}},
		leaveDetails: []domain.EHRMSLeaveDetailRecord{
			{
				"record_id": "ehrms-1", "emp_id": "IKM018", "date": "2026-07-22", "leave_type": "annual", "leave_code": "001",
				"start": "2026-07-22 09:00:00", "end": "2026-07-22 17:00:00", "hours": "7",
				"deduct_hours": "60分鐘",
			},
			{
				"record_id": "ehrms-2", "emp_id": "IKM018", "date": "2026-07-22", "leave_type": "annual", "leave_code": "001",
				"start": "2026-07-22 09:00:00", "end": "2026-07-22 17:00:00", "hours": "7",
				"deduct_hours": "60分鐘",
			},
		},
	}
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
		ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "IKM018", Name: "Employee",
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
	currentTime := now
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{EHRMSClient: client, Now: func() time.Time { return currentTime }})
	employeeCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	reviewerCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reviewer"}

	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}
	initialRecords, err := store.ListLeaveRecords(t.Context(), "tenant-1")
	if err != nil || len(initialRecords) != 2 {
		t.Fatalf("event_date must capture first record creation time, err=%v records=%+v", err, initialRecords)
	}
	for _, record := range initialRecords {
		if !record.EventDate.Equal(now) {
			t.Fatalf("event_date must capture first record creation time: %+v", record)
		}
	}
	currentTime = now.Add(2 * time.Hour)
	request, err := svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-07-22T09:00:00+08:00", EndAt: "2026-07-22T17:00:00+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Workflow().ApproveForm(reviewerCtx, request.FormInstanceID, domain.ApproveFormInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Attendance().SyncEHRMSAttendance(reviewerCtx, domain.EHRMSAttendanceSyncInput{}); err != nil {
		t.Fatal(err)
	}

	records, err := store.ListLeaveRecords(t.Context(), "tenant-1")
	if err != nil || len(records) != 3 {
		t.Fatalf("expected one Nexus and two eHRMS records, err=%v records=%+v", err, records)
	}
	var nexus, matchedEHRMS domain.LeaveRecord
	matchedCount := 0
	unmatchedCount := 0
	for _, record := range records {
		if record.Source == "nexus" {
			nexus = record
		} else if record.Source == "ehrms" && record.MatchedRecordID != "" {
			matchedEHRMS = record
			matchedCount++
		} else if record.Source == "ehrms" && record.ReconciliationStatus == "unmatched" {
			unmatchedCount++
		}
	}
	if nexus.ID != request.ID || nexus.BalanceID == "" || nexus.EntitlementYear != 2026 {
		t.Fatalf("invalid Nexus leave record: %+v", nexus)
	}
	if matchedCount != 1 || unmatchedCount != 1 || matchedEHRMS.MatchedRecordID != nexus.ID ||
		matchedEHRMS.ReconciliationStatus != "matched" || matchedEHRMS.BalanceID != nexus.BalanceID {
		t.Fatalf("expected exactly one eHRMS match, matched=%d unmatched=%d record=%+v", matchedCount, unmatchedCount, matchedEHRMS)
	}
	if !matchedEHRMS.EventDate.Equal(now) {
		t.Fatalf("event_date changed during eHRMS upsert: got %s want %s", matchedEHRMS.EventDate, now)
	}
	active, err := store.ListActiveLeaveRecordsByQuery(t.Context(), "tenant-1", []string{"emp-1"}, nexus.StartAt.Add(-time.Hour), nexus.EndAt.Add(time.Hour))
	if err != nil || len(active) != 2 {
		t.Fatalf("matched eHRMS row must be excluded while the unmatched duplicate remains visible, err=%v records=%+v", err, active)
	}
	for _, record := range active {
		if record.ID == matchedEHRMS.ID {
			t.Fatalf("matched eHRMS row duplicated attendance leave: %+v", active)
		}
	}
}

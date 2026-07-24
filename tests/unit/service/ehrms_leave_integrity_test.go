package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

func TestSyncEHRMSLeaveDetailPersistsWithoutBalanceAndRepairsLater(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	detail := domain.EHRMSLeaveDetailRecord{
		"record_id":       "LEAVE-1001",
		"emp_id":          "IKM017",
		"date":            "2026-06-11",
		"leave_type":      "annual",
		"start":           "09:00",
		"end":             "13:00",
		"hours":           "4",
		"approval_status": "approved",
		"updated_at":      "2026-06-12 10:30:00",
	}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{
		EHRMSClient: fakeEHRMSClient{leaveDetails: []domain.EHRMSLeaveDetailRecord{detail}},
		Now:         func() time.Time { return syncNow },
	})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-ehrms", TenantID: "tenant-1", EmployeeNo: "IKM017",
		Name: "測試員工", Status: "active", EmploymentStatus: "active",
		CreatedAt: syncNow, UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}

	first, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if first.LeaveDetailsCreated != 1 || first.LeaveDetailsFailed != 0 || first.LeaveDetailsUnmatched != 1 {
		t.Fatalf("missing entitlement must not reject a valid leave detail: %+v", first)
	}
	records, err := store.ListLeaveRecords(context.Background(), "tenant-1")
	if err != nil || len(records) != 1 {
		t.Fatalf("expected one unmatched detail, records=%+v err=%v", records, err)
	}
	unmatched := records[0]
	if unmatched.BalanceID != "" || unmatched.BalanceMatchStatus != "unmatched" ||
		unmatched.BalanceMatchReason != "annual_balance_not_found" || unmatched.ExternalRef == "" {
		t.Fatalf("unexpected unmatched balance state: %+v", unmatched)
	}
	if unmatched.SourcePayload["approval_status"] != "approved" || unmatched.SourceUpdatedAt == nil {
		t.Fatalf("expected detail extension fields and source timestamp, got %+v", unmatched)
	}

	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-2026", TenantID: "tenant-1", EmployeeID: "emp-ehrms",
		LeaveType: "annual", LeaveTypeID: domain.StableLeaveTypeID("annual"),
		EntitlementYear: 2026, RemainingMinutes: 8 * 60, Source: "ehrms",
		UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}
	second, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if second.LeaveDetailsUpdated != 1 || second.LeaveDetailsFailed != 0 || second.LeaveDetailsUnmatched != 0 {
		t.Fatalf("later entitlement must repair the existing detail: %+v", second)
	}
	repaired, ok, err := store.GetLeaveRecord(context.Background(), "tenant-1", unmatched.ID)
	if err != nil || !ok {
		t.Fatalf("expected repaired leave detail, ok=%v err=%v", ok, err)
	}
	if repaired.BalanceID != "lb-2026" || repaired.BalanceMatchStatus != "matched" || repaired.BalanceMatchReason != "" {
		t.Fatalf("unexpected repaired balance state: %+v", repaired)
	}
}

func TestSyncEHRMSLeaveDetailPreservesCrossYearInterval(t *testing.T) {
	syncNow := time.Date(2027, 1, 2, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{
		EHRMSClient: fakeEHRMSClient{leaveDetails: []domain.EHRMSLeaveDetailRecord{{
			"record_id": "LEAVE-CROSS-YEAR", "emp_id": "IKM017", "date": "2026-12-31",
			"leave_type": "annual", "start": "2026-12-31 23:00", "end": "2027-01-01 01:00", "hours": "2",
		}}},
		Now: func() time.Time { return syncNow },
	})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-ehrms", TenantID: "tenant-1", EmployeeNo: "IKM017",
		Name: "測試員工", Status: "active", EmploymentStatus: "active",
		CreatedAt: syncNow, UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveDetailsCreated != 1 || result.LeaveDetailsFailed != 0 {
		t.Fatalf("cross-year upstream fact must be preserved, got %+v", result)
	}
	records, err := store.ListLeaveRecords(context.Background(), "tenant-1")
	if err != nil || len(records) != 1 || records[0].EntitlementYear != 2026 ||
		records[0].EndAt.Year() != 2027 {
		t.Fatalf("unexpected cross-year record: records=%+v err=%v", records, err)
	}
}

func TestSyncEHRMSLeaveBalancePreservesExtensionsAndNegativeRemaining(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{
		EHRMSClient: fakeEHRMSClient{leaveBalances: []domain.EHRMSLeaveBalanceRecord{{
			"emp_id": "IKM017", "year": "2026", "leave_type": "annual", "unit": "hours",
			"quota": "0", "used": "1", "remaining": "-1", "carry_in": "2",
			"updated_at": "2026-06-19 12:00:00",
		}}},
		Now: func() time.Time { return syncNow },
	})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-ehrms", TenantID: "tenant-1", EmployeeNo: "IKM017",
		Name: "測試員工", Status: "active", EmploymentStatus: "active",
		CreatedAt: syncNow, UpdatedAt: syncNow,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveBalancesUpserted != 1 || result.LeaveBalancesFailed != 0 {
		t.Fatalf("expected negative upstream balance snapshot to persist, got %+v", result)
	}
	balances, err := store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil || len(balances) != 1 {
		t.Fatalf("expected one balance, balances=%+v err=%v", balances, err)
	}
	if balances[0].RemainingMinutes != -60 || balances[0].SourcePayload["carry_in"] != "2" ||
		balances[0].SourceUpdatedAt == nil {
		t.Fatalf("unexpected extended balance mapping: %+v", balances[0])
	}
}

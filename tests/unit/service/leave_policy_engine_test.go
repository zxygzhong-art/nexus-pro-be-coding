package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestCreateLeaveRequestAcceptsLegacyLeaveTypeCode(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingHours: 16, UpdatedAt: now,
	})

	created, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "特",
		StartAt:   "2026-06-10",
		EndAt:     "2026-06-11",
		Hours:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.LeaveType != "annual" {
		t.Fatalf("expected normalized leave type annual, got %s", created.LeaveType)
	}
	if created.LeaveBalanceID != "lb-1" {
		t.Fatalf("expected request to retain the exact reserved balance id, got %+v", created)
	}
}

func TestCreateLeaveRequestAllowsUnlimitedWithoutBalance(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)

	created, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "official",
		StartAt:   "2026-06-10",
		EndAt:     "2026-06-11",
		Hours:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.LeaveType != "official" {
		t.Fatalf("unexpected leave type: %s", created.LeaveType)
	}
}

func TestCreateLeaveRequestRejectsUnknownLeaveType(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)

	_, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "not-a-real-leave",
		StartAt:   "2026-06-10",
		EndAt:     "2026-06-11",
		Hours:     8,
	})
	if err == nil {
		t.Fatal("expected unknown leave type to be rejected")
	}
}

func TestGrantLeaveBalancesProratesAnnualByHireDate(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	hire := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active",
		EmploymentStatus: "active", HireDate: &hire, CreatedAt: now, UpdatedAt: now,
	})

	result, err := svc.Attendance().GrantLeaveBalances(ctx, domain.GrantLeaveBalancesInput{
		EmployeeID:  "emp-1",
		PeriodStart: "2026-01-01",
		PeriodEnd:   "2026-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Granted < 1 {
		t.Fatalf("expected at least one grant, got %+v", result)
	}

	balances, err := store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	var annual *domain.LeaveBalance
	for i := range balances {
		if balances[i].EmployeeID == "emp-1" && balances[i].LeaveType == "annual" {
			annual = &balances[i]
			break
		}
	}
	if annual == nil {
		t.Fatal("annual balance was not granted")
	}
	// 2026 is not leap; Apr 1..Dec 31 = 275 days; base 24h * 275/365 ≈ 18.082 → 18.0
	if annual.GrantedHours != 18 {
		t.Fatalf("expected prorated annual grant 18h, got %v (remaining=%v ratio=%v)", annual.GrantedHours, annual.RemainingHours, annual.ProrateRatio)
	}
}

func TestGrantLeaveBalancesUsesSeniorEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	hire := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-senior", TenantID: "tenant-1", Name: "Senior", Status: "active",
		EmploymentStatus: "active", HireDate: &hire,
		EmploymentInfo: map[string]any{"job_level": "senior"},
		CreatedAt:      now, UpdatedAt: now,
	})

	_, err := svc.Attendance().GrantLeaveBalances(ctx, domain.GrantLeaveBalancesInput{
		EmployeeID:  "emp-senior",
		PeriodStart: "2026-01-01",
		PeriodEnd:   "2026-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	balances, err := store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, balance := range balances {
		if balance.EmployeeID == "emp-senior" && balance.LeaveType == "annual" {
			if balance.GrantedHours != 128 {
				t.Fatalf("expected senior annual 128h, got %v", balance.GrantedHours)
			}
			return
		}
	}
	t.Fatal("senior annual balance missing")
}

func TestGrantLeaveBalancesFailsWithoutHireDate(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-no-hire", TenantID: "tenant-1", Name: "No Hire", Status: "active",
		EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	})

	result, err := svc.Attendance().GrantLeaveBalances(ctx, domain.GrantLeaveBalancesInput{
		EmployeeID:  "emp-no-hire",
		PeriodStart: "2026-01-01",
		PeriodEnd:   "2026-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || len(result.RowErrors) == 0 {
		t.Fatalf("expected hire_date failure, got %+v", result)
	}
	balances, err := store.ListLeaveBalances(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, balance := range balances {
		if balance.EmployeeID == "emp-no-hire" {
			t.Fatalf("should not grant balances without hire date: %+v", balance)
		}
	}
}

func TestUpdateAttendancePolicyIncrementsVersion(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)
	input := domain.UpdateAttendancePolicyInput{
		WorkTime: domain.AttendancePolicyWorkTime{
			StandardStart: "09:00",
			StandardEnd:   "18:00",
			BreakStart:    "12:00",
			BreakEnd:      "13:00",
			Weekend:       "週六、週日",
			CycleStart:    "1 日",
			CycleEnd:      "本月 月底（最後一日）",
		},
		LeaveTypes: []domain.AttendanceLeaveType{
			{Code: "annual", Name: "特休假", Quota: "依年資", Rule: "折算", Proof: "—"},
		},
	}
	first, err := svc.Attendance().UpdateAttendancePolicy(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Attendance().UpdateAttendancePolicy(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || second.Version != 2 {
		t.Fatalf("expected versions 1 then 2, got %d then %d", first.Version, second.Version)
	}
	if first.LeaveTypes[0].GrantMode != domain.LeaveGrantModeAnnualGrant {
		t.Fatalf("expected structured grant_mode, got %+v", first.LeaveTypes[0])
	}
}

func newLeavePolicyEngineFixture(t *testing.T, now time.Time) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-leave",
		TenantID: "tenant-1",
		Name:     "Leave Admin",
		Permissions: []domain.Permission{
			{Resource: "attendance.leave", Action: "create", Scope: "all"},
			{Resource: "attendance.leave", Action: "read", Scope: "all"},
			{Resource: "attendance.leave", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", DisplayName: "Admin", EmployeeID: "emp-1",
		Status: "active", DirectPermissionSetIDs: []string{"ps-leave"}, CreatedAt: now,
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", Status: "active",
		EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	})
	return store, service.New(store), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

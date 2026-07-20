package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestAttendanceLeaveTypeJSONPreservesZeroPaidRatio(t *testing.T) {
	encoded, err := json.Marshal(domain.AttendanceLeaveType{Code: "personal", Name: "事假", PaidRatio: 0})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	paidRatio, found := payload["paid_ratio"]
	if !found || paidRatio != float64(0) {
		t.Fatalf("expected zero paid_ratio to remain in policy JSON, got %s", encoded)
	}
}

func TestValidateCustomLeaveTypePreservesStructuredZeroPaidRatio(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)
	result, err := svc.Attendance().ValidateAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{
		BaseVersion: 1,
		WorkTime: domain.AttendancePolicyWorkTime{
			StandardStart: "09:00", StandardEnd: "18:00", BreakStart: "12:00", BreakEnd: "13:00",
			Weekend: "週六、週日", CycleStart: "1 日", CycleEnd: "本月 月底（最後一日）",
		},
		LeaveTypes: []domain.AttendanceLeaveType{{
			Code: "wellness_leave", Name: "身心調整假", Quota: "3 天 / 年", Rule: "依公司政策", Proof: "—",
			Unit: "day", GrantMode: domain.LeaveGrantModeEvent, PaidRatio: 0, Active: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(result.Policy.LeaveTypes) != 1 {
		t.Fatalf("expected custom leave type to validate, got %+v", result)
	}
	custom := result.Policy.LeaveTypes[0]
	if custom.ID != "lt_wellness_leave" || custom.Code != "wellness_leave" || custom.PaidRatio != 0 {
		t.Fatalf("expected stable custom identity and zero paid ratio, got %+v", custom)
	}
}

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
	if created.LeaveTypeID != "lt_annual" || created.PolicyVersion != 1 {
		t.Fatalf("expected stable leave identity and policy version, got %+v", created)
	}
	if created.RuleSnapshot["requires_balance"] != true || created.EvaluationSnapshot["status"] != "eligible" {
		t.Fatalf("expected immutable rule and decision snapshots, got rule=%+v evaluation=%+v", created.RuleSnapshot, created.EvaluationSnapshot)
	}
}

func TestEvaluateLeaveRequestUsesPublishedPolicyWithoutMutation(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)

	eventLeave, err := svc.Attendance().EvaluateLeaveRequest(ctx, domain.EvaluateLeaveRequestInput{
		LeaveType: "official", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !eventLeave.Eligible || eventLeave.BalanceRequired || eventLeave.PolicyVersion != 1 {
		t.Fatalf("expected event leave to be eligible without balance, got %+v", eventLeave)
	}
	personal, err := svc.Attendance().EvaluateLeaveRequest(ctx, domain.EvaluateLeaveRequestInput{
		LeaveType: "personal", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !personal.Eligible || personal.BalanceRequired || personal.Rule.RequiresBalance || personal.Status != "eligible" {
		t.Fatalf("expected personal leave to be eligible without balance, got %+v", personal)
	}

	annual, err := svc.Attendance().EvaluateLeaveRequest(ctx, domain.EvaluateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !annual.Eligible || annual.Status != "eligible_without_balance" || annual.BalanceRequired || !annual.Rule.RequiresBalance || annual.BalanceFallbackReason != "balance_not_initialized" {
		t.Fatalf("expected missing annual balance to fall back without reservation, got %+v", annual)
	}
	if requests, err := store.ListLeaveRequests(context.Background(), "tenant-1"); err != nil || len(requests) != 0 {
		t.Fatalf("evaluation must not create requests, got %+v err=%v", requests, err)
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
	if first.Version != 2 || second.Version != 3 {
		t.Fatalf("expected versions 2 then 3 after the implicit default, got %d then %d", first.Version, second.Version)
	}
	if first.LeaveTypes[0].GrantMode != domain.LeaveGrantModeAnnualGrant {
		t.Fatalf("expected structured grant_mode, got %+v", first.LeaveTypes[0])
	}
}

func TestValidateAttendancePolicyDoesNotPublishDraft(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	input := domain.UpdateAttendancePolicyInput{
		BaseVersion: 1,
		WorkTime: domain.AttendancePolicyWorkTime{
			StandardStart: "09:00", StandardEnd: "18:00", BreakStart: "12:00", BreakEnd: "13:00",
			Weekend: "週六、週日", CycleStart: "1 日", CycleEnd: "本月 月底（最後一日）",
		},
		LeaveTypes: []domain.AttendanceLeaveType{
			{Code: "annual", Name: "特休假", Quota: "依年資", Rule: "折算", Proof: "—"},
		},
	}

	validation, err := svc.Attendance().ValidateAttendancePolicy(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || validation.ProjectedVersion != 2 {
		t.Fatalf("unexpected validation result: %+v", validation)
	}
	if _, found, err := store.GetAttendancePolicy(context.Background(), "tenant-1"); err != nil || found {
		t.Fatalf("validation must not publish the draft, found=%v err=%v", found, err)
	}
	invalid := input
	invalid.LeaveTypes = append(append([]domain.AttendanceLeaveType(nil), input.LeaveTypes...), input.LeaveTypes[0])
	invalidResult, err := svc.Attendance().ValidateAttendancePolicy(ctx, invalid)
	if err != nil {
		t.Fatal(err)
	}
	if invalidResult.Valid || invalidResult.ProjectedVersion != 2 || len(invalidResult.Issues) == 0 || len(invalidResult.Policy.LeaveTypes) != 2 {
		t.Fatalf("expected a parseable invalid draft response, got %+v", invalidResult)
	}

	published, err := svc.Attendance().PublishAttendancePolicy(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if published.Version != 2 {
		t.Fatalf("expected first published version, got %+v", published)
	}
	if len(published.LeaveTypes) != 1 || published.LeaveTypes[0].ID != "lt_annual" {
		t.Fatalf("expected a stable leave type id in the published policy, got %+v", published.LeaveTypes)
	}
	if stored, found, err := store.GetAttendancePolicy(context.Background(), "tenant-1"); err != nil || !found || stored.Version != 2 {
		t.Fatalf("published policy was not persisted, found=%v policy=%+v err=%v", found, stored, err)
	}
	if _, err := svc.Attendance().PublishAttendancePolicy(ctx, input); err == nil {
		t.Fatal("expected stale base_version publish to be rejected")
	}
	renamed := domain.UpdateAttendancePolicyInput{
		BaseVersion: 2,
		WorkTime:    published.WorkTime,
		LeaveTypes:  append([]domain.AttendanceLeaveType(nil), published.LeaveTypes...),
	}
	renamed.LeaveTypes[0].Code = "paid_time_off"
	renamedPolicy, err := svc.Attendance().PublishAttendancePolicy(ctx, renamed)
	if err != nil {
		t.Fatal(err)
	}
	if renamedPolicy.Version != 3 || renamedPolicy.LeaveTypes[0].ID != "lt_annual" || renamedPolicy.LeaveTypes[0].Code != "paid_time_off" {
		t.Fatalf("code changes must retain stable leave identity, got %+v", renamedPolicy)
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
	return store, newDirectAttendanceWorkflowService(t, store, now, "leave-request"), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

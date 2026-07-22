package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestCreateLeaveRequestAcceptsMappedAliasForSystemLeaveType(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	_ = store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", LeaveTypeID: "lt_annual", RemainingHours: 16, Source: "ehrms", UpdatedAt: now,
	})

	created, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "特", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.LeaveType != "annual" || created.LeaveTypeID != "lt_annual" || created.LeaveBalanceID != "lb-1" {
		t.Fatalf("expected annual balance reservation, got %+v", created)
	}
	if created.RuleSnapshot["requires_balance"] != true || created.RuleSnapshot["grant_mode"] != "annual_grant" || created.EvaluationSnapshot["status"] != "eligible" {
		t.Fatalf("expected local rule and decision snapshots, got rule=%+v evaluation=%+v", created.RuleSnapshot, created.EvaluationSnapshot)
	}
}

func TestEvaluateLeaveRequestUsesSystemCatalogWithoutMutation(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)

	evaluated, err := svc.Attendance().EvaluateLeaveRequest(ctx, domain.EvaluateLeaveRequestInput{
		LeaveType: "official", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !evaluated.Eligible || evaluated.Status != "eligible" || evaluated.BalanceRequired || evaluated.Rule.RequiresBalance || evaluated.Rule.GrantMode != "unlimited" || evaluated.BalanceFallbackReason != "" {
		t.Fatalf("expected a system-catalog rule without balance reservation, got %+v", evaluated)
	}
	if requests, err := store.ListLeaveRequests(context.Background(), "tenant-1"); err != nil || len(requests) != 0 {
		t.Fatalf("evaluation must not create requests, got %+v err=%v", requests, err)
	}
}

func TestCreateLeaveRequestRejectsUnknownLeaveType(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)
	_, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "not-a-real-leave", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err == nil {
		t.Fatal("expected a code absent from the system catalog to be rejected")
	}
}

func TestLeaveRequestRejectsExternalCodeOutsideSystemCatalog(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)

	evaluated, err := svc.Attendance().EvaluateLeaveRequest(ctx, domain.EvaluateLeaveRequestInput{
		LeaveType: "I900-1", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evaluated.Eligible || evaluated.Status != service.LeaveEvaluationUnsupported {
		t.Fatalf("expected external-only code to be unsupported, got %+v", evaluated)
	}

	_, err = svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "I900-1", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
	})
	if err == nil {
		t.Fatal("expected external-only code to be rejected at create time")
	}
}

func TestGrantLeaveBalancesRequiresEHRMSSync(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	_, err := svc.Attendance().GrantLeaveBalances(ctx, domain.GrantLeaveBalancesInput{
		EmployeeID: "emp-1", PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31",
	})
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 400 || !strings.Contains(appErr.Message, "EHRMS attendance sync") {
		t.Fatalf("expected retired grant endpoint to direct callers to EHRMS sync, got %v", err)
	}
	if balances, listErr := store.ListLeaveBalances(context.Background(), "tenant-1"); listErr != nil || len(balances) != 0 {
		t.Fatalf("retired grant endpoint must not mutate balances, got %+v err=%v", balances, listErr)
	}
}

func TestAttendancePolicyPublishesImmutableWorkTimeVersion(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	current := domain.AttendancePolicy{
		TenantID: "tenant-1", WorkTime: defaultPolicyWorkTime(), Version: 7,
		EffectiveFrom: &now, PublishedAt: now,
	}
	if err := store.InsertAttendancePolicyVersion(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	workTime := current.WorkTime
	workTime.StandardEnd = "18:00"
	published, err := svc.Attendance().PublishAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{BaseVersion: 7, WorkTime: workTime})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(published)
	if err != nil {
		t.Fatal(err)
	}
	if published.Version != 8 || strings.Contains(string(raw), "leave_types") {
		t.Fatalf("policy response must contain work time only, got %s", raw)
	}
	stored, found, err := store.GetAttendancePolicy(context.Background(), "tenant-1")
	if err != nil || !found || stored.Version != 8 || stored.WorkTime.StandardEnd != "18:00" {
		t.Fatalf("latest immutable policy version was not stored: found=%v stored=%+v err=%v", found, stored, err)
	}
}

func TestAttendancePolicyRejectsStaleBaseVersion(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)
	first := defaultPolicyWorkTime()
	first.StandardEnd = "18:00"
	if _, err := svc.Attendance().PublishAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{BaseVersion: 1, WorkTime: first}); err != nil {
		t.Fatal(err)
	}
	second := defaultPolicyWorkTime()
	second.StandardEnd = "19:00"
	_, err := svc.Attendance().PublishAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{BaseVersion: 1, WorkTime: second})
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 409 {
		t.Fatalf("expected stale policy publish to return 409, got %v", err)
	}
}

func TestAttendancePolicyVersionCannotBeOverwritten(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	original := domain.AttendancePolicy{
		TenantID: "tenant-1", Version: 1, WorkTime: defaultPolicyWorkTime(),
		EffectiveFrom: &now, PublishedAt: now,
	}
	if err := store.InsertAttendancePolicyVersion(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	replacement := original
	replacement.WorkTime.StandardEnd = "19:00"
	err := store.InsertAttendancePolicyVersion(context.Background(), replacement)
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 409 {
		t.Fatalf("expected immutable version overwrite to return 409, got %v", err)
	}
	stored, found, err := store.GetAttendancePolicy(context.Background(), "tenant-1")
	if err != nil || !found || stored.WorkTime.StandardEnd != original.WorkTime.StandardEnd {
		t.Fatalf("immutable version was overwritten: found=%v stored=%+v err=%v", found, stored, err)
	}
}

func TestValidateAttendancePolicyDoesNotPublishDraft(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newLeavePolicyEngineFixture(t, now)
	workTime := defaultPolicyWorkTime()
	workTime.StandardEnd = "18:00"
	validation, err := svc.Attendance().ValidateAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{BaseVersion: 1, WorkTime: workTime})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || validation.ProjectedVersion != 2 {
		t.Fatalf("unexpected validation result: %+v", validation)
	}
	if _, found, err := store.GetAttendancePolicy(context.Background(), "tenant-1"); err != nil || found {
		t.Fatalf("validation must not publish the draft, found=%v err=%v", found, err)
	}
}

func TestPublishAttendancePolicyRejectsInvalidWorkTimeOptions(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	_, svc, ctx := newLeavePolicyEngineFixture(t, now)
	workTime := defaultPolicyWorkTime()
	workTime.StandardStart = "09:15"
	_, err := svc.Attendance().PublishAttendancePolicy(ctx, domain.UpdateAttendancePolicyInput{BaseVersion: 1, WorkTime: workTime})
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 400 {
		t.Fatalf("expected invalid time option to return 400, got %v", err)
	}
}

func defaultPolicyWorkTime() domain.AttendancePolicyWorkTime {
	return domain.AttendancePolicyWorkTime{
		ClockMode: "flexible", FlexibleClockInEarliest: "00:00", FlexibleClockOutLatest: "23:30",
		StandardStart: "09:00", StandardEnd: "17:00", BreakStart: "12:00", BreakEnd: "13:00",
		Weekend: "週六、週日", CycleStart: "1 日", CycleEnd: "本月 月底（最後一日）",
	}
}

func newLeavePolicyEngineFixture(t *testing.T, now time.Time) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-leave", TenantID: "tenant-1", Name: "Leave Admin",
		Permissions: []domain.Permission{
			{Resource: "attendance.leave", Action: "create", Scope: "all"},
			{Resource: "attendance.leave", Action: "read", Scope: "all"},
			{Resource: "attendance.leave", Action: "update", Scope: "all"},
		}, CreatedAt: now,
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

package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestDirectAttendanceCompatibilitySupportsTenantDefaultTemplates preserves the optional legacy API fields.
func TestDirectAttendanceCompatibilitySupportsTenantDefaultTemplates(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-attendance-self", TenantID: "tenant-1", Name: "Attendance self service",
		Permissions: []domain.Permission{{Resource: "attendance.leave", Action: "create", Scope: "self"}},
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", AccountID: "acct-manager",
		Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-applicant", TenantID: "tenant-1", Name: "Applicant", AccountID: "acct-applicant",
		ManagerEmployeeID: "emp-manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-manager", TenantID: "tenant-1", DisplayName: "Manager", EmployeeID: "emp-manager",
		Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-applicant", TenantID: "tenant-1", DisplayName: "Applicant", EmployeeID: "emp-applicant",
		Status: "active", DirectPermissionSetIDs: []string{"ps-attendance-self"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }})
	if _, err := svc.EnsureTenantDefaultFormTemplates(context.Background(), "tenant-1"); err != nil {
		t.Fatal(err)
	}
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-applicant"}
	leave, err := svc.Attendance().CreateLeaveRequest(ctx, domain.CreateLeaveRequestInput{
		LeaveType: "annual", StartAt: "2026-07-16T09:00:00+08:00", EndAt: "2026-07-16T18:00:00+08:00", Hours: 8,
	})
	if err != nil {
		t.Fatalf("default leave template rejected legacy optional reason: %v", err)
	}
	if leave.Status != "pending_approval" || leave.Reason == "" {
		t.Fatalf("unexpected leave compatibility projection: %+v", leave)
	}
	overtime, err := svc.Attendance().CreateOvertimeRequest(ctx, domain.CreateOvertimeRequestInput{
		StartAt: "2026-07-16T18:00:00+08:00", EndAt: "2026-07-16T21:00:00+08:00", Hours: 3,
	})
	if err != nil {
		t.Fatalf("default overtime template rejected legacy optional fields: %v", err)
	}
	if overtime.Status != "pending_approval" || overtime.OvertimeType != "weekday" || overtime.CompensationType != "leave" || overtime.Reason == "" {
		t.Fatalf("unexpected overtime compatibility projection: %+v", overtime)
	}
}

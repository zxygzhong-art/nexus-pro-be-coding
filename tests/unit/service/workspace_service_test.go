package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestWorkspaceOverviewAggregatesVisibleHRAndAttendance verifies the first workspace overview contract.
func TestWorkspaceOverviewAggregatesVisibleHRAndAttendance(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-2", EmployeeNo: "IKL002", Name: "張琪", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-3", EmployeeNo: "IKL003", Name: "陳俊", Status: "resigned", EmploymentStatus: "resigned", HireDate: ptrTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), ResignDate: ptrTime(time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-4", EmployeeNo: "IKL004", Name: "李雅琳", Status: "onboarding", EmploymentStatus: "onboarding", HireDate: ptrTime(time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertAttendanceClockRecord(context.Background(), domain.AttendanceClockRecord{ID: "clk-1", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_in", ClockedAt: now, RecordStatus: "accepted", Source: "geofence", CreatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lv-1", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", StartAt: now, EndAt: now.Add(8 * time.Hour), Hours: 8, Status: "approved", CreatedAt: now})

	got, err := svc.Workspace().WorkspaceOverview(ctx, domain.WorkspaceOverviewQuery{Year: 2026, Month: 6, Date: "2026-06-10"})
	if err != nil {
		t.Fatal(err)
	}
	if got.HRSummary.Active != 3 || got.HRSummary.Hires != 2 || got.HRSummary.Separations != 1 {
		t.Fatalf("unexpected HR summary: %+v", got.HRSummary)
	}
	if got.Attendance.CheckedIn != 1 || got.Attendance.Leave != 1 || got.Attendance.Absent != 0 {
		t.Fatalf("unexpected attendance summary: %+v", got.Attendance)
	}
	if len(got.TodoCategories) == 0 || got.TodoCategories[0].Key != "onboarding" || got.TodoCategories[0].Count != 1 {
		t.Fatalf("unexpected todo categories: %+v", got.TodoCategories)
	}
}

// TestWorkspaceOrganizationBuildsManagerTree verifies organization rows use employee numbers and manager levels.
func TestWorkspaceOrganizationBuildsManagerTree(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-eng", TenantID: "tenant-1", Name: "產品開發部", Path: []string{"ou-eng"}, CreatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-manager", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-eng", Position: "VP Engineering", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-child", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-eng", ManagerEmployeeID: "emp-manager", Position: "Engineer", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceOrganization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("expected two org rows, got %+v", got.Rows)
	}
	if got.Rows[0].ID != "IKL001" || !got.Rows[0].IsManager || got.Rows[0].ParentID != "__none__" {
		t.Fatalf("unexpected manager row: %+v", got.Rows[0])
	}
	if got.Rows[1].ID != "IKL002" || got.Rows[1].ParentID != "IKL001" || got.Rows[1].Level != 2 {
		t.Fatalf("unexpected child row: %+v", got.Rows[1])
	}
}

// TestWorkspaceAttendanceBuildsLeaveAndClockMatrices verifies leave cells and abnormal clock records are surfaced.
func TestWorkspaceAttendanceBuildsLeaveAndClockMatrices(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", CompanyEmail: "wei@example.com", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lv-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", StartAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 10, 23, 0, 0, 0, time.UTC), Hours: 8, Status: "approved", CreatedAt: now})
	_ = store.UpsertAttendanceClockRecord(context.Background(), domain.AttendanceClockRecord{ID: "clk-in", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-11", Direction: "clock_in", ClockedAt: time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC), RecordStatus: "accepted", Source: "geofence", CreatedAt: now})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Dates) != 30 || len(got.Attendance.Rows) != 1 || len(got.Clock.Rows) != 1 {
		t.Fatalf("unexpected matrix sizes: dates=%d attendance=%d clock=%d", len(got.Dates), len(got.Attendance.Rows), len(got.Clock.Rows))
	}
	if cell := got.Attendance.Rows[0].Cells[9]; cell.Type != "leave" || cell.Leave != "特" || cell.Hours != 8 {
		t.Fatalf("unexpected leave cell: %+v", cell)
	}
	if len(got.Clock.Abnormals) != 1 || got.Clock.Abnormals[0].Record.Reason != "缺下班卡" {
		t.Fatalf("unexpected clock abnormals: %+v", got.Clock.Abnormals)
	}
}

// TestCurrentAttendancePolicyReturnsDefaultCatalog verifies the policy read endpoint contract.
func TestCurrentAttendancePolicyReturnsDefaultCatalog(t *testing.T) {
	_, svc, ctx := newWorkspaceFixture(t)
	got, err := svc.Attendance().CurrentAttendancePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkTime.StandardStart != "09:00" || got.WorkTime.StandardEnd != "18:00" {
		t.Fatalf("unexpected work time: %+v", got.WorkTime)
	}
	if len(got.WorkTime.TimeOptions) != 48 || len(got.LeaveTypes) != 14 {
		t.Fatalf("unexpected policy option sizes: time=%d leave=%d", len(got.WorkTime.TimeOptions), len(got.LeaveTypes))
	}
}

// TestWorkspaceAdminsProjectsIAMAssignments verifies IAM grants become administrator rows and candidates.
func TestWorkspaceAdminsProjectsIAMAssignments(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-hr", TenantID: "tenant-1", Name: "人力資源部", Path: []string{"ou-hr"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-reader", TenantID: "tenant-1", Name: "Reader", Permissions: []domain.Permission{{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"}}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-hr-admin", TenantID: "tenant-1", Name: "HR Admin", Permissions: []domain.Permission{{Resource: "hr.employee", Action: "update", Scope: "all"}, {Resource: "attendance.leave", Action: "read", Scope: "all"}, {Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"}}, CreatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-reader", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-hr", Position: "HR Director", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-target", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-hr", Position: "HR Manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-candidate", EmployeeNo: "IKL003", Name: "陳俊", OrgUnitID: "ou-hr", Position: "Recruiter", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-reader", TenantID: "tenant-1", EmployeeID: "emp-reader", Status: "active", DirectPermissionSetIDs: []string{"ps-reader"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-target", TenantID: "tenant-1", EmployeeID: "emp-target", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-candidate", TenantID: "tenant-1", EmployeeID: "emp-candidate", Status: "active", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{ID: "psa-target", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-target", PermissionSetID: "ps-hr-admin", Effect: "allow", CreatedAt: now})

	got, err := service.New(store).Workspace().WorkspaceAdmins(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reader"})
	if err != nil {
		t.Fatal(err)
	}
	var target *domain.WorkspaceAdmin
	for i := range got.Admins {
		if got.Admins[i].AccountID == "acct-target" {
			target = &got.Admins[i]
			break
		}
	}
	if target == nil || target.Permissions["employees"] != "edit" || target.Permissions["admins"] != "view" {
		t.Fatalf("target admin not projected correctly: %+v", got.Admins)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].AccountID != "acct-candidate" {
		t.Fatalf("unexpected candidates: %+v", got.Candidates)
	}
}

// TestWorkspaceAuditLogsFiltersAndProjects verifies audit filters and page projection.
func TestWorkspaceAuditLogsFiltersAndProjects(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-audit", TenantID: "tenant-1", Name: "Audit", Permissions: []domain.Permission{{Resource: "audit.log", Action: "read", Scope: "all"}}, CreatedAt: now})
	seedWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", EmployeeID: "emp-1", Status: "active", DirectPermissionSetIDs: []string{"ps-audit"}, CreatedAt: now})
	_ = store.AppendAuditLog(context.Background(), domain.AuditLog{ID: "audit-1", TenantID: "tenant-1", ActorAccountID: "acct-1", Action: "hr.employee.create", Resource: "hr.employee", Target: "emp-2", Severity: "medium", Details: map[string]any{"name": "張琪"}, CreatedAt: now})
	_ = store.AppendAuditLog(context.Background(), domain.AuditLog{ID: "audit-2", TenantID: "tenant-1", ActorAccountID: "acct-1", Action: "attendance.shift.update", Resource: "attendance.shift", Target: "shift-1", Severity: "medium", CreatedAt: now.Add(-24 * time.Hour)})

	got, err := service.New(store).Workspace().WorkspaceAuditLogs(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}, domain.WorkspaceAuditLogQuery{Type: "員工管理", Keyword: "張琪"}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Items[0].ID != "audit-1" || got.Items[0].Operator != "王偉" {
		t.Fatalf("unexpected audit projection: %+v", got)
	}
}

// newWorkspaceFixture creates a tenant, account, and permissions for workspace service tests.
func newWorkspaceFixture(t *testing.T) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workspace",
		TenantID: "tenant-1",
		Name:     "Workspace",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.org_unit", Action: "read", Scope: "all"},
			{Resource: "attendance.clock", Action: "read", Scope: "all"},
			{Resource: "attendance.leave", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-workspace"}, CreatedAt: now})
	return store, service.New(store), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

// seedWorkspaceEmployee fills tenant defaults before storing an employee.
func seedWorkspaceEmployee(t *testing.T, store *memory.Store, employee domain.Employee) {
	t.Helper()
	employee.TenantID = "tenant-1"
	if employee.Status == "" {
		employee.Status = "active"
	}
	if employee.EmploymentStatus == "" {
		employee.EmploymentStatus = employee.Status
	}
	if employee.CreatedAt.IsZero() {
		employee.CreatedAt = time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	}
	if employee.UpdatedAt.IsZero() {
		employee.UpdatedAt = employee.CreatedAt
	}
	if err := store.UpsertEmployee(context.Background(), employee); err != nil {
		t.Fatal(err)
	}
}

// ptrTime returns a pointer for inline date literals.
func ptrTime(value time.Time) *time.Time {
	return &value
}

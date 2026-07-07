package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestWorkspaceOverviewAggregatesVisibleHRAndAttendance 驗證工作區總覽 aggregates 可見 HR and 考勤。
func TestWorkspaceOverviewAggregatesVisibleHRAndAttendance(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-2", EmployeeNo: "IKL002", Name: "張琪", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-3", EmployeeNo: "IKL003", Name: "陳俊", Status: "resigned", EmploymentStatus: "resigned", HireDate: ptrTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), ResignDate: ptrTime(time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-4", EmployeeNo: "IKL004", Name: "李雅琳", Status: "onboarding", EmploymentStatus: "onboarding", HireDate: ptrTime(time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
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

// TestWorkspaceOrganizationBuildsManagerTree 驗證工作區 organization builds 主管 tree。
func TestWorkspaceOrganizationBuildsManagerTree(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-eng", TenantID: "tenant-1", Name: "產品開發部", Path: []string{"ou-eng"}, CreatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-manager", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-eng", Position: "VP Engineering", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-child", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-eng", ManagerEmployeeID: "emp-manager", Position: "Engineer", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})

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

// TestWorkspaceAttendanceBuildsLeaveAndClockMatrices 驗證工作區考勤 builds 請假 and 打卡 matrices。
func TestWorkspaceAttendanceBuildsLeaveAndClockMatrices(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", CompanyEmail: "wei@example.com", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
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

// TestWorkspaceAttendanceCountsOnlyApprovedLeaveAndOvertime 驗證工時統計只計已核准的請假與加班。
func TestWorkspaceAttendanceCountsOnlyApprovedLeaveAndOvertime(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	// pending 請假不應計入工時扣減。
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lv-pending", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", StartAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 10, 23, 0, 0, 0, time.UTC), Hours: 8, Status: "pending_approval", CreatedAt: now})
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lv-approved", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", StartAt: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 11, 23, 0, 0, 0, time.UTC), Hours: 8, Status: "approved", CreatedAt: now})
	// 只有 approved 加班會累計時數。
	_ = store.UpsertOvertimeRequest(context.Background(), domain.OvertimeRequest{ID: "ot-approved", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-12", StartAt: time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 12, 21, 0, 0, 0, time.UTC), Hours: 3, OvertimeType: "weekday", CompensationType: "leave", Status: "approved", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertOvertimeRequest(context.Background(), domain.OvertimeRequest{ID: "ot-pending", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-13", StartAt: time.Date(2026, 6, 13, 18, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 13, 20, 0, 0, 0, time.UTC), Hours: 2, OvertimeType: "weekday", CompensationType: "leave", Status: "pending_approval", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	row := got.Attendance.Rows[0]
	if cell := row.Cells[9]; cell.Type == "leave" {
		t.Fatalf("pending leave should not create a leave cell, got %+v", cell)
	}
	if cell := row.Cells[10]; cell.Type != "leave" || cell.Hours != 8 {
		t.Fatalf("approved leave should create a leave cell, got %+v", cell)
	}
	if cell := row.Cells[11]; cell.Overtime != 3 {
		t.Fatalf("approved overtime should mark the day cell, got %+v", cell)
	}
	if cell := row.Cells[12]; cell.Overtime != 0 {
		t.Fatalf("pending overtime should not mark the day cell, got %+v", cell)
	}
	if row.Summary.LeaveHours != 8 || row.Summary.OvertimeHours != 3 {
		t.Fatalf("unexpected summary hours: %+v", row.Summary)
	}
	expectedAttended := row.Summary.DueHours - 8 + 3
	if row.Summary.AttendedHours != expectedAttended {
		t.Fatalf("expected attended hours %v, got %v", expectedAttended, row.Summary.AttendedHours)
	}
	if got.Attendance.Summary.OvertimeHours != 3 {
		t.Fatalf("unexpected matrix overtime summary: %+v", got.Attendance.Summary)
	}
}

// TestWorkspaceClockShortHoursExemptedByApprovedLeave 驗證核准請假可豁免工時不足異常。
func TestWorkspaceClockShortHoursExemptedByApprovedLeave(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	// 半天請假 + 半天出勤：工時 4 小時但有 4 小時核准請假，不應標記異常。
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lv-half", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", StartAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), Hours: 4, Status: "approved", CreatedAt: now})
	_ = store.UpsertAttendanceClockRecord(context.Background(), domain.AttendanceClockRecord{ID: "clk-in", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_in", ClockedAt: time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC), RecordStatus: "accepted", Source: "geofence", CreatedAt: now})
	_ = store.UpsertAttendanceClockRecord(context.Background(), domain.AttendanceClockRecord{ID: "clk-out", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_out", ClockedAt: time.Date(2026, 6, 10, 18, 0, 0, 0, time.UTC), RecordStatus: "accepted", Source: "geofence", CreatedAt: now})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Clock.Abnormals) != 0 {
		t.Fatalf("expected short hours covered by approved leave, got abnormals %+v", got.Clock.Abnormals)
	}
}

// TestPlatformWorkspaceEmployeesFiltersAndNormalizesStatus 驗證平台工作區員工篩選 and normalizes 狀態。
func TestPlatformWorkspaceEmployeesFiltersAndNormalizesStatus(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-hr", TenantID: "tenant-1", Name: "人力資源部", Path: []string{"ou-hr"}, CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-sales", TenantID: "tenant-1", Name: "業務部", Path: []string{"ou-sales"}, CreatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-active", EmployeeNo: "E001", Name: "Active HR", CompanyEmail: "active@example.com", OrgUnitID: "ou-hr", Position: "HRBP", Status: "active", EmploymentStatus: "active", CreatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-probation", EmployeeNo: "E002", Name: "Probation HR", CompanyEmail: "probation@example.com", OrgUnitID: "ou-hr", Position: "Recruiter", Status: "probation", EmploymentStatus: "probation", CreatedAt: now.Add(time.Minute)})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-onboarding", EmployeeNo: "E003", Name: "Onboarding Sales", CompanyEmail: "onboarding@example.com", OrgUnitID: "ou-sales", Position: "AE", Status: "onboarding", EmploymentStatus: "onboarding", CreatedAt: now.Add(2 * time.Minute)})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned", EmployeeNo: "E004", Name: "Resigned HR", CompanyEmail: "resigned@example.com", OrgUnitID: "ou-hr", Position: "Former HR", Status: "resigned", EmploymentStatus: "resigned", CreatedAt: now.Add(3 * time.Minute)})

	activeHR, err := svc.Platform().WorkspaceEmployees(ctx, domain.PlatformWorkspaceEmployeesQuery{DepartmentID: "ou-hr", Status: "在職"})
	if err != nil {
		t.Fatal(err)
	}
	if len(activeHR.Employees) != 2 {
		t.Fatalf("expected two active HR rows, got %+v", activeHR.Employees)
	}
	for _, item := range activeHR.Employees {
		if item.Status != "在職" {
			t.Fatalf("expected active HR row status to match FE enum, got %+v", item)
		}
	}

	onboarding, err := svc.Platform().WorkspaceEmployees(ctx, domain.PlatformWorkspaceEmployeesQuery{Status: "待加入", Keyword: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(onboarding.Employees) != 1 || onboarding.Employees[0].ID != "E003" || onboarding.Employees[0].Status != "待加入" {
		t.Fatalf("unexpected onboarding filter result: %+v", onboarding.Employees)
	}
}

// TestCurrentAttendancePolicyReturnsDefaultCatalog 驗證目前考勤政策 returns 預設目錄。
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

// TestUpdateAttendancePolicyPersistsWorkspaceSettings 驗證考勤政策 persists 工作區 settings。
func TestUpdateAttendancePolicyPersistsWorkspaceSettings(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	ctx.ApprovalConfirmed = true
	input := domain.UpdateAttendancePolicyInput{
		WorkTime: domain.AttendancePolicyWorkTime{
			StandardStart: "08:30",
			StandardEnd:   "17:30",
			BreakStart:    "12:30",
			BreakEnd:      "13:30",
			Weekend:       "週日",
			CycleStart:    "5 日",
			CycleEnd:      "次月 4 日",
		},
		LeaveTypes: []domain.AttendanceLeaveType{
			{Code: "特", Name: "特休假", Quota: "20 天 / 年", Rule: "可遞延一年", Proof: "—"},
			{Code: "病", Name: "全薪病假", Quota: "30 天 / 年", Rule: "無累計", Proof: "診斷證明"},
		},
	}

	got, err := svc.Attendance().UpdateAttendancePolicy(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkTime.StandardStart != "08:30" || got.WorkTime.Weekend != "週日" || len(got.LeaveTypes) != 2 {
		t.Fatalf("unexpected updated policy: %+v", got)
	}
	stored, ok, err := store.GetAttendancePolicy(context.Background(), "tenant-1")
	if err != nil || !ok {
		t.Fatalf("policy was not stored: ok=%v err=%v", ok, err)
	}
	if stored.UpdatedByAccountID != "acct-1" || stored.WorkTime.CycleEnd != "次月 4 日" {
		t.Fatalf("unexpected stored policy: %+v", stored)
	}
	workspace, err := svc.Platform().Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.LeavePolicy.WorkTime.StandardStart != "08:30" || workspace.LeavePolicy.LeaveTypes[0].Quota != "20 天 / 年" {
		t.Fatalf("workspace did not project updated policy: %+v", workspace.LeavePolicy)
	}
}

// TestWorkspaceAdminsProjectsIAMAssignments 驗證工作區 admins projects IAM 指派。
func TestWorkspaceAdminsProjectsIAMAssignments(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-hr", TenantID: "tenant-1", Name: "人力資源部", Path: []string{"ou-hr"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-reader", TenantID: "tenant-1", Name: "Reader", Permissions: []domain.Permission{{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"}, {Resource: "hr.employee", Action: "read", Scope: "all"}}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-hr-admin", TenantID: "tenant-1", Name: "HR Admin", Permissions: []domain.Permission{{Resource: "hr.employee", Action: "update", Scope: "all"}, {Resource: "attendance.leave", Action: "read", Scope: "all"}, {Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"}}, CreatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-reader", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-hr", Position: "HR Director", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-target", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-hr", Position: "HR Manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-candidate", EmployeeNo: "IKL003", Name: "陳俊", OrgUnitID: "ou-hr", Position: "Recruiter", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
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

// TestWorkspaceAdminsRespectsHRDataScope 驗證工作區 admins respects HR 資料範圍。
func TestWorkspaceAdminsRespectsHRDataScope(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-hr", TenantID: "tenant-1", Name: "人力資源部", Path: []string{"ou-hr"}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-reader", TenantID: "tenant-1", Name: "Reader", Permissions: []domain.Permission{{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"}, {Resource: "hr.employee", Action: "read", Scope: "self"}}, CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-hr-admin", TenantID: "tenant-1", Name: "HR Admin", Permissions: []domain.Permission{{Resource: "hr.employee", Action: "update", Scope: "all"}, {Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"}}, CreatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-reader", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-hr", Position: "HR Director", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-target", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-hr", Position: "HR Manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-candidate", EmployeeNo: "IKL003", Name: "陳俊", OrgUnitID: "ou-hr", Position: "Recruiter", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-reader", TenantID: "tenant-1", EmployeeID: "emp-reader", Status: "active", DirectPermissionSetIDs: []string{"ps-reader"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-target", TenantID: "tenant-1", EmployeeID: "emp-target", Status: "active", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-candidate", TenantID: "tenant-1", EmployeeID: "emp-candidate", Status: "active", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{ID: "psa-target", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-target", PermissionSetID: "ps-hr-admin", Effect: "allow", CreatedAt: now})

	got, err := service.New(store).Workspace().WorkspaceAdmins(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-reader"})
	if err != nil {
		t.Fatal(err)
	}
	for _, admin := range got.Admins {
		if admin.AccountID == "acct-target" || admin.AccountID == "acct-candidate" {
			t.Fatalf("admin projection leaked out-of-scope account: %+v", got.Admins)
		}
	}
	for _, candidate := range got.Candidates {
		if candidate.AccountID == "acct-target" || candidate.AccountID == "acct-candidate" {
			t.Fatalf("candidate projection leaked out-of-scope account: %+v", got.Candidates)
		}
	}
}

// TestPlatformWorkspaceRequiresWorkflowFormTemplateRead 驗證平台工作區 requires 流程表單範本 read。
func TestPlatformWorkspaceRequiresWorkflowFormTemplateRead(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-workspace-no-form",
		TenantID: "tenant-1",
		Name:     "Workspace Without Form Design",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "attendance.leave", Action: "read", Scope: "all"},
			{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"},
			{Resource: "audit.log", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-workspace-no-form"}, CreatedAt: now})

	_, err := service.New(store).Platform().Workspace(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil {
		t.Fatal("expected workspace aggregate to require workflow form template read")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 403 || appErr.ReasonCode != "menu_denied" {
		t.Fatalf("expected workflow form template read denial, got %v", err)
	}
}

// TestWorkspaceAuditLogsFiltersAndProjects 驗證工作區稽核 logs 篩選 and projects。
func TestWorkspaceAuditLogsFiltersAndProjects(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-audit", TenantID: "tenant-1", Name: "Audit", Permissions: []domain.Permission{{Resource: "audit.log", Action: "read", Scope: "all"}}, CreatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
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

// newWorkspaceFixture 驗證工作區 fixture。
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
			{Resource: "attendance.leave", Action: "update", Scope: "all"},
			{Resource: "iam.permission_set_assignment", Action: "read", Scope: "all"},
			{Resource: "audit.log", Action: "read", Scope: "all"},
			{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-workspace"}, CreatedAt: now})
	return store, service.New(store), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
}

// insertWorkspaceEmployee 驗證 insert 工作區員工。
func insertWorkspaceEmployee(t *testing.T, store *memory.Store, employee domain.Employee) {
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

// ptrTime 驗證 ptr 時間。
func ptrTime(value time.Time) *time.Time {
	return &value
}

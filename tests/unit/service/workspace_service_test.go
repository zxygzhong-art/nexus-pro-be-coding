package service_test

import (
	"context"
	"strings"
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

// TestWorkspaceTurnoverFiltersClosedPositionsAndOrgUnits 驗證在職分析排除已停用崗位與已關閉組織。
func TestWorkspaceTurnoverFiltersClosedPositionsAndOrgUnits(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	for _, unit := range []domain.OrgUnit{
		{ID: "ou-active", TenantID: "tenant-1", Name: "啟用部門", Path: []string{"ou-active"}, CreatedAt: now, UpdatedAt: now},
		{ID: "ou-closed", TenantID: "tenant-1", Name: "已關閉部門", Path: []string{"ou-closed"}, Closed: true, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertOrgUnit(context.Background(), unit); err != nil {
			t.Fatal(err)
		}
	}
	for _, position := range []domain.Position{
		{ID: "pos-active", TenantID: "tenant-1", Code: "ACTIVE", Name: "啟用崗位", OrgUnitID: "ou-active", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now},
		{ID: "pos-disabled", TenantID: "tenant-1", Code: "DISABLED", Name: "停用崗位", OrgUnitID: "ou-active", Status: string(domain.PositionStatusDisabled), CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertPosition(context.Background(), position); err != nil {
			t.Fatal(err)
		}
	}
	hireDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-active", EmployeeNo: "E001", Name: "啟用員工", OrgUnitID: "ou-active", PositionID: "pos-active", Status: "active", EmploymentStatus: "active", HireDate: &hireDate, CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-disabled-position", EmployeeNo: "E002", Name: "停用崗位員工", OrgUnitID: "ou-active", PositionID: "pos-disabled", Status: "active", EmploymentStatus: "active", HireDate: &hireDate, CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-closed-org", EmployeeNo: "E003", Name: "關閉組織員工", OrgUnitID: "ou-closed", PositionID: "pos-active", Status: "active", EmploymentStatus: "active", HireDate: &hireDate, CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-unassigned", EmployeeNo: "E004", Name: "未分配員工", Status: "active", EmploymentStatus: "active", HireDate: &hireDate, CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceTurnover(ctx, domain.WorkspaceTurnoverQuery{Year: 2026, Month: 6, AnnualYear: 2026})
	if err != nil {
		t.Fatal(err)
	}
	monthlyTotal := got.Monthly.Rows[len(got.Monthly.Rows)-1]
	if monthlyTotal.RowType != "total" || monthlyTotal.End != 2 {
		t.Fatalf("expected only active and unassigned employees in monthly total, got %+v", monthlyTotal)
	}
	for _, row := range got.Monthly.Rows {
		if row.Dept == "已關閉部門" {
			t.Fatalf("closed org unit leaked into monthly rows: %+v", row)
		}
	}
	annualTotal := got.Annual.Rows[len(got.Annual.Rows)-1]
	if annualTotal.BU != "總計" || annualTotal.End != 2 {
		t.Fatalf("expected only active and unassigned employees in annual total, got %+v", annualTotal)
	}
}

func TestWorkspaceInsightsUsesRequestedMonthForOverviewMetrics(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-may", EmployeeNo: "IKL101", Name: "May Hire", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-jun", EmployeeNo: "IKL102", Name: "June Hire", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})

	may, err := svc.Workspace().Insights(ctx, domain.PlatformInsightsQuery{Month: "2026-05"})
	if err != nil {
		t.Fatal(err)
	}
	june, err := svc.Workspace().Insights(ctx, domain.PlatformInsightsQuery{Month: "2026-06"})
	if err != nil {
		t.Fatal(err)
	}
	mayDeptTasks := may.Reports["dept_tasks"].(map[string]any)
	juneDeptTasks := june.Reports["dept_tasks"].(map[string]any)
	mayHires := insightMetricValueByID(t, mayDeptTasks, "hires")
	juneHires := insightMetricValueByID(t, juneDeptTasks, "hires")
	if may.Month != "2026-05" || mayHires != 1 {
		t.Fatalf("expected May insights to use May overview metrics, got month=%s dept_tasks=%+v", may.Month, mayDeptTasks)
	}
	if june.Month != "2026-06" || juneHires != 1 {
		t.Fatalf("expected June insights to use June overview metrics, got month=%s dept_tasks=%+v", june.Month, juneDeptTasks)
	}
}

// TestWorkspaceInsightsProjectsAttendanceMembers 驗證洞察成員明細來自真實月度假勤矩陣。
func TestWorkspaceInsightsProjectsAttendanceMembers(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-2", EmployeeNo: "IKL002", Name: "張琪", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	if err := store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{
		ID: "sum-leave", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10",
		LeaveType: "annual", LeaveHours: 8, LeaveCounted: true, Source: "ehrms",
		ExternalRef: "IKL001:2026-06-10", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Workspace().Insights(ctx, domain.PlatformInsightsQuery{Month: "2026-06"})
	if err != nil {
		t.Fatal(err)
	}
	report := got.Reports["dept_tasks"].(map[string]any)
	members := report["members"].([]map[string]any)
	if len(members) != 2 {
		t.Fatalf("expected two attendance members, got %+v", members)
	}
	if members[0]["id"] != "IKL001" || members[0]["leave_days"] != float64(1) || members[0]["leave_type"] != "特休假" {
		t.Fatalf("unexpected first member projection: %+v", members[0])
	}
	if members[0]["primary_product"] != "—" || members[0]["task_count"] != 0 || len(members[0]["tasks"].([]map[string]any)) != 0 {
		t.Fatalf("unsourced task data must stay empty, got %+v", members[0])
	}
	memberHours := report["member_hours"].([]map[string]any)
	leaveChart := report["leave_chart"].([]map[string]any)
	if len(memberHours) != 2 || len(leaveChart) != 1 || leaveChart[0]["id"] != "IKL001" {
		t.Fatalf("unexpected member charts: hours=%+v leave=%+v", memberHours, leaveChart)
	}
	totalHours := insightMetricValueByID(t, report, "dept-total-hours").(float64)
	wantTotal := members[0]["hours"].(float64) + members[1]["hours"].(float64)
	if totalHours != wantTotal {
		t.Fatalf("expected total hours %.1f from member rows, got %.1f", wantTotal, totalHours)
	}
}

func TestWorkspaceInsightsMarksSalesAndFinanceAsNotConnected(t *testing.T) {
	_, svc, ctx := newWorkspaceFixture(t)
	got, err := svc.Workspace().Insights(ctx, domain.PlatformInsightsQuery{Month: "2026-06"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sales", "finance"} {
		report := got.Reports[key].(map[string]any)
		if report["source_status"] != "not_connected" || report["caveat"] == "" {
			t.Fatalf("expected %s report to be marked not_connected with caveat, got %+v", key, report)
		}
		if metrics := report["metrics"].([]map[string]any); len(metrics) != 0 {
			t.Fatalf("expected %s metrics to be empty until data source is connected, got %+v", key, metrics)
		}
	}
}

func insightMetricValueByID(t *testing.T, report map[string]any, id string) any {
	t.Helper()
	metrics := report["metrics"].([]map[string]any)
	for _, metric := range metrics {
		if metric["id"] == id {
			return metric["value"]
		}
	}
	t.Fatalf("metric %s not found in %+v", id, report)
	return nil
}

// TestWorkspaceOrganizationBuildsManagerTree 驗證工作區 organization builds 主管 tree。
func TestWorkspaceOrganizationBuildsManagerTree(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertPosition(context.Background(), domain.Position{
		ID: "pos-eng-dir", TenantID: "tenant-1", Code: "ENG-DIR", Name: "Engineering Director",
		OrgUnitID: "ou-eng", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now,
	})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "ou-eng", TenantID: "tenant-1", Name: "產品開發部", Path: []string{"ou-eng"},
		ManagerPositionID: "pos-eng-dir", CreatedAt: now, UpdatedAt: now,
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID: "emp-manager", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-eng",
		PositionID: "pos-eng-dir", Position: "VP Engineering", Status: "active", EmploymentStatus: "active",
		CreatedAt: now, UpdatedAt: now,
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID: "emp-child", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-eng",
		Position: "Engineer", Status: "active", EmploymentStatus: "active",
		CreatedAt: now, UpdatedAt: now,
	})

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
	if got.Rows[1].ManagerSource != "org_unit" || got.Rows[1].IsOverride {
		t.Fatalf("expected derived org_unit manager, got %+v", got.Rows[1])
	}
}

// TestWorkspaceOrganizationExcludesDepartedEmployees 驗證組織架構排除離職員工。
func TestWorkspaceOrganizationExcludesDepartedEmployees(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-active", EmployeeNo: "IKL001", Name: "在職員工", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned", EmployeeNo: "IKL002", Name: "離職員工", Status: "resigned", EmploymentStatus: "resigned", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-deleted", EmployeeNo: "IKL003", Name: "已刪除員工", Status: "deleted", EmploymentStatus: "deleted", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceOrganization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 || got.Rows[0].ID != "IKL001" {
		t.Fatalf("expected only active employee in organization, got %+v", got.Rows)
	}
}

// TestWorkspaceOrganizationFallsBackWhenOverrideManagerDeparted 驗證人工主管離職後回退至組織單元主管。
func TestWorkspaceOrganizationFallsBackWhenOverrideManagerDeparted(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertPosition(context.Background(), domain.Position{ID: "pos-manager", TenantID: "tenant-1", Code: "MGR", Name: "Manager", OrgUnitID: "ou-1", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-1", TenantID: "tenant-1", Name: "產品部", Path: []string{"ou-1"}, ManagerPositionID: "pos-manager", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-manager", EmployeeNo: "IKL001", Name: "現任主管", OrgUnitID: "ou-1", PositionID: "pos-manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned-manager", EmployeeNo: "IKL002", Name: "離職主管", OrgUnitID: "ou-1", Status: "resigned", EmploymentStatus: "resigned", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-member", EmployeeNo: "IKL003", Name: "成員", OrgUnitID: "ou-1", ManagerEmployeeID: "emp-resigned-manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceOrganization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]domain.WorkspaceOrganizationRow{}
	for _, row := range got.Rows {
		rows[row.ID] = row
	}
	if len(rows) != 2 || rows["IKL003"].ParentID != "IKL001" || rows["IKL003"].ManagerSource != "org_unit" {
		t.Fatalf("expected departed override to fall back to org unit manager, got %+v", got.Rows)
	}
}

// TestWorkspaceOrganizationManagerReportsToParentUnitManager 驗證單元主管會向父級單元主管匯報。
func TestWorkspaceOrganizationManagerReportsToParentUnitManager(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	for _, position := range []domain.Position{
		{ID: "pos-root-manager", TenantID: "tenant-1", Code: "ROOT-MGR", Name: "Root Manager", OrgUnitID: "ou-root", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now},
		{ID: "pos-child-manager", TenantID: "tenant-1", Code: "CHILD-MGR", Name: "Child Manager", OrgUnitID: "ou-child", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertPosition(context.Background(), position); err != nil {
			t.Fatal(err)
		}
	}
	for _, unit := range []domain.OrgUnit{
		{ID: "ou-root", TenantID: "tenant-1", Name: "公司", Path: []string{"ou-root"}, ManagerPositionID: "pos-root-manager", CreatedAt: now, UpdatedAt: now},
		{ID: "ou-child", TenantID: "tenant-1", Name: "產品部", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, ManagerPositionID: "pos-child-manager", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertOrgUnit(context.Background(), unit); err != nil {
			t.Fatal(err)
		}
	}
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-root", EmployeeNo: "IKL001", Name: "總主管", OrgUnitID: "ou-root", PositionID: "pos-root-manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-child-manager", EmployeeNo: "IKL002", Name: "產品主管", OrgUnitID: "ou-child", PositionID: "pos-child-manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-child", EmployeeNo: "IKL003", Name: "產品成員", OrgUnitID: "ou-child", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceOrganization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]domain.WorkspaceOrganizationRow{}
	for _, row := range got.Rows {
		rows[row.ID] = row
	}
	if rows["IKL002"].ParentID != "IKL001" || rows["IKL002"].Level != 2 {
		t.Fatalf("expected child unit manager to report to root manager, got %+v", rows["IKL002"])
	}
	if rows["IKL003"].ParentID != "IKL002" || rows["IKL003"].Level != 3 {
		t.Fatalf("expected child member to report to child unit manager, got %+v", rows["IKL003"])
	}
}

// TestWorkspaceOrganizationReportsManagerConfigurationIssue 驗證主管崗空缺時仍回退並回報配置異常。
func TestWorkspaceOrganizationReportsManagerConfigurationIssue(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertPosition(context.Background(), domain.Position{ID: "pos-root-manager", TenantID: "tenant-1", Code: "ROOT-MGR", Name: "Root Manager", OrgUnitID: "ou-root", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertPosition(context.Background(), domain.Position{ID: "pos-empty-manager", TenantID: "tenant-1", Code: "EMPTY-MGR", Name: "Empty Manager", OrgUnitID: "ou-child", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-root", TenantID: "tenant-1", Name: "公司", Path: []string{"ou-root"}, ManagerPositionID: "pos-root-manager", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{ID: "ou-child", TenantID: "tenant-1", Name: "產品部", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}, ManagerPositionID: "pos-empty-manager", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-root", EmployeeNo: "IKL001", Name: "總主管", OrgUnitID: "ou-root", PositionID: "pos-root-manager", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-child", EmployeeNo: "IKL002", Name: "產品成員", OrgUnitID: "ou-child", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceOrganization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range got.Rows {
		if row.ID == "IKL002" {
			found = true
			if row.ParentID != "IKL001" || row.ManagerIssue != "manager_position_unstaffed" {
				t.Fatalf("expected ancestor fallback with unstaffed warning, got %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("expected child employee row")
	}
}

// TestWorkspaceOrganizationOverrideBeatsOrgUnitManager 驗證覆蓋主管優先於組織單元主管崗。
func TestWorkspaceOrganizationOverrideBeatsOrgUnitManager(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	_ = store.UpsertPosition(context.Background(), domain.Position{
		ID: "pos-eng-dir", TenantID: "tenant-1", Code: "ENG-DIR", Name: "Engineering Director",
		OrgUnitID: "ou-eng", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now,
	})
	_ = store.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "ou-eng", TenantID: "tenant-1", Name: "產品開發部", Path: []string{"ou-eng"},
		ManagerPositionID: "pos-eng-dir", CreatedAt: now, UpdatedAt: now,
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID: "emp-manager", EmployeeNo: "IKL001", Name: "王偉", OrgUnitID: "ou-eng",
		PositionID: "pos-eng-dir", Position: "VP Engineering", Status: "active", EmploymentStatus: "active",
		CreatedAt: now, UpdatedAt: now,
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID: "emp-override", EmployeeNo: "IKL099", Name: "代理主管", OrgUnitID: "ou-eng",
		Position: "Acting Lead", Status: "active", EmploymentStatus: "active",
		CreatedAt: now, UpdatedAt: now,
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID: "emp-child", EmployeeNo: "IKL002", Name: "張琪", OrgUnitID: "ou-eng",
		ManagerEmployeeID: "emp-override", Position: "Engineer", Status: "active", EmploymentStatus: "active",
		CreatedAt: now, UpdatedAt: now,
	})

	got, err := svc.Workspace().WorkspaceOrganization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var child domain.WorkspaceOrganizationRow
	for _, row := range got.Rows {
		if row.ID == "IKL002" {
			child = row
			break
		}
	}
	if child.ParentID != "IKL099" || child.ManagerSource != "override" || !child.IsOverride {
		t.Fatalf("expected override manager, got %+v", child)
	}
}

// TestWorkspaceAttendanceBuildsLeaveAndClockMatrices 驗證工作區考勤 builds 請假 and 打卡 matrices。
func TestWorkspaceAttendanceBuildsLeaveAndClockMatrices(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", CompanyEmail: "wei@example.com", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{ID: "sum-leave", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", LeaveType: "annual", LeaveHours: 8, LeaveCounted: true, Source: "ehrms", ExternalRef: "IKL001:2026-06-10", CreatedAt: now, UpdatedAt: now})
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

// TestWorkspaceAttendanceMarksOnlyPastEligibleMissingDaysAbsent verifies absence boundaries and raw clock evidence.
func TestWorkspaceAttendanceMarksOnlyPastEligibleMissingDaysAbsent(t *testing.T) {
	store, _, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 7, 10, 0, 30, 0, 0, time.UTC)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-active", EmployeeNo: "E001", Name: "Active", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-mid-hire", EmployeeNo: "E002", Name: "Mid Hire", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned", EmployeeNo: "E003", Name: "Resigned", Status: "resigned", EmploymentStatus: "resigned", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), ResignDate: ptrTime(time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC))})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-future-hire", EmployeeNo: "E004", Name: "Future Hire", Status: "onboarding", EmploymentStatus: "onboarding", HireDate: ptrTime(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC))})
	_ = store.UpsertAttendanceClockRecord(context.Background(), domain.AttendanceClockRecord{ID: "clk-in", TenantID: "tenant-1", EmployeeID: "emp-active", WorkDate: "2026-07-08", Direction: "clock_in", ClockedAt: time.Date(2026, 7, 8, 1, 0, 0, 0, time.UTC), RecordStatus: "accepted", Source: "geofence", CreatedAt: now})
	_ = store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{ID: "summary", TenantID: "tenant-1", EmployeeID: "emp-active", WorkDate: "2026-07-09", ClockHours: 8, Source: "ehrms", ExternalRef: "E001:2026-07-09", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 7})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]domain.WorkspaceAttendanceRow{}
	for _, row := range got.Attendance.Rows {
		rows[row.Employee.ID] = row
	}
	if cell := rows["E001"].Cells[6]; cell.Type != "absence" || cell.Label != "缺勤" {
		t.Fatalf("expected elapsed missing July 7 to be absent, got %+v", cell)
	}
	if cell := rows["E001"].Cells[7]; cell.Type != "work" || !cell.Recorded {
		t.Fatalf("expected raw July 8 clock evidence to count as attendance, got %+v", cell)
	}
	if cell := rows["E001"].Cells[8]; cell.Type != "work" || !cell.Recorded || cell.Hours != 8 {
		t.Fatalf("expected July 9 daily summary to count as attendance, got %+v", cell)
	}
	if cell := rows["E001"].Cells[9]; cell.Type != "work" || cell.Recorded {
		t.Fatalf("expected current day to remain pending, got %+v", cell)
	}
	if cell := rows["E001"].Cells[12]; cell.Type != "work" || cell.Recorded {
		t.Fatalf("expected future workday to remain pending, got %+v", cell)
	}
	if cell := rows["E002"].Cells[6]; cell.Type != "empty" {
		t.Fatalf("expected pre-hire date to stay empty, got %+v", cell)
	}
	if cell := rows["E002"].Cells[7]; cell.Type != "absence" {
		t.Fatalf("expected elapsed hire-date workday without facts to be absent, got %+v", cell)
	}
	if cell := rows["E003"].Cells[2]; cell.Type != "absence" {
		t.Fatalf("expected pre-resignation workday without facts to be absent, got %+v", cell)
	}
	if cell := rows["E003"].Cells[5]; cell.Type != "empty" {
		t.Fatalf("expected post-resignation date to stay empty, got %+v", cell)
	}
	if cell := rows["E004"].Cells[8]; cell.Type != "empty" {
		t.Fatalf("expected onboarding pre-hire date to stay empty, got %+v", cell)
	}
}

// TestWorkspaceAttendanceNormalizesEHRMSLeaveTypes 驗證 eHRMS 假別名稱穩定映射且非請假明細不進入矩陣。
func TestWorkspaceAttendanceNormalizesEHRMSLeaveTypes(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	types := []struct {
		leaveType string
		code      string
	}{
		{leaveType: "Additional Leave", code: "彈"},
		{leaveType: "Full Pay Sick Leave", code: "病"},
		{leaveType: "Half Pay Sick Leave", code: "半"},
		{leaveType: "Menstruation Leave", code: "理"},
		{leaveType: "Personal Leave", code: "事"},
		{leaveType: "Compensatory Leave", code: "補"},
		{leaveType: "特休假", code: "特"},
		{leaveType: "Future Leave Type", code: "其"},
	}
	workdays := []int{1, 2, 3, 4, 5, 8, 9, 10}
	for i, item := range types {
		workDate := time.Date(2026, 6, workdays[i], 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		if err := store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{ID: "sum-" + workDate, TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: workDate, LeaveType: item.leaveType, LeaveHours: 8, LeaveCounted: true, Source: "ehrms", ExternalRef: "IKL001:" + workDate, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	row := got.Attendance.Rows[0]
	for i, item := range types {
		if cell := row.Cells[workdays[i]-1]; cell.Type != "leave" || cell.Leave != item.code {
			t.Fatalf("leave type %q expected code %q, got %+v", item.leaveType, item.code, cell)
		}
	}
}

// TestWorkspaceAttendanceMergesApprovedLocalLeaveWithDailyFacts 驗證矩陣即時合併已核准本地請假，且不重複計算每日假勤。
func TestWorkspaceAttendanceUsesDailyLeaveFactsInsteadOfLeaveRanges(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKM229", Name: "測試員工", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	if err := store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{
		ID: "lv-cross-weekend", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		StartAt: time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 7, 14, 17, 0, 0, 0, time.UTC),
		Hours: 28, Status: "approved", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{
		ID: "sum-ikm229-0709", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-07-09",
		LeaveType: "annual", LeaveHours: 7, LeaveCounted: true,
		Source: "ehrms", ExternalRef: "IKM229:2026-07-09", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 7})
	if err != nil {
		t.Fatal(err)
	}
	row := got.Attendance.Rows[0]
	if cell := row.Cells[8]; cell.Type != "leave" || cell.Hours != 7 || cell.Leave != "特" {
		t.Fatalf("expected July 9 daily fact only, got %+v", cell)
	}
	for _, day := range []int{10, 13, 14} {
		if cell := row.Cells[day-1]; cell.Type != "leave" || cell.Hours != 7 || cell.Leave != "特" {
			t.Fatalf("approved local leave should cover business day %d, got %+v", day, cell)
		}
	}
	if row.Summary.LeaveHours != 28 {
		t.Fatalf("expected merged leave total without duplicate July 9, got %+v", row.Summary)
	}
}

// TestWorkspaceAttendanceExcludesUndatedResignedEmployees 驗證離職狀態但缺少離職日的人員不汙染當月矩陣。
func TestWorkspaceAttendanceExcludesUndatedResignedEmployees(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-active", EmployeeNo: "E001", Name: "Active", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned-undated", EmployeeNo: "E002", Name: "Undated", Status: "resigned", EmploymentStatus: "resigned", CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned-before", EmployeeNo: "E003", Name: "Before", Status: "resigned", EmploymentStatus: "resigned", ResignDate: ptrTime(time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-resigned-during", EmployeeNo: "E004", Name: "During", Status: "resigned", EmploymentStatus: "resigned", ResignDate: ptrTime(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Attendance.Rows) != 2 || got.Attendance.Rows[0].Employee.ID != "E001" || got.Attendance.Rows[1].Employee.ID != "E004" {
		t.Fatalf("expected active and in-month resigned employees only, got %+v", got.Attendance.Rows)
	}
}

func TestWorkspaceAttendanceCSVExportNeutralizesSpreadsheetFormulas(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID: "emp-formula", EmployeeNo: "IKL099", Name: "=cmd", CompanyEmail: "formula@example.com",
		Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		CreatedAt: now, UpdatedAt: now,
	})

	raw, filename, err := svc.Workspace().ExportWorkspaceAttendanceCSV(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6}, "attendance")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if filename != "workspace-attendance-attendance-2026-06.csv" {
		t.Fatalf("unexpected filename %q", filename)
	}
	if !strings.HasPrefix(body, "\ufeff") {
		gotPrefix := body
		if len(gotPrefix) > 4 {
			gotPrefix = gotPrefix[:4]
		}
		t.Fatalf("expected UTF-8 BOM, got %q", gotPrefix)
	}
	if !strings.Contains(body, ",'=cmd,") {
		t.Fatalf("expected formula cell to be neutralized, got %q", body)
	}
}

// TestWorkspaceAttendanceUsesEHRMSDailySummaries 驗證工作區考勤 uses eHRMS 日彙總 without 打卡時間。
func TestWorkspaceAttendanceUsesEHRMSDailySummaries(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	if err := store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{
		ID:          "ads-1",
		TenantID:    "tenant-1",
		EmployeeID:  "emp-1",
		WorkDate:    "2026-06-10",
		ShiftStart:  "09:00",
		ShiftEnd:    "18:00",
		ShiftHours:  8,
		DailyHours:  8,
		ClockHours:  8,
		ClockStart:  "09:00",
		ClockEnd:    "18:00",
		Source:      "ehrms",
		ExternalRef: "IKL001:2026-06-10",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	attendanceCell := got.Attendance.Rows[0].Cells[9]
	if attendanceCell.Type != "work" || attendanceCell.Hours != 8 || attendanceCell.Label != "eHRMS" {
		t.Fatalf("expected eHRMS summary to mark attendance cell, got %+v", attendanceCell)
	}
	clockCell := got.Clock.Rows[0].Cells[9]
	if clockCell.Type != "work" || clockCell.In != "09:00" || clockCell.Out != "18:00" || clockCell.Abnormal {
		t.Fatalf("expected eHRMS summary to project clock times, got %+v", clockCell)
	}
	if got.Attendance.Rows[0].Summary.AttendedHours < 8 {
		t.Fatalf("expected eHRMS summary hours to count as attended, got %+v", got.Attendance.Rows[0].Summary)
	}
}

// TestWorkspaceAttendanceMarksIncompleteEHRMSClockSummaryAbnormal 驗證 eHRMS 缺少下班卡時會保留上班時間並列入異常。
func TestWorkspaceAttendanceMarksIncompleteEHRMSClockSummaryAbnormal(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now})
	if err := store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{
		ID: "ads-incomplete", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10",
		DailyHours: 8, ClockHours: 4, ClockStart: "09:00", Source: "ehrms", ExternalRef: "IKL001:2026-06-10", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	cell := got.Clock.Rows[0].Cells[9]
	if cell.In != "09:00" || cell.Out != "" || !cell.Abnormal || cell.Reason != "缺下班卡" {
		t.Fatalf("expected incomplete eHRMS clock summary to be abnormal, got %+v", cell)
	}
	if got.Clock.Summary.AbnormalDays != 1 || got.Clock.Summary.AbnormalPeople != 1 || got.Clock.Summary.NormalDays != 0 {
		t.Fatalf("unexpected clock summary: %+v", got.Clock.Summary)
	}
}

// TestWorkspaceAttendanceCountsApprovedLeaveAndOvertime 驗證工時統計合併每日假勤、本地核準請假與核准加班。
func TestWorkspaceAttendanceCountsDailyLeaveAndApprovedOvertime(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	// 本地核準請假與 eHRMS 每日事實合併，同一天取較完整的時數，避免重複累加。
	_ = store.UpsertLeaveRequest(context.Background(), domain.LeaveRequest{ID: "lv-approved", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", StartAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 12, 23, 0, 0, 0, time.UTC), Hours: 24, Status: "approved", CreatedAt: now})
	_ = store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{ID: "sum-leave", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-11", LeaveType: "annual", LeaveHours: 4, LeaveCounted: true, Leave2Type: "personal", Leave2Hours: 4, Leave2Counted: true, Source: "ehrms", ExternalRef: "IKL001:2026-06-11", CreatedAt: now, UpdatedAt: now})
	// 只有 approved 加班會累計時數。
	_ = store.UpsertOvertimeRequest(context.Background(), domain.OvertimeRequest{ID: "ot-approved", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-12", StartAt: time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 12, 21, 0, 0, 0, time.UTC), Hours: 3, OvertimeType: "weekday", CompensationType: "leave", Status: "approved", CreatedAt: now, UpdatedAt: now})
	_ = store.UpsertOvertimeRequest(context.Background(), domain.OvertimeRequest{ID: "ot-pending", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-13", StartAt: time.Date(2026, 6, 13, 18, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 13, 20, 0, 0, 0, time.UTC), Hours: 2, OvertimeType: "weekday", CompensationType: "leave", Status: "pending_approval", CreatedAt: now, UpdatedAt: now})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{Year: 2026, Month: 6})
	if err != nil {
		t.Fatal(err)
	}
	row := got.Attendance.Rows[0]
	if cell := row.Cells[9]; cell.Type != "leave" || cell.Hours != 7 {
		t.Fatalf("approved local leave should create a leave cell, got %+v", cell)
	}
	if cell := row.Cells[10]; cell.Type != "leave" || cell.Hours != 8 {
		t.Fatalf("two daily leave segments should create one exact 8-hour cell, got %+v", cell)
	}
	if cell := row.Cells[11]; cell.Overtime != 3 {
		t.Fatalf("approved overtime should mark the day cell, got %+v", cell)
	}
	if cell := row.Cells[12]; cell.Overtime != 0 {
		t.Fatalf("pending overtime should not mark the day cell, got %+v", cell)
	}
	if row.Summary.LeaveHours != 22 || row.Summary.OvertimeHours != 3 {
		t.Fatalf("unexpected summary hours: %+v", row.Summary)
	}
	expectedAttended := row.Summary.DueHours - 22 + 3
	if row.Summary.AttendedHours != expectedAttended {
		t.Fatalf("expected attended hours %v, got %v", expectedAttended, row.Summary.AttendedHours)
	}
	if got.Attendance.Summary.OvertimeHours != 3 {
		t.Fatalf("unexpected matrix overtime summary: %+v", got.Attendance.Summary)
	}
}

// TestWorkspaceClockShortHoursExemptedByDailyLeave 驗證每日假勤可豁免工時不足異常。
func TestWorkspaceClockShortHoursExemptedByDailyLeave(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	insertWorkspaceEmployee(t, store, domain.Employee{ID: "emp-1", EmployeeNo: "IKL001", Name: "王偉", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), CreatedAt: now, UpdatedAt: now})
	// 半天假勤 + 半天出勤：工時不足由同日每日假勤補足，不應標記異常。
	_ = store.UpsertAttendanceDailySummary(context.Background(), domain.AttendanceDailySummary{ID: "sum-half", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", LeaveType: "annual", LeaveHours: 4, LeaveCounted: true, Source: "ehrms", ExternalRef: "IKL001:2026-06-10", CreatedAt: now, UpdatedAt: now})
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

	activeHR, err := svc.Workspace().WorkspaceEmployees(ctx, domain.PlatformWorkspaceEmployeesQuery{DepartmentID: "ou-hr", Status: "在職"})
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

	onboarding, err := svc.Workspace().WorkspaceEmployees(ctx, domain.PlatformWorkspaceEmployeesQuery{Status: "待加入", Keyword: "sales"})
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
	if got.WorkTime.ClockMode != "flexible" || got.WorkTime.FlexibleClockInEarliest != "00:00" || got.WorkTime.FlexibleClockOutLatest != "23:30" || got.WorkTime.StandardStart != "09:00" || got.WorkTime.StandardEnd != "17:00" {
		t.Fatalf("unexpected work time: %+v", got.WorkTime)
	}
	if len(got.WorkTime.TimeOptions) != 48 || len(got.LeaveTypes) != 14 {
		t.Fatalf("unexpected policy option sizes: time=%d leave=%d", len(got.WorkTime.TimeOptions), len(got.LeaveTypes))
	}
}

// TestUpdateAttendancePolicyPersistsWorkspaceSettings 驗證考勤政策 persists 工作區 settings。
func TestUpdateAttendancePolicyPersistsWorkspaceSettings(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	input := domain.UpdateAttendancePolicyInput{
		WorkTime: domain.AttendancePolicyWorkTime{
			ClockMode:               "fixed",
			FlexibleClockInEarliest: "08:00",
			FlexibleClockOutLatest:  "20:00",
			StandardStart:           "08:30",
			StandardEnd:             "17:30",
			BreakStart:              "12:30",
			BreakEnd:                "13:30",
			Weekend:                 "週日",
			CycleStart:              "5 日",
			CycleEnd:                "次月 4 日",
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
	if got.WorkTime.ClockMode != "fixed" || got.WorkTime.StandardStart != "08:30" || got.WorkTime.Weekend != "週日" || len(got.LeaveTypes) != 2 {
		t.Fatalf("unexpected updated policy: %+v", got)
	}
	stored, ok, err := store.GetAttendancePolicy(context.Background(), "tenant-1")
	if err != nil || !ok {
		t.Fatalf("policy was not stored: ok=%v err=%v", ok, err)
	}
	if stored.UpdatedByAccountID != "acct-1" || stored.WorkTime.ClockMode != "fixed" || stored.WorkTime.FlexibleClockInEarliest != "08:00" || stored.WorkTime.FlexibleClockOutLatest != "20:00" || stored.WorkTime.CycleEnd != "次月 4 日" {
		t.Fatalf("unexpected stored policy: %+v", stored)
	}
	workspace, err := svc.Workspace().Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.LeavePolicy.WorkTime.StandardStart != "08:30" || workspace.LeavePolicy.LeaveTypes[0].Quota != "20 天 / 年" {
		t.Fatalf("workspace did not project updated policy: %+v", workspace.LeavePolicy)
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

	_, err := service.New(store).Workspace().Workspace(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
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

	got, err := service.New(store).Workspace().WorkspaceAuditLogs(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.WorkspaceAuditLogQuery{Type: "員工管理", Keyword: "張琪"}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Items[0].ID != "audit-1" || got.Items[0].Operator != "王偉" {
		t.Fatalf("unexpected audit projection: %+v", got)
	}
	if got.Items[0].Time != "2026-06-10T08:00:00Z" {
		t.Fatalf("expected RFC3339 UTC audit time, got %q", got.Items[0].Time)
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

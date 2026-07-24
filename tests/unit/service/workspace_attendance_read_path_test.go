package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
)

func TestWorkspaceAttendanceProjectsForDisplayWithoutPersistingReadModels(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 18, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID:               "emp-display",
		EmployeeNo:       "E001",
		Name:             "Display Only",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	for _, record := range []domain.AttendanceClockRecord{
		{
			ID:           "display-in",
			TenantID:     "tenant-1",
			EmployeeID:   "emp-display",
			WorkDate:     "2026-06-10",
			Direction:    "clock_in",
			ClockedAt:    time.Date(2026, 6, 10, 9, 0, 0, 0, now.Location()),
			RecordStatus: "accepted",
			Source:       "geofence",
			CreatedAt:    now,
		},
		{
			ID:           "display-out",
			TenantID:     "tenant-1",
			EmployeeID:   "emp-display",
			WorkDate:     "2026-06-10",
			Direction:    "clock_out",
			ClockedAt:    time.Date(2026, 6, 10, 18, 0, 0, 0, now.Location()),
			RecordStatus: "accepted",
			Source:       "geofence",
			CreatedAt:    now,
		},
	} {
		if err := store.UpsertAttendanceClockRecord(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{
		Year: 2026, Month: 6, Projection: "attendance", Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	cell := got.Attendance.Rows[0].Cells[9]
	if cell.Type != "work" || cell.ActualHours != 8 || cell.Label != "打卡" {
		t.Fatalf("expected an in-memory attendance projection for display, got %+v", cell)
	}

	projections, err := store.ListAttendanceDayProjections(
		context.Background(), "tenant-1", []string{"emp-display"}, "2026-06-01", "2026-06-30",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projections) != 0 {
		t.Fatalf("workspace GET must not persist attendance projections, got %+v", projections)
	}
	dailyRecords, err := store.ListAttendanceDailyRecords(
		context.Background(), "tenant-1", []string{"emp-display"}, "2026-06-01", "2026-06-30", "local",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dailyRecords) != 0 {
		t.Fatalf("workspace GET must not persist local daily records, got %+v", dailyRecords)
	}
	if _, ok, err := store.GetAttendanceDailyReconciliation(
		context.Background(), "tenant-1", "emp-display", "2026-06-10",
	); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("workspace GET must not persist daily reconciliation")
	}
}

func TestWorkspaceAttendanceExcludesSuperAdminsBeforePagination(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-super-admin",
		TenantID: "tenant-1",
		Name:     "Platform Admin",
		Permissions: []domain.Permission{
			{Resource: "*", Action: "*", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-super-admin",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-super-admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-super-admin"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID:               "emp-super-admin",
		AccountID:        "acct-super-admin",
		EmployeeNo:       "ADMIN001",
		Name:             "Super Admin",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID:               "emp-attendance",
		EmployeeNo:       "E001",
		Name:             "Attendance Employee",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})

	got, err := svc.Workspace().WorkspaceAttendance(ctx, domain.WorkspaceAttendanceQuery{
		Year: 2026, Month: 6, Projection: "attendance", Page: 1, PageSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Pagination == nil || got.Pagination.Total != 1 {
		t.Fatalf("super admin must be excluded before attendance pagination, got %+v", got.Pagination)
	}
	if len(got.Attendance.Rows) != 1 || got.Attendance.Rows[0].Employee.EmployeeID != "emp-attendance" {
		t.Fatalf("expected only attendance employee, got %+v", got.Attendance.Rows)
	}
}

func TestBusinessEmployeeQueriesExcludeSuperAdminsByDefault(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-business-super-admin",
		TenantID: "tenant-1",
		Name:     "Platform Admin",
		Permissions: []domain.Permission{
			{Resource: "*", Action: "*", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-business-workflow",
		TenantID: "tenant-1",
		Name:     "Business workflow reader",
		Permissions: []domain.Permission{
			{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	caller, ok, err := store.GetAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("workspace fixture account is missing")
	}
	caller.DirectPermissionSetIDs = append(caller.DirectPermissionSetIDs, "ps-business-workflow")
	if err := store.UpsertAccount(context.Background(), caller); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-business-super-admin",
		TenantID:               "tenant-1",
		EmployeeID:             "emp-business-super-admin",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-business-super-admin"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID:               "emp-business-super-admin",
		AccountID:        "acct-business-super-admin",
		EmployeeNo:       "ADMIN001",
		Name:             "Business Super Admin",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID:               "emp-business-user",
		EmployeeNo:       "E001",
		Name:             "Business Employee",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})

	page, err := svc.HR().QueryEmployees(ctx, domain.EmployeeQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "emp-business-user" {
		t.Fatalf("business employee list exposed super admin: %+v", page)
	}
	stats, err := svc.HR().EmployeeStats(ctx, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 || stats.Active != 1 {
		t.Fatalf("business employee stats included super admin: %+v", stats)
	}
	if _, err := svc.HR().GetEmployee(ctx, "emp-business-super-admin"); err == nil {
		t.Fatal("business employee detail must not expose a super admin")
	}
	catalog, err := svc.Workflow().FormDataSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range catalog.DataSources {
		if source.ID != "employees" {
			continue
		}
		if len(source.Records) != 1 || source.Records[0]["id"] != "emp-business-user" {
			t.Fatalf("workflow employee data source exposed super admin: %+v", source.Records)
		}
	}

	raw, err := store.ListEmployeesByQuery(context.Background(), "tenant-1", domain.EmployeeQuery{
		IncludeSuperAdmins: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2 {
		t.Fatalf("system-internal employee query must retain explicit super-admin access, got %+v", raw)
	}
}

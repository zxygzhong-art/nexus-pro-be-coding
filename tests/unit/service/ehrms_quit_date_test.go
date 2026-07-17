package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

func TestSyncEHRMSEmployeesMapsQuitDateForResignedEmployee(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"emp_id":       "IKM099",
		"name_zh":      "離職員工",
		"work_status":  "離職",
		"quit_date":    "2026/06/30",
		"dept_code":    "M0101",
		"dept_name_zh": "Nexus",
		"job_code":     "0704",
		"job_title_zh": "工程師",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID:               "emp-existing",
		TenantID:         "tenant-1",
		EmployeeNo:       "IKM099",
		Name:             "在職員工",
		Status:           "active",
		EmploymentStatus: "active",
		CreatedAt:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 0 || result.Updated != 1 || result.Failed != 0 {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}

	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "IKM099")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected resigned employee to be updated")
	}
	wantQuitDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if employee.EmploymentStatus != "resigned" || employee.ResignDate == nil || !employee.ResignDate.Equal(wantQuitDate) {
		t.Fatalf("expected quit_date to populate resigned employee, got %+v", employee)
	}
	if employee.EmploymentInfo["resign_date"] != "2026-06-30" {
		t.Fatalf("expected normalized resign_date in employment info, got %+v", employee.EmploymentInfo)
	}
}

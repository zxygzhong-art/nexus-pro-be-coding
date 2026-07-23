package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
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
	seedOrgUnitCodes(t, store, "tenant-1", "M0101")
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

func TestSyncEHRMSEmployeesUsesSeniorityStartForPendingHire(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"emp_id":          "IKM100",
		"name_zh":         "待入職員工",
		"work_status":     "待入职",
		"seniority_start": "2026/08/01",
		"probation_end":   "2026/11/01",
		"dept_code":       "M0101",
		"dept_name_zh":    "Nexus",
		"job_code":        "0704",
		"job_title_zh":    "工程師",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, "tenant-1", "M0101")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Failed != 0 {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}
	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "IKM100")
	if err != nil || !ok {
		t.Fatalf("expected pending employee, ok=%v err=%v", ok, err)
	}
	if employee.ID != "IKM100" {
		t.Fatalf("expected employees.id to equal emp_id, got %q", employee.ID)
	}
	wantHireDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if employee.EmploymentStatus != "onboarding" || employee.HireDate == nil || !employee.HireDate.Equal(wantHireDate) {
		t.Fatalf("expected pending hire status and date, got %+v", employee)
	}
	if employee.EmploymentInfo["hire_date"] != "2026-08-01" ||
		employee.EmploymentInfo["tenure_start_date"] != "2026-08-01" ||
		employee.EmploymentInfo["probation_end_date"] != "2026-11-01" {
		t.Fatalf("unexpected employment dates: %+v", employee.EmploymentInfo)
	}
}

func TestNormalizeEmployeeStatusSupportsEHRMSWorkStatuses(t *testing.T) {
	for raw, want := range map[string]string{
		"在職":  "active",
		"在职":  "active",
		"離職":  "resigned",
		"离职":  "resigned",
		"待入職": "onboarding",
		"待入职": "onboarding",
	} {
		if got := domain.NormalizeEmployeeStatus(raw); got != want {
			t.Fatalf("NormalizeEmployeeStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

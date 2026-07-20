package service_test

import (
	"context"
	"errors"
	"testing"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// TestEHRMSBulkSyncRejectsScopedGrantsBeforeFetch verifies scoped grants cannot trigger tenant-wide upstream reads or writes.
func TestEHRMSBulkSyncRejectsScopedGrantsBeforeFetch(t *testing.T) {
	tests := []struct {
		name       string
		permission domain.Permission
		client     fakeEHRMSClient
		run        func(*service.Service, domain.RequestContext) error
	}{
		{
			name:       "org units",
			permission: domain.Permission{Resource: "hr.org_unit", Action: "create", Scope: domain.ScopeSelf},
			client:     fakeEHRMSClient{departmentsErr: errors.New("scoped request must not fetch departments")},
			run: func(svc *service.Service, ctx domain.RequestContext) error {
				_, err := svc.HR().SyncEHRMSOrgUnits(ctx)
				return err
			},
		},
		{
			name:       "positions",
			permission: domain.Permission{Resource: "hr.position", Action: "create", Scope: domain.ScopeSelf},
			client:     fakeEHRMSClient{positionsErr: errors.New("scoped request must not fetch positions")},
			run: func(svc *service.Service, ctx domain.RequestContext) error {
				_, err := svc.HR().SyncEHRMSPositions(ctx)
				return err
			},
		},
		{
			name:       "employees self scope",
			permission: domain.Permission{Resource: "hr.employee", Action: "import", Scope: domain.ScopeSelf},
			client: fakeEHRMSClient{
				departmentsErr: errors.New("scoped request must not fetch departments for employee sync"),
				positionsErr:   errors.New("scoped request must not fetch positions for employee sync"),
				err:            errors.New("scoped request must not fetch employees"),
			},
			run: func(svc *service.Service, ctx domain.RequestContext) error {
				_, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
				return err
			},
		},
		{
			name:       "employees department scope",
			permission: domain.Permission{Resource: "hr.employee", Action: "import", Scope: domain.ScopeDepartment},
			client: fakeEHRMSClient{
				departmentsErr: errors.New("department-scoped request must not fetch departments for employee sync"),
				positionsErr:   errors.New("department-scoped request must not fetch positions for employee sync"),
				err:            errors.New("department-scoped request must not fetch employees"),
			},
			run: func(svc *service.Service, ctx domain.RequestContext) error {
				_, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
				return err
			},
		},
		{
			name:       "attendance",
			permission: domain.Permission{Resource: "attendance.clock", Action: "import", Scope: domain.ScopeSelf},
			client:     fakeEHRMSClient{attendanceErr: errors.New("scoped request must not fetch attendance")},
			run: func(svc *service.Service, ctx domain.RequestContext) error {
				_, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{test.permission}, service.Options{EHRMSClient: test.client})
			account, ok, err := store.GetAccount(context.Background(), ctx.TenantID, ctx.AccountID)
			if err != nil || !ok {
				t.Fatalf("account fixture lookup failed: ok=%v err=%v", ok, err)
			}
			account.EmployeeID = "emp-1"
			if err := store.UpsertAccount(context.Background(), account); err != nil {
				t.Fatal(err)
			}
			err = test.run(svc, ctx)
			appErr, ok := domain.AsAppError(err)
			if !ok || appErr.Status != 403 || appErr.ReasonCode != "data_scope_denied" || appErr.Message != "tenant-wide eHRMS sync requires all-tenant access" {
				t.Fatalf("expected tenant-wide scope denial before upstream fetch, got %v", err)
			}
		})
	}
}

// TestEHRMSBulkSyncAcceptsTenantScope verifies an explicitly tenant-wide grant reaches every bulk sync path.
func TestEHRMSBulkSyncAcceptsTenantScope(t *testing.T) {
	client := fakeEHRMSClient{
		departmentRows: []domain.EHRMSDepartmentRecord{{"code": "C01", "name": "Corporate", "closed": "false"}},
		positionRows:   []domain.EHRMSPositionRecord{{"job_code": "0901", "job_title_zh": "經理"}},
		attendanceRows: []domain.EHRMSAttendanceRecord{{
			"emp_id": "MISSING", "date": "2026-06-10", "shift_start": "09:00", "shift_end": "18:00", "clock_hours": "8",
		}},
	}
	_, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: domain.ScopeTenant},
		{Resource: "hr.position", Action: "create", Scope: domain.ScopeTenant},
		{Resource: "attendance.clock", Action: "import", Scope: domain.ScopeTenant},
	}, service.Options{EHRMSClient: client})

	orgResult, err := svc.HR().SyncEHRMSOrgUnits(ctx)
	if err != nil || orgResult.Fetched != 1 || orgResult.Upserted != 1 {
		t.Fatalf("expected tenant-scoped org sync success, result=%+v err=%v", orgResult, err)
	}
	positionResult, err := svc.HR().SyncEHRMSPositions(ctx)
	if err != nil || positionResult.Fetched != 1 || positionResult.Upserted != 1 {
		t.Fatalf("expected tenant-scoped position sync success, result=%+v err=%v", positionResult, err)
	}
	attendanceResult, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil || attendanceResult.Fetched != 1 || attendanceResult.Skipped != 1 {
		t.Fatalf("expected tenant-scoped attendance sync success, result=%+v err=%v", attendanceResult, err)
	}
}

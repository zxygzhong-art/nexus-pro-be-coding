package service_test

import (
	"context"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestAttendanceEmploymentEligibilityControlsClockStatusAndCreate(t *testing.T) {
	tests := []struct {
		status  string
		allowed bool
	}{
		{status: "active", allowed: true},
		{status: "resigned", allowed: false},
		{status: "deleted", allowed: false},
		{status: "inactive", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			store, svc, employeeCtx := newAttendanceEmploymentFixture(t, tt.status)

			status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
			if err != nil {
				t.Fatal(err)
			}
			if tt.allowed {
				if !status.CanClockIn || status.CanClockOut || status.NextAction != "clock_in" {
					t.Fatalf("active employee should retain clock actions, got %+v", status)
				}
			} else if status.CanClockIn || status.CanClockOut || status.NextAction != "complete" {
				t.Fatalf("inactive employment must expose a read-only clock status, got %+v", status)
			}

			_, createErr := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
				Direction:      "clock_in",
				ClientEventID:  "eligibility-" + tt.status,
				Latitude:       25.033964,
				Longitude:      121.564468,
				AccuracyMeters: 5,
			})
			records, err := store.ListAttendanceClockRecords(t.Context(), "tenant-1", domain.AttendanceClockRecordQuery{EmployeeID: "emp-1"})
			if err != nil {
				t.Fatal(err)
			}
			if tt.allowed {
				if createErr != nil || len(records) != 1 {
					t.Fatalf("active employee clock create failed: err=%v records=%d", createErr, len(records))
				}
				return
			}
			assertAttendanceEmployeeInactive(t, createErr)
			if len(records) != 0 {
				t.Fatalf("rejected clock create left %d records", len(records))
			}
		})
	}
}

func TestInactiveEmploymentBlocksAttendanceSubmissionSideEffects(t *testing.T) {
	for _, employmentStatus := range []string{"resigned", "deleted", "inactive"} {
		t.Run(employmentStatus, func(t *testing.T) {
			store, svc, employeeCtx := newAttendanceEmploymentFixture(t, employmentStatus)
			if err := store.UpsertLeaveBalance(t.Context(), domain.LeaveBalance{
				ID: "lb-1", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
				RemainingHours: 16, UpdatedAt: attendanceFixtureClockInTime(),
			}); err != nil {
				t.Fatal(err)
			}

			_, err := svc.Attendance().CreateAttendanceClockRecord(employeeCtx, domain.CreateAttendanceClockRecordInput{
				Direction: "clock_in", ClientEventID: "blocked-clock-" + employmentStatus,
				Latitude: 25.033964, Longitude: 121.564468, AccuracyMeters: 5,
			})
			assertAttendanceEmployeeInactive(t, err)

			_, err = svc.Attendance().CreateAttendanceCorrection(employeeCtx, domain.CreateAttendanceCorrectionInput{
				CorrectionType:     "add_record",
				Direction:          "clock_in",
				RequestedClockedAt: "2026-06-10T09:00:00+08:00",
				Reason:             "eligibility regression",
			})
			assertAttendanceEmployeeInactive(t, err)

			_, err = svc.Attendance().CreateLeaveRequest(employeeCtx, domain.CreateLeaveRequestInput{
				LeaveType: "annual", StartAt: "2026-06-10", EndAt: "2026-06-11", Hours: 8,
			})
			assertAttendanceEmployeeInactive(t, err)

			_, err = svc.Attendance().CreateOvertimeRequest(employeeCtx, domain.CreateOvertimeRequestInput{
				StartAt: "2026-06-10T18:00:00+08:00", EndAt: "2026-06-10T20:00:00+08:00",
				Hours: 2, OvertimeType: "weekday", CompensationType: "pay",
			})
			assertAttendanceEmployeeInactive(t, err)

			_, err = svc.Workflow().SubmitForm(employeeCtx, domain.SubmitFormInput{
				TemplateKey: "leave-request",
				Payload: map[string]any{
					"leave_type": "annual", "start_at": "2026-06-10", "end_at": "2026-06-11",
				},
			})
			assertAttendanceEmployeeInactive(t, err)

			_, err = svc.Workflow().SubmitForm(employeeCtx, domain.SubmitFormInput{
				TemplateKey: "overtime-approval",
				Payload: map[string]any{
					"start_at": "2026-06-10T18:00:00+08:00", "end_at": "2026-06-10T20:00:00+08:00",
					"hours": 2, "overtime_type": "weekday", "compensation_type": "pay",
				},
			})
			assertAttendanceEmployeeInactive(t, err)

			assertNoAttendanceSubmissionSideEffects(t, store)
			balance, ok, err := store.GetLeaveBalance(t.Context(), "tenant-1", "lb-1")
			if err != nil || !ok || balance.RemainingHours != 16 {
				t.Fatalf("rejected submissions changed leave balance: ok=%v err=%v balance=%+v", ok, err, balance)
			}
		})
	}
}

func TestInactiveEmploymentPreflightWinsBeforeWorkflowStageResolution(t *testing.T) {
	store, svc, employeeCtx := newAttendanceEmploymentFixture(t, "resigned")
	template, ok, err := store.GetFormTemplate(t.Context(), "tenant-1", "ft-leave-request")
	if err != nil || !ok {
		t.Fatalf("leave template lookup failed: ok=%v err=%v", ok, err)
	}
	template.Schema = workflowEnabledTemplateSchema()
	if err := store.UpsertFormTemplate(t.Context(), template); err != nil {
		t.Fatal(err)
	}

	_, err = svc.Workflow().SubmitForm(employeeCtx, domain.SubmitFormInput{
		TemplateKey: "leave-request",
		Payload: map[string]any{
			"leave_type": "annual", "start_at": "2026-06-10", "end_at": "2026-06-11",
		},
	})
	assertAttendanceEmployeeInactive(t, err)
	assertNoAttendanceSubmissionSideEffects(t, store)
}

func TestResignedEmployeeRetainsHistoricalAttendanceAndFormReads(t *testing.T) {
	store, svc, employeeCtx := newAttendanceEmploymentFixture(t, "resigned")
	now := attendanceFixtureClockInTime()
	if err := store.UpsertAttendanceClockRecord(t.Context(), domain.AttendanceClockRecord{
		ID: "acr-history", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10",
		Direction: "clock_in", ClockedAt: now, RecordStatus: "accepted", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormInstance(t.Context(), domain.FormInstance{
		ID: "fi-history", TenantID: "tenant-1", TemplateID: "ft-leave-request",
		ApplicantAccountID: "acct-employee", Status: "approved", SubmittedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	status, err := svc.Attendance().AttendanceClockStatus(employeeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if status.ClockIn == nil || status.ClockIn.ID != "acr-history" || status.CanClockIn || status.CanClockOut || status.NextAction != "complete" {
		t.Fatalf("historical clock status should remain visible but read-only, got %+v", status)
	}
	page, err := svc.Attendance().ListAttendanceClockRecordPage(employeeCtx, domain.AttendanceClockRecordQuery{}, domain.PageRequest{})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "acr-history" {
		t.Fatalf("historical clock list unavailable: err=%v page=%+v", err, page)
	}
	detail, err := svc.Workflow().GetFormInstanceDetail(employeeCtx, "fi-history")
	if err != nil || detail.ID != "fi-history" {
		t.Fatalf("historical form detail unavailable: err=%v detail=%+v", err, detail)
	}
}

func newAttendanceEmploymentFixture(t *testing.T, employmentStatus string) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	store, _, employeeCtx, _, _ := newAttendanceFixture(t)
	permissionSet, ok, err := store.GetPermissionSet(t.Context(), "tenant-1", "ps-attendance-self")
	if err != nil || !ok {
		t.Fatalf("attendance permission set lookup failed: ok=%v err=%v", ok, err)
	}
	permissionSet.Permissions = append(permissionSet.Permissions,
		domain.Permission{Resource: "attendance.leave", Action: "read", Scope: "self"},
		domain.Permission{Resource: "attendance.leave", Action: "create", Scope: "self"},
		domain.Permission{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
		domain.Permission{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
	)
	if err := store.UpsertPermissionSet(t.Context(), permissionSet); err != nil {
		t.Fatal(err)
	}
	employee, ok, err := store.GetEmployee(t.Context(), "tenant-1", "emp-1")
	if err != nil || !ok {
		t.Fatalf("employee lookup failed: ok=%v err=%v", ok, err)
	}
	employee.Status = employmentStatus
	employee.EmploymentStatus = employmentStatus
	if err := store.UpsertEmployee(t.Context(), employee); err != nil {
		t.Fatal(err)
	}
	svc := newDirectAttendanceWorkflowService(t, store, attendanceFixtureClockInTime(), "leave-request", "overtime-approval")
	return store, svc, employeeCtx
}

func assertAttendanceEmployeeInactive(t *testing.T, err error) {
	t.Helper()
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 403 || appErr.ReasonCode != "attendance_employee_inactive" {
		t.Fatalf("expected attendance_employee_inactive 403, got %v", err)
	}
}

func assertNoAttendanceSubmissionSideEffects(t *testing.T, store *memory.Store) {
	t.Helper()
	ctx := context.Background()
	clocks, err := store.ListAttendanceClockRecords(ctx, "tenant-1", domain.AttendanceClockRecordQuery{})
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListAttendanceCorrectionRequests(ctx, "tenant-1", domain.AttendanceCorrectionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	leaves, err := store.ListLeaveRequests(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	overtime, err := store.ListOvertimeRequestsByQuery(ctx, "tenant-1", domain.OvertimeRequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	forms, err := store.ListFormInstances(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(clocks) != 0 || len(corrections) != 0 || len(leaves) != 0 || len(overtime) != 0 || len(forms) != 0 {
		t.Fatalf("rejected submissions left business side effects: clocks=%d corrections=%d leaves=%d overtime=%d forms=%d",
			len(clocks), len(corrections), len(leaves), len(overtime), len(forms))
	}
}

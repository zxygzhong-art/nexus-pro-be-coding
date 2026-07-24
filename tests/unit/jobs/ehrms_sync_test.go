package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/jobs"
)

func TestEHRMSSyncSchedulerRunsOrderedUnifiedSync(t *testing.T) {
	order := []string{}
	hrService := &recordingEHRMSSyncHRService{
		order:          &order,
		orgResult:      domain.EHRMSOrgUnitSyncResponse{Fetched: 2, Upserted: 2},
		employeeResult: domain.EHRMSEmployeeSyncResponse{Fetched: 2, Created: 1, Updated: 1, Mode: "upsert"},
	}
	attendanceService := &recordingEHRMSSyncAttendanceService{
		order:           &order,
		leaveTypeResult: domain.EHRMSLeaveTypeSyncResponse{Fetched: 3, Upserted: 3},
		result:          domain.EHRMSAttendanceSyncResponse{Fetched: 2, Created: 1, Updated: 1, Mode: "upsert"},
	}
	scheduler := jobs.NewEHRMSSyncScheduler(hrService, attendanceService, nil)

	result, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSSyncOptions{
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Mode:      "upsert",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OrgUnits.Upserted != 2 || result.Employees.Fetched != 2 || result.LeaveTypes.Upserted != 3 || result.Attendance.Fetched != 2 {
		t.Fatalf("unexpected unified result: %+v", result)
	}
	wantOrder := []string{"org_units", "employees", "leave_types", "attendance"}
	if len(order) != len(wantOrder) {
		t.Fatalf("unexpected sync order: %v", order)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("unexpected sync order: %v", order)
		}
	}
	if hrService.ctx.TenantID != "tenant-1" || hrService.ctx.AccountID != "acct-1" {
		t.Fatalf("unexpected scheduled request context: %+v", hrService.ctx)
	}
	if hrService.ctx.RequestID == "" || hrService.ctx.TraceID != hrService.ctx.RequestID {
		t.Fatalf("expected scheduled request/trace id, got %+v", hrService.ctx)
	}
	if hrService.employeeInput.Mode != "upsert" || attendanceService.input.Mode != "upsert" ||
		attendanceService.input.Start == "" || attendanceService.input.End == "" || !attendanceService.input.SkipLeaveTypes {
		t.Fatalf("unexpected sync inputs: employee=%+v attendance=%+v", hrService.employeeInput, attendanceService.input)
	}
}

func TestEHRMSSyncSchedulerLogsStageStatus(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	scheduler := jobs.NewEHRMSSyncScheduler(
		&recordingEHRMSSyncHRService{
			orgResult:      domain.EHRMSOrgUnitSyncResponse{Fetched: 2, Upserted: 2},
			employeeResult: domain.EHRMSEmployeeSyncResponse{Fetched: 3, Updated: 3, Mode: "upsert"},
		},
		&recordingEHRMSSyncAttendanceService{
			result: domain.EHRMSAttendanceSyncResponse{Fetched: 4, Created: 4, Mode: "upsert"},
		},
		logger,
	)

	if _, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSSyncOptions{
		TenantID: "tenant-1", AccountID: "acct-1", Mode: "upsert",
	}); err != nil {
		t.Fatal(err)
	}
	logs := output.String()
	for _, expected := range []string{
		"msg=\"eHRMS sync started\"",
		"msg=\"eHRMS sync stages completed\"",
		"stage=org_units_positions",
		"stage=employees",
		"stage=leave_types",
		"stage=attendance_leave",
	} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("missing sync status log %q in:\n%s", expected, logs)
		}
	}
	if got := strings.Count(logs, "msg=\"eHRMS sync stage started\""); got != 4 {
		t.Fatalf("sync stage started logs = %d, want 4:\n%s", got, logs)
	}
	if got := strings.Count(logs, "msg=\"eHRMS sync stage completed\""); got != 4 {
		t.Fatalf("sync stage completed logs = %d, want 4:\n%s", got, logs)
	}
}

func TestEHRMSSyncSchedulerRequiresActor(t *testing.T) {
	scheduler := jobs.NewEHRMSSyncScheduler(&recordingEHRMSSyncHRService{}, &recordingEHRMSSyncAttendanceService{}, nil)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSSyncOptions{AccountID: "acct-1"})
	if err == nil || err.Error() != "EHRMS_SYNC_TENANT_ID is required" {
		t.Fatalf("expected missing tenant error, got %v", err)
	}
	_, err = scheduler.SyncOnce(context.Background(), jobs.EHRMSSyncOptions{TenantID: "tenant-1"})
	if err == nil || err.Error() != "EHRMS_SYNC_ACCOUNT_ID is required" {
		t.Fatalf("expected missing account error, got %v", err)
	}
}

func TestEHRMSSyncSchedulerStopsAfterFailedStep(t *testing.T) {
	order := []string{}
	hrService := &recordingEHRMSSyncHRService{order: &order, employeeErr: errors.New("eHRMS unavailable")}
	attendanceService := &recordingEHRMSSyncAttendanceService{order: &order}
	scheduler := jobs.NewEHRMSSyncScheduler(hrService, attendanceService, nil)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSSyncOptions{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil || err.Error() != "eHRMS unavailable" {
		t.Fatalf("expected service error, got %v", err)
	}
	if len(order) != 2 || order[0] != "org_units" || order[1] != "employees" {
		t.Fatalf("attendance must not run after an employee failure: %v", order)
	}
}

func TestEHRMSSyncSchedulerSeparatesDailyCatalogsAndTodayData(t *testing.T) {
	order := []string{}
	attendanceService := &recordingEHRMSSyncAttendanceService{order: &order}
	scheduler := jobs.NewEHRMSSyncScheduler(
		&recordingEHRMSSyncHRService{order: &order},
		attendanceService,
		nil,
	)
	opts := jobs.EHRMSSyncOptions{TenantID: "tenant-1", AccountID: "acct-1", Mode: "upsert"}

	if _, err := scheduler.SyncDailyCatalogs(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "org_units,employees,leave_types" {
		t.Fatalf("daily catalog order = %q", got)
	}
	if attendanceService.input.Start != "" || attendanceService.input.End != "" {
		t.Fatalf("daily catalog sync must not call attendance: %+v", attendanceService.input)
	}

	order = nil
	result, err := scheduler.SyncToday(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "attendance" {
		t.Fatalf("30-minute sync order = %q", got)
	}
	start, err := time.Parse(time.DateOnly, attendanceService.input.Start)
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse(time.DateOnly, attendanceService.input.End)
	if err != nil {
		t.Fatal(err)
	}
	if !end.Equal(start.AddDate(0, 0, 1)) || !attendanceService.input.SkipLeaveTypes {
		t.Fatalf("expected one-day attendance-only bounds, input=%+v", attendanceService.input)
	}
	if result.LeaveTypesFetched != 0 {
		t.Fatalf("30-minute sync must not refresh leave types: %+v", result)
	}
}

func TestEHRMSSyncSchedulerRunsOnceOnStartup(t *testing.T) {
	callCh := make(chan string, 4)
	hrService := &recordingEHRMSSyncHRService{callCh: callCh}
	attendanceService := &recordingEHRMSSyncAttendanceService{callCh: callCh}
	scheduler := jobs.NewEHRMSSyncScheduler(
		hrService,
		attendanceService,
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		scheduler.Run(ctx, jobs.EHRMSSyncOptions{TenantID: "tenant-1", AccountID: "acct-1", Mode: "create"})
	}()

	for _, want := range []string{"org_units", "employees", "leave_types", "attendance"} {
		select {
		case got := <-callCh:
			if got != want {
				t.Fatalf("startup sync call = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for startup %s sync", want)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop after cancellation")
	}
	if hrService.employeeInput.Mode != jobs.ScheduledEHRMSSyncMode ||
		attendanceService.input.Mode != jobs.ScheduledEHRMSSyncMode {
		t.Fatalf("scheduled mode must stay upsert, employee=%q attendance=%q", hrService.employeeInput.Mode, attendanceService.input.Mode)
	}
}

type recordingEHRMSSyncHRService struct {
	order          *[]string
	ctx            domain.RequestContext
	employeeInput  domain.EHRMSEmployeeSyncInput
	orgResult      domain.EHRMSOrgUnitSyncResponse
	employeeResult domain.EHRMSEmployeeSyncResponse
	orgErr         error
	employeeErr    error
	callCh         chan string
}

func (s *recordingEHRMSSyncHRService) SyncEHRMSOrgUnits(ctx domain.RequestContext) (domain.EHRMSOrgUnitSyncResponse, error) {
	s.ctx = ctx
	if s.order != nil {
		*s.order = append(*s.order, "org_units")
	}
	if s.callCh != nil {
		s.callCh <- "org_units"
	}
	return s.orgResult, s.orgErr
}

func (s *recordingEHRMSSyncHRService) SyncEHRMSEmployees(ctx domain.RequestContext, input domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error) {
	s.ctx = ctx
	s.employeeInput = input
	if s.order != nil {
		*s.order = append(*s.order, "employees")
	}
	if s.callCh != nil {
		s.callCh <- "employees"
	}
	return s.employeeResult, s.employeeErr
}

type recordingEHRMSSyncAttendanceService struct {
	order           *[]string
	ctx             domain.RequestContext
	input           domain.EHRMSAttendanceSyncInput
	leaveTypeResult domain.EHRMSLeaveTypeSyncResponse
	result          domain.EHRMSAttendanceSyncResponse
	leaveTypeErr    error
	err             error
	callCh          chan string
}

func (s *recordingEHRMSSyncAttendanceService) SyncEHRMSLeaveTypes(ctx domain.RequestContext) (domain.EHRMSLeaveTypeSyncResponse, error) {
	s.ctx = ctx
	if s.order != nil {
		*s.order = append(*s.order, "leave_types")
	}
	if s.callCh != nil {
		s.callCh <- "leave_types"
	}
	return s.leaveTypeResult, s.leaveTypeErr
}

func (s *recordingEHRMSSyncAttendanceService) SyncEHRMSAttendance(ctx domain.RequestContext, input domain.EHRMSAttendanceSyncInput) (domain.EHRMSAttendanceSyncResponse, error) {
	s.ctx = ctx
	s.input = input
	if s.order != nil {
		*s.order = append(*s.order, "attendance")
	}
	if s.callCh != nil {
		s.callCh <- "attendance"
	}
	return s.result, s.err
}

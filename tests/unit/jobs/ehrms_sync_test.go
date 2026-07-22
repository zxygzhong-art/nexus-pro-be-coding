package jobs_test

import (
	"context"
	"errors"
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
		order:  &order,
		result: domain.EHRMSAttendanceSyncResponse{Fetched: 2, Created: 1, Updated: 1, Mode: "upsert"},
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
	if result.OrgUnits.Upserted != 2 || result.Employees.Fetched != 2 || result.Attendance.Fetched != 2 {
		t.Fatalf("unexpected unified result: %+v", result)
	}
	wantOrder := []string{"org_units", "employees", "attendance"}
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
	if hrService.employeeInput.Mode != "upsert" || attendanceService.input.Mode != "upsert" {
		t.Fatalf("unexpected sync inputs: employee=%+v attendance=%+v", hrService.employeeInput, attendanceService.input)
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

func TestEHRMSSyncSchedulerRunsOnceOnStartup(t *testing.T) {
	callCh := make(chan string, 3)
	scheduler := jobs.NewEHRMSSyncScheduler(
		&recordingEHRMSSyncHRService{callCh: callCh},
		&recordingEHRMSSyncAttendanceService{callCh: callCh},
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		scheduler.Run(ctx, jobs.EHRMSSyncOptions{TenantID: "tenant-1", AccountID: "acct-1"})
	}()

	for _, want := range []string{"org_units", "employees", "attendance"} {
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
	order  *[]string
	ctx    domain.RequestContext
	input  domain.EHRMSAttendanceSyncInput
	result domain.EHRMSAttendanceSyncResponse
	err    error
	callCh chan string
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

package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
	"nexus-pro-be/internal/repository/memory"
)

// TestEHRMSEmployeeSyncSchedulerSyncOnceUsesConfiguredActor 驗證 eHRMS 員工 sync scheduler sync once uses configured actor。
func TestEHRMSEmployeeSyncSchedulerSyncOnceUsesConfiguredActor(t *testing.T) {
	service := &recordingEHRMSSyncService{
		result: domain.EHRMSEmployeeSyncResponse{Fetched: 2, Created: 1, Updated: 1, Mode: "upsert"},
	}
	scheduler := jobs.NewEHRMSEmployeeSyncScheduler(service, nil)

	result, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSEmployeeSyncOptions{
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Mode:      "upsert",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.Created != 1 || result.Updated != 1 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if service.ctx.TenantID != "tenant-1" || service.ctx.AccountID != "acct-1" {
		t.Fatalf("unexpected scheduled request context: %+v", service.ctx)
	}
	if service.input.Mode != "upsert" {
		t.Fatalf("expected upsert mode, got %+v", service.input)
	}
	if service.ctx.RequestID == "" || service.ctx.TraceID != service.ctx.RequestID {
		t.Fatalf("expected scheduled request/trace id, got %+v", service.ctx)
	}
}

// TestEHRMSPipelinePersistsPartialRun 驗證單列失敗會形成可查詢的 partial 運行。
func TestEHRMSPipelinePersistsPartialRun(t *testing.T) {
	store := memory.NewStore()
	employees := &recordingEHRMSSyncService{result: domain.EHRMSEmployeeSyncResponse{Fetched: 2, Created: 1, Failed: 1, Mode: "upsert"}}
	scheduler := jobs.NewEHRMSPipelineScheduler(employees, &recordingEHRMSAttendanceSyncService{}, nil).WithRunStore(store)

	if _, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSPipelineOptions{EmployeeTenantID: "tenant-1", EmployeeAccountID: "acct-1", EmployeeMode: "upsert"}); err != nil {
		t.Fatal(err)
	}
	runs, total, err := store.ListEHRMSSyncRuns(context.Background(), "tenant-1", domain.PageRequest{})
	if err != nil || total != 1 || len(runs) != 1 {
		t.Fatalf("unexpected runs total=%d items=%+v err=%v", total, runs, err)
	}
	if runs[0].Status != domain.EHRMSSyncRunStatusPartial || runs[0].Summary["employees"] == nil {
		t.Fatalf("unexpected partial run: %+v", runs[0])
	}
	steps, err := store.ListEHRMSSyncRunSteps(context.Background(), "tenant-1", runs[0].ID)
	if err != nil || len(steps) != 1 || steps[0].Status != domain.EHRMSSyncRunStatusPartial {
		t.Fatalf("unexpected steps: %+v err=%v", steps, err)
	}
}

// TestEHRMSPipelineRetriesTemporaryFailure 驗證暫時錯誤會有限重試並持久化 attempts。
func TestEHRMSPipelineRetriesTemporaryFailure(t *testing.T) {
	store := memory.NewStore()
	var calls []string
	employees := &recordingEHRMSSyncService{err: errors.New("fetch eHRMS employees failed: unavailable"), calls: &calls}
	scheduler := jobs.NewEHRMSPipelineScheduler(employees, &recordingEHRMSAttendanceSyncService{}, nil).WithRunStore(store)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSPipelineOptions{EmployeeTenantID: "tenant-1", EmployeeAccountID: "acct-1", RetryAttempts: 3, RetryBaseDelay: time.Nanosecond})
	if err == nil {
		t.Fatal("expected retry exhaustion error")
	}
	if len(calls) != 3 {
		t.Fatalf("employee calls = %d, want 3", len(calls))
	}
	runs, _, _ := store.ListEHRMSSyncRuns(context.Background(), "tenant-1", domain.PageRequest{})
	if len(runs) != 1 || runs[0].Status != domain.EHRMSSyncRunStatusFailed || runs[0].Attempt != 3 || !runs[0].Retryable {
		t.Fatalf("unexpected failed run: %+v", runs)
	}
	steps, _ := store.ListEHRMSSyncRunSteps(context.Background(), "tenant-1", runs[0].ID)
	if len(steps) != 3 {
		t.Fatalf("steps = %d, want one employee step per attempt", len(steps))
	}
}

// TestEHRMSEmployeeSyncSchedulerRequiresActor 驗證 eHRMS 員工 sync scheduler requires actor。
func TestEHRMSEmployeeSyncSchedulerRequiresActor(t *testing.T) {
	scheduler := jobs.NewEHRMSEmployeeSyncScheduler(&recordingEHRMSSyncService{}, nil)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSEmployeeSyncOptions{AccountID: "acct-1"})
	if err == nil || err.Error() != "EHRMS_SYNC_TENANT_ID is required" {
		t.Fatalf("expected missing tenant error, got %v", err)
	}

	_, err = scheduler.SyncOnce(context.Background(), jobs.EHRMSEmployeeSyncOptions{TenantID: "tenant-1"})
	if err == nil || err.Error() != "EHRMS_SYNC_ACCOUNT_ID is required" {
		t.Fatalf("expected missing account error, got %v", err)
	}
}

// TestEHRMSEmployeeSyncSchedulerPropagatesServiceError 驗證 eHRMS 員工 sync scheduler propagates 服務錯誤。
func TestEHRMSEmployeeSyncSchedulerPropagatesServiceError(t *testing.T) {
	service := &recordingEHRMSSyncService{err: errors.New("eHRMS unavailable")}
	scheduler := jobs.NewEHRMSEmployeeSyncScheduler(service, nil)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSEmployeeSyncOptions{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil || err.Error() != "eHRMS unavailable" {
		t.Fatalf("expected service error, got %v", err)
	}
}

// TestEHRMSAttendanceSyncSchedulerSyncOnceUsesConfiguredActor 驗證 eHRMS 考勤 sync scheduler uses configured actor。
func TestEHRMSAttendanceSyncSchedulerSyncOnceUsesConfiguredActor(t *testing.T) {
	service := &recordingEHRMSAttendanceSyncService{
		result: domain.EHRMSAttendanceSyncResponse{Fetched: 2, Created: 1, Updated: 1, Mode: "upsert"},
	}
	scheduler := jobs.NewEHRMSAttendanceSyncScheduler(service, nil)

	result, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSAttendanceSyncOptions{
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Mode:      "upsert",
		Since:     "2026-06-01",
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.Created != 1 || result.Updated != 1 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if service.ctx.TenantID != "tenant-1" || service.ctx.AccountID != "acct-1" {
		t.Fatalf("unexpected scheduled request context: %+v", service.ctx)
	}
	if service.input.Mode != "upsert" || service.input.Since != "2026-06-01" {
		t.Fatalf("unexpected attendance sync input: %+v", service.input)
	}
	if service.ctx.RequestID == "" || service.ctx.TraceID != service.ctx.RequestID {
		t.Fatalf("expected scheduled request/trace id, got %+v", service.ctx)
	}
}

// TestEHRMSAttendanceSyncSchedulerDefaultsTenantAndAccount 驗證 eHRMS 考勤 sync scheduler falls back to employee sync actor。
func TestEHRMSAttendanceSyncSchedulerDefaultsTenantAndAccount(t *testing.T) {
	service := &recordingEHRMSAttendanceSyncService{}
	scheduler := jobs.NewEHRMSAttendanceSyncScheduler(service, nil)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSAttendanceSyncOptions{
		DefaultTenantID:  "tenant-default",
		DefaultAccountID: "acct-default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.ctx.TenantID != "tenant-default" || service.ctx.AccountID != "acct-default" {
		t.Fatalf("expected fallback actor, got %+v", service.ctx)
	}
}

type recordingEHRMSSyncService struct {
	ctx    domain.RequestContext
	input  domain.EHRMSEmployeeSyncInput
	result domain.EHRMSEmployeeSyncResponse
	err    error
	calls  *[]string
	callCh chan string
}

// SyncEHRMSEmployees 驗證 eHRMS 員工。
func (s *recordingEHRMSSyncService) SyncEHRMSEmployees(ctx domain.RequestContext, input domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error) {
	s.ctx = ctx
	s.input = input
	if s.calls != nil {
		*s.calls = append(*s.calls, "employees")
	}
	if s.callCh != nil {
		s.callCh <- "employees"
	}
	return s.result, s.err
}

type recordingEHRMSAttendanceSyncService struct {
	ctx    domain.RequestContext
	input  domain.EHRMSAttendanceSyncInput
	result domain.EHRMSAttendanceSyncResponse
	err    error
	calls  *[]string
	callCh chan string
}

// SyncEHRMSAttendance 驗證 eHRMS 考勤。
func (s *recordingEHRMSAttendanceSyncService) SyncEHRMSAttendance(ctx domain.RequestContext, input domain.EHRMSAttendanceSyncInput) (domain.EHRMSAttendanceSyncResponse, error) {
	s.ctx = ctx
	s.input = input
	if s.calls != nil {
		*s.calls = append(*s.calls, "attendance")
	}
	if s.callCh != nil {
		s.callCh <- "attendance"
	}
	return s.result, s.err
}

// TestEHRMSPipelineSchedulerRunsEmployeesThenAttendance 驗證 pipeline 依序執行員工再考勤。
func TestEHRMSPipelineSchedulerRunsEmployeesThenAttendance(t *testing.T) {
	var calls []string
	employees := &recordingEHRMSSyncService{
		result: domain.EHRMSEmployeeSyncResponse{Fetched: 1, Created: 1, Mode: "upsert", DepartmentsUpserted: 1, PositionsUpserted: 1},
		calls:  &calls,
	}
	attendance := &recordingEHRMSAttendanceSyncService{
		result: domain.EHRMSAttendanceSyncResponse{Fetched: 2, Created: 2, Mode: "upsert"},
		calls:  &calls,
	}
	scheduler := jobs.NewEHRMSPipelineScheduler(employees, attendance, nil)

	result, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSPipelineOptions{
		EmployeeTenantID: "tenant-1", EmployeeAccountID: "acct-1", EmployeeMode: "upsert",
		AttendanceEnabled: true, AttendanceMode: "upsert", AttendanceSince: "2026-06-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "employees" || calls[1] != "attendance" {
		t.Fatalf("expected employees then attendance, got %v", calls)
	}
	if result.Employees.Fetched != 1 || result.Attendance.Fetched != 2 {
		t.Fatalf("unexpected pipeline result: %+v", result)
	}
	if attendance.input.Since != "2026-06-01" || attendance.ctx.TenantID != "tenant-1" {
		t.Fatalf("unexpected attendance call: ctx=%+v input=%+v", attendance.ctx, attendance.input)
	}
}

// TestEHRMSPipelineSchedulerStopsAfterEmployeeFailure 驗證前置員工同步失敗後不會寫入考勤。
func TestEHRMSPipelineSchedulerStopsAfterEmployeeFailure(t *testing.T) {
	var calls []string
	employees := &recordingEHRMSSyncService{
		err:   errors.New("eHRMS employee sync contains invalid rows"),
		calls: &calls,
	}
	attendance := &recordingEHRMSAttendanceSyncService{
		result: domain.EHRMSAttendanceSyncResponse{Fetched: 1, Updated: 1, Mode: "upsert"},
		calls:  &calls,
	}
	scheduler := jobs.NewEHRMSPipelineScheduler(employees, attendance, nil)

	result, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSPipelineOptions{
		EmployeeTenantID:  "tenant-1",
		EmployeeAccountID: "acct-1",
		AttendanceEnabled: true,
	})
	if err == nil || err.Error() != "eHRMS employee sync contains invalid rows" {
		t.Fatalf("expected employee error, got %v", err)
	}
	if len(calls) != 1 || calls[0] != "employees" {
		t.Fatalf("expected pipeline to stop after employee failure, got %v", calls)
	}
	if result.Attendance.Fetched != 0 || result.Attendance.Updated != 0 || len(result.Attendance.Results) != 0 {
		t.Fatalf("expected no attendance result after employee failure, got %+v", result.Attendance)
	}
}

// TestEHRMSPipelineSchedulerSkipsDisabledAttendance 驗證關閉考勤後 pipeline 只執行員工同步。
func TestEHRMSPipelineSchedulerSkipsDisabledAttendance(t *testing.T) {
	var calls []string
	scheduler := jobs.NewEHRMSPipelineScheduler(
		&recordingEHRMSSyncService{calls: &calls},
		&recordingEHRMSAttendanceSyncService{calls: &calls},
		nil,
	)

	if _, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSPipelineOptions{
		EmployeeTenantID: "tenant-1", EmployeeAccountID: "acct-1",
	}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "employees" {
		t.Fatalf("expected disabled attendance to be skipped, got %v", calls)
	}
}

// TestEHRMSPipelineSchedulerUsesAttendanceSchedule 驗證考勤 interval 與 run_on_start 獨立生效。
func TestEHRMSPipelineSchedulerUsesAttendanceSchedule(t *testing.T) {
	tests := []struct {
		name                 string
		attendanceInterval   time.Duration
		attendanceRunOnStart bool
	}{
		{name: "interval", attendanceInterval: 10 * time.Millisecond},
		{name: "run on start", attendanceInterval: time.Hour, attendanceRunOnStart: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCh := make(chan string, 16)
			scheduler := jobs.NewEHRMSPipelineScheduler(
				&recordingEHRMSSyncService{callCh: callCh},
				&recordingEHRMSAttendanceSyncService{callCh: callCh},
				nil,
			)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan struct{})
			go func() {
				defer close(done)
				scheduler.Run(ctx, jobs.EHRMSPipelineOptions{
					Interval: time.Hour, EmployeeTenantID: "tenant-1", EmployeeAccountID: "acct-1",
					AttendanceEnabled: true, AttendanceInterval: tt.attendanceInterval, AttendanceRunOnStart: tt.attendanceRunOnStart,
				})
			}()

			for _, want := range []string{"employees", "attendance"} {
				select {
				case got := <-callCh:
					if got != want {
						t.Fatalf("pipeline call = %q, want %q", got, want)
					}
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for %s call", want)
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("pipeline did not stop after cancellation")
			}
		})
	}
}

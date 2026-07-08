package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
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
	if service.ctx.TenantID != "tenant-1" || service.ctx.AccountID != "acct-1" || !service.ctx.ApprovalConfirmed {
		t.Fatalf("unexpected scheduled request context: %+v", service.ctx)
	}
	if service.input.Mode != "upsert" {
		t.Fatalf("expected upsert mode, got %+v", service.input)
	}
	if service.ctx.RequestID == "" || service.ctx.TraceID != service.ctx.RequestID {
		t.Fatalf("expected scheduled request/trace id, got %+v", service.ctx)
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
	if service.ctx.TenantID != "tenant-1" || service.ctx.AccountID != "acct-1" || !service.ctx.ApprovalConfirmed {
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
}

// SyncEHRMSEmployees 驗證 eHRMS 員工。
func (s *recordingEHRMSSyncService) SyncEHRMSEmployees(ctx domain.RequestContext, input domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error) {
	s.ctx = ctx
	s.input = input
	return s.result, s.err
}

type recordingEHRMSAttendanceSyncService struct {
	ctx    domain.RequestContext
	input  domain.EHRMSAttendanceSyncInput
	result domain.EHRMSAttendanceSyncResponse
	err    error
}

// SyncEHRMSAttendance 驗證 eHRMS 考勤。
func (s *recordingEHRMSAttendanceSyncService) SyncEHRMSAttendance(ctx domain.RequestContext, input domain.EHRMSAttendanceSyncInput) (domain.EHRMSAttendanceSyncResponse, error) {
	s.ctx = ctx
	s.input = input
	return s.result, s.err
}

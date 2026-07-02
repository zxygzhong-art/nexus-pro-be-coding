package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
)

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

func TestEHRMSEmployeeSyncSchedulerPropagatesServiceError(t *testing.T) {
	service := &recordingEHRMSSyncService{err: errors.New("eHRMS unavailable")}
	scheduler := jobs.NewEHRMSEmployeeSyncScheduler(service, nil)

	_, err := scheduler.SyncOnce(context.Background(), jobs.EHRMSEmployeeSyncOptions{TenantID: "tenant-1", AccountID: "acct-1"})
	if err == nil || err.Error() != "eHRMS unavailable" {
		t.Fatalf("expected service error, got %v", err)
	}
}

type recordingEHRMSSyncService struct {
	ctx    domain.RequestContext
	input  domain.EHRMSEmployeeSyncInput
	result domain.EHRMSEmployeeSyncResponse
	err    error
}

func (s *recordingEHRMSSyncService) SyncEHRMSEmployees(ctx domain.RequestContext, input domain.EHRMSEmployeeSyncInput) (domain.EHRMSEmployeeSyncResponse, error) {
	s.ctx = ctx
	s.input = input
	return s.result, s.err
}

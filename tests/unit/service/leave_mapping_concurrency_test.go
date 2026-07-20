package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

type leaveMappingBarrierStore struct {
	*memory.Store
	readers atomic.Int32
	release chan struct{}
	once    sync.Once
}

// ListLeaveTypeExternalMappings aligns pre-transaction reads so the race is deterministic.
func (s *leaveMappingBarrierStore) ListLeaveTypeExternalMappings(ctx context.Context, tenantID string) ([]domain.LeaveTypeExternalMapping, error) {
	items, err := s.Store.ListLeaveTypeExternalMappings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if s.readers.Add(1) == 2 {
		s.once.Do(func() { close(s.release) })
	}
	<-s.release
	return items, nil
}

// TestSaveLeaveTypeExternalMappingSerializesOverlappingWrites verifies validation and write share one lock boundary.
func TestSaveLeaveTypeExternalMappingSerializesOverlappingWrites(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	store, _, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.leave", Action: "update", Scope: "all"},
	}, service.Options{Now: func() time.Time { return now }})
	barrierStore := &leaveMappingBarrierStore{Store: store, release: make(chan struct{})}
	svc := service.New(barrierStore, service.Options{Now: func() time.Time { return now }})
	inputs := []domain.SaveLeaveTypeExternalMappingInput{
		{Source: "ehrms", ExternalCode: "Annual Leave", LeaveTypeID: "lt_annual", EffectiveFrom: "2026-01-01"},
		{Source: "ehrms", ExternalCode: "Annual Leave", LeaveTypeID: "lt_annual", EffectiveFrom: "2026-06-01"},
	}

	start := make(chan struct{})
	errs := make([]error, len(inputs))
	var wg sync.WaitGroup
	for index := range inputs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, errs[index] = svc.Attendance().SaveLeaveTypeExternalMapping(ctx, inputs[index])
		}(index)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one overlapping mapping write to succeed, errors=%v", errs)
	}
	mappings, err := store.ListLeaveTypeExternalMappings(context.Background(), ctx.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected one persisted mapping, got %+v", mappings)
	}
}

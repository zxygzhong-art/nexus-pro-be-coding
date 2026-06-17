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

func TestAuthzOutboxProcessorWritesOpenFGATupleAndMarksSucceeded(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendAuthzOutboxEvent(ctx, domain.AuthzOutboxEvent{
		ID:        "outbox-1",
		TenantID:  "tenant-1",
		EventType: string(domain.EventOpenFGARelationshipWrite),
		Payload: map[string]any{
			"object_type":  "hr.employee",
			"object_id":    "emp-1",
			"relation":     "owner",
			"subject_type": "account",
			"subject_id":   "acct-1",
		},
		Status:    "pending",
		CreatedAt: now,
	})
	writer := &recordingTupleWriter{}
	processor := jobs.NewAuthzOutboxProcessor(store, writer, nil)

	processed, err := processor.ProcessTenant(ctx, "tenant-1", jobs.AuthzOutboxOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(writer.changes) != 1 {
		t.Fatalf("expected one OpenFGA write, got %+v", writer.changes)
	}
	change := writer.changes[0]
	if change.Operation != domain.AuthzRelationshipTupleWrite || change.Tuple.ObjectType != "hr.employee" || change.Tuple.ObjectID != "emp-1" || change.Tuple.Relation != "owner" || change.Tuple.SubjectID != "acct-1" {
		t.Fatalf("unexpected tuple change: %+v", change)
	}
	events, err := store.ListAuthzOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "succeeded" || events[0].LastError != "" || events[0].ProcessedAt == nil {
		t.Fatalf("expected succeeded event, got %+v", events[0])
	}
}

func TestAuthzOutboxProcessorRetriesFailedOpenFGATuple(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendAuthzOutboxEvent(ctx, domain.AuthzOutboxEvent{
		ID:        "outbox-1",
		TenantID:  "tenant-1",
		EventType: string(domain.EventOpenFGARelationshipDelete),
		Payload: map[string]any{
			"object_type":  "hr.employee",
			"object_id":    "emp-1",
			"relation":     "owner",
			"subject_type": "account",
			"subject_id":   "acct-old",
		},
		Status:    "pending",
		CreatedAt: now,
	})
	writer := &recordingTupleWriter{err: errors.New("openfga unavailable")}
	processor := jobs.NewAuthzOutboxProcessor(store, writer, nil)

	if _, err := processor.ProcessTenant(ctx, "tenant-1", jobs.AuthzOutboxOptions{BatchSize: 10, MaxRetries: 2}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListAuthzOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "failed" || events[0].RetryCount != 1 || events[0].LastError == "" {
		t.Fatalf("expected failed event with retry metadata, got %+v", events[0])
	}

	writer.err = nil
	if _, err := processor.ProcessTenant(ctx, "tenant-1", jobs.AuthzOutboxOptions{BatchSize: 10, MaxRetries: 2}); err != nil {
		t.Fatal(err)
	}
	events, err = store.ListAuthzOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "succeeded" || events[0].RetryCount != 1 || events[0].LastError != "" {
		t.Fatalf("expected retried event to succeed, got %+v", events[0])
	}
	if len(writer.changes) != 2 || writer.changes[1].Operation != domain.AuthzRelationshipTupleDelete {
		t.Fatalf("expected delete tuple to be retried, got %+v", writer.changes)
	}
}

type recordingTupleWriter struct {
	err     error
	changes []domain.AuthzRelationshipTupleChange
}

func (w *recordingTupleWriter) WriteRelationshipTuples(_ context.Context, changes []domain.AuthzRelationshipTupleChange) error {
	w.changes = append(w.changes, changes...)
	return w.err
}

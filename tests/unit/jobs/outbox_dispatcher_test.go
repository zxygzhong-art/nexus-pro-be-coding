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

// TestOutboxDispatcherWritesOpenFGATupleAndMarksSucceeded 驗證 outbox dispatcher writes OpenFGA tuple and marks succeeded。
func TestOutboxDispatcherWritesOpenFGATupleAndMarksSucceeded(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventOpenFGARelationshipWrite),
		AggregateType: domain.OutboxAggregateAuthz,
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
	dispatcher := jobs.NewOutboxDispatcher(store, writer, nil)

	processed, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10})
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
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "succeeded" || events[0].LastError != "" || events[0].ProcessedAt == nil {
		t.Fatalf("expected succeeded event, got %+v", events[0])
	}
}

// TestOutboxDispatcherRetriesFailedOpenFGATuple 驗證 outbox dispatcher retries failed OpenFGA tuple。
func TestOutboxDispatcherRetriesFailedOpenFGATuple(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventOpenFGARelationshipDelete),
		AggregateType: domain.OutboxAggregateAuthz,
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
	dispatcher := jobs.NewOutboxDispatcher(store, writer, nil)

	if _, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10, MaxRetries: 2}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "failed" || events[0].RetryCount != 1 || events[0].LastError == "" {
		t.Fatalf("expected failed event with retry metadata, got %+v", events[0])
	}

	writer.err = nil
	if _, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10, MaxRetries: 2}); err != nil {
		t.Fatal(err)
	}
	events, err = store.ListOutboxEvents(ctx, "tenant-1")
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

// TestOutboxDispatcherSkipsEventsWithoutHandler 驗證沒有 handler 的領域事件不會被消費或標記失敗。
func TestOutboxDispatcherSkipsEventsWithoutHandler(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-domain-1",
		TenantID:      "tenant-1",
		EventType:     "hr.employee.created",
		AggregateType: "hr.employee",
		AggregateID:   "emp-1",
		Status:        "pending",
		CreatedAt:     now,
	})
	dispatcher := jobs.NewOutboxDispatcher(store, &recordingTupleWriter{}, nil)

	processed, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "pending" {
		t.Fatalf("expected domain event to stay pending, got %+v", events[0])
	}
}

// TestOutboxDispatcherPublishesWhenNATSEnabled 驗證 NATS 模式發布事件 and marks succeeded。
func TestOutboxDispatcherPublishesWhenNATSEnabled(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventOpenFGARelationshipWrite),
		AggregateType: domain.OutboxAggregateAuthz,
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
	publisher := &recordingEventPublisher{}
	dispatcher := jobs.NewOutboxDispatcher(store, writer, nil).WithEventPublisher(publisher)

	processed, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(writer.changes) != 0 {
		t.Fatalf("expected no direct OpenFGA writes in NATS mode, got %+v", writer.changes)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected one published event, got %+v", publisher.events)
	}
	published := publisher.events[0]
	if published.subject != "events.iam.relationship.write" || published.envelope.EventID != "outbox-1" || published.envelope.SchemaVersion != domain.DomainEventSchemaVersion {
		t.Fatalf("unexpected published event: %+v", published)
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "succeeded" || events[0].ProcessedAt == nil {
		t.Fatalf("expected published event to be marked succeeded, got %+v", events[0])
	}
}

// TestOutboxDispatcherKeepsUnmappedEventsPendingInNATSMode 驗證 NATS 模式無 subject 映射事件保持 pending。
func TestOutboxDispatcherKeepsUnmappedEventsPendingInNATSMode(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.AppendOutboxEvent(ctx, domain.OutboxEvent{
		ID:            "outbox-domain-1",
		TenantID:      "tenant-1",
		EventType:     string(domain.EventEmployeeCreated),
		AggregateType: "hr.employee",
		AggregateID:   "emp-1",
		Status:        "pending",
		CreatedAt:     now,
	})
	dispatcher := jobs.NewOutboxDispatcher(store, &recordingTupleWriter{}, nil).WithEventPublisher(&recordingEventPublisher{})

	processed, err := dispatcher.ProcessTenant(ctx, "tenant-1", jobs.OutboxDispatchOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
	events, err := store.ListOutboxEvents(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Status != "pending" {
		t.Fatalf("expected unmapped event to stay pending, got %+v", events[0])
	}
}

type recordingTupleWriter struct {
	err     error
	changes []domain.AuthzRelationshipTupleChange
}

// WriteRelationshipTuples 驗證關係 tuple。
func (w *recordingTupleWriter) WriteRelationshipTuples(_ context.Context, changes []domain.AuthzRelationshipTupleChange) error {
	w.changes = append(w.changes, changes...)
	return w.err
}

type recordingEventPublisher struct {
	err    error
	events []publishedEvent
}

type publishedEvent struct {
	subject  string
	envelope domain.DomainEventEnvelope
}

func (p *recordingEventPublisher) Publish(_ context.Context, subject string, envelope domain.DomainEventEnvelope) error {
	p.events = append(p.events, publishedEvent{subject: subject, envelope: envelope})
	return p.err
}

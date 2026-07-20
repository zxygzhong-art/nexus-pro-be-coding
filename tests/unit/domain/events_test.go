package domain_test

import (
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
)

func TestEventSubjectForType(t *testing.T) {
	tests := map[string]string{
		string(domain.EventOpenFGARelationshipWrite):  "events.iam.relationship.write",
		string(domain.EventOpenFGARelationshipDelete): "events.iam.relationship.delete",
	}
	for eventType, want := range tests {
		got, err := domain.EventSubjectForType(eventType)
		if err != nil {
			t.Fatalf("EventSubjectForType(%q) returned error: %v", eventType, err)
		}
		if got != want {
			t.Fatalf("EventSubjectForType(%q) = %q, want %q", eventType, got, want)
		}
	}
}

func TestEventSubjectForTypeRejectsUnmappedType(t *testing.T) {
	if _, err := domain.EventSubjectForType(string(domain.EventEmployeeCreated)); err == nil {
		t.Fatal("expected unmapped event type to return error")
	}
}

func TestNewDomainEventEnvelopeSetsSchemaVersion(t *testing.T) {
	createdAt := time.Date(2026, 7, 8, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	envelope := domain.NewDomainEventEnvelope(domain.OutboxEvent{
		ID:        "outbox-1",
		TenantID:  "tenant-1",
		EventType: string(domain.EventOpenFGARelationshipWrite),
		CreatedAt: createdAt,
		Payload:   map[string]any{"object_id": "emp-1"},
	})

	if envelope.SchemaVersion != domain.DomainEventSchemaVersion {
		t.Fatalf("unexpected schema version: %+v", envelope)
	}
	if envelope.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %+v", envelope)
	}
	if envelope.OccurredAt.Location() != time.UTC {
		t.Fatalf("expected occurred_at to be UTC, got %s", envelope.OccurredAt.Location())
	}
}

package domain

import (
	"fmt"
	"strings"
	"time"
)

const DomainEventSchemaVersion = 1

// DomainEventEnvelope is the canonical JetStream message body for domain events.
type DomainEventEnvelope struct {
	EventID       string         `json:"event_id"`
	EventType     string         `json:"event_type"`
	TenantID      string         `json:"tenant_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	SchemaVersion int            `json:"schema_version"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// NewDomainEventEnvelope builds the canonical event envelope from an outbox event.
func NewDomainEventEnvelope(event OutboxEvent) DomainEventEnvelope {
	return DomainEventEnvelope{
		EventID:       strings.TrimSpace(event.ID),
		EventType:     strings.TrimSpace(event.EventType),
		TenantID:      strings.TrimSpace(event.TenantID),
		OccurredAt:    event.CreatedAt.UTC(),
		SchemaVersion: DomainEventSchemaVersion,
		Payload:       event.Payload,
	}
}

// EventSubjectForType maps a domain event type to its JetStream subject.
func EventSubjectForType(eventType string) (string, error) {
	switch strings.TrimSpace(eventType) {
	case string(EventOpenFGARelationshipWrite):
		return "events.iam.relationship.write", nil
	case string(EventOpenFGARelationshipDelete):
		return "events.iam.relationship.delete", nil
	default:
		return "", fmt.Errorf("no JetStream subject mapping for event type %q", eventType)
	}
}

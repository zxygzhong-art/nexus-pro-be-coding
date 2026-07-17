package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
	"nexus-pro-be/internal/platform/natsbus"
	"nexus-pro-be/internal/repository/memory"
)

func TestOpenFGAConsumerWritesTupleAndAcks(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	writer := &recordingTupleWriter{}
	message := newOpenFGAEventMessage(t, "outbox-1", string(domain.EventOpenFGARelationshipWrite), "tenant-1", 1)
	subscriber := &fakeEventSubscriber{messages: []*fakeEventMessage{message}}
	consumer := jobs.NewOpenFGAConsumer(subscriber, writer, store, nil)

	consumer.Run(ctx, jobs.OpenFGAConsumerOptions{Stream: "NEXUS_EVENTS", ConsumerPrefix: "nexus"})

	if subscriber.opts.Durable != "nexus-openfga" || subscriber.opts.FilterSubject != "events.iam.relationship.*" || subscriber.opts.MaxDeliver != 5 {
		t.Fatalf("unexpected subscription options: %+v", subscriber.opts)
	}
	if len(writer.changes) != 1 {
		t.Fatalf("expected one tuple write, got %+v", writer.changes)
	}
	change := writer.changes[0]
	if change.Operation != domain.AuthzRelationshipTupleWrite || change.Tuple.TenantID != "tenant-1" || change.Tuple.SubjectID != "acct-1" {
		t.Fatalf("unexpected tuple change: %+v", change)
	}
	if message.acks != 1 || message.naks != 0 {
		t.Fatalf("expected ack=1 nak=0, got ack=%d nak=%d", message.acks, message.naks)
	}
}

func TestOpenFGAConsumerDedupesRepeatedEventID(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	writer := &recordingTupleWriter{}
	first := newOpenFGAEventMessage(t, "outbox-1", string(domain.EventOpenFGARelationshipWrite), "tenant-1", 1)
	second := newOpenFGAEventMessage(t, "outbox-1", string(domain.EventOpenFGARelationshipWrite), "tenant-1", 2)
	subscriber := &fakeEventSubscriber{messages: []*fakeEventMessage{first, second}}
	consumer := jobs.NewOpenFGAConsumer(subscriber, writer, store, nil)

	consumer.Run(ctx, jobs.OpenFGAConsumerOptions{})

	if len(writer.changes) != 1 {
		t.Fatalf("expected duplicate event_id to be skipped, got %+v", writer.changes)
	}
	if first.acks != 1 || second.acks != 1 || first.naks != 0 || second.naks != 0 {
		t.Fatalf("expected both duplicate deliveries to be acked, got first ack/nak=%d/%d second ack/nak=%d/%d", first.acks, first.naks, second.acks, second.naks)
	}
}

func TestOpenFGAConsumerNaksOnProcessingFailure(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	writer := &recordingTupleWriter{err: errors.New("openfga unavailable")}
	message := newOpenFGAEventMessage(t, "outbox-1", string(domain.EventOpenFGARelationshipDelete), "tenant-1", 1)
	subscriber := &fakeEventSubscriber{messages: []*fakeEventMessage{message}}
	consumer := jobs.NewOpenFGAConsumer(subscriber, writer, store, nil)

	consumer.Run(ctx, jobs.OpenFGAConsumerOptions{})

	if message.acks != 0 || message.naks != 1 {
		t.Fatalf("expected processing failure to nak, got ack=%d nak=%d", message.acks, message.naks)
	}
	if len(subscriber.errs) != 1 {
		t.Fatalf("expected handler error to be reported, got %+v", subscriber.errs)
	}
}

func TestOpenFGAConsumerRecordsDeadLetterAtMaxDeliver(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	writer := &recordingTupleWriter{err: errors.New("openfga unavailable")}
	message := newOpenFGAEventMessage(t, "outbox-1", string(domain.EventOpenFGARelationshipWrite), "tenant-1", 5)
	subscriber := &fakeEventSubscriber{messages: []*fakeEventMessage{message}}
	consumer := jobs.NewOpenFGAConsumer(subscriber, writer, store, nil)

	consumer.Run(ctx, jobs.OpenFGAConsumerOptions{MaxDeliver: 5})

	logs, err := store.ListAuditLogs(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Action != "platform.event.dead_letter" || logs[0].Target != "outbox-1" {
		t.Fatalf("expected dead letter audit, got %+v", logs)
	}
	if message.naks != 1 {
		t.Fatalf("expected failed max-deliver message to be nacked, got %d", message.naks)
	}
}

func newOpenFGAEventMessage(t *testing.T, eventID, eventType, tenantID string, deliveries uint64) *fakeEventMessage {
	t.Helper()
	envelope := domain.DomainEventEnvelope{
		EventID:       eventID,
		EventType:     eventType,
		TenantID:      tenantID,
		OccurredAt:    time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC),
		SchemaVersion: domain.DomainEventSchemaVersion,
		Payload: map[string]any{
			"object_type":  "hr.employee",
			"object_id":    "emp-1",
			"relation":     "owner",
			"subject_type": "account",
			"subject_id":   "acct-1",
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeEventMessage{
		subject: "events.iam.relationship.write",
		headers: map[string]string{
			natsbus.HeaderEventID:   eventID,
			natsbus.HeaderEventType: eventType,
			natsbus.HeaderTenantID:  tenantID,
		},
		data:     raw,
		metadata: natsbus.MessageMetadata{NumDelivered: deliveries},
	}
}

type fakeEventSubscriber struct {
	opts     natsbus.SubscriptionOptions
	messages []*fakeEventMessage
	err      error
	errs     []error
}

func (s *fakeEventSubscriber) Subscribe(ctx context.Context, opts natsbus.SubscriptionOptions, handler natsbus.MessageHandler) (natsbus.Subscription, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.opts = opts
	subscription := &fakeEventSubscription{closed: make(chan struct{})}
	for _, message := range s.messages {
		if err := handler(ctx, message); err != nil {
			s.errs = append(s.errs, err)
		}
	}
	close(subscription.closed)
	return subscription, nil
}

type fakeEventSubscription struct {
	closed chan struct{}
}

func (s *fakeEventSubscription) Stop() {}

func (s *fakeEventSubscription) Drain() {}

func (s *fakeEventSubscription) Closed() <-chan struct{} {
	return s.closed
}

type fakeEventMessage struct {
	subject  string
	headers  map[string]string
	data     []byte
	metadata natsbus.MessageMetadata
	acks     int
	naks     int
	ackErr   error
	nakErr   error
}

func (m *fakeEventMessage) Subject() string {
	return m.subject
}

func (m *fakeEventMessage) Header(key string) string {
	return m.headers[key]
}

func (m *fakeEventMessage) Data() []byte {
	return m.data
}

func (m *fakeEventMessage) Metadata() natsbus.MessageMetadata {
	return m.metadata
}

func (m *fakeEventMessage) Ack() error {
	m.acks++
	return m.ackErr
}

func (m *fakeEventMessage) Nak() error {
	m.naks++
	return m.nakErr
}


// TestOpenFGAConsumerProcessedIDsBounded 驗證去重集合達上限時整體重建，最舊事件可被重新處理。
func TestOpenFGAConsumerProcessedIDsBounded(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	writer := &recordingTupleWriter{}
	messages := make([]*fakeEventMessage, 0, 10002)
	for i := 0; i <= 10000; i++ {
		messages = append(messages, newOpenFGAEventMessage(t, fmt.Sprintf("outbox-%d", i), string(domain.EventOpenFGARelationshipWrite), "tenant-1", 1))
	}
	// After the cap resets the dedupe set, the oldest event ID is processed again.
	messages = append(messages, newOpenFGAEventMessage(t, "outbox-0", string(domain.EventOpenFGARelationshipWrite), "tenant-1", 2))
	subscriber := &fakeEventSubscriber{messages: messages}
	consumer := jobs.NewOpenFGAConsumer(subscriber, writer, store, nil)

	consumer.Run(ctx, jobs.OpenFGAConsumerOptions{})

	if len(writer.changes) != 10002 {
		t.Fatalf("expected the bounded dedupe set to reprocess the oldest event after reset, got %d writes", len(writer.changes))
	}
}

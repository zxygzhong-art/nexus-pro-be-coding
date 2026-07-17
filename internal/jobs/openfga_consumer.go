package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/natsbus"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

const (
	defaultOpenFGAConsumerMaxDeliver = 5
	defaultOpenFGAConsumerAckWait    = 30 * time.Second
	openFGAConsumerFilterSubject     = "events.iam.relationship.*"
	// openFGAConsumerMaxProcessedIDs bounds the in-process dedupe set. It is only
	// a second line of defense behind JetStream's server-side dedupe window, so
	// reaching the cap resets the set instead of growing without limit.
	openFGAConsumerMaxProcessedIDs = 10000
)

// OpenFGAConsumerOptions defines OpenFGA durable consumer options.
type OpenFGAConsumerOptions struct {
	Stream         string
	ConsumerPrefix string
	Durable        string
	MaxDeliver     int
	AckWait        time.Duration
	MaxAckPending  int
}

// OpenFGAConsumer consumes relationship tuple events from JetStream.
type OpenFGAConsumer struct {
	subscriber natsbus.EventSubscriber
	writer     RelationshipTupleWriter
	store      repository.Store
	logger     *slog.Logger
	now        func() time.Time

	mu        sync.Mutex
	processed map[string]struct{}
}

// NewOpenFGAConsumer builds the OpenFGA tuple sync consumer.
func NewOpenFGAConsumer(subscriber natsbus.EventSubscriber, writer RelationshipTupleWriter, store repository.Store, logger *slog.Logger) *OpenFGAConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &OpenFGAConsumer{
		subscriber: subscriber,
		writer:     writer,
		store:      store,
		logger:     logger,
		now:        time.Now,
		processed:  map[string]struct{}{},
	}
}

// Run starts the durable consumer and blocks until context cancellation or subscription close.
func (c *OpenFGAConsumer) Run(ctx context.Context, opts OpenFGAConsumerOptions) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeOpenFGAConsumerOptions(opts)
	if c == nil || c.subscriber == nil {
		return
	}
	subscription, err := c.subscriber.Subscribe(ctx, natsbus.SubscriptionOptions{
		Stream:        opts.Stream,
		Durable:       opts.Durable,
		FilterSubject: openFGAConsumerFilterSubject,
		MaxDeliver:    opts.MaxDeliver,
		AckWait:       opts.AckWait,
		MaxAckPending: opts.MaxAckPending,
	}, func(ctx context.Context, message natsbus.Message) error {
		return c.processMessage(ctx, message, opts)
	})
	if err != nil {
		c.logger.ErrorContext(ctx, "openfga event consumer failed to start", "error", err)
		return
	}
	c.logger.InfoContext(ctx, "openfga event consumer started", "stream", opts.Stream, "durable", opts.Durable, "filter_subject", openFGAConsumerFilterSubject)

	select {
	case <-ctx.Done():
		subscription.Drain()
		select {
		case <-subscription.Closed():
		case <-time.After(5 * time.Second):
			c.logger.WarnContext(ctx, "openfga event consumer did not drain before timeout", "durable", opts.Durable)
			subscription.Stop()
		}
	case <-subscription.Closed():
	}
}

func (c *OpenFGAConsumer) processMessage(ctx context.Context, message natsbus.Message, opts OpenFGAConsumerOptions) error {
	eventID := strings.TrimSpace(message.Header(natsbus.HeaderEventID))
	eventType := strings.TrimSpace(message.Header(natsbus.HeaderEventType))
	tenantID := strings.TrimSpace(message.Header(natsbus.HeaderTenantID))
	if eventID == "" || eventType == "" || tenantID == "" {
		return c.nakMessage(ctx, message, opts, eventID, tenantID, eventType, errors.New("nats event missing Nexus headers"))
	}
	if c.isProcessed(eventID) {
		if err := message.Ack(); err != nil {
			return err
		}
		c.logger.InfoContext(ctx, "duplicate openfga event acknowledged", "event_id", eventID, "event_type", eventType, "tenant_id", tenantID)
		return nil
	}

	envelope, err := decodeDomainEventEnvelope(message, eventID, eventType, tenantID)
	if err != nil {
		return c.nakMessage(ctx, message, opts, eventID, tenantID, eventType, err)
	}
	if !isOpenFGARelationshipEvent(envelope.EventType) {
		return c.nakMessage(ctx, message, opts, eventID, tenantID, eventType, errors.New("openfga consumer received unsupported event type "+envelope.EventType))
	}
	if c.writer == nil {
		return c.nakMessage(ctx, message, opts, eventID, tenantID, eventType, errors.New("openfga consumer requires relationship tuple writer"))
	}
	change, err := relationshipChangeFromOutboxEvent(domain.OutboxEvent{
		ID:        envelope.EventID,
		TenantID:  envelope.TenantID,
		EventType: envelope.EventType,
		Payload:   envelope.Payload,
		CreatedAt: envelope.OccurredAt,
	})
	if err != nil {
		return c.nakMessage(ctx, message, opts, eventID, tenantID, eventType, err)
	}
	if err := c.writer.WriteRelationshipTuples(ctx, []domain.AuthzRelationshipTupleChange{change}); err != nil {
		return c.nakMessage(ctx, message, opts, eventID, tenantID, eventType, err)
	}
	c.markProcessed(eventID)
	if err := message.Ack(); err != nil {
		return err
	}
	c.logger.InfoContext(ctx, "openfga event consumed", "event_id", eventID, "event_type", eventType, "tenant_id", tenantID)
	return nil
}

func decodeDomainEventEnvelope(message natsbus.Message, eventID, eventType, tenantID string) (domain.DomainEventEnvelope, error) {
	var envelope domain.DomainEventEnvelope
	if err := json.Unmarshal(message.Data(), &envelope); err != nil {
		return domain.DomainEventEnvelope{}, err
	}
	if envelope.SchemaVersion != domain.DomainEventSchemaVersion {
		return domain.DomainEventEnvelope{}, errors.New("unsupported domain event schema version")
	}
	if strings.TrimSpace(envelope.EventID) != eventID {
		return domain.DomainEventEnvelope{}, errors.New("event_id header does not match envelope")
	}
	if strings.TrimSpace(envelope.EventType) != eventType {
		return domain.DomainEventEnvelope{}, errors.New("event_type header does not match envelope")
	}
	if strings.TrimSpace(envelope.TenantID) != tenantID {
		return domain.DomainEventEnvelope{}, errors.New("tenant_id header does not match envelope")
	}
	return envelope, nil
}

func (c *OpenFGAConsumer) nakMessage(ctx context.Context, message natsbus.Message, opts OpenFGAConsumerOptions, eventID, tenantID, eventType string, cause error) error {
	meta := message.Metadata()
	if opts.MaxDeliver > 0 && meta.NumDelivered >= uint64(opts.MaxDeliver) {
		c.recordDeadLetter(ctx, message, eventID, tenantID, eventType, meta.NumDelivered, cause)
	}
	if err := message.Nak(); err != nil {
		return errors.Join(cause, err)
	}
	c.logger.WarnContext(ctx, "openfga event processing failed", "event_id", eventID, "event_type", eventType, "tenant_id", tenantID, "deliveries", meta.NumDelivered, "error", cause)
	return cause
}

func (c *OpenFGAConsumer) recordDeadLetter(ctx context.Context, message natsbus.Message, eventID, tenantID, eventType string, deliveries uint64, cause error) {
	if c == nil || c.store == nil || strings.TrimSpace(tenantID) == "" {
		return
	}
	err := c.store.AppendAuditLog(ctx, domain.AuditLog{
		ID:       utils.NewID("audit"),
		TenantID: tenantID,
		Action:   "platform.event.dead_letter",
		Resource: "event",
		Target:   eventID,
		Result:   "failed",
		Severity: "critical",
		Details: map[string]any{
			"event_type": eventType,
			"subject":    message.Subject(),
			"deliveries": deliveries,
			"error":      truncateOutboxError(cause.Error()),
		},
		CreatedAt: c.now().UTC(),
	})
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to record event dead letter audit", "event_id", eventID, "tenant_id", tenantID, "error", err)
		return
	}
	c.logger.ErrorContext(ctx, "openfga event moved to dead letter audit", "event_id", eventID, "event_type", eventType, "tenant_id", tenantID, "deliveries", deliveries, "error", cause)
}

func (c *OpenFGAConsumer) isProcessed(eventID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.processed[eventID]
	return ok
}

func (c *OpenFGAConsumer) markProcessed(eventID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.processed) >= openFGAConsumerMaxProcessedIDs {
		c.processed = make(map[string]struct{}, openFGAConsumerMaxProcessedIDs)
	}
	c.processed[eventID] = struct{}{}
}

func normalizeOpenFGAConsumerOptions(opts OpenFGAConsumerOptions) OpenFGAConsumerOptions {
	opts.Stream = strings.TrimSpace(opts.Stream)
	if opts.Stream == "" {
		opts.Stream = natsbus.DefaultStream
	}
	opts.ConsumerPrefix = strings.Trim(strings.TrimSpace(opts.ConsumerPrefix), "-")
	if opts.ConsumerPrefix == "" {
		opts.ConsumerPrefix = natsbus.DefaultConsumerPrefix
	}
	opts.Durable = strings.TrimSpace(opts.Durable)
	if opts.Durable == "" {
		opts.Durable = opts.ConsumerPrefix + "-openfga"
	}
	if opts.MaxDeliver <= 0 {
		opts.MaxDeliver = defaultOpenFGAConsumerMaxDeliver
	}
	if opts.AckWait <= 0 {
		opts.AckWait = defaultOpenFGAConsumerAckWait
	}
	if opts.MaxAckPending <= 0 {
		opts.MaxAckPending = 100
	}
	return opts
}

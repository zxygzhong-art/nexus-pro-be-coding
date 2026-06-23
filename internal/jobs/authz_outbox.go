package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
)

const (
	defaultAuthzOutboxBatchSize  = 100
	defaultAuthzOutboxMaxRetries = 5
	defaultAuthzOutboxInterval   = 30 * time.Second
	maxOutboxErrorLength         = 500
)

// RelationshipTupleWriter writes authorization tuple changes to an external relationship system.
type RelationshipTupleWriter interface {
	WriteRelationshipTuples(context.Context, []domain.AuthzRelationshipTupleChange) error
}

// AuthzOutboxOptions controls polling, retry, and batch behavior for tuple sync.
type AuthzOutboxOptions struct {
	BatchSize  int
	MaxRetries int
	Interval   time.Duration
}

// AuthzOutboxProcessor drains authorization tuple events from the repository outbox.
type AuthzOutboxProcessor struct {
	store  repository.Store
	writer RelationshipTupleWriter
	logger *slog.Logger
	now    func() time.Time
}

// NewAuthzOutboxProcessor creates a processor for syncing local authz events externally.
func NewAuthzOutboxProcessor(store repository.Store, writer RelationshipTupleWriter, logger *slog.Logger) *AuthzOutboxProcessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthzOutboxProcessor{
		store:  store,
		writer: writer,
		logger: logger,
		now:    time.Now,
	}
}

// Run processes the outbox immediately and then polls until the context is canceled.
func (p *AuthzOutboxProcessor) Run(ctx context.Context, opts AuthzOutboxOptions) {
	opts = normalizeAuthzOutboxOptions(opts)
	p.processAllTenantsAndLog(ctx, opts)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processAllTenantsAndLog(ctx, opts)
		}
	}
}

// ProcessAllTenants drains queued authorization events for every known tenant.
func (p *AuthzOutboxProcessor) ProcessAllTenants(ctx context.Context, opts AuthzOutboxOptions) (int, error) {
	opts = normalizeAuthzOutboxOptions(opts)
	if p == nil || p.store == nil {
		return 0, errors.New("authz outbox processor requires store")
	}
	tenants, err := p.store.ListTenants(ctx)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, tenant := range tenants {
		n, err := p.ProcessTenant(ctx, tenant.ID, opts)
		if err != nil {
			return processed, err
		}
		processed += n
	}
	return processed, nil
}

// ProcessTenant drains queued authorization events for a single tenant.
func (p *AuthzOutboxProcessor) ProcessTenant(ctx context.Context, tenantID string, opts AuthzOutboxOptions) (int, error) {
	opts = normalizeAuthzOutboxOptions(opts)
	if p == nil || p.store == nil {
		return 0, errors.New("authz outbox processor requires store")
	}
	if p.writer == nil {
		return 0, errors.New("authz outbox processor requires OpenFGA writer")
	}
	events, err := p.store.ListAuthzOutboxEvents(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if processed >= opts.BatchSize {
			break
		}
		if !isRetryableOpenFGAEvent(event, opts.MaxRetries) {
			continue
		}
		if err := p.processEvent(ctx, event); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (p *AuthzOutboxProcessor) processEvent(ctx context.Context, event domain.AuthzOutboxEvent) error {
	event.Status = "processing"
	event.LastError = ""
	event.ProcessedAt = nil
	if err := p.store.UpdateAuthzOutboxEvent(ctx, event); err != nil {
		return err
	}

	change, err := relationshipChangeFromOutboxEvent(event)
	if err == nil {
		err = p.writer.WriteRelationshipTuples(ctx, []domain.AuthzRelationshipTupleChange{change})
	}
	now := p.now().UTC()
	event.ProcessedAt = &now
	if err != nil {
		event.Status = "failed"
		event.RetryCount++
		event.LastError = truncateOutboxError(err.Error())
		return p.store.UpdateAuthzOutboxEvent(ctx, event)
	}
	event.Status = "succeeded"
	event.LastError = ""
	return p.store.UpdateAuthzOutboxEvent(ctx, event)
}

func (p *AuthzOutboxProcessor) processAllTenantsAndLog(ctx context.Context, opts AuthzOutboxOptions) {
	processed, err := p.ProcessAllTenants(ctx, opts)
	if err != nil {
		p.logger.WarnContext(ctx, "authz outbox processing failed", "error", err)
		return
	}
	if processed > 0 {
		p.logger.InfoContext(ctx, "authz outbox processed", "events", processed)
	}
}

func normalizeAuthzOutboxOptions(opts AuthzOutboxOptions) AuthzOutboxOptions {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultAuthzOutboxBatchSize
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = defaultAuthzOutboxMaxRetries
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultAuthzOutboxInterval
	}
	return opts
}

func isRetryableOpenFGAEvent(event domain.AuthzOutboxEvent, maxRetries int) bool {
	if !isOpenFGARelationshipEvent(event.EventType) {
		return false
	}
	if event.RetryCount >= maxRetries {
		return false
	}
	return event.Status == "pending" || event.Status == "failed" || event.Status == "processing"
}

func isOpenFGARelationshipEvent(eventType string) bool {
	return eventType == string(domain.EventOpenFGARelationshipWrite) || eventType == string(domain.EventOpenFGARelationshipDelete)
}

func relationshipChangeFromOutboxEvent(event domain.AuthzOutboxEvent) (domain.AuthzRelationshipTupleChange, error) {
	operation := domain.AuthzRelationshipTupleWrite
	if event.EventType == string(domain.EventOpenFGARelationshipDelete) {
		operation = domain.AuthzRelationshipTupleDelete
	}
	tuple := domain.AuthzRelationshipTuple{
		TenantID:    event.TenantID,
		ObjectType:  payloadString(event.Payload, "object_type"),
		ObjectID:    payloadString(event.Payload, "object_id"),
		Relation:    payloadString(event.Payload, "relation"),
		SubjectType: payloadString(event.Payload, "subject_type"),
		SubjectID:   payloadString(event.Payload, "subject_id"),
	}
	if tuple.ObjectType == "" || tuple.ObjectID == "" || tuple.Relation == "" || tuple.SubjectType == "" || tuple.SubjectID == "" {
		return domain.AuthzRelationshipTupleChange{}, errors.New("openfga outbox payload missing relationship tuple fields")
	}
	return domain.AuthzRelationshipTupleChange{Operation: operation, Tuple: tuple}, nil
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func truncateOutboxError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxOutboxErrorLength {
		return value
	}
	return value[:maxOutboxErrorLength]
}

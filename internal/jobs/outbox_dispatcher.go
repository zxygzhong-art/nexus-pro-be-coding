package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/natsbus"
	"nexus-pro-be/internal/repository"
)

const (
	defaultOutboxBatchSize  = 100
	defaultOutboxMaxRetries = 5
	defaultOutboxInterval   = 30 * time.Second
	maxOutboxErrorLength    = 500
)

// RelationshipTupleWriter 定義關係 tuple writer 的行為契約。
type RelationshipTupleWriter interface {
	WriteRelationshipTuples(context.Context, []domain.AuthzRelationshipTupleChange) error
}

// OutboxDispatchOptions 定義 outbox dispatcher 選項的資料結構。
type OutboxDispatchOptions struct {
	BatchSize  int
	MaxRetries int
	Interval   time.Duration
}

// OutboxDispatcher 消費統一 outbox_events,依 event_type 路由到對應 handler。
type OutboxDispatcher struct {
	store     repository.Store
	writer    RelationshipTupleWriter
	publisher natsbus.EventPublisher
	logger    *slog.Logger
	now       func() time.Time
}

// NewOutboxDispatcher 建立統一 outbox dispatcher。
func NewOutboxDispatcher(store repository.Store, writer RelationshipTupleWriter, logger *slog.Logger) *OutboxDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxDispatcher{
		store:  store,
		writer: writer,
		logger: logger,
		now:    time.Now,
	}
}

// WithEventPublisher enables JetStream publishing mode for dispatchable events.
func (p *OutboxDispatcher) WithEventPublisher(publisher natsbus.EventPublisher) *OutboxDispatcher {
	if p == nil {
		return p
	}
	p.publisher = publisher
	return p
}

// Run 執行背景工作主迴圈。
func (p *OutboxDispatcher) Run(ctx context.Context, opts OutboxDispatchOptions) {
	opts = normalizeOutboxDispatchOptions(opts)
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

// ProcessAllTenants 處理 all 租戶。
func (p *OutboxDispatcher) ProcessAllTenants(ctx context.Context, opts OutboxDispatchOptions) (int, error) {
	opts = normalizeOutboxDispatchOptions(opts)
	if p == nil || p.store == nil {
		return 0, errors.New("outbox dispatcher requires store")
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

// ProcessTenant 處理租戶。
func (p *OutboxDispatcher) ProcessTenant(ctx context.Context, tenantID string, opts OutboxDispatchOptions) (int, error) {
	opts = normalizeOutboxDispatchOptions(opts)
	if p == nil || p.store == nil {
		return 0, errors.New("outbox dispatcher requires store")
	}
	events, err := p.store.ListOutboxEvents(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if processed >= opts.BatchSize {
			break
		}
		if !p.isDispatchableEvent(event, opts.MaxRetries) {
			continue
		}
		if err := p.processEvent(ctx, event); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

// processEvent 處理事件。
func (p *OutboxDispatcher) processEvent(ctx context.Context, event domain.OutboxEvent) error {
	event.Status = "processing"
	event.LastError = ""
	event.ProcessedAt = nil
	if err := p.store.UpdateOutboxEvent(ctx, event); err != nil {
		return err
	}

	err := p.dispatchEvent(ctx, event)
	now := p.now().UTC()
	event.ProcessedAt = &now
	if err != nil {
		event.Status = "failed"
		event.RetryCount++
		event.LastError = truncateOutboxError(err.Error())
		return p.store.UpdateOutboxEvent(ctx, event)
	}
	event.Status = "succeeded"
	event.LastError = ""
	return p.store.UpdateOutboxEvent(ctx, event)
}

// dispatchEvent 依事件類型路由到對應 handler。
func (p *OutboxDispatcher) dispatchEvent(ctx context.Context, event domain.OutboxEvent) error {
	if p.publisher != nil {
		subject, err := domain.EventSubjectForType(event.EventType)
		if err != nil {
			return err
		}
		envelope := domain.NewDomainEventEnvelope(event)
		return p.publisher.Publish(ctx, subject, envelope)
	}
	switch {
	case isOpenFGARelationshipEvent(event.EventType):
		if p.writer == nil {
			return errors.New("outbox dispatcher requires OpenFGA writer for relationship events")
		}
		change, err := relationshipChangeFromOutboxEvent(event)
		if err != nil {
			return err
		}
		return p.writer.WriteRelationshipTuples(ctx, []domain.AuthzRelationshipTupleChange{change})
	default:
		return errors.New("no handler registered for outbox event type " + event.EventType)
	}
}

// processAllTenantsAndLog 處理 all 租戶 and log。
func (p *OutboxDispatcher) processAllTenantsAndLog(ctx context.Context, opts OutboxDispatchOptions) {
	processed, err := p.ProcessAllTenants(ctx, opts)
	if err != nil {
		p.logger.WarnContext(ctx, "outbox dispatch failed", "error", err)
		return
	}
	if processed > 0 {
		p.logger.InfoContext(ctx, "outbox events dispatched", "events", processed)
	}
}

// normalizeOutboxDispatchOptions 正規化 outbox dispatcher 選項。
func normalizeOutboxDispatchOptions(opts OutboxDispatchOptions) OutboxDispatchOptions {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultOutboxBatchSize
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = defaultOutboxMaxRetries
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultOutboxInterval
	}
	return opts
}

// isDispatchableEvent 判斷事件是否可派發。
// NATS 啟用時改由 subject mapping 決定是否可發布;無映射事件保持 pending。
func (p *OutboxDispatcher) isDispatchableEvent(event domain.OutboxEvent, maxRetries int) bool {
	if p != nil && p.publisher != nil {
		if _, err := domain.EventSubjectForType(event.EventType); err != nil {
			return false
		}
	} else if !isOpenFGARelationshipEvent(event.EventType) {
		return false
	}
	if event.RetryCount >= maxRetries {
		return false
	}
	return event.Status == "pending" || event.Status == "failed" || event.Status == "processing"
}

// isOpenFGARelationshipEvent 判斷是否為open fga 關係事件。
func isOpenFGARelationshipEvent(eventType string) bool {
	return eventType == string(domain.EventOpenFGARelationshipWrite) || eventType == string(domain.EventOpenFGARelationshipDelete)
}

// relationshipChangeFromOutboxEvent 處理關係 change 來源 outbox 事件。
func relationshipChangeFromOutboxEvent(event domain.OutboxEvent) (domain.AuthzRelationshipTupleChange, error) {
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

// payloadString 處理 payload 字串。
func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

// truncateOutboxError 截斷 outbox 錯誤。
func truncateOutboxError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxOutboxErrorLength {
		return value
	}
	return value[:maxOutboxErrorLength]
}

package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/natsbus"
	"nexus-pro-api/internal/repository"
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

// AgentModelSyncHandler 定義模型 outbox 事件的本地處理器。
type AgentModelSyncHandler interface {
	HandleAgentModelSyncEvent(context.Context, domain.OutboxEvent) error
}

// NoopRelationshipTupleWriter 在未配置 OpenFGA 時接受 Write 並直接成功。
type NoopRelationshipTupleWriter struct{}

// WriteRelationshipTuples 以 no-op 方式接受關係 tuple 變更。
func (NoopRelationshipTupleWriter) WriteRelationshipTuples(context.Context, []domain.AuthzRelationshipTupleChange) error {
	return nil
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
	modelSync AgentModelSyncHandler
	logger    *slog.Logger
	now       func() time.Time
}

// WithAgentModelSyncHandler 註冊不經 NATS 的 LiteLLM 模型同步 handler。
func (p *OutboxDispatcher) WithAgentModelSyncHandler(handler AgentModelSyncHandler) *OutboxDispatcher {
	if p == nil {
		return p
	}
	p.modelSync = handler
	return p
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
	var errs []error
	for _, tenant := range tenants {
		n, err := p.ProcessTenant(ctx, tenant.ID, opts)
		if err != nil {
			// 單一租戶失敗不阻塞其他租戶,迴圈結束後聚合回傳錯誤。
			p.logger.WarnContext(ctx, "outbox dispatch failed for tenant", "tenant_id", tenant.ID, "error", err)
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenant.ID, err))
			continue
		}
		processed += n
	}
	return processed, errors.Join(errs...)
}

// ProcessTenant 處理租戶。
func (p *OutboxDispatcher) ProcessTenant(ctx context.Context, tenantID string, opts OutboxDispatchOptions) (int, error) {
	opts = normalizeOutboxDispatchOptions(opts)
	if p == nil || p.store == nil {
		return 0, errors.New("outbox dispatcher requires store")
	}
	events, err := p.store.ClaimOutboxEvents(ctx, tenantID, opts.BatchSize, opts.MaxRetries)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if !p.isDispatchableEventType(event) {
			// Park unsupported types explicitly so operators can distinguish them
			// from events currently owned by a worker.
			// Reset to pending when a handler/publisher is later registered.
			event.Status = "parked"
			event.LastError = truncateOutboxError("no handler registered for outbox event type " + event.EventType)
			event.ProcessedAt = nil
			if err := p.store.UpdateOutboxEvent(ctx, event); err != nil {
				return processed, err
			}
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
	err := p.dispatchEvent(ctx, event)
	now := p.now().UTC()
	event.ProcessedAt = &now
	if errors.Is(err, ErrLiteLLMModelSyncNotConfigured) {
		event.Status = "pending"
		event.ProcessedAt = nil
		event.LastError = truncateOutboxError(err.Error())
		return p.store.UpdateOutboxEvent(ctx, event)
	}
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
	if isAgentModelSyncEvent(event.EventType) {
		if p.modelSync == nil {
			return errors.New("outbox dispatcher requires agent model sync handler")
		}
		return p.modelSync.HandleAgentModelSyncEvent(ctx, event)
	}
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

// isDispatchableEventType 判斷事件類型是否有可用 handler（狀態/重試由 Claim 過濾）。
// NATS 啟用時改由 subject mapping 決定是否可發布;無映射事件保持不可派發。
func (p *OutboxDispatcher) isDispatchableEventType(event domain.OutboxEvent) bool {
	if isAgentModelSyncEvent(event.EventType) {
		return p != nil && p.modelSync != nil
	}
	if p != nil && p.publisher != nil {
		_, err := domain.EventSubjectForType(event.EventType)
		return err == nil
	}
	return isOpenFGARelationshipEvent(event.EventType)
}

// isAgentModelSyncEvent 判斷是否為 LiteLLM 模型同步事件。
func isAgentModelSyncEvent(eventType string) bool {
	return eventType == string(domain.EventAgentModelUpsert) || eventType == string(domain.EventAgentModelDelete)
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
	payload, err := domain.DecodeOpenFGARelationshipPayload(event.Payload)
	if err != nil {
		return domain.AuthzRelationshipTupleChange{}, err
	}
	tuple := domain.AuthzRelationshipTuple{
		TenantID:    event.TenantID,
		ObjectType:  strings.TrimSpace(payload.ObjectType),
		ObjectID:    strings.TrimSpace(payload.ObjectID),
		Relation:    strings.TrimSpace(payload.Relation),
		SubjectType: strings.TrimSpace(payload.SubjectType),
		SubjectID:   strings.TrimSpace(payload.SubjectID),
	}
	if tuple.ObjectType == "" || tuple.ObjectID == "" || tuple.Relation == "" || tuple.SubjectType == "" || tuple.SubjectID == "" {
		return domain.AuthzRelationshipTupleChange{}, errors.New("openfga outbox payload missing relationship tuple fields")
	}
	return domain.AuthzRelationshipTupleChange{Operation: operation, Tuple: tuple}, nil
}

// truncateOutboxError 截斷 outbox 錯誤。
func truncateOutboxError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxOutboxErrorLength {
		return value
	}
	return value[:maxOutboxErrorLength]
}

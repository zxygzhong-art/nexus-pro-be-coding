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
	"nexus-pro-api/internal/utils"
)

const (
	defaultOutboxBatchSize    = 100
	defaultOutboxMaxRetries   = 5
	defaultOutboxInterval     = 30 * time.Second
	defaultOutboxClaimLease   = 5 * time.Minute
	initialOutboxRetryBackoff = 30 * time.Second
	maximumOutboxRetryBackoff = 10 * time.Minute
	maxOutboxErrorLength      = 500
	workflowStartOutboxEvent  = "workflow.form_approval.start_requested"
)

// ErrOutboxClaimLost indicates that another worker reclaimed an expired lease
// before this worker persisted its result.
var ErrOutboxClaimLost = errors.New("outbox claim is no longer owned by this worker")

// RelationshipTupleWriter 定義關係 tuple writer 的行為契約。
type RelationshipTupleWriter interface {
	WriteRelationshipTuples(context.Context, []domain.AuthzRelationshipTupleChange) error
}

// AgentModelSyncHandler 定義模型 outbox 事件的本地處理器。
type AgentModelSyncHandler interface {
	HandleAgentModelSyncEvent(context.Context, domain.OutboxEvent) error
}

// WorkflowStartHandler consumes workflow start requests locally before the
// generic NATS publisher. This prevents infrastructure commands from being
// published without a registered local convergence path.
type WorkflowStartHandler interface {
	HandleWorkflowStartEvent(context.Context, domain.OutboxEvent) error
}

type permanentOutboxError struct {
	err error
}

func (e permanentOutboxError) Error() string { return e.err.Error() }
func (e permanentOutboxError) Unwrap() error { return e.err }

// MarkOutboxErrorPermanent marks invalid payload/configuration errors that
// require operator correction instead of automatic retries.
func MarkOutboxErrorPermanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentOutboxError{err: err}
}

// NoopRelationshipTupleWriter 在未配置 OpenFGA 時接受 Write 並直接成功。
type NoopRelationshipTupleWriter struct{}

// WriteRelationshipTuples 以 no-op 方式接受關係 tuple 變更。
func (NoopRelationshipTupleWriter) WriteRelationshipTuples(context.Context, []domain.AuthzRelationshipTupleChange) error {
	return nil
}

// OutboxDispatchOptions 定義 outbox dispatcher 選項的資料結構。
type OutboxDispatchOptions struct {
	BatchSize int
	// MaxRetries is retained for source compatibility. Retry limits are now
	// stored per event in max_attempts.
	MaxRetries    int
	Interval      time.Duration
	LeaseDuration time.Duration
}

// OutboxDispatcher 消費統一 outbox_events,依 event_type 路由到對應 handler。
type OutboxDispatcher struct {
	store         repository.Store
	writer        RelationshipTupleWriter
	publisher     natsbus.EventPublisher
	modelSync     AgentModelSyncHandler
	workflowStart WorkflowStartHandler
	logger        *slog.Logger
	now           func() time.Time
	wake          <-chan struct{}
	workerID      string
}

// WithAgentModelSyncHandler 註冊不經 NATS 的 LiteLLM 模型同步 handler。
func (p *OutboxDispatcher) WithAgentModelSyncHandler(handler AgentModelSyncHandler) *OutboxDispatcher {
	if p == nil {
		return p
	}
	p.modelSync = handler
	return p
}

// WithWorkflowStartHandler registers the local workflow-start convergence path.
func (p *OutboxDispatcher) WithWorkflowStartHandler(handler WorkflowStartHandler) *OutboxDispatcher {
	if p == nil {
		return p
	}
	p.workflowStart = handler
	return p
}

// WithWakeChannel lets committed producers wake the dispatcher without waiting
// for the periodic crash-recovery ticker.
func (p *OutboxDispatcher) WithWakeChannel(wake <-chan struct{}) *OutboxDispatcher {
	if p == nil {
		return p
	}
	p.wake = wake
	return p
}

// WithClock installs a deterministic clock for tests and controlled jobs.
func (p *OutboxDispatcher) WithClock(now func() time.Time) *OutboxDispatcher {
	if p == nil || now == nil {
		return p
	}
	p.now = now
	return p
}

// NewOutboxDispatcher 建立統一 outbox dispatcher。
func NewOutboxDispatcher(store repository.Store, writer RelationshipTupleWriter, logger *slog.Logger) *OutboxDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxDispatcher{
		store:    store,
		writer:   writer,
		logger:   logger,
		now:      time.Now,
		workerID: utils.NewID("outbox-worker"),
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
		case _, ok := <-p.wake:
			if !ok {
				p.wake = nil
				continue
			}
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
	claimedAt := p.now().UTC()
	events, err := p.store.ClaimOutboxEvents(
		ctx,
		tenantID,
		opts.BatchSize,
		claimedAt,
		claimedAt.Add(opts.LeaseDuration),
		p.workerID,
		utils.NewID("outbox-claim"),
	)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if !p.isDispatchableEventType(event) {
			// Park unsupported types explicitly so operators can distinguish them
			// from events currently owned by a worker.
			// Reset to pending when a handler/publisher is later registered.
			now := p.now().UTC()
			event.Status = domain.OutboxStatusParked
			event.LastError = truncateOutboxError("no handler registered for outbox event type " + event.EventType)
			event.UpdatedAt = now
			event.ProcessedAt = nil
			event.DeadLetteredAt = nil
			if err := p.finalizeClaim(ctx, event); err != nil {
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
	event.UpdatedAt = now
	if errors.Is(err, ErrLiteLLMModelSyncNotConfigured) {
		// Missing optional runtime configuration should not exhaust a durable
		// command. Release the lease and try later with backoff.
		event.Status = domain.OutboxStatusPending
		if event.AttemptCount > 0 {
			event.AttemptCount--
		}
		event.NextAttemptAt = now.Add(initialOutboxRetryBackoff)
		event.ProcessedAt = nil
		event.DeadLetteredAt = nil
		event.LastError = truncateOutboxError(err.Error())
		return p.finalizeClaim(ctx, event)
	}
	if err != nil {
		var permanent permanentOutboxError
		if errors.As(err, &permanent) {
			event.Status = domain.OutboxStatusParked
			event.ProcessedAt = nil
			event.DeadLetteredAt = nil
			event.LastError = truncateOutboxError(err.Error())
			return p.finalizeClaim(ctx, event)
		}
		event.RetryCount++
		event.LastError = truncateOutboxError(err.Error())
		maxAttempts := domain.DefaultOutboxMaxAttempts
		if event.MaxAttempts != nil {
			maxAttempts = *event.MaxAttempts
		}
		if maxAttempts > 0 && event.AttemptCount >= maxAttempts {
			event.Status = domain.OutboxStatusDeadLettered
			event.ProcessedAt = &now
			event.DeadLetteredAt = &now
			return p.finalizeClaim(ctx, event)
		}
		event.Status = domain.OutboxStatusFailed
		event.NextAttemptAt = now.Add(outboxRetryBackoff(event.AttemptCount))
		event.ProcessedAt = nil
		event.DeadLetteredAt = nil
		return p.finalizeClaim(ctx, event)
	}
	event.Status = domain.OutboxStatusSucceeded
	event.LastError = ""
	event.ProcessedAt = &now
	event.DeadLetteredAt = nil
	return p.finalizeClaim(ctx, event)
}

func (p *OutboxDispatcher) finalizeClaim(ctx context.Context, event domain.OutboxEvent) error {
	updated, err := p.store.FinalizeOutboxEvent(ctx, event)
	if err != nil {
		return err
	}
	if !updated {
		return fmt.Errorf("%w: event %s", ErrOutboxClaimLost, event.ID)
	}
	return nil
}

// dispatchEvent 依事件類型路由到對應 handler。
func (p *OutboxDispatcher) dispatchEvent(ctx context.Context, event domain.OutboxEvent) error {
	if event.EventType == workflowStartOutboxEvent {
		if p.workflowStart == nil {
			return errors.New("outbox dispatcher requires workflow start handler")
		}
		err := p.workflowStart.HandleWorkflowStartEvent(ctx, event)
		if appErr, ok := domain.AsAppError(err); ok && appErr.Status >= 400 && appErr.Status < 500 {
			return MarkOutboxErrorPermanent(err)
		}
		return err
	}
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
	if opts.LeaseDuration <= 0 {
		opts.LeaseDuration = defaultOutboxClaimLease
	}
	return opts
}

// isDispatchableEventType 判斷事件類型是否有可用 handler（狀態/重試由 Claim 過濾）。
// NATS 啟用時改由 subject mapping 決定是否可發布;無映射事件保持不可派發。
func (p *OutboxDispatcher) isDispatchableEventType(event domain.OutboxEvent) bool {
	if event.EventType == workflowStartOutboxEvent {
		return p != nil && p.workflowStart != nil
	}
	if isAgentModelSyncEvent(event.EventType) {
		return p != nil && p.modelSync != nil
	}
	if p != nil && p.publisher != nil {
		_, err := domain.EventSubjectForType(event.EventType)
		return err == nil
	}
	return isOpenFGARelationshipEvent(event.EventType)
}

func outboxRetryBackoff(attemptCount int) time.Duration {
	delay := initialOutboxRetryBackoff
	for attempt := 1; attempt < attemptCount && delay < maximumOutboxRetryBackoff; attempt++ {
		delay *= 2
		if delay >= maximumOutboxRetryBackoff {
			return maximumOutboxRetryBackoff
		}
	}
	return delay
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
		return domain.AuthzRelationshipTupleChange{}, MarkOutboxErrorPermanent(err)
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
		return domain.AuthzRelationshipTupleChange{}, MarkOutboxErrorPermanent(errors.New("openfga outbox payload missing relationship tuple fields"))
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

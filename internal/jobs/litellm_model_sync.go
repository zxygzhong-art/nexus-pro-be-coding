package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
)

const defaultLiteLLMModelReconcileInterval = 5 * time.Minute

// ErrLiteLLMModelSyncNotConfigured 讓 outbox 保留 pending，待下次以完整設定啟動後重試。
var ErrLiteLLMModelSyncNotConfigured = errors.New("LiteLLM model syncer is not configured")

// LiteLLMModelAdmin 定義背景同步需要的 LiteLLM 管理操作。
type LiteLLMModelAdmin interface {
	SyncModel(context.Context, domain.AgentModel) (string, error)
	DeleteModel(context.Context, string) (string, error)
	ListManagedModelIDs(context.Context) ([]string, error)
}

// LiteLLMModelSyncOptions 定義完整模型對帳的執行間隔。
type LiteLLMModelSyncOptions struct {
	Interval time.Duration
}

// LiteLLMModelSyncer 消費模型 outbox 並定期以本地資料修復 LiteLLM registry。
type LiteLLMModelSyncer struct {
	store            repository.Store
	client           LiteLLMModelAdmin
	credentialCipher interface {
		Decrypt(ciphertext string, associatedData []byte) ([]byte, error)
	}
	logger *slog.Logger
	now    func() time.Time
}

// WithCredentialCipher enables just-in-time decryption of persisted model credentials.
func (s *LiteLLMModelSyncer) WithCredentialCipher(cipher interface {
	Decrypt(ciphertext string, associatedData []byte) ([]byte, error)
}) *LiteLLMModelSyncer {
	s.credentialCipher = cipher
	return s
}

// NewLiteLLMModelSyncer 建立 LiteLLM 模型同步器。
func NewLiteLLMModelSyncer(store repository.Store, client LiteLLMModelAdmin, logger *slog.Logger) *LiteLLMModelSyncer {
	if logger == nil {
		logger = slog.Default()
	}
	return &LiteLLMModelSyncer{store: store, client: client, logger: logger, now: time.Now}
}

// Configured 回報同步器是否具備可用的 store 與 LiteLLM client。
func (s *LiteLLMModelSyncer) Configured() bool {
	return s != nil && s.store != nil && s.client != nil
}

// HandleAgentModelSyncEvent 執行單筆模型 upsert 或 delete outbox 事件。
func (s *LiteLLMModelSyncer) HandleAgentModelSyncEvent(ctx context.Context, event domain.OutboxEvent) error {
	if !s.Configured() {
		return ErrLiteLLMModelSyncNotConfigured
	}
	modelID := event.AggregateID
	if modelID == "" {
		payload, err := domain.DecodeAgentModelSyncPayload(event.Payload)
		if err != nil {
			return err
		}
		modelID = payload.ModelID
	}
	if modelID == "" {
		return errors.New("agent model outbox event is missing model id")
	}
	switch event.EventType {
	case string(domain.EventAgentModelDelete):
		_, err := s.client.DeleteModel(ctx, modelID)
		return err
	case string(domain.EventAgentModelUpsert):
		model, ok, err := s.store.GetAgentModel(ctx, event.TenantID, modelID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return s.reconcileModel(ctx, model)
	default:
		return fmt.Errorf("unsupported agent model event type %s", event.EventType)
	}
}

// ReconcileAll 逐租戶以本地模型真源修復 LiteLLM registry。
func (s *LiteLLMModelSyncer) ReconcileAll(ctx context.Context) (int, error) {
	if !s.Configured() {
		return 0, ErrLiteLLMModelSyncNotConfigured
	}
	tenants, err := s.store.ListTenants(ctx)
	if err != nil {
		return 0, err
	}
	processed := 0
	localIDs := make(map[string]struct{})
	var reconcileErr error
	for _, tenant := range tenants {
		models, err := s.store.ListAgentModels(ctx, tenant.ID)
		if err != nil {
			reconcileErr = errors.Join(reconcileErr, err)
			continue
		}
		for _, model := range models {
			localIDs[model.ID] = struct{}{}
			processed++
			if err := s.reconcileModel(ctx, model); err != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("model %s: %w", model.ID, err))
			}
		}
	}
	remoteIDs, err := s.client.ListManagedModelIDs(ctx)
	if err != nil {
		reconcileErr = errors.Join(reconcileErr, err)
	} else {
		for _, remoteID := range remoteIDs {
			if _, exists := localIDs[remoteID]; exists {
				continue
			}
			processed++
			if _, err := s.client.DeleteModel(ctx, remoteID); err != nil {
				reconcileErr = errors.Join(reconcileErr, fmt.Errorf("orphan model %s: %w", remoteID, err))
			}
		}
	}
	return processed, reconcileErr
}

// Run 啟動立即一次、其後按固定間隔執行的完整模型對帳。
func (s *LiteLLMModelSyncer) Run(ctx context.Context, opts LiteLLMModelSyncOptions) {
	if !s.Configured() {
		return
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultLiteLLMModelReconcileInterval
	}
	s.reconcileAllAndLog(ctx)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileAllAndLog(ctx)
		}
	}
}

// reconcileModel 依模型啟停狀態建立、更新或移除遠端路由並寫回結果。
func (s *LiteLLMModelSyncer) reconcileModel(ctx context.Context, model domain.AgentModel) error {
	if strings.TrimSpace(model.APIKeyCiphertext) != "" {
		if s.credentialCipher == nil {
			return errors.New("agent model credential cipher is not configured")
		}
		plaintext, err := s.credentialCipher.Decrypt(model.APIKeyCiphertext, domain.AgentModelCredentialAAD(model.TenantID, model.ID))
		if err != nil {
			return fmt.Errorf("decrypt agent model credential: %w", err)
		}
		model.APIKey = string(plaintext)
	}
	if model.Status != domain.AgentModelStatusDisabled && strings.TrimSpace(model.APIKey) == "" {
		return errors.New("agent model credential is not configured; refusing to overwrite the existing LiteLLM route")
	}
	var syncErr error
	if model.Status == domain.AgentModelStatusDisabled {
		_, syncErr = s.client.DeleteModel(ctx, model.ID)
	} else {
		_, syncErr = s.client.SyncModel(ctx, model)
	}
	now := s.now().UTC()
	if syncErr != nil {
		_, _, updateErr := s.store.UpdateAgentModelSyncResult(ctx, model.TenantID, model.ID, domain.AgentModelSyncStatusFailed, syncErr.Error(), model.SyncedConfigHash, model.LastSyncedAt, now)
		return errors.Join(syncErr, updateErr)
	}
	_, ok, err := s.store.UpdateAgentModelSyncResult(ctx, model.TenantID, model.ID, domain.AgentModelSyncStatusSynced, "", domain.AgentModelSyncConfigHash(model), &now, now)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("agent model %s disappeared while updating sync result", model.ID)
	}
	return nil
}

// reconcileAllAndLog 執行完整對帳並輸出摘要。
func (s *LiteLLMModelSyncer) reconcileAllAndLog(ctx context.Context) {
	processed, err := s.ReconcileAll(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "LiteLLM model reconciliation failed", "models", processed, "error", err)
		return
	}
	s.logger.InfoContext(ctx, "LiteLLM model reconciliation completed", "models", processed)
}

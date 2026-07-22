package v1_test

import (
	"context"
	"sync"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
)

// apiAgentConfirmationTestStore adds the v2 confirmation contract to the legacy API memory fixture.
type apiAgentConfirmationTestStore struct {
	repository.Store
	state *apiAgentConfirmationTestState
}

type apiAgentConfirmationTestState struct {
	mu      sync.Mutex
	records map[string]domain.AgentConfirmationRecord
}

func newAPIAgentConfirmationTestStore(store repository.Store) *apiAgentConfirmationTestStore {
	return &apiAgentConfirmationTestStore{
		Store: store,
		state: &apiAgentConfirmationTestState{records: map[string]domain.AgentConfirmationRecord{}},
	}
}

func (s *apiAgentConfirmationTestStore) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	return repository.WithinTenantTransaction(ctx, s.Store, tenantID, func(tx repository.Store) error {
		return fn(&apiAgentConfirmationTestStore{Store: tx, state: s.state})
	})
}

func (s *apiAgentConfirmationTestStore) UpsertAgentConfirmation(_ context.Context, record domain.AgentConfirmationRecord) error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	s.state.records[apiAgentConfirmationKey(record.TenantID, record.ID)] = record
	return nil
}

func (s *apiAgentConfirmationTestStore) ListPendingAgentConfirmations(_ context.Context, tenantID, accountID, conversationID, segmentID string, now time.Time) ([]domain.AgentConfirmationRecord, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	items := make([]domain.AgentConfirmationRecord, 0)
	for _, record := range s.state.records {
		if record.TenantID == tenantID && record.AccountID == accountID && record.ConversationID == conversationID && record.SegmentID == segmentID && record.Status == domain.AgentConfirmationStatusPending && record.ExpiresAt.After(now) {
			items = append(items, record)
		}
	}
	return items, nil
}

func (s *apiAgentConfirmationTestStore) ClaimAgentConfirmation(ctx context.Context, tenantID, accountID, id string, now time.Time) (domain.AgentConfirmationRecord, bool, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	key := apiAgentConfirmationKey(tenantID, id)
	record, ok := s.state.records[key]
	if !ok || record.AccountID != accountID || record.Status != domain.AgentConfirmationStatusPending {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	session, ok, err := s.Store.GetAgentSession(ctx, tenantID, record.ConversationID)
	if err != nil {
		return domain.AgentConfirmationRecord{}, false, err
	}
	if !ok || session.SegmentID != record.SegmentID {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	record.UpdatedAt = now
	if record.ExpiresAt.After(now) {
		record.Status = domain.AgentConfirmationStatusExecuting
	} else {
		record.Status = domain.AgentConfirmationStatusExpired
		record.ConsumedAt = &now
	}
	s.state.records[key] = record
	return record, true, nil
}

func (s *apiAgentConfirmationTestStore) UpdateAgentConfirmation(_ context.Context, record domain.AgentConfirmationRecord) (domain.AgentConfirmationRecord, bool, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	key := apiAgentConfirmationKey(record.TenantID, record.ID)
	current, ok := s.state.records[key]
	if !ok || current.Status != domain.AgentConfirmationStatusExecuting || !current.Status.CanTransitionTo(record.Status) {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	s.state.records[key] = record
	return record, true, nil
}

func apiAgentConfirmationKey(tenantID, id string) string {
	return tenantID + "\x00" + id
}

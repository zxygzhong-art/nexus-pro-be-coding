package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

type confirmedExternalToolStore struct {
	*agentConfirmationTestStore
	mu           sync.Mutex
	connections  map[string]domain.AgentExternalTool
	capabilities map[string]domain.ExternalToolCapability
	steps        map[string]domain.ExecutionStep
	stepOrder    []string
	stepHistory  []domain.ExecutionStepStatus
	nextSequence map[string]int
}

func newConfirmedExternalToolStore(base *memory.Store) *confirmedExternalToolStore {
	return &confirmedExternalToolStore{
		agentConfirmationTestStore: newAgentConfirmationTestStore(base),
		connections:                map[string]domain.AgentExternalTool{},
		capabilities:               map[string]domain.ExternalToolCapability{},
		steps:                      map[string]domain.ExecutionStep{},
		nextSequence:               map[string]int{},
	}
}

func (s *confirmedExternalToolStore) GetAgentExternalTool(_ context.Context, tenantID, id string) (domain.AgentExternalTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.connections[tenantID+"\x00"+id]
	return item, ok, nil
}

func (s *confirmedExternalToolStore) GetAgentExternalToolCapability(_ context.Context, tenantID, id string) (domain.ExternalToolCapability, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.capabilities[tenantID+"\x00"+id]
	return item, ok, nil
}

func (s *confirmedExternalToolStore) AppendExecutionStep(_ context.Context, step domain.ExecutionStep) (domain.ExecutionStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSequence[step.ExecutionID]++
	step.SequenceNo = s.nextSequence[step.ExecutionID]
	s.steps[step.ID] = step
	s.stepOrder = append(s.stepOrder, step.ID)
	s.stepHistory = append(s.stepHistory, step.Status)
	return step, nil
}

func (s *confirmedExternalToolStore) UpsertExecutionStep(_ context.Context, step domain.ExecutionStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.steps[step.ID]; !ok {
		return errors.New("execution step was not appended")
	}
	s.steps[step.ID] = step
	s.stepHistory = append(s.stepHistory, step.Status)
	return nil
}

func (s *confirmedExternalToolStore) putTarget(connection domain.AgentExternalTool, capability domain.ExternalToolCapability) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[connection.TenantID+"\x00"+connection.ID] = connection
	s.capabilities[capability.TenantID+"\x00"+capability.ID] = capability
}

func (s *confirmedExternalToolStore) updateCapability(capability domain.ExternalToolCapability) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capabilities[capability.TenantID+"\x00"+capability.ID] = capability
}

func (s *confirmedExternalToolStore) updateConnection(connection domain.AgentExternalTool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[connection.TenantID+"\x00"+connection.ID] = connection
}

func (s *confirmedExternalToolStore) storedSteps() ([]domain.ExecutionStep, []domain.ExecutionStepStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	steps := make([]domain.ExecutionStep, 0, len(s.stepOrder))
	for _, id := range s.stepOrder {
		steps = append(steps, s.steps[id])
	}
	return steps, append([]domain.ExecutionStepStatus(nil), s.stepHistory...)
}

type confirmedExternalToolHTTPExecutor struct {
	mu       sync.Mutex
	calls    int
	errors   []error
	requests []externaltool.Request
	result   externaltool.Result
}

func (e *confirmedExternalToolHTTPExecutor) Call(_ context.Context, request externaltool.Request) (externaltool.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	e.requests = append(e.requests, request)
	if len(e.errors) > 0 {
		err := e.errors[0]
		e.errors = e.errors[1:]
		if err != nil {
			return externaltool.Result{}, err
		}
	}
	return e.result, nil
}

func (*confirmedExternalToolHTTPExecutor) Probe(context.Context, externaltool.ProbeRequest) (externaltool.ProbeResult, error) {
	return externaltool.ProbeResult{}, nil
}

func (e *confirmedExternalToolHTTPExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func (e *confirmedExternalToolHTTPExecutor) lastRequest() (externaltool.Request, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.requests) == 0 {
		return externaltool.Request{}, false
	}
	return e.requests[len(e.requests)-1], true
}

type confirmedExternalToolTemporaryError struct{}

func (confirmedExternalToolTemporaryError) Error() string   { return "temporary network failure" }
func (confirmedExternalToolTemporaryError) Temporary() bool { return true }

type confirmedExternalToolFixture struct {
	now        time.Time
	base       *memory.Store
	store      *confirmedExternalToolStore
	executor   *confirmedExternalToolHTTPExecutor
	service    *service.Service
	ctx        domain.RequestContext
	connection domain.AgentExternalTool
	capability domain.ExternalToolCapability
}

func newConfirmedExternalToolFixture(t *testing.T) confirmedExternalToolFixture {
	t.Helper()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	base := memory.NewStore()
	capabilityID := "ext-write-ticket"
	seedAgentConfirmationAccount(t, base, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission(capabilityID),
	})
	session := domain.AgentSession{
		ID: "session-external-confirmation", TenantID: "tenant-1", AccountID: "acct-employee",
		SegmentID: "segment-external-confirmation", Status: domain.AgentSessionStatusActive, ContextVersion: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := base.UpsertAgentSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	store := newConfirmedExternalToolStore(base)
	connection := domain.AgentExternalTool{
		ID: "connection-support", TenantID: "tenant-1", Name: "Support API",
		Kind: string(domain.ExternalToolConnectionKindHTTP), Transport: string(domain.ExternalToolTransportHTTP),
		EndpointURL: "https://support.example.test/v1", AuthType: string(domain.ExternalToolAuthTypeNone),
		TimeoutSeconds: 10, Status: string(domain.ExternalToolConnectionStatusActive), CreatedAt: now, UpdatedAt: now,
	}
	capability := domain.ExternalToolCapability{
		ID: capabilityID, TenantID: "tenant-1", ConnectionID: connection.ID,
		ToolName: "close_ticket", HTTPMethod: http.MethodPost, HTTPPath: "tickets/close",
		InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"},
		Readonly: false, Enabled: true, SchemaChecksum: "sha256:write-v1", DiscoveredAt: now, UpdatedAt: now,
	}
	store.putTarget(connection, capability)
	executor := &confirmedExternalToolHTTPExecutor{result: externaltool.Result{
		StatusCode: http.StatusOK, ContentType: "application/json", JSON: map[string]any{"closed": true},
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, ExternalHTTPExecutor: executor})
	requestContext := context.WithValue(context.Background(), struct{}{}, "unrelated")
	requestContext = service.WithAgentChatExecutionContext(requestContext, service.AgentChatExecutionContext{
		SessionID: session.ID, SegmentID: session.SegmentID, RunID: "run-external-confirmation",
	})
	return confirmedExternalToolFixture{
		now: now, base: base, store: store, executor: executor, service: svc,
		ctx:        domain.RequestContext{Context: requestContext, TenantID: "tenant-1", AccountID: "acct-employee"},
		connection: connection, capability: capability,
	}
}

func (f confirmedExternalToolFixture) createConfirmation(t *testing.T) *domain.AgentConfirmation {
	t.Helper()
	confirmation, err := f.service.CreateExternalToolConfirmation(f.ctx, domain.CreateExternalToolConfirmationInput{
		ConnectionID: f.connection.ID, CapabilityID: f.capability.ID, SchemaChecksum: f.capability.SchemaChecksum,
		Arguments: map[string]any{"ticket_id": "T-42", "secret_note": "super-secret-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return confirmation
}

func TestExternalToolMutationWaitsForConfirmationAndExecutesExactlyOnce(t *testing.T) {
	fixture := newConfirmedExternalToolFixture(t)
	confirmation := fixture.createConfirmation(t)
	if got := fixture.executor.callCount(); got != 0 {
		t.Fatalf("confirmation preview made %d remote calls", got)
	}
	publicJSON, err := json.Marshal(confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), "super-secret-value") || !strings.Contains(string(publicJSON), "secret_note") {
		t.Fatalf("public confirmation must show field names but not values: %s", publicJSON)
	}
	record, ok := fixture.store.confirmation("tenant-1", confirmation.ID)
	if !ok {
		t.Fatal("confirmation was not persisted")
	}
	keys := make([]string, 0, len(record.ActionPayload))
	for key := range record.ActionPayload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if want := []string{"arguments", "capability_id", "connection_id", "schema_checksum"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("unexpected protected action keys: got %v want %v", keys, want)
	}

	executed, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.executor.callCount() != 1 || executed.Status != "completed" || executed.Data["status_code"] != http.StatusOK {
		t.Fatalf("unexpected execution: calls=%d result=%+v", fixture.executor.callCount(), executed)
	}
	if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected one-time confirmation replay to fail")
	}
	if fixture.executor.callCount() != 1 {
		t.Fatalf("replay made another remote call: %d", fixture.executor.callCount())
	}
	steps, history := fixture.store.storedSteps()
	if len(steps) != 1 || steps[0].Status != domain.ExecutionStepStatusCompleted || steps[0].ExternalToolID != fixture.capability.ID {
		t.Fatalf("unexpected execution steps: %+v", steps)
	}
	if want := []domain.ExecutionStepStatus{domain.ExecutionStepStatusRunning, domain.ExecutionStepStatusCompleted}; !reflect.DeepEqual(history, want) {
		t.Fatalf("unexpected step transitions: got %v want %v", history, want)
	}
	stepJSON, _ := json.Marshal(steps[0])
	if strings.Contains(string(stepJSON), "super-secret-value") {
		t.Fatalf("execution audit leaked argument value: %s", stepJSON)
	}
	logs, err := fixture.base.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	foundAudit := false
	for _, log := range logs {
		if log.Action == "agent.confirmation.execute" && log.Target == confirmation.ID {
			foundAudit = log.Details["capability_id"] == fixture.capability.ID && log.Details["connection_id"] == fixture.connection.ID
		}
	}
	if !foundAudit {
		t.Fatalf("successful external confirmation audit was not recorded: %+v", logs)
	}
}

func TestExternalToolConfirmationUsesCurrentCredentialAndTimeout(t *testing.T) {
	fixture := newConfirmedExternalToolFixture(t)
	cipher := newTestCredentialCipher(t)
	fixture.connection.AuthType = string(domain.ExternalToolAuthTypeBearer)
	fixture.connection.CredentialSecretID = "credential-support"
	fixture.connection.TimeoutSeconds = 17
	ciphertext, err := cipher.Encrypt(
		[]byte("current-support-token"),
		domain.CredentialSecretAAD("tenant-1", fixture.connection.CredentialSecretID),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.connection.AuthSecretCiphertext = ciphertext
	fixture.store.updateConnection(fixture.connection)
	fixture.service = service.New(fixture.store, service.Options{
		Now: func() time.Time { return fixture.now }, CredentialCipher: cipher, ExternalHTTPExecutor: fixture.executor,
	})
	confirmation := fixture.createConfirmation(t)
	if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err != nil {
		t.Fatal(err)
	}
	request, ok := fixture.executor.lastRequest()
	if !ok || request.Headers.Get("Authorization") != "Bearer current-support-token" || request.Timeout != 17*time.Second {
		t.Fatalf("confirmed call did not use current credential/timeout: request=%+v ok=%v", request, ok)
	}
}

func TestExternalToolConfirmationCannotExecuteAfterContextClear(t *testing.T) {
	fixture := newConfirmedExternalToolFixture(t)
	confirmation := fixture.createConfirmation(t)
	session, ok, err := fixture.base.GetAgentSession(context.Background(), "tenant-1", "session-external-confirmation")
	if err != nil || !ok {
		t.Fatalf("session lookup failed: ok=%v err=%v", ok, err)
	}
	session.SegmentID = "segment-after-clear"
	session.ContextVersion++
	if err := fixture.base.UpsertAgentSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected cleared conversation context to reject confirmation")
	}
	if fixture.executor.callCount() != 0 {
		t.Fatalf("cleared context made %d remote calls", fixture.executor.callCount())
	}
}

func TestExternalToolConfirmationRechecksCapabilityPermission(t *testing.T) {
	fixture := newConfirmedExternalToolFixture(t)
	confirmation := fixture.createConfirmation(t)
	if err := fixture.base.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-acct-employee", TenantID: "tenant-1", Name: "Agent confirmation",
		Permissions: []domain.Permission{{Resource: "agent.run", Action: "create", Scope: "all"}},
		CreatedAt:   fixture.now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected revoked capability permission to deny execution")
	}
	if fixture.executor.callCount() != 0 {
		t.Fatalf("permission denial made %d remote calls", fixture.executor.callCount())
	}
	steps, history := fixture.store.storedSteps()
	if len(steps) != 1 || steps[0].Status != domain.ExecutionStepStatusFailed {
		t.Fatalf("permission denial was not safely audited: %+v", steps)
	}
	if want := []domain.ExecutionStepStatus{domain.ExecutionStepStatusRunning, domain.ExecutionStepStatusFailed}; !reflect.DeepEqual(history, want) {
		t.Fatalf("unexpected permission-denial step transitions: got %v want %v", history, want)
	}
}

func TestExternalToolConfirmationRejectsSchemaDriftAndArchive(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*confirmedExternalToolFixture)
	}{
		{
			name: "schema drift",
			mutate: func(f *confirmedExternalToolFixture) {
				f.capability.SchemaChecksum = "sha256:write-v2"
				f.store.updateCapability(f.capability)
			},
		},
		{
			name: "archived capability",
			mutate: func(f *confirmedExternalToolFixture) {
				archivedAt := f.now
				f.capability.ArchivedAt = &archivedAt
				f.capability.Enabled = false
				f.store.updateCapability(f.capability)
			},
		},
		{
			name: "archived connection",
			mutate: func(f *confirmedExternalToolFixture) {
				archivedAt := f.now
				f.connection.ArchivedAt = &archivedAt
				f.connection.Status = string(domain.ExternalToolConnectionStatusArchived)
				f.store.updateConnection(f.connection)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newConfirmedExternalToolFixture(t)
			confirmation := fixture.createConfirmation(t)
			test.mutate(&fixture)
			if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
				t.Fatal("expected stale external confirmation to fail")
			}
			if fixture.executor.callCount() != 0 {
				t.Fatalf("stale confirmation made %d remote calls", fixture.executor.callCount())
			}
			record, ok := fixture.store.confirmation("tenant-1", confirmation.ID)
			if !ok || record.Status != domain.AgentConfirmationStatusFailed {
				t.Fatalf("stale confirmation was not terminal: record=%+v ok=%v", record, ok)
			}
		})
	}
}

func TestExternalToolConfirmationRestoresPendingAfterTemporaryNetworkFailure(t *testing.T) {
	fixture := newConfirmedExternalToolFixture(t)
	fixture.executor.errors = []error{confirmedExternalToolTemporaryError{}, nil}
	confirmation := fixture.createConfirmation(t)
	if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected temporary network failure")
	}
	record, ok := fixture.store.confirmation("tenant-1", confirmation.ID)
	if !ok || record.Status != domain.AgentConfirmationStatusPending {
		t.Fatalf("temporary failure did not restore pending: record=%+v ok=%v", record, ok)
	}
	if _, err := agentservice.New(fixture.service).ExecuteConfirmation(fixture.ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err != nil {
		t.Fatal(err)
	}
	if fixture.executor.callCount() != 2 {
		t.Fatalf("expected one retry, got %d calls", fixture.executor.callCount())
	}
	steps, history := fixture.store.storedSteps()
	if len(steps) != 2 {
		t.Fatalf("expected one audited step per attempt, got %+v", steps)
	}
	if want := []domain.ExecutionStepStatus{
		domain.ExecutionStepStatusRunning, domain.ExecutionStepStatusFailed,
		domain.ExecutionStepStatusRunning, domain.ExecutionStepStatusCompleted,
	}; !reflect.DeepEqual(history, want) {
		t.Fatalf("unexpected retry step transitions: got %v want %v", history, want)
	}
}

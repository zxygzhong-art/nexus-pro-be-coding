package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/repository/memory"
	baseservice "nexus-pro-api/internal/service"
)

type externalRuntimeHTTPExecutor struct {
	mu    sync.Mutex
	calls []externaltool.Request
}

func (e *externalRuntimeHTTPExecutor) Probe(context.Context, externaltool.ProbeRequest) (externaltool.ProbeResult, error) {
	return externaltool.ProbeResult{StatusCode: http.StatusNoContent}, nil
}

func (e *externalRuntimeHTTPExecutor) Call(_ context.Context, request externaltool.Request) (externaltool.Result, error) {
	e.mu.Lock()
	e.calls = append(e.calls, request)
	e.mu.Unlock()
	if fail, _ := request.Arguments["fail"].(bool); fail {
		return externaltool.Result{}, errors.New("upstream unavailable: private detail")
	}
	return externaltool.Result{
		StatusCode: http.StatusOK, ContentType: "application/json",
		JSON: map[string]any{"customer_name": "Alice-secret-output"},
	}, nil
}

func (e *externalRuntimeHTTPExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

type externalRuntimeFunc func(context.Context, baseservice.AgentChatRuntimeRequest, baseservice.AgentChatEmitFunc) error

func (f externalRuntimeFunc) RunAgentChat(ctx context.Context, req baseservice.AgentChatRuntimeRequest, emit baseservice.AgentChatEmitFunc) error {
	return f(ctx, req, emit)
}

func TestPublishedExternalToolsExecuteForRootAndMemberWithSafeStepAudit(t *testing.T) {
	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedExternalRuntimeAccount(t, store, now)
	model := domain.AgentModel{
		ID: "model-1", TenantID: "tenant-1", Name: "Test", ModelName: "test-model", LiteLLMModel: "test-model",
		Status: domain.AgentModelStatusActive, TimeoutSeconds: 30, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.UpsertAgentModel(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	connection := domain.AgentExternalTool{
		ID: "extconn-1", TenantID: "tenant-1", Name: "Customer API", Kind: "http", Transport: "http",
		EndpointURL: "https://api.example.com/v1", AuthType: "none", TimeoutSeconds: 12,
		Status: string(domain.ExternalToolConnectionStatusActive), CreatedAt: now, UpdatedAt: now,
	}
	if err := store.InsertAgentExternalTool(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	readCapability := domain.ExternalToolCapability{
		ID: "exttool-read", TenantID: "tenant-1", ConnectionID: connection.ID,
		ToolName: "get-customer unsafe remote name", Description: "Read one customer.",
		HTTPMethod: http.MethodGet, HTTPPath: "customers/get",
		InputSchema: map[string]any{
			"type": "object", "required": []any{"customer_id"},
			"properties": map[string]any{"customer_id": map[string]any{"type": "string", "description": "Customer identifier."}},
		},
		OutputSchema: map[string]any{}, Readonly: true, Enabled: true,
		SchemaChecksum: "published-read-checksum", DiscoveredAt: now, UpdatedAt: now,
	}
	writeCapability := domain.ExternalToolCapability{
		ID: "exttool-write", TenantID: "tenant-1", ConnectionID: connection.ID,
		ToolName: "delete_customer", Description: "Delete one customer.",
		HTTPMethod: http.MethodDelete, HTTPPath: "customers/delete",
		InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{},
		Readonly: false, Enabled: true, SchemaChecksum: "published-write-checksum", DiscoveredAt: now, UpdatedAt: now,
	}
	if err := store.ReplaceAgentExternalToolCapabilities(context.Background(), "tenant-1", connection.ID, []domain.ExternalToolCapability{readCapability, writeCapability}); err != nil {
		t.Fatal(err)
	}
	member := domain.AgentTeamMember{
		ID: "member-1", Name: "Customer researcher", Role: "Read customer facts", ModelID: model.ID,
		ModelConfigChecksum: domain.AgentModelSyncConfigHash(model),
		ExternalToolIDs:     []string{readCapability.ID, writeCapability.ID},
	}
	revision := domain.AgentDefinitionVersion{
		ID: "arev-1", TenantID: "tenant-1", AgentID: "agent-1", Version: 1,
		Name: "Customer assistant", Category: domain.AgentCategoryWorkflow, Visibility: domain.AgentVisibilityAll,
		MainAgentRole: "Coordinate customer research", SubAgents: []domain.AgentTeamMember{member},
		ExternalToolIDs: []string{readCapability.ID, writeCapability.ID}, ModelID: model.ID,
		ModelConfigChecksum: domain.AgentModelSyncConfigHash(model),
		TimeoutSeconds:      30, ConfigSchemaVersion: 1, Checksum: "revision-checksum", CreatedAt: now,
	}
	if err := store.InsertAgentDefinitionVersion(context.Background(), revision); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: "agent-1", TenantID: "tenant-1", DraftRevisionID: revision.ID, PublishedRevisionID: revision.ID,
		Name: revision.Name, Category: revision.Category, ModelID: revision.ModelID, MainAgentRole: revision.MainAgentRole,
		SubAgents: revision.SubAgents, ExternalToolIDs: revision.ExternalToolIDs,
		Status: domain.AgentDefinitionStatusPublished, Visibility: domain.AgentVisibilityAll,
		TimeoutSeconds: 30, Version: 1, PublishedVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	executor := &externalRuntimeHTTPExecutor{}
	runtimeCalls := 0
	var pendingConfirmation *domain.AgentConfirmation
	runtime := externalRuntimeFunc(func(ctx context.Context, req baseservice.AgentChatRuntimeRequest, emit baseservice.AgentChatEmitFunc) error {
		runtimeCalls++
		if runtimeCalls == 2 {
			if len(req.ToolSpecs) != 0 || len(req.SubAgents) != 1 || len(req.SubAgents[0].ToolSpecs) != 0 {
				t.Fatalf("schema-drifted capability must disappear from root and member: root=%+v member=%+v", req.ToolSpecs, req.SubAgents)
			}
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "drift safely blocked"})
		}
		if len(req.ToolSpecs) != 2 || len(req.SubAgents) != 1 || len(req.SubAgents[0].ToolSpecs) != 2 {
			t.Fatalf("expected read and confirmation-gated write capabilities for root and member: root=%+v members=%+v", req.ToolSpecs, req.SubAgents)
		}
		var readRuntimeName, writeRuntimeName string
		for name, spec := range req.ToolSpecs {
			if strings.Contains(spec.Description, "requires explicit user confirmation") {
				writeRuntimeName = name
				continue
			}
			readRuntimeName = name
			if name == readCapability.ToolName || !strings.Contains(spec.Description, "Read one customer") {
				t.Fatalf("runtime name/description not normalized: name=%q spec=%+v", name, spec)
			}
			properties, _ := spec.InputSchema["properties"].(map[string]any)
			if properties["customer_id"] == nil {
				t.Fatalf("capability input schema was not forwarded: %+v", spec.InputSchema)
			}
		}
		if readRuntimeName == "" || writeRuntimeName == "" {
			t.Fatalf("runtime did not distinguish read and confirmation-gated write tools: %+v", req.ToolSpecs)
		}
		type readonlyCallResult struct {
			result map[string]any
			err    error
		}
		calls := make(chan readonlyCallResult, 2)
		go func() {
			result, err := req.Tools[readRuntimeName](ctx, map[string]any{"customer_id": "customer-secret-input"})
			calls <- readonlyCallResult{result: result, err: err}
		}()
		go func() {
			result, err := req.SubAgents[0].Tools[readRuntimeName](ctx, map[string]any{"customer_id": "another-secret-input", "fail": true})
			calls <- readonlyCallResult{result: result, err: err}
		}()
		var succeeded, failed int
		for range 2 {
			call := <-calls
			if call.err != nil {
				failed++
			} else if call.result["status_code"] == http.StatusOK {
				succeeded++
			}
		}
		if succeeded != 1 || failed != 1 {
			t.Fatalf("concurrent root/member calls did not preserve their results: succeeded=%d failed=%d", succeeded, failed)
		}
		confirmationResult, err := req.Tools[writeRuntimeName](ctx, map[string]any{"customer_id": "write-secret-input"})
		if err != nil {
			t.Fatalf("write capability should create confirmation without a remote call: %v", err)
		}
		pendingConfirmation, _ = confirmationResult["confirmation"].(*domain.AgentConfirmation)
		if pendingConfirmation == nil || confirmationResult["status"] != "confirmation_required" {
			t.Fatalf("write capability did not return confirmation: %+v", confirmationResult)
		}
		driftedRead := readCapability
		driftedRead.SchemaChecksum = "drifted-between-resolution-and-call"
		driftedRead.UpdatedAt = now.Add(time.Second)
		if err := store.ReplaceAgentExternalToolCapabilities(ctx, "tenant-1", connection.ID, []domain.ExternalToolCapability{driftedRead, writeCapability}); err != nil {
			t.Fatalf("drift capability before invocation: %v", err)
		}
		if _, err := req.Tools[readRuntimeName](ctx, map[string]any{"customer_id": "must-not-leave-process"}); err == nil {
			t.Fatal("readonly capability drifted after resolution should fail before remote invocation")
		}
		if err := store.ReplaceAgentExternalToolCapabilities(ctx, "tenant-1", connection.ID, []domain.ExternalToolCapability{readCapability, writeCapability}); err != nil {
			t.Fatalf("restore capability after TOCTOU assertion: %v", err)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "external calls audited"})
	})
	base := baseservice.New(store, baseservice.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime, ExternalHTTPExecutor: executor})
	requestCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	run, err := New(base).Chat(requestCtx, domain.AgentChatInput{AgentID: "agent-1", Message: "read a customer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if executor.callCount() != 2 {
		t.Fatalf("write preview must not call the remote system; got %d calls", executor.callCount())
	}
	steps, err := store.ListExecutionSteps(context.Background(), "tenant-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstTwoTerminal := map[domain.ExecutionStepStatus]int{}
	if len(steps) >= 2 {
		firstTwoTerminal[steps[0].Status]++
		firstTwoTerminal[steps[1].Status]++
	}
	if len(steps) != 4 || steps[0].SequenceNo != 1 || steps[1].SequenceNo != 2 || steps[2].SequenceNo != 3 || steps[3].SequenceNo != 4 || firstTwoTerminal[domain.ExecutionStepStatusCompleted] != 1 || firstTwoTerminal[domain.ExecutionStepStatusFailed] != 1 || steps[2].Status != domain.ExecutionStepStatusCompleted || steps[3].Status != domain.ExecutionStepStatusFailed {
		t.Fatalf("unexpected external execution step lifecycle: %+v", steps)
	}
	execution, err := base.ExecuteAgentConfirmation(requestCtx, pendingConfirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatalf("confirmed external mutation failed: %v", err)
	}
	if execution.Status != "completed" || executor.callCount() != 3 {
		t.Fatalf("confirmation did not perform exactly one remote mutation: execution=%+v calls=%d", execution, executor.callCount())
	}
	steps, err = store.ListExecutionSteps(context.Background(), "tenant-1", run.ID)
	if err != nil || len(steps) != 5 || steps[4].SequenceNo != 5 || steps[4].Status != domain.ExecutionStepStatusCompleted {
		t.Fatalf("confirmed mutation step was not appended and completed: steps=%+v err=%v", steps, err)
	}
	auditJSON, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"customer-secret-input", "another-secret-input", "write-secret-input", "must-not-leave-process", "Alice-secret-output", "private detail"} {
		if strings.Contains(string(auditJSON), secret) {
			t.Fatalf("execution step audit persisted raw external data %q: %s", secret, auditJSON)
		}
	}

	readCapability.SchemaChecksum = "rediscovered-drift-checksum"
	readCapability.UpdatedAt = now.Add(time.Minute)
	writeCapability.SchemaChecksum = "rediscovered-write-drift-checksum"
	writeCapability.UpdatedAt = now.Add(time.Minute)
	if err := store.ReplaceAgentExternalToolCapabilities(context.Background(), "tenant-1", connection.ID, []domain.ExternalToolCapability{readCapability, writeCapability}); err != nil {
		t.Fatal(err)
	}
	secondRun, err := New(base).Chat(requestCtx, domain.AgentChatInput{AgentID: "agent-1", Message: "read it again"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if executor.callCount() != 3 {
		t.Fatal("schema drift must block remote invocation")
	}
	secondSteps, err := store.ListExecutionSteps(context.Background(), "tenant-1", secondRun.ID)
	if err != nil || len(secondSteps) != 0 {
		t.Fatalf("schema-drifted capability should create no execution step: steps=%+v err=%v", secondSteps, err)
	}
}

func TestPublishedAgentDefinitionFailsClosedOnRootAndMemberModelDrift(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedExternalRuntimeAccount(t, store, now)
	rootModel := domain.AgentModel{
		ID: "model-root", TenantID: "tenant-1", Name: "Root", ModelName: "root-model", LiteLLMModel: "root-model",
		Status: domain.AgentModelStatusActive, TimeoutSeconds: 30, CreatedAt: now, UpdatedAt: now,
	}
	memberModel := domain.AgentModel{
		ID: "model-member", TenantID: "tenant-1", Name: "Member", ModelName: "member-model", LiteLLMModel: "member-model",
		Status: domain.AgentModelStatusActive, TimeoutSeconds: 30, CreatedAt: now, UpdatedAt: now,
	}
	for _, model := range []domain.AgentModel{rootModel, memberModel} {
		if err := store.UpsertAgentModel(context.Background(), model); err != nil {
			t.Fatal(err)
		}
	}
	revision := domain.AgentDefinitionVersion{
		ID: "arev-models", TenantID: "tenant-1", AgentID: "agent-models", Version: 1,
		Name: "Model-bound agent", Category: domain.AgentCategoryWorkflow, Visibility: domain.AgentVisibilityAll,
		MainAgentRole: "Coordinate", ModelID: rootModel.ID, ModelConfigChecksum: domain.AgentModelSyncConfigHash(rootModel),
		SubAgents: []domain.AgentTeamMember{{
			ID: "member-models", Name: "Worker", Role: "Work", ModelID: memberModel.ID,
			ModelConfigChecksum: domain.AgentModelSyncConfigHash(memberModel),
		}},
		TimeoutSeconds: 30, ConfigSchemaVersion: 1, Checksum: "models", CreatedAt: now,
	}
	if err := store.InsertAgentDefinitionVersion(context.Background(), revision); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: revision.AgentID, TenantID: revision.TenantID, PublishedRevisionID: revision.ID,
		Name: revision.Name, Category: revision.Category, ModelID: rootModel.ID, SubAgents: revision.SubAgents,
		Status: domain.AgentDefinitionStatusPublished, Visibility: domain.AgentVisibilityAll,
		Version: 1, PublishedVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := New(baseservice.New(store))
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	if _, err := svc.publishedAgentDefinition(ctx, revision.AgentID); err != nil {
		t.Fatalf("unchanged model bindings should remain executable: %v", err)
	}

	memberModel.TimeoutSeconds++
	if err := store.UpsertAgentModel(context.Background(), memberModel); err != nil {
		t.Fatal(err)
	}
	assertAgentModelDrift(t, svc, ctx, revision.AgentID)
	memberModel.TimeoutSeconds--
	if err := store.UpsertAgentModel(context.Background(), memberModel); err != nil {
		t.Fatal(err)
	}
	rootModel.Status = domain.AgentModelStatusDisabled
	if err := store.UpsertAgentModel(context.Background(), rootModel); err != nil {
		t.Fatal(err)
	}
	assertAgentModelDrift(t, svc, ctx, revision.AgentID)
}

func assertAgentModelDrift(t *testing.T, svc AgentService, ctx domain.RequestContext, agentID string) {
	t.Helper()
	_, err := svc.publishedAgentDefinition(ctx, agentID)
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.ReasonCode != "agent_model_config_drift" {
		t.Fatalf("expected fail-closed model drift error, got %v", err)
	}
}

func seedExternalRuntimeAccount(t *testing.T, store *memory.Store, now time.Time) {
	t.Helper()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-agent-runtime", TenantID: "tenant-1", Name: "Agent Runtime",
		Permissions: []domain.Permission{
			{Resource: "agent.run", Action: "create", Scope: "all"},
			{Resource: "agent.tool", Action: "call", Target: "exttool-read", Scope: "all"},
			{Resource: "agent.tool", Action: "call", Target: "exttool-write", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-1", TenantID: "tenant-1", DisplayName: "Agent User", Email: "agent@example.com",
		Status: "active", DirectPermissionSetIDs: []string{"ps-agent-runtime"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

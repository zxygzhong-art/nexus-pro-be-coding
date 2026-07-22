package service_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

func TestAgentCreatesDraftAndRequiresConfirmationBeforeSubmission(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	store := memory.NewStore()
	seedAgentConfirmationAccount(t, store, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.run", Action: "read", Scope: "all"},
		agentToolTestPermission("create_form_draft"),
		agentToolTestPermission("preview_form_submission"),
		{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
		{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-manager", TenantID: "tenant-1", DisplayName: "Manager", Status: "active", CreatedAt: now})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-manager", TenantID: "tenant-1", Name: "Manager", AccountID: "acct-manager", Status: "active", CreatedAt: now})
	employee, _, _ := store.GetEmployee(context.Background(), "tenant-1", "emp-acct-employee")
	employee.ManagerEmployeeID = "emp-manager"
	_ = store.UpsertEmployee(context.Background(), employee)
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-general", TenantID: "tenant-1", Key: "general", Name: "通用申請單",
		Status: "published", Schema: workflowEnabledTemplateSchema(), CreatedAt: now, UpdatedAt: now,
	})

	var confirmation *domain.AgentConfirmation
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "general",
			"payload":      map[string]any{"subject": "請假", "description": "家庭安排"},
		})
		if err != nil {
			return err
		}
		draft := draftResult["draft"].(domain.FormInstance)
		if _, err := req.Tools["preview_form_submission"](ctx, map[string]any{"draft_id": draft.ID}); err != nil {
			return err
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "草稿已準備，請確認提交。"})
	}}
	confirmationStore := newAgentConfirmationTestStore(store)
	svc, _ := newServiceWithFakeFormApprovalWorkflows(confirmationStore, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "幫我創建請假單"}, func(_ context.Context, event domain.AgentChatEvent) error {
		if event.Event == domain.AgentChatEventConfirmation {
			confirmation = event.Confirmation
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != string(domain.AgentRunStatusCompleted) || confirmation == nil || confirmation.Kind != "form_submit" {
		t.Fatalf("expected a completed chat with submit confirmation, run=%+v confirmation=%+v", run, confirmation)
	}
	messagePage, err := agentservice.New(svc).ListSessionMessages(ctx, run.SessionID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	messages := messagePage.Items
	artifactNames := map[string]int{}
	for _, message := range messages {
		raw, _ := message.Metadata["agent_artifact_json"].(string)
		if raw == "" {
			continue
		}
		var artifact map[string]any
		if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
			t.Fatalf("invalid persisted artifact: %v", err)
		}
		if name, _ := artifact["name"].(string); name != "" {
			artifactNames[name]++
		}
		if event, _ := artifact["event"].(string); event != "" {
			artifactNames[event]++
		}
	}
	if artifactNames["create_form_draft"] != 1 || artifactNames["confirmation_required"] != 1 {
		t.Fatalf("expected replayable draft and pending confirmation artifacts, got %v", artifactNames)
	}
	drafts, err := store.ListFormInstances(context.Background(), "tenant-1")
	if err != nil || len(drafts) != 1 || drafts[0].Status != "draft" {
		t.Fatalf("preview must not submit the form, forms=%+v err=%v", drafts, err)
	}

	executed, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if executed.FormInstance == nil || executed.FormInstance.Status != "in_review" {
		t.Fatalf("expected confirmed draft to enter workflow, got %+v", executed)
	}
	messagePage, err = agentservice.New(svc).ListSessionMessages(ctx, run.SessionID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	messages = messagePage.Items
	for _, message := range messages {
		raw, _ := message.Metadata["agent_artifact_json"].(string)
		if raw != "" && strings.Contains(raw, "confirmation_required") {
			t.Fatal("consumed confirmation must not be restored from session history")
		}
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected one-time confirmation replay to be rejected")
	}
}

// TestAgentLeaveDraftDefaultsMissingTimes verifies leave drafts use today's configured work period when both times are absent.
func TestAgentLeaveDraftDefaultsMissingTimes(t *testing.T) {
	now := time.Date(2026, 7, 14, 2, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentConfirmationAccount(t, store, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission("check_leave_eligibility"),
		agentToolTestPermission("create_form_draft"),
		agentToolTestPermission("preview_form_submission"),
		{Resource: "attendance.leave", Action: "read", Scope: "self"},
		{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
		{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
	})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-proxy", TenantID: "tenant-1", Name: "Demo", EmployeeNo: "E-PROXY", Status: "active", CreatedAt: now,
	})
	leaveSchema := workflowEnabledTemplateSchema()
	leaveSchema["workspace_design"].(map[string]any)["fields"] = []map[string]any{
		{"id": "proxy", "type": "select", "label": "代理人", "binding": map[string]any{"source_id": "employees", "label_field": "name", "value_field": "id"}},
		{"id": "leave_type", "type": "select", "label": "假勤名稱", "required": true, "binding": map[string]any{"source_id": "leave_types", "label_field": "name", "value_field": "code"}},
		{"id": "start_at", "type": "datetime", "label": "開始時間", "required": true},
		{"id": "end_at", "type": "datetime", "label": "結束時間", "required": true},
		{"id": "hours", "type": "number", "label": "請假時數", "required": true},
		{"id": "reason", "type": "textarea", "label": "請假原因", "required": true},
	}
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-leave-default", TenantID: "tenant-1", Key: "leave-request", Name: "請假申請單",
		Status: "published", Schema: leaveSchema, CreatedAt: now, UpdatedAt: now,
	})
	if err := store.InsertAttendancePolicyVersion(context.Background(), domain.AttendancePolicy{
		TenantID: "tenant-1", Version: 1,
		WorkTime: domain.AttendancePolicyWorkTime{
			ClockMode: "fixed", StandardStart: "09:00", StandardEnd: "18:00",
			BreakStart: "12:00", BreakEnd: "13:00", Weekend: "週六、週日",
			CycleStart: "1 日", CycleEnd: "本月 月底（最後一日）",
		},
		EffectiveFrom: &now, PublishedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	var draft domain.FormInstance
	var normalizedFields []string
	var explicitDraft domain.FormInstance
	var explicitNormalizedFields []string
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, _ service.AgentChatEmitFunc) error {
		eligibility, err := req.Tools["check_leave_eligibility"](ctx, map[string]any{
			"leave_type": "annual", "date": "2026-07-14", "hours": float64(8),
		})
		if err != nil {
			return err
		}
		if eligibility["eligible"] != true || eligibility["balance_required"] != false || eligibility["balance_fallback_reason"] != "balance_not_initialized" {
			t.Fatalf("missing balance should continue to draft creation: %+v", eligibility)
		}
		result, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "leave-request",
			"payload":      map[string]any{"leave_type": "annual", "reason": "家庭安排", "proxy": "Demo"},
		})
		if err != nil {
			return err
		}
		draft = result["draft"].(domain.FormInstance)
		normalizedFields, _ = result["normalized_fields"].([]string)
		if _, err = req.Tools["preview_form_submission"](ctx, map[string]any{"draft_id": draft.ID}); err != nil {
			return err
		}
		explicitResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "leave-request",
			"payload": map[string]any{
				"leave_type": "annual", "reason": "已指定時間", "hours": float64(9),
				"proxy":    "emp-proxy",
				"start_at": "2026-07-20T09:00:00+08:00", "end_at": "2026-07-20T18:00:00+08:00",
			},
		})
		if err != nil {
			return err
		}
		explicitDraft = explicitResult["draft"].(domain.FormInstance)
		explicitNormalizedFields, _ = explicitResult["normalized_fields"].([]string)
		return nil
	}}
	confirmationStore := newAgentConfirmationTestStore(store)
	svc, _ := newServiceWithFakeFormApprovalWorkflows(confirmationStore, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "幫我申請特休，原因是家庭安排"}, func(context.Context, domain.AgentChatEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}

	if got := draft.Payload["start_at"]; got != "2026-07-14T09:00:00+08:00" {
		t.Fatalf("expected today's standard start, got %v", got)
	}
	if got := draft.Payload["end_at"]; got != "2026-07-14T18:00:00+08:00" {
		t.Fatalf("expected today's standard end, got %v", got)
	}
	if got := draft.Payload["hours"]; got != float64(8) {
		t.Fatalf("expected policy-derived eight leave hours, got %v", got)
	}
	if draft.Payload["proxy"] != "emp-proxy" {
		t.Fatalf("expected the unique proxy label to normalize to its employee ID, got %v", draft.Payload["proxy"])
	}
	if len(normalizedFields) != 4 {
		t.Fatalf("expected proxy, start_at, end_at, and hours normalization to be disclosed, got %v", normalizedFields)
	}
	if len(explicitNormalizedFields) != 1 || explicitNormalizedFields[0] != "hours" || explicitDraft.Payload["start_at"] != "2026-07-20T09:00:00+08:00" || explicitDraft.Payload["end_at"] != "2026-07-20T18:00:00+08:00" || explicitDraft.Payload["hours"] != float64(8) {
		t.Fatalf("expected explicit leave times to remain unchanged and hours to be recalculated, draft=%v normalized=%v", explicitDraft.Payload, explicitNormalizedFields)
	}
}

// TestAgentConfirmationExpiryUsesServiceClock 確認 TTL 只依賴可注入的服務時鐘，且過期令牌不可重放。
func TestAgentConfirmationExpiryUsesServiceClock(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentConfirmationAccount(t, store, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission("create_form_draft"),
		agentToolTestPermission("preview_form_submission"),
		{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
		{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
	})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-expiry", TenantID: "tenant-1", Key: "expiry-form", Name: "時鐘確認單",
		Status: "published", Schema: workflowEnabledTemplateSchema("acct-employee"), CreatedAt: now, UpdatedAt: now,
	})

	var confirmation *domain.AgentConfirmation
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, _ service.AgentChatEmitFunc) error {
		draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "expiry-form",
			"payload":      map[string]any{"description": "驗證 TTL"},
		})
		if err != nil {
			return err
		}
		draft := draftResult["draft"].(domain.FormInstance)
		preview, err := req.Tools["preview_form_submission"](ctx, map[string]any{"draft_id": draft.ID})
		if err != nil {
			return err
		}
		confirmation = preview["confirmation"].(*domain.AgentConfirmation)
		return nil
	}}
	clock := now
	confirmationStore := newAgentConfirmationTestStore(store)
	svc, _ := newServiceWithFakeFormApprovalWorkflows(confirmationStore, service.Options{Now: func() time.Time { return clock }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "創建確認單"}, func(context.Context, domain.AgentChatEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if confirmation == nil {
		t.Fatal("expected confirmation")
	}

	clock = now.Add(11 * time.Minute)
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected expired confirmation to be rejected")
	}
	if record, ok := confirmationStore.confirmation("tenant-1", confirmation.ID); !ok || record.Status != domain.AgentConfirmationStatusExpired {
		t.Fatalf("expected expired confirmation state, record=%+v ok=%v", record, ok)
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected expired confirmation replay to be rejected")
	}
}

func TestAgentConfirmationIsHiddenAfterContextClear(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, time.UTC)
	baseStore := memory.NewStore()
	seedAgentConfirmationAccount(t, baseStore, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	confirmationStore := newAgentConfirmationTestStore(baseStore)
	session := domain.AgentSession{
		ID: "session-clear-confirmation", TenantID: "tenant-1", AccountID: "acct-employee",
		SegmentID: "segment-before-clear", Status: domain.AgentSessionStatusActive, ContextVersion: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := baseStore.UpsertAgentSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	public := domain.AgentConfirmation{
		ID: "aconf-before-clear", Kind: "form_submit", Title: "確認提交",
		Action: "submit", ActionLabel: "確認", ExpiresAt: now.Add(10 * time.Minute),
	}
	publicRaw, _ := json.Marshal(public)
	publicPayload := map[string]any{}
	_ = json.Unmarshal(publicRaw, &publicPayload)
	if err := confirmationStore.UpsertAgentConfirmation(context.Background(), domain.AgentConfirmationRecord{
		ID: public.ID, TenantID: "tenant-1", AccountID: "acct-employee",
		ConversationID: session.ID, SegmentID: session.SegmentID,
		Kind: public.Kind, Title: public.Title, Action: public.Action,
		PublicPayload: publicPayload, ActionPayload: map[string]any{}, ResultPayload: map[string]any{},
		Status: domain.AgentConfirmationStatusPending, ExpiresAt: public.ExpiresAt, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(confirmationStore, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	pending, err := svc.PendingAgentConfirmationMessages(ctx, ctx.AccountID, session)
	if err != nil || len(pending) != 1 {
		t.Fatalf("expected one pending confirmation before clear, pending=%+v err=%v", pending, err)
	}
	cleared, err := agentservice.New(svc).ClearSessionContext(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.SegmentID == session.SegmentID {
		t.Fatal("context clear must allocate a new segment")
	}
	pending, err = svc.PendingAgentConfirmationMessages(ctx, ctx.AccountID, cleared)
	if err != nil || len(pending) != 0 {
		t.Fatalf("old-segment confirmation leaked after clear, pending=%+v err=%v", pending, err)
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, public.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("old-segment confirmation must not be claimable after clear")
	}
}

// TestAgentConfirmationRetriesTransientFailureButConsumesConflict 驗證暫時性失敗可重試，stale 衝突仍維持一次性。
func TestAgentConfirmationRetriesTransientFailureButConsumesConflict(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	baseStore := memory.NewStore()
	confirmationStore := newAgentConfirmationTestStore(baseStore)
	store := &transientAgentConfirmationStore{agentConfirmationTestStore: confirmationStore}
	seedAgentConfirmationAccount(t, baseStore, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission("create_form_draft"),
		agentToolTestPermission("preview_form_submission"),
		{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
		{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
	})
	_ = baseStore.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-retry", TenantID: "tenant-1", Key: "retry-form", Name: "重試確認單",
		Status: "published", Schema: workflowEnabledTemplateSchema("acct-employee"), CreatedAt: now, UpdatedAt: now,
	})

	var confirmation *domain.AgentConfirmation
	var draftID string
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, _ service.AgentChatEmitFunc) error {
		draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "retry-form",
			"payload":      map[string]any{"description": "驗證瞬時失敗"},
		})
		if err != nil {
			return err
		}
		draft := draftResult["draft"].(domain.FormInstance)
		draftID = draft.ID
		preview, err := req.Tools["preview_form_submission"](ctx, map[string]any{"draft_id": draft.ID})
		if err != nil {
			return err
		}
		confirmation = preview["confirmation"].(*domain.AgentConfirmation)
		return nil
	}}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	prepare := func() {
		t.Helper()
		confirmation = nil
		draftID = ""
		if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "創建確認單"}, func(context.Context, domain.AgentChatEvent) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if confirmation == nil || draftID == "" {
			t.Fatal("expected prepared confirmation and draft")
		}
	}

	prepare()
	store.failNextFormInstanceGet = true
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected transient execution failure")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 503 {
		t.Fatalf("expected original 503 error, got %v", err)
	}
	if record, ok := confirmationStore.confirmation("tenant-1", confirmation.ID); !ok || record.Status != domain.AgentConfirmationStatusPending {
		t.Fatalf("expected retryable failure to restore pending, record=%+v ok=%v", record, ok)
	}
	executed, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if executed.FormInstance == nil || executed.FormInstance.Status != "in_review" {
		t.Fatalf("expected restored confirmation to submit successfully, got %+v", executed)
	}

	prepare()
	store.cancelNextFormInstanceGet = true
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err != context.Canceled {
		t.Fatalf("expected original cancellation, got %v", err)
	}
	if record, ok := confirmationStore.confirmation("tenant-1", confirmation.ID); !ok || record.Status != domain.AgentConfirmationStatusCancelled {
		t.Fatalf("expected cancelled execution to become terminal, record=%+v ok=%v", record, ok)
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected cancelled confirmation replay to remain consumed")
	}

	prepare()
	draft, ok, err := baseStore.GetFormInstance(context.Background(), "tenant-1", draftID)
	if err != nil || !ok {
		t.Fatalf("draft lookup failed ok=%v err=%v", ok, err)
	}
	draft.Payload["description"] = "確認後被修改"
	if err := baseStore.UpsertFormInstance(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected stale confirmation conflict")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 409 {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if record, ok := confirmationStore.confirmation("tenant-1", confirmation.ID); !ok || record.Status != domain.AgentConfirmationStatusFailed {
		t.Fatalf("expected deterministic conflict to become failed, record=%+v ok=%v", record, ok)
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected stale confirmation replay to remain consumed")
	}

	prepare()
	store.failConfirmationAudit = true
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected confirmation audit failure")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 503 {
		t.Fatalf("expected original audit error, got %v", err)
	}
	submitted, ok, err := baseStore.GetFormInstance(context.Background(), "tenant-1", draftID)
	if err != nil || !ok || submitted.Status != "in_review" {
		t.Fatalf("side effect should remain completed despite audit failure, instance=%+v ok=%v err=%v", submitted, ok, err)
	}
	if _, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected audit failure not to restore a completed confirmation")
	}
	if record, ok := confirmationStore.confirmation("tenant-1", confirmation.ID); !ok || record.Status != domain.AgentConfirmationStatusCompleted {
		t.Fatalf("expected audit failure to leave confirmation completed, record=%+v ok=%v", record, ok)
	}
}

func TestAgentPreparesAndExecutesFixedBulkReview(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentConfirmationAccount(t, store, now, "acct-manager", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission("prepare_bulk_review"),
		{Resource: "workflow.form_instance", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "approve", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "update", Scope: "all"},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-applicant", TenantID: "tenant-1", DisplayName: "Applicant", Status: "active", CreatedAt: now})
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-general", TenantID: "tenant-1", Key: "general", Name: "通用申請",
		Status: "published", Schema: workflowEnabledTemplateSchema("acct-manager"), CreatedAt: now, UpdatedAt: now,
	})
	for _, id := range []string{"fi-one", "fi-two"} {
		_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
			ID: id, TenantID: "tenant-1", TemplateID: "ft-general", ApplicantAccountID: "acct-applicant",
			Status: "submitted", Payload: map[string]any{"description": id}, SubmittedAt: now, UpdatedAt: now,
		})
	}

	var confirmation *domain.AgentConfirmation
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		_, err := req.Tools["prepare_bulk_review"](ctx, map[string]any{
			"form_instance_ids": []any{"fi-one", "fi-two"},
			"action":            "approve",
			"reason":            "資料完整",
		})
		if err != nil {
			return err
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "請確認批量批准。"})
	}}
	confirmationStore := newAgentConfirmationTestStore(store)
	svc, _ := newServiceWithFakeFormApprovalWorkflows(confirmationStore, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	for _, id := range []string{"fi-one", "fi-two"} {
		startWorkflowRunForTest(t, svc, store, "tenant-1", id, "acct-applicant")
	}
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-manager"}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "批准這兩筆"}, func(_ context.Context, event domain.AgentChatEvent) error {
		if event.Event == domain.AgentChatEventConfirmation {
			confirmation = event.Confirmation
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if confirmation == nil || len(confirmation.Items) != 2 || confirmation.Action != "approve" {
		t.Fatalf("expected fixed bulk confirmation, got %+v", confirmation)
	}

	executed, err := agentservice.New(svc).ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if executed.Status != "completed" || executed.BulkReview == nil || len(executed.BulkReview.Results) != 2 {
		t.Fatalf("unexpected bulk execution: %+v", executed)
	}
	for _, id := range []string{"fi-one", "fi-two"} {
		item, ok, err := store.GetFormInstance(context.Background(), "tenant-1", id)
		if err != nil || !ok || item.Status != "approved" {
			t.Fatalf("expected %s approved, item=%+v ok=%v err=%v", id, item, ok, err)
		}
	}
}

func seedAgentConfirmationAccount(t *testing.T, store *memory.Store, now time.Time, accountID string, permissions []domain.Permission) {
	t.Helper()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant", CreatedAt: now})
	permissionSetID := "ps-" + accountID
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: permissionSetID, TenantID: "tenant-1", Name: "Agent confirmation", Permissions: permissions, CreatedAt: now,
	})
	employeeID := "emp-" + accountID
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID: employeeID, TenantID: "tenant-1", Name: accountID, AccountID: accountID, Status: "active", CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: accountID, TenantID: "tenant-1", DisplayName: accountID, EmployeeID: employeeID,
		Status: "active", DirectPermissionSetIDs: []string{permissionSetID}, CreatedAt: now,
	})
}

func agentToolTestPermission(name string) domain.Permission {
	return domain.Permission{Resource: "agent.tool", Action: "call", Target: name, Scope: "all"}
}

type agentConfirmationTestState struct {
	mu      sync.Mutex
	records map[string]domain.AgentConfirmationRecord
}

// agentConfirmationTestStore adds the narrow Agent v2 confirmation contract to the legacy memory store.
type agentConfirmationTestStore struct {
	repository.Store
	confirmations *agentConfirmationTestState
}

func newAgentConfirmationTestStore(store repository.Store) *agentConfirmationTestStore {
	return &agentConfirmationTestStore{
		Store: store,
		confirmations: &agentConfirmationTestState{
			records: map[string]domain.AgentConfirmationRecord{},
		},
	}
}

func (s *agentConfirmationTestStore) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	return repository.WithinTenantTransaction(ctx, s.Store, tenantID, func(tx repository.Store) error {
		return fn(&agentConfirmationTestStore{Store: tx, confirmations: s.confirmations})
	})
}

func (s *agentConfirmationTestStore) UpsertAgentConfirmation(_ context.Context, record domain.AgentConfirmationRecord) error {
	s.confirmations.mu.Lock()
	defer s.confirmations.mu.Unlock()
	s.confirmations.records[agentConfirmationTestKey(record.TenantID, record.ID)] = record
	return nil
}

func (s *agentConfirmationTestStore) ListPendingAgentConfirmations(_ context.Context, tenantID, accountID, conversationID, segmentID string, now time.Time) ([]domain.AgentConfirmationRecord, error) {
	s.confirmations.mu.Lock()
	defer s.confirmations.mu.Unlock()
	items := make([]domain.AgentConfirmationRecord, 0)
	for _, record := range s.confirmations.records {
		if record.TenantID == tenantID && record.AccountID == accountID && record.ConversationID == conversationID && record.SegmentID == segmentID && record.Status == domain.AgentConfirmationStatusPending && record.ExpiresAt.After(now) {
			items = append(items, record)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ExpiresAt.Equal(items[j].ExpiresAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].ExpiresAt.Before(items[j].ExpiresAt)
	})
	return items, nil
}

func (s *agentConfirmationTestStore) ClaimAgentConfirmation(ctx context.Context, tenantID, accountID, id string, now time.Time) (domain.AgentConfirmationRecord, bool, error) {
	s.confirmations.mu.Lock()
	defer s.confirmations.mu.Unlock()
	key := agentConfirmationTestKey(tenantID, id)
	record, ok := s.confirmations.records[key]
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
	if !record.ExpiresAt.After(now) {
		record.Status = domain.AgentConfirmationStatusExpired
		record.ConsumedAt = &now
	} else {
		record.Status = domain.AgentConfirmationStatusExecuting
	}
	s.confirmations.records[key] = record
	return record, true, nil
}

func (s *agentConfirmationTestStore) UpdateAgentConfirmation(_ context.Context, record domain.AgentConfirmationRecord) (domain.AgentConfirmationRecord, bool, error) {
	s.confirmations.mu.Lock()
	defer s.confirmations.mu.Unlock()
	key := agentConfirmationTestKey(record.TenantID, record.ID)
	current, ok := s.confirmations.records[key]
	if !ok || current.Status != domain.AgentConfirmationStatusExecuting {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	switch record.Status {
	case domain.AgentConfirmationStatusPending,
		domain.AgentConfirmationStatusCompleted,
		domain.AgentConfirmationStatusFailed,
		domain.AgentConfirmationStatusCancelled,
		domain.AgentConfirmationStatusExpired:
	default:
		return domain.AgentConfirmationRecord{}, false, nil
	}
	s.confirmations.records[key] = record
	return record, true, nil
}

func (s *agentConfirmationTestStore) confirmation(tenantID, id string) (domain.AgentConfirmationRecord, bool) {
	s.confirmations.mu.Lock()
	defer s.confirmations.mu.Unlock()
	record, ok := s.confirmations.records[agentConfirmationTestKey(tenantID, id)]
	return record, ok
}

func agentConfirmationTestKey(tenantID, id string) string {
	return tenantID + "\x00" + id
}

type transientAgentConfirmationStore struct {
	*agentConfirmationTestStore
	failNextFormInstanceGet   bool
	cancelNextFormInstanceGet bool
	failConfirmationAudit     bool
}

// WithTenantTransaction 保留底層 memory store 的交易語義，僅讓 wrapper 注入單次讀取故障。
func (s *transientAgentConfirmationStore) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	return s.agentConfirmationTestStore.WithTenantTransaction(ctx, tenantID, fn)
}

// GetFormInstance 注入一次明確的 503，模擬 side effect 開始前的暫時性儲存故障。
func (s *transientAgentConfirmationStore) GetFormInstance(ctx context.Context, tenantID, id string) (domain.FormInstance, bool, error) {
	if s.cancelNextFormInstanceGet {
		s.cancelNextFormInstanceGet = false
		return domain.FormInstance{}, false, context.Canceled
	}
	if s.failNextFormInstanceGet {
		s.failNextFormInstanceGet = false
		return domain.FormInstance{}, false, domain.E(503, "repository_unavailable", "temporary form instance lookup failure")
	}
	return s.Store.GetFormInstance(ctx, tenantID, id)
}

// AppendAuditLog 只注入 confirmation 成功後的稽覈故障，證明完成的 side effect 不會恢復 token。
func (s *transientAgentConfirmationStore) AppendAuditLog(ctx context.Context, log domain.AuditLog) error {
	if s.failConfirmationAudit && log.Action == "agent.confirmation.execute" {
		s.failConfirmationAudit = false
		return domain.E(503, "audit_unavailable", "temporary confirmation audit failure")
	}
	return s.Store.AppendAuditLog(ctx, log)
}

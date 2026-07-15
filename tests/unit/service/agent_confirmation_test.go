package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
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
		ID: "ft-general", TenantID: "tenant-1", Key: "general", Name: "通用申请单",
		Status: "published", Schema: workflowEnabledTemplateSchema(), CreatedAt: now, UpdatedAt: now,
	})

	var confirmation *domain.AgentConfirmation
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "general",
			"payload":      map[string]any{"subject": "请假", "description": "家庭安排"},
		})
		if err != nil {
			return err
		}
		draft := draftResult["draft"].(domain.FormInstance)
		if _, err := req.Tools["preview_form_submission"](ctx, map[string]any{"draft_id": draft.ID}); err != nil {
			return err
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "草稿已准备，请确认提交。"})
	}}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	run, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "帮我创建请假单"}, func(_ context.Context, event domain.AgentChatEvent) error {
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
	messages, err := svc.Agent().ListSessionMessages(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
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

	executed, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if executed.FormInstance == nil || executed.FormInstance.Status != "in_review" {
		t.Fatalf("expected confirmed draft to enter workflow, got %+v", executed)
	}
	messages, err = svc.Agent().ListSessionMessages(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		raw, _ := message.Metadata["agent_artifact_json"].(string)
		if raw != "" && strings.Contains(raw, "confirmation_required") {
			t.Fatal("consumed confirmation must not be restored from session history")
		}
	}
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected one-time confirmation replay to be rejected")
	}
}

// TestAgentLeaveDraftDefaultsMissingTimes verifies leave drafts use today's configured work period when both times are absent.
func TestAgentLeaveDraftDefaultsMissingTimes(t *testing.T) {
	now := time.Date(2026, 7, 14, 2, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentConfirmationAccount(t, store, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission("create_form_draft"),
		agentToolTestPermission("preview_form_submission"),
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
		ID: "ft-leave-default", TenantID: "tenant-1", Key: "leave-request", Name: "请假申请单",
		Status: "published", Schema: leaveSchema, CreatedAt: now, UpdatedAt: now,
	})
	if err := store.UpsertAttendancePolicy(context.Background(), domain.AttendancePolicy{
		ID: "current", TenantID: "tenant-1", Version: 1,
		WorkTime: domain.AttendancePolicyWorkTime{
			ClockMode: "fixed", StandardStart: "09:00", StandardEnd: "18:00",
			BreakStart: "12:00", BreakEnd: "13:00", Weekend: "週六、週日",
			CycleStart: "1 日", CycleEnd: "本月 月底（最後一日）",
		},
		LeaveTypes: []domain.AttendanceLeaveType{{Code: "annual", Name: "特休", Active: true}},
		CreatedAt:  now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	var draft domain.FormInstance
	var normalizedFields []string
	var explicitDraft domain.FormInstance
	var explicitNormalizedFields []string
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, _ service.AgentChatEmitFunc) error {
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
				"leave_type": "annual", "reason": "已指定时间", "hours": float64(9),
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
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "帮我申请特休，原因是家庭安排"}, func(context.Context, domain.AgentChatEvent) error { return nil }); err != nil {
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
		ID: "ft-expiry", TenantID: "tenant-1", Key: "expiry-form", Name: "时钟确认单",
		Status: "published", Schema: workflowEnabledTemplateSchema("acct-employee"), CreatedAt: now, UpdatedAt: now,
	})

	var confirmation *domain.AgentConfirmation
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, _ service.AgentChatEmitFunc) error {
		draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "expiry-form",
			"payload":      map[string]any{"description": "验证 TTL"},
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
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return clock }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-employee"}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "创建确认单"}, func(context.Context, domain.AgentChatEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if confirmation == nil {
		t.Fatal("expected confirmation")
	}

	clock = now.Add(11 * time.Minute)
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected expired confirmation to be rejected")
	}
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected expired confirmation replay to be rejected")
	}
}

// TestAgentConfirmationRetriesTransientFailureButConsumesConflict 驗證暫時性失敗可重試，stale 衝突仍維持一次性。
func TestAgentConfirmationRetriesTransientFailureButConsumesConflict(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	baseStore := memory.NewStore()
	store := &transientAgentConfirmationStore{Store: baseStore}
	seedAgentConfirmationAccount(t, baseStore, now, "acct-employee", []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		agentToolTestPermission("create_form_draft"),
		agentToolTestPermission("preview_form_submission"),
		{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "submit", Scope: "self"},
		{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
	})
	_ = baseStore.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-retry", TenantID: "tenant-1", Key: "retry-form", Name: "重试确认单",
		Status: "published", Schema: workflowEnabledTemplateSchema("acct-employee"), CreatedAt: now, UpdatedAt: now,
	})

	var confirmation *domain.AgentConfirmation
	var draftID string
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, _ service.AgentChatEmitFunc) error {
		draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
			"template_key": "retry-form",
			"payload":      map[string]any{"description": "验证瞬时失败"},
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
		if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "创建确认单"}, func(context.Context, domain.AgentChatEvent) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if confirmation == nil || draftID == "" {
			t.Fatal("expected prepared confirmation and draft")
		}
	}

	prepare()
	store.failNextFormInstanceGet = true
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected transient execution failure")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 503 {
		t.Fatalf("expected original 503 error, got %v", err)
	}
	executed, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if executed.FormInstance == nil || executed.FormInstance.Status != "in_review" {
		t.Fatalf("expected restored confirmation to submit successfully, got %+v", executed)
	}

	prepare()
	draft, ok, err := baseStore.GetFormInstance(context.Background(), "tenant-1", draftID)
	if err != nil || !ok {
		t.Fatalf("draft lookup failed ok=%v err=%v", ok, err)
	}
	draft.Payload["description"] = "确认后被修改"
	if err := baseStore.UpsertFormInstance(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected stale confirmation conflict")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 409 {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected stale confirmation replay to remain consumed")
	}

	prepare()
	store.failConfirmationAudit = true
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected confirmation audit failure")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 503 {
		t.Fatalf("expected original audit error, got %v", err)
	}
	submitted, ok, err := baseStore.GetFormInstance(context.Background(), "tenant-1", draftID)
	if err != nil || !ok || submitted.Status != "in_review" {
		t.Fatalf("side effect should remain completed despite audit failure, instance=%+v ok=%v err=%v", submitted, ok, err)
	}
	if _, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{}); err == nil {
		t.Fatal("expected audit failure not to restore a completed confirmation")
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
		ID: "ft-general", TenantID: "tenant-1", Key: "general", Name: "通用申请",
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
			"reason":            "资料完整",
		})
		if err != nil {
			return err
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "请确认批量批准。"})
	}}
	svc, _ := newServiceWithFakeFormApprovalWorkflows(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	for _, id := range []string{"fi-one", "fi-two"} {
		startWorkflowRunForTest(t, svc, store, "tenant-1", id, "acct-applicant")
	}
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-manager"}
	if _, err := svc.Agent().Chat(ctx, domain.AgentChatInput{Message: "批准这两笔"}, func(_ context.Context, event domain.AgentChatEvent) error {
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

	executed, err := svc.Agent().ExecuteConfirmation(ctx, confirmation.ID, domain.ExecuteAgentConfirmationInput{})
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

type transientAgentConfirmationStore struct {
	repository.Store
	failNextFormInstanceGet bool
	failConfirmationAudit   bool
}

// WithTenantTransaction 保留底層 memory store 的交易語義，僅讓 wrapper 注入單次讀取故障。
func (s *transientAgentConfirmationStore) WithTenantTransaction(ctx context.Context, tenantID string, fn func(repository.Store) error) error {
	return repository.WithinTenantTransaction(ctx, s.Store, tenantID, fn)
}

// GetFormInstance 注入一次明確的 503，模擬 side effect 開始前的暫時性儲存故障。
func (s *transientAgentConfirmationStore) GetFormInstance(ctx context.Context, tenantID, id string) (domain.FormInstance, bool, error) {
	if s.failNextFormInstanceGet {
		s.failNextFormInstanceGet = false
		return domain.FormInstance{}, false, domain.E(503, "repository_unavailable", "temporary form instance lookup failure")
	}
	return s.Store.GetFormInstance(ctx, tenantID, id)
}

// AppendAuditLog 只注入 confirmation 成功後的稽核故障，證明完成的 side effect 不會恢復 token。
func (s *transientAgentConfirmationStore) AppendAuditLog(ctx context.Context, log domain.AuditLog) error {
	if s.failConfirmationAudit && log.Action == "agent.confirmation.execute" {
		s.failConfirmationAudit = false
		return domain.E(503, "audit_unavailable", "temporary confirmation audit failure")
	}
	return s.Store.AppendAuditLog(ctx, log)
}

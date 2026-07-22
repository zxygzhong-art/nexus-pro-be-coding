package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

func TestAgentChatUsesInjectedRuntimeAndPersistsRun(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "me", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "get_my_profile", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			if req.RequestContext.TenantID != "tenant-1" || req.RequestContext.AccountID != "acct-1" {
				t.Fatalf("unexpected request context: %+v", req.RequestContext)
			}
			if req.RunID == "" || req.SessionID == "" {
				t.Fatalf("expected run and session ids, got %+v", req)
			}
			if _, ok := req.Tools["knowledge.search"]; !ok {
				t.Fatalf("tool catalog entry knowledge.search is missing from runtime tools: %+v", req.Tools)
			}
			if !strings.Contains(req.Message, "Known facts:") || !strings.Contains(req.Message, "User: 幫我看一下資料") {
				t.Fatalf("expected runtime message to retain assembled context, got %q", req.Message)
			}
			req.RecordUsage(domain.AgentTokenUsage{InputTokens: 100, CachedTokens: 40, OutputTokens: 20, TotalTokens: 120})
			req.RecordUsage(domain.AgentTokenUsage{InputTokens: 30, OutputTokens: 10, TotalTokens: 40})
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "您好，已完成分析。"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})
	events := []domain.AgentChatEvent{}

	run, err := agentservice.New(svc).Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AgentChatInput{Message: "幫我看一下資料", Mode: "assistant_chat"},
		func(_ context.Context, event domain.AgentChatEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != string(domain.AgentRunStatusCompleted) || run.Answer != "您好，已完成分析。" || run.Mode != "assistant_chat" || run.Prompt != "幫我看一下資料" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if run.LLMCallCount != 2 || run.InputTokens != 130 || run.CachedTokens != 40 || run.OutputTokens != 30 || run.TotalTokens != 160 || !run.UsageComplete {
		t.Fatalf("unexpected token usage: %+v", run)
	}
	if len(events) != 3 || events[0].Event != domain.AgentChatEventSession || events[1].Event != domain.AgentChatEventMessageDelta || events[2].Event != domain.AgentChatEventDone {
		t.Fatalf("unexpected events: %+v", events)
	}
	stored, err := store.ListAgentRunsByAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ID != run.ID || stored[0].Answer != run.Answer || stored[0].Prompt != "幫我看一下資料" || stored[0].TotalTokens != 160 {
		t.Fatalf("expected persisted completed run, got %+v", stored)
	}
}

func TestAgentChatRecommendationUsesVisibleAssistantCatalog(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	for _, agent := range []domain.AgentDefinition{
		{
			ID: "agent-payroll", TenantID: "tenant-1", Name: "薪資助理", Description: "協助薪資查詢與差異說明",
			Category: domain.AgentCategoryAnalytics, Status: domain.AgentDefinitionStatusPublished,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "agent-draft", TenantID: "tenant-1", Name: "未發佈助理", Description: "不應出現在推薦上下文",
			Category: domain.AgentCategoryWorkflow, Status: domain.AgentDefinitionStatusDraft,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertAgentDefinition(context.Background(), agent); err != nil {
			t.Fatal(err)
		}
	}
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		if req.Mode != "assistant_recommendation" || req.AgentName != "助理推薦" {
			t.Fatalf("unexpected recommendation runtime request: %+v", req)
		}
		if !strings.Contains(req.Message, "薪資助理") || strings.Contains(req.Message, "未發佈助理") {
			t.Fatalf("recommendation prompt did not contain only visible published assistants: %s", req.Message)
		}
		if len(req.Tools) != 0 {
			t.Fatalf("recommendation mode should not expose business tools: %+v", req.Tools)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "建議使用薪資助理。"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "誰能解釋薪資差異？", Mode: "assistant_recommendation"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Answer != "建議使用薪資助理。" || run.SessionID == "" {
		t.Fatalf("unexpected recommendation run: %+v", run)
	}
	session, ok, err := store.GetAgentSession(context.Background(), "tenant-1", run.SessionID)
	if err != nil || !ok {
		t.Fatalf("expected recommendation session, ok=%v err=%v", ok, err)
	}
	if session.Title != "誰能解釋薪資差異？" {
		t.Fatalf("expected user text to remain the conversation title, got %q", session.Title)
	}
}

func TestAgentChatRecommendationFallsBackToVisibleCatalog(t *testing.T) {
	now := time.Date(2026, 7, 13, 11, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	for _, agent := range []domain.AgentDefinition{
		{
			ID: "agent-leave", TenantID: "tenant-1", Name: "請假流程助理", Description: "協助員工請假申請與流程說明",
			Category: domain.AgentCategoryWorkflow, Status: domain.AgentDefinitionStatusPublished,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "agent-sales", TenantID: "tenant-1", Name: "業務分析助理", Description: "分析銷售與業績報表",
			Category: domain.AgentCategoryAnalytics, Status: domain.AgentDefinitionStatusPublished,
			Visibility: domain.AgentVisibilityAll, CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertAgentDefinition(context.Background(), agent); err != nil {
			t.Fatal(err)
		}
	}
	runtime := fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
		return errors.New("model unavailable")
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	events := []domain.AgentChatEvent{}

	run, err := agentservice.New(svc).Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AgentChatInput{Message: "請推薦處理員工請假的助理", Mode: "assistant_recommendation"},
		func(_ context.Context, event domain.AgentChatEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != string(domain.AgentRunStatusCompleted) || !strings.Contains(run.Answer, "請假流程助理") {
		t.Fatalf("expected completed catalog fallback, got %+v", run)
	}
	if len(events) != 3 || events[1].Event != domain.AgentChatEventMessageDelta || events[2].Event != domain.AgentChatEventDone {
		t.Fatalf("unexpected fallback events: %+v", events)
	}
	messages, err := store.ListAgentSessionMessages(context.Background(), "tenant-1", run.SessionID, domain.KeysetPage{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Role != domain.AgentMessageRoleAssistant || messages[1].Content != run.Answer {
		t.Fatalf("expected persisted fallback answer, got %+v", messages)
	}
}

func TestAgentChatReadOnlyToolsUseRequestContext(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "me", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "get_my_profile", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			profile, err := req.Tools["get_my_profile"](ctx, map[string]any{
				"tenant_id":  "attacker-tenant",
				"account_id": "attacker-account",
			})
			if err != nil {
				return err
			}
			account, _ := profile["account"].(map[string]any)
			if account["id"] != "acct-1" {
				t.Fatalf("tool trusted forged args instead of request context: %+v", profile)
			}
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})

	if _, err := agentservice.New(svc).Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AgentChatInput{Message: "我的資料"},
		func(context.Context, domain.AgentChatEvent) error { return nil },
	); err != nil {
		t.Fatal(err)
	}
}

// TestAgentChatPersonalLeaveDoesNotRequireBalance verifies ordinary personal leave stays non-blocking.
func TestAgentChatPersonalLeaveDoesNotRequireBalance(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, agentLeaveToolPermissions("self"))
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		balances, err := req.Tools["my_leave_balances"](ctx, nil)
		if err != nil {
			return err
		}
		if balances["employee_id"] != "emp-1" || balances["initialized"] != false || balances["status"] != "not_initialized" || balances["total"] != 0 {
			t.Fatalf("missing balance rows were not reported as uninitialized: %+v", balances)
		}
		eligibility, err := req.Tools["check_leave_eligibility"](ctx, map[string]any{
			"leave_type": "事假", "date": "2026-07-15", "hours": float64(8),
		})
		if err != nil {
			return err
		}
		if eligibility["eligible"] != true || eligibility["balance_required"] != false || eligibility["policy_balance_required"] != false || eligibility["status"] != "eligible" || eligibility["reason"] != "balance_not_required" {
			t.Fatalf("personal leave unexpectedly required a balance: %+v", eligibility)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	if _, err := agentservice.New(svc).Chat(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.AgentChatInput{Message: "申請事假"}, nil); err != nil {
		t.Fatal(err)
	}
}

// TestAgentChatLeaveEligibilityFallsBackFromRealZeroBalance verifies zero balance still permits a draft.
func TestAgentChatLeaveEligibilityFallsBackFromRealZeroBalance(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, agentLeaveToolPermissions("self"))
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-annual", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 0, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		eligibility, err := req.Tools["check_leave_eligibility"](ctx, map[string]any{
			"leave_type": "annual", "date": "2026-07-15", "hours": float64(8),
		})
		if err != nil {
			return err
		}
		if eligibility["eligible"] != true || eligibility["balance_initialized"] != true || eligibility["balance_required"] != false || eligibility["policy_balance_required"] != true || eligibility["balance_fallback_applied"] != true || eligibility["balance_fallback_reason"] != "insufficient_balance" || eligibility["status"] != "eligible_without_balance" || eligibility["remaining_hours"] != float64(0) {
			t.Fatalf("real zero balance did not fall back to a no-balance request: %+v", eligibility)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	if _, err := agentservice.New(svc).Chat(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.AgentChatInput{Message: "申請特休"}, nil); err != nil {
		t.Fatal(err)
	}
}

// TestAgentChatLeaveEligibilityAcceptsSufficientBalance verifies policy and balance produce a positive decision.
func TestAgentChatLeaveEligibilityAcceptsSufficientBalance(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, agentLeaveToolPermissions("self"))
	if err := store.UpsertLeaveBalance(context.Background(), domain.LeaveBalance{
		ID: "lb-annual", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 16 * 60, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		eligibility, err := req.Tools["check_leave_eligibility"](ctx, map[string]any{
			"leave_type": "特休", "date": "2026-07-15", "hours": float64(8),
		})
		if err != nil {
			return err
		}
		if eligibility["eligible"] != true || eligibility["balance_required"] != true || eligibility["policy_balance_required"] != true || eligibility["balance_fallback_applied"] != false || eligibility["balance_initialized"] != true || eligibility["status"] != "eligible" || eligibility["remaining_hours"] != float64(16) {
			t.Fatalf("sufficient annual leave balance was not selected for reservation: %+v", eligibility)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	if _, err := agentservice.New(svc).Chat(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.AgentChatInput{Message: "申請特休"}, nil); err != nil {
		t.Fatal(err)
	}
}

// TestAgentChatLeaveBalanceAdminScopeStaysSelfOnly verifies broad attendance permission cannot widen a my-* tool.
func TestAgentChatLeaveBalanceAdminScopeStaysSelfOnly(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, agentLeaveToolPermissions("all"))
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-2", TenantID: "tenant-1", Name: "Other Employee", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, balance := range []domain.LeaveBalance{
		{ID: "lb-self", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual", RemainingMinutes: 8 * 60, UpdatedAt: now},
		{ID: "lb-other", TenantID: "tenant-1", EmployeeID: "emp-2", LeaveType: "annual", RemainingMinutes: 80 * 60, UpdatedAt: now},
	} {
		if err := store.UpsertLeaveBalance(context.Background(), balance); err != nil {
			t.Fatal(err)
		}
	}
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		result, err := req.Tools["my_leave_balances"](ctx, nil)
		if err != nil {
			return err
		}
		items, ok := result["items"].([]domain.LeaveBalance)
		if !ok || len(items) != 1 || items[0].EmployeeID != "emp-1" || result["total"] != 1 {
			t.Fatalf("admin-scoped my_leave_balances leaked another employee: %+v", result)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	if _, err := agentservice.New(svc).Chat(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.AgentChatInput{Message: "查詢我的餘額"}, nil); err != nil {
		t.Fatal(err)
	}
}

// TestAgentChatAttendanceSummaryAndFormHistoryStaySelfScoped verifies consolidated attendance queries never widen to another account.
func TestAgentChatAttendanceSummaryAndFormHistoryStaySelfScoped(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "my_attendance_summary", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "my_form_history", Scope: "all"},
		{Resource: "attendance.clock", Action: "read", Scope: "self"},
		{Resource: "attendance.leave", Action: "read", Scope: "self"},
		{Resource: "workflow.form_template", Action: "read", Scope: "all"},
		{Resource: "workflow.form_instance", Action: "read", Scope: "self"},
	})
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-2", TenantID: "tenant-1", DisplayName: "Other User", EmployeeID: "emp-2", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-2", TenantID: "tenant-1", Name: "Other Employee", AccountID: "acct-2", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-leave", TenantID: "tenant-1", Key: "leave-request", Name: "Leave Request", Status: "published", CurrentVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, instance := range []domain.FormInstance{
		{ID: "fi-self", TenantID: "tenant-1", TemplateID: "ft-leave", ApplicantAccountID: "acct-1", Status: "approved", Payload: map[string]any{"hours": 8}, SubmittedAt: now, UpdatedAt: now},
		{ID: "fi-other", TenantID: "tenant-1", TemplateID: "ft-leave", ApplicantAccountID: "acct-2", Status: "approved", Payload: map[string]any{"hours": 80}, SubmittedAt: now, UpdatedAt: now},
	} {
		if err := store.UpsertFormInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
	}
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		summary, err := req.Tools["my_attendance_summary"](ctx, nil)
		if err != nil {
			return err
		}
		if summary["month"] != "2026-07" || summary["worked_hours"] != float64(0) {
			t.Fatalf("unexpected attendance summary: %+v", summary)
		}
		history, err := req.Tools["my_form_history"](ctx, map[string]any{"template_key": "leave-request"})
		if err != nil {
			return err
		}
		items, ok := history["items"].([]map[string]any)
		if !ok || len(items) != 1 || items[0]["id"] != "fi-self" || history["total"] != 1 {
			t.Fatalf("form history leaked another account or lost the self item: %+v", history)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "ok"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	if _, err := agentservice.New(svc).Chat(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.AgentChatInput{Message: "查看本月考勤和歷史請假"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAgentChatToolRequiresAgentToolPermission(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	runtime := fakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			_, err := req.Tools["get_my_profile"](ctx, nil)
			return err
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})

	run, err := agentservice.New(svc).Chat(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		domain.AgentChatInput{Message: "我的資料"},
		func(context.Context, domain.AgentChatEvent) error { return nil },
	)
	if err == nil {
		t.Fatal("expected missing tool permission to fail chat")
	}
	if run.Status != string(domain.AgentRunStatusFailed) || run.Answer == "" {
		t.Fatalf("expected failed run to be persisted with error answer, got %+v", run)
	}
}

func TestAgentChatSanitizesRuntimeFailure(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	const rawFailure = "upstream 500 token=secret-value"
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now },
		AgentChatRuntime: fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
			return errors.New(rawFailure)
		}},
	})
	ctx := domain.RequestContext{
		TenantID: "tenant-1", AccountID: "acct-1", RequestID: "request-agent-runtime", TraceID: "trace-agent-runtime",
	}

	run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "hello"}, nil)
	if err == nil {
		t.Fatal("expected runtime failure")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.ReasonCode != agentservice.AgentRuntimeFailureReasonCode || appErr.TraceID != ctx.TraceID {
		t.Fatalf("expected safe runtime app error, got %#v", err)
	}
	if strings.Contains(err.Error(), rawFailure) || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("runtime error leaked through service boundary: %v", err)
	}
	if run.Status != string(domain.AgentRunStatusFailed) ||
		!strings.Contains(run.Answer, agentservice.AgentRuntimeFailureMessage) ||
		!strings.Contains(run.Answer, "reason_code="+agentservice.AgentRuntimeFailureReasonCode) ||
		!strings.Contains(run.Answer, "trace_id="+ctx.TraceID) ||
		strings.Contains(run.Answer, rawFailure) {
		t.Fatalf("expected sanitized failed run, got %+v", run)
	}
	stored, err := store.ListAgentRunsByAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Answer != run.Answer || strings.Contains(stored[0].Answer, "secret-value") {
		t.Fatalf("runtime failure leaked into run history: %+v", stored)
	}
	messages, err := store.ListAgentSessionMessages(context.Background(), "tenant-1", run.SessionID, domain.KeysetPage{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != domain.AgentMessageRoleUser || messages[1].Role != domain.AgentMessageRoleAssistant {
		t.Fatalf("expected user and assistant failure messages, got %+v", messages)
	}
	failureMessage := messages[1]
	if failureMessage.Content != run.Answer || failureMessage.Metadata["status"] != "failed" || failureMessage.Metadata["reason_code"] != agentservice.AgentRuntimeFailureReasonCode {
		t.Fatalf("expected a replayable assistant failure marker, got %+v", failureMessage)
	}
	if strings.Contains(failureMessage.Content, rawFailure) || strings.Contains(fmt.Sprint(failureMessage.Metadata), "secret-value") {
		t.Fatalf("runtime failure leaked into assistant history: %+v", failureMessage)
	}
	session, ok, err := store.GetAgentSession(context.Background(), "tenant-1", run.SessionID)
	if err != nil || !ok || session.LastMessageAt == nil {
		t.Fatalf("expected failed conversation to update session activity, ok=%v session=%+v err=%v", ok, session, err)
	}
}

func TestAgentChatBlocksActiveRunInSameSession(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now },
		AgentChatRuntime: fakeAgentChatRuntime{run: func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error {
			t.Fatal("runtime should not be called while an active run exists")
			return nil
		}},
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := agentservice.New(svc).CreateSession(ctx, domain.CreateAgentSessionInput{Title: "Busy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentRun(context.Background(), domain.AgentRun{
		ID:        "active-run",
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		SessionID: session.ID,
		Mode:      "assistant_chat",
		Prompt:    "busy",
		Status:    string(domain.AgentRunStatusRunning),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{SessionID: session.ID, Message: "hi"}, nil); err == nil {
		t.Fatal("expected active run conflict")
	}
}

func TestAgentChatRecoversStaleRunInSameSession(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	runtimeCalls := 0
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now },
		AgentChatRuntime: fakeAgentChatRuntime{run: func(ctx context.Context, _ service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			runtimeCalls++
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "recovered"})
		}},
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := agentservice.New(svc).CreateSession(ctx, domain.CreateAgentSessionInput{Title: "Interrupted"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentRun(context.Background(), domain.AgentRun{
		ID:        "stale-run",
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		SessionID: session.ID,
		Mode:      "assistant_chat",
		Prompt:    "old message",
		Status:    string(domain.AgentRunStatusRunning),
		CreatedAt: now.Add(-2 * time.Minute),
		UpdatedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{SessionID: session.ID, Message: "retry"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeCalls != 1 || run.Status != string(domain.AgentRunStatusCompleted) || run.Answer != "recovered" {
		t.Fatalf("expected the conversation to recover, run=%+v calls=%d", run, runtimeCalls)
	}
	runs, err := store.ListAgentRunsByAccount(context.Background(), "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range runs {
		if item.ID == "stale-run" {
			if item.Status != string(domain.AgentRunStatusFailed) || item.Answer != "agent chat was interrupted before completion" {
				t.Fatalf("expected stale run to be failed with an interruption reason, got %+v", item)
			}
			return
		}
	}
	t.Fatal("stale run was not persisted")
}

func TestAgentChatUsesSessionBoundAgentAndRejectsAgentSwitch(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	boundModel := domain.AgentModel{
		ID: "model-bound", TenantID: "tenant-1", Name: "Bound Model", ModelName: "gpt-bound",
		LiteLLMModel: "openai/gpt-bound", Status: domain.AgentModelStatusActive, TimeoutSeconds: 45,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.UpsertAgentModel(context.Background(), boundModel); err != nil {
		t.Fatal(err)
	}
	boundRevision := domain.AgentDefinitionVersion{
		ID: "arev-bound", TenantID: "tenant-1", AgentID: "agent-bound", Version: 1,
		Name: "Bound Agent", Category: domain.AgentCategoryWorkflow, Visibility: domain.AgentVisibilityAll,
		SystemPrompt: "Bound system prompt", ModelID: boundModel.ID,
		ModelConfigChecksum: domain.AgentModelSyncConfigHash(boundModel),
		TimeoutSeconds:      30, ConfigSchemaVersion: 1, Checksum: "bound-checksum", CreatedAt: now,
	}
	if err := store.InsertAgentDefinitionVersion(context.Background(), boundRevision); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
		ID: "agent-bound", TenantID: "tenant-1", DraftRevisionID: boundRevision.ID, PublishedRevisionID: boundRevision.ID,
		Name: boundRevision.Name, Category: boundRevision.Category, ModelID: boundRevision.ModelID,
		SystemPrompt: boundRevision.SystemPrompt, Status: domain.AgentDefinitionStatusPublished,
		Visibility: boundRevision.Visibility, TimeoutSeconds: boundRevision.TimeoutSeconds,
		Version: 1, PublishedVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentSession(context.Background(), domain.AgentSession{
		ID: "session-bound", TenantID: "tenant-1", AccountID: "acct-1", AgentID: "agent-bound",
		Title: "Bound conversation", Status: domain.AgentSessionStatusActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	runtimeCalls := 0
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		runtimeCalls++
		if req.ModelName != "openai/gpt-bound" || !strings.Contains(req.Message, "Bound system prompt") {
			t.Fatalf("session did not restore its bound agent config: %+v", req)
		}
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 44*time.Second || remaining > 46*time.Second {
			t.Fatalf("expected the model's 45 second timeout to win over the legacy agent value")
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "bound answer"})
	}}})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{SessionID: "session-bound", Message: "continue"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.AgentID != "agent-bound" || runtimeCalls != 1 {
		t.Fatalf("expected bound agent run, got run=%+v calls=%d", run, runtimeCalls)
	}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{SessionID: "session-bound", AgentID: "other-agent", Message: "switch"}, nil); err == nil {
		t.Fatal("expected switching the agent bound to an existing session to fail")
	}
	if runtimeCalls != 1 {
		t.Fatalf("runtime ran for a mismatched agent: %d", runtimeCalls)
	}
}

func TestAgentChatClearContextDropsPreviousHistoryFromPrompt(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	call := 0
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		call++
		if call == 2 {
			if len(req.History) != 0 {
				t.Fatalf("expected history after clear to be empty, got %+v", req.History)
			}
			if strings.Contains(req.Message, "first question") || strings.Contains(req.Message, "first answer") {
				t.Fatalf("prompt retained history before clear: %s", req.Message)
			}
		}
		answer := "first answer"
		if call == 2 {
			answer = "second answer"
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: answer})
	}}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	first, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{Message: "first question"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := agentservice.New(svc).ClearSessionContext(ctx, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ContextVersion != 2 || cleared.LastMessageAt != nil {
		t.Fatalf("expected clear to advance the visible context partition, got %+v", cleared)
	}
	visiblePage, err := agentservice.New(svc).ListSessionMessages(ctx, first.SessionID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	visible := visiblePage.Items
	if len(visible) != 0 {
		t.Fatalf("expected messages before clear to be invisible, got %+v", visible)
	}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{SessionID: first.SessionID, Message: "second question"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAgentChatFilesAreBoundToMessagesAndHiddenAfterClear(t *testing.T) {
	now := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
	})
	objectStore := service.NewMemoryObjectStore()
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		if !strings.Contains(req.Message, "quarterly revenue is 42") {
			t.Fatalf("expected parsed attachment content in runtime context, got %q", req.Message)
		}
		if !strings.Contains(req.Message, "untrusted user-provided data") {
			t.Fatalf("expected attachment content to be marked untrusted, got %q", req.Message)
		}
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "file reviewed"})
	}}
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now }, AgentChatRuntime: runtime, ObjectStore: objectStore,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := agentservice.New(svc).CreateSession(ctx, domain.CreateAgentSessionInput{Title: "File review"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := agentservice.New(svc).UploadSessionFile(ctx, session.ID, domain.UploadAgentSessionFileInput{
		Filename: "report.txt", ContentType: "text/plain", Content: []byte("quarterly revenue is 42"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.State != "draft" || file.ContextVersion != 1 {
		t.Fatalf("unexpected staged file: %+v", file)
	}
	download, err := agentservice.New(svc).DownloadSessionFile(ctx, session.ID, file.ID)
	if err != nil || string(download.Content) != "quarterly revenue is 42" {
		t.Fatalf("unexpected staged download: content=%q err=%v", download.Content, err)
	}
	if _, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{
		SessionID: session.ID, Message: "review this", AttachmentIDs: []string{file.ID},
	}, nil); err != nil {
		t.Fatal(err)
	}
	messagePage, err := agentservice.New(svc).ListSessionMessages(ctx, session.ID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	messages := messagePage.Items
	if len(messages) != 2 || len(messages[0].Attachments) != 1 || messages[0].Attachments[0].ID != file.ID {
		t.Fatalf("expected turn-level attachment provenance, got %+v", messages)
	}
	rawMessage, err := json.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawMessage), "object_key") || strings.Contains(string(rawMessage), file.ObjectKey) {
		t.Fatalf("storage key must not be serialized through message attachments: %s", rawMessage)
	}
	if _, err := agentservice.New(svc).ClearSessionContext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	messagePage, err = agentservice.New(svc).ListSessionMessages(ctx, session.ID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	messages = messagePage.Items
	if len(messages) != 0 {
		t.Fatalf("expected cleared messages to be invisible, got %+v", messages)
	}
	files, err := agentservice.New(svc).ListSessionFiles(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected cleared files to be invisible, got %+v", files)
	}
	if _, err := agentservice.New(svc).DownloadSessionFile(ctx, session.ID, file.ID); err == nil {
		t.Fatal("expected old context file download to be hidden")
	}
}

// TestAgentChatClearCannotSplitAnActiveRunAcrossContextVersions verifies clear waits for a complete turn boundary.
func TestAgentChatClearCannotSplitAnActiveRunAcrossContextVersions(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentChatAccount(t, store, now, []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.run", Action: "read", Scope: "all"},
	})
	started := make(chan struct{})
	release := make(chan struct{})
	runtime := fakeAgentChatRuntime{run: func(ctx context.Context, _ service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		close(started)
		<-release
		return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "done"})
	}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AgentChatRuntime: runtime})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	session, err := agentservice.New(svc).CreateSession(ctx, domain.CreateAgentSessionInput{Title: "active turn"})
	if err != nil {
		t.Fatal(err)
	}
	type chatResult struct {
		run domain.AgentRun
		err error
	}
	result := make(chan chatResult, 1)
	go func() {
		run, err := agentservice.New(svc).Chat(ctx, domain.AgentChatInput{SessionID: session.ID, Message: "hello"}, nil)
		result <- chatResult{run: run, err: err}
	}()
	<-started
	if _, err := agentservice.New(svc).ClearSessionContext(ctx, session.ID); err == nil {
		t.Fatal("expected clear to reject while the session has an active run")
	}
	close(release)
	completed := <-result
	if completed.err != nil || completed.run.Status != string(domain.AgentRunStatusCompleted) {
		t.Fatalf("expected the active turn to complete atomically, run=%+v err=%v", completed.run, completed.err)
	}
	cleared, err := agentservice.New(svc).ClearSessionContext(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ContextVersion != session.ContextVersion+1 {
		t.Fatalf("expected context version to advance after the turn, got %+v", cleared)
	}
	messagePage, err := agentservice.New(svc).ListSessionMessages(ctx, session.ID, domain.ListAgentSessionMessagesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	messages := messagePage.Items
	if len(messages) != 0 {
		t.Fatalf("expected the completed old-version turn to be hidden, got %+v", messages)
	}
}

type fakeAgentChatRuntime struct {
	run func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error
}

func (f fakeAgentChatRuntime) RunAgentChat(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
	return f.run(ctx, req, emit)
}

func seedAgentChatAccount(t *testing.T, store *memory.Store, now time.Time, permissions []domain.Permission) {
	t.Helper()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-agent-chat",
		TenantID:    "tenant-1",
		Name:        "Agent Chat",
		Permissions: permissions,
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{ID: "emp-1", TenantID: "tenant-1", Name: "Employee One", AccountID: "acct-1", Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Agent User",
		Email:                  "agent@example.com",
		EmployeeID:             "emp-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-agent-chat"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
}

// agentLeaveToolPermissions returns the business and tool permissions needed by leave-tool tests.
func agentLeaveToolPermissions(attendanceScope domain.Scope) []domain.Permission {
	return []domain.Permission{
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "my_leave_balances", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "check_leave_eligibility", Scope: "all"},
		{Resource: "attendance.leave", Action: "read", Scope: attendanceScope},
	}
}

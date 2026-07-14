package v1_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestAgentChatEndpointReturnsUnavailableWhenDisabled(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	handler := v1api.New(service.New(store), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when agent chat disabled, got %d: %s", rec.Code, rec.Body.String())
	}
	errPayload := decodeError(t, rec.Body.Bytes())
	if errPayload.Code != domain.ErrorCodeAgentChatDisabled || errPayload.ReasonCode != "agent_chat_disabled" {
		t.Fatalf("expected agent_chat_disabled reason, got %+v", errPayload)
	}
}

func TestAgentChatEndpointStreamsSSE(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	populateDemoFixture(store)
	runtime := apiFakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "streamed answer"})
		},
	}
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		AgentChatRuntime: runtime,
	})
	handler := v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"message":"分析一下","mode":"assistant_chat"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for chat stream, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	body := rec.Body.String()
	for _, expected := range []string{
		"event: session\n",
		"event: message_delta\n",
		`data: {"delta":"streamed answer"}`,
		"event: done\n",
		`"status":"completed"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected SSE body to contain %q, got:\n%s", expected, body)
		}
	}
}

// TestAgentSessionFileEndpointsHidePreviousContext validates upload, authorized download, and clear visibility at the HTTP boundary.
func TestAgentSessionFileEndpointsHidePreviousContext(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	svc := service.New(store, service.Options{ObjectStore: service.NewMemoryObjectStore()})
	handler := v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/agents/sessions", strings.NewReader(`{"title":"附件对话"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected session creation, got %d: %s", createRec.Code, createRec.Body.String())
	}
	session := decodeData[domain.AgentSession](t, createRec.Body.Bytes())

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("context from sftpgo")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/agents/sessions/"+session.ID+"/files", &form)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("expected file upload, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}
	if strings.Contains(uploadRec.Body.String(), "object_key") ||
		strings.Contains(uploadRec.Body.String(), "object_provider") ||
		strings.Contains(uploadRec.Body.String(), "object_bucket") ||
		strings.Contains(uploadRec.Body.String(), "tenants/demo/") {
		t.Fatalf("storage coordinates must stay private: %s", uploadRec.Body.String())
	}
	file := decodeData[domain.AgentSessionFile](t, uploadRec.Body.Bytes())

	downloadPath := "/v1/agents/sessions/" + session.ID + "/files/" + file.ID
	downloadReq := httptest.NewRequest(http.MethodGet, downloadPath, nil)
	downloadRec := httptest.NewRecorder()
	handler.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK || downloadRec.Body.String() != "context from sftpgo" {
		t.Fatalf("expected authorized file bytes, got %d: %q", downloadRec.Code, downloadRec.Body.String())
	}

	clearReq := httptest.NewRequest(http.MethodPost, "/v1/agents/sessions/"+session.ID+"/clear-context", nil)
	clearRec := httptest.NewRecorder()
	handler.ServeHTTP(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected context clear, got %d: %s", clearRec.Code, clearRec.Body.String())
	}
	hiddenReq := httptest.NewRequest(http.MethodGet, downloadPath, nil)
	hiddenRec := httptest.NewRecorder()
	handler.ServeHTTP(hiddenRec, hiddenReq)
	if hiddenRec.Code != http.StatusNotFound {
		t.Fatalf("expected old context file to be hidden, got %d: %s", hiddenRec.Code, hiddenRec.Body.String())
	}
}

func TestAgentConfirmationEndpointSubmitsPreparedDraft(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	populateDemoFixture(store)
	confirmationID := ""
	runtime := apiFakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			draftResult, err := req.Tools["create_form_draft"](ctx, map[string]any{
				"template_key": "leave-request",
				"payload": map[string]any{
					"leave_type": "annual", "start_at": "2026-07-14T09:00:00+08:00", "end_at": "2026-07-14T18:00:00+08:00",
					"hours": 8, "proxy": "emp-zxy1", "reason": "家庭安排",
				},
			})
			if err != nil {
				return err
			}
			draft := draftResult["draft"].(domain.FormInstance)
			preview, err := req.Tools["preview_form_submission"](ctx, map[string]any{"draft_id": draft.ID})
			if err != nil {
				return err
			}
			confirmationID = preview["confirmation"].(*domain.AgentConfirmation).ID
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "请确认提交"})
		},
	}
	workflowClient := &apiFakeFormApprovalWorkflowClient{started: map[string]domain.FormApprovalWorkflowStart{}}
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now }, AgentChatRuntime: runtime, FormApprovalWorkflows: workflowClient,
	})
	workflowClient.service = svc
	handler := v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-employee", TenantID: "demo", AccountID: "acct-employee"}, ok: true},
	}).Routes()

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"message":"帮我提交请假单"}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK || confirmationID == "" || !strings.Contains(chatRec.Body.String(), "event: confirmation_required") {
		t.Fatalf("expected chat confirmation, status=%d id=%q body=%s", chatRec.Code, confirmationID, chatRec.Body.String())
	}

	executeReq := httptest.NewRequest(http.MethodPost, "/v1/agents/confirmations/"+confirmationID+"/execute", strings.NewReader(`{}`))
	executeReq.Header.Set("Content-Type", "application/json")
	executeRec := httptest.NewRecorder()
	handler.ServeHTTP(executeRec, executeReq)
	if executeRec.Code != http.StatusOK || !strings.Contains(executeRec.Body.String(), `"status":"completed"`) || !strings.Contains(executeRec.Body.String(), `"status":"in_review"`) {
		t.Fatalf("expected confirmed submission, status=%d body=%s", executeRec.Code, executeRec.Body.String())
	}
}

type apiFakeAgentChatRuntime struct {
	run func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error
}

func (f apiFakeAgentChatRuntime) RunAgentChat(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
	return f.run(ctx, req, emit)
}

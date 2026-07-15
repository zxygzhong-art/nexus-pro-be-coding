package v1_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-cache, no-transform" {
		t.Fatalf("expected streaming cache control, got %q", cacheControl)
	}
	if buffering := rec.Header().Get("X-Accel-Buffering"); buffering != "no" {
		t.Fatalf("expected proxy buffering disabled, got %q", buffering)
	}
	if connection := rec.Header().Get("Connection"); connection != "" {
		t.Fatalf("expected no hop-by-hop connection header, got %q", connection)
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

func TestAgentChatEndpointSanitizesRuntimeFailure(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	const rawFailure = "upstream 500 token=secret-value"
	runtime := apiFakeAgentChatRuntime{run: func(ctx context.Context, _ service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
		if err := emit(ctx, domain.AgentChatEvent{
			Event: domain.AgentChatEventError, Message: rawFailure, Data: map[string]any{"provider_error": rawFailure},
		}); err != nil {
			return err
		}
		return errors.New(rawFailure)
	}}
	handler := v1api.New(service.New(store, service.Options{AgentChatRuntime: runtime}), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "trace-agent-runtime")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected streamed runtime error to keep 200 SSE status, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{
		"event: error\n",
		`"message":"` + service.AgentRuntimeFailureMessage + `"`,
		`"reason_code":"` + service.AgentRuntimeFailureReasonCode + `"`,
		`"trace_id":"trace-agent-runtime"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected sanitized SSE body to contain %q, got:\n%s", expected, body)
		}
	}
	if strings.Contains(body, rawFailure) || strings.Contains(body, "secret-value") {
		t.Fatalf("runtime failure leaked into SSE body: %s", body)
	}
	if count := strings.Count(body, "event: error\n"); count != 1 {
		t.Fatalf("expected one sanitized SSE error event, got %d:\n%s", count, body)
	}
	runs, err := store.ListAgentRunsByAccount(context.Background(), "demo", "acct-admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || strings.Contains(runs[0].Answer, "secret-value") || !strings.Contains(runs[0].Answer, "trace_id=trace-agent-runtime") {
		t.Fatalf("expected sanitized run history, got %+v", runs)
	}
}

// TestAgentChatEndpointStreamsSSEOverHTTP1 exercises net/http chunk framing instead of only a response recorder.
func TestAgentChatEndpointStreamsSSEOverHTTP1(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	runtime := apiFakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			return emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: "wire answer"})
		},
	}
	handler := v1api.New(service.New(store, service.Options{AgentChatRuntime: runtime}), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/agents/chat", strings.NewReader(`{"message":"wire probe"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read HTTP/1.1 SSE stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for HTTP/1.1 stream, got %d: %s", resp.StatusCode, body)
	}
	if len(resp.TransferEncoding) != 1 || resp.TransferEncoding[0] != "chunked" {
		t.Fatalf("expected one chunked transfer encoding, got %v", resp.TransferEncoding)
	}
	if resp.Header.Get("Connection") != "" {
		t.Fatalf("expected no upstream connection header, got %q", resp.Header.Get("Connection"))
	}
	if !strings.Contains(string(body), "event: message_delta\n") || !strings.Contains(string(body), `"delta":"wire answer"`) {
		t.Fatalf("expected decoded SSE body, got:\n%s", body)
	}
}

// TestAgentChatEndpointSerializesConcurrentSSEWrites verifies parallel tool events cannot corrupt the HTTP stream.
func TestAgentChatEndpointSerializesConcurrentSSEWrites(t *testing.T) {
	store := memory.NewStore()
	populateDemoFixture(store)
	runtime := apiFakeAgentChatRuntime{
		run: func(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
			const workers = 24
			start := make(chan struct{})
			errs := make(chan error, workers)
			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					errs <- emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolCall, Name: "parallel_tool", Status: "started"})
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	handler := v1api.New(service.New(store, service.Options{AgentChatRuntime: runtime}), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	recorder := newConcurrentWriteDetectingRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"message":"parallel wire probe"}`))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, req)

	if recorder.overlapped.Load() {
		t.Fatal("parallel agent events wrote to the HTTP response concurrently")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "event: done\n") {
		t.Fatalf("expected a completed SSE stream, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

type concurrentWriteDetectingRecorder struct {
	*httptest.ResponseRecorder
	activeWrites atomic.Int32
	overlapped   atomic.Bool
	recorderMu   sync.Mutex
}

// newConcurrentWriteDetectingRecorder builds a recorder that flags overlapping Write or Flush calls.
func newConcurrentWriteDetectingRecorder() *concurrentWriteDetectingRecorder {
	return &concurrentWriteDetectingRecorder{ResponseRecorder: httptest.NewRecorder()}
}

// Write widens the overlap window while keeping the underlying recorder internally safe.
func (r *concurrentWriteDetectingRecorder) Write(payload []byte) (int, error) {
	r.beginWrite()
	defer r.endWrite()
	r.recorderMu.Lock()
	defer r.recorderMu.Unlock()
	return r.ResponseRecorder.Write(payload)
}

// Flush participates in overlap detection because net/http chunk framing is finalized during flushes.
func (r *concurrentWriteDetectingRecorder) Flush() {
	r.beginWrite()
	defer r.endWrite()
	r.recorderMu.Lock()
	defer r.recorderMu.Unlock()
	r.ResponseRecorder.Flush()
}

// beginWrite records concurrent response activity and makes scheduling overlap deterministic.
func (r *concurrentWriteDetectingRecorder) beginWrite() {
	if r.activeWrites.Add(1) > 1 {
		r.overlapped.Store(true)
	}
	time.Sleep(time.Millisecond)
}

// endWrite closes one response activity window.
func (r *concurrentWriteDetectingRecorder) endWrite() {
	r.activeWrites.Add(-1)
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

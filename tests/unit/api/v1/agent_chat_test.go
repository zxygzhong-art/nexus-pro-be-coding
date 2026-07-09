package v1_test

import (
	"context"
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
	if errPayload.ReasonCode != "agent_chat_disabled" {
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
		AgentChatEnabled: true,
		AgentChatRuntime: runtime,
	})
	handler := v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{Provider: "keycloak", Subject: "acct-admin", TenantID: "demo", AccountID: "acct-admin"}, ok: true},
	}).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"message":"分析一下","mode":"assistant_chat"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Approval-Confirmed", "true")
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

type apiFakeAgentChatRuntime struct {
	run func(context.Context, service.AgentChatRuntimeRequest, service.AgentChatEmitFunc) error
}

func (f apiFakeAgentChatRuntime) RunAgentChat(ctx context.Context, req service.AgentChatRuntimeRequest, emit service.AgentChatEmitFunc) error {
	return f.run(ctx, req, emit)
}

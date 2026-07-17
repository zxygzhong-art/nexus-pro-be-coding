package service_test

import (
	"context"
	"iter"
	"strings"
	"sync"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// capturingLLM records every LLM request so tests can inspect the contents the
// ADK session provides to the model.
type capturingLLM struct {
	mu       sync.Mutex
	requests []*model.LLMRequest
}

func (m *capturingLLM) Name() string { return "capturing-model" }

func (m *capturingLLM) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{genai.NewPartFromText("好的")}},
		}, nil)
	}
}

func runADKChatForTest(t *testing.T, runtime *service.ADKAgentChatRuntime, req service.AgentChatRuntimeRequest) {
	t.Helper()
	emit := func(context.Context, domain.AgentChatEvent) error { return nil }
	if err := runtime.RunAgentChat(context.Background(), req, emit); err != nil {
		t.Fatal(err)
	}
}

func adkChatRequestForTest(sessionID, message string, history []domain.AgentSessionMessage) service.AgentChatRuntimeRequest {
	return service.AgentChatRuntimeRequest{
		RequestContext: domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		SessionID:      sessionID,
		Message:        message,
		History:        history,
	}
}

func requestTexts(req *model.LLMRequest) string {
	var parts []string
	for _, content := range req.Contents {
		for _, part := range content.Parts {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "|")
}

// TestADKRuntimeDeletesSessionAfterRun proves the in-memory ADK session is
// removed after every run: a later run of the same chat session must not
// replay earlier turns unless they are re-seeded from the DB history.
func TestADKRuntimeDeletesSessionAfterRun(t *testing.T) {
	llm := &capturingLLM{}
	runtime, err := service.NewADKAgentChatRuntime(llm)
	if err != nil {
		t.Fatal(err)
	}

	runADKChatForTest(t, runtime, adkChatRequestForTest("sess-1", "first turn question", nil))
	runADKChatForTest(t, runtime, adkChatRequestForTest("sess-1", "second turn question", nil))

	if len(llm.requests) != 2 {
		t.Fatalf("expected two LLM calls, got %d", len(llm.requests))
	}
	if got := requestTexts(llm.requests[1]); strings.Contains(got, "first turn question") {
		t.Fatalf("expected ADK session to be deleted between runs, second request replays turn one: %s", got)
	}
}

// TestADKRuntimeSeedsHistoryFromRequest proves DB-persisted history is seeded
// into the per-run ADK session so multi-turn context survives session cleanup.
func TestADKRuntimeSeedsHistoryFromRequest(t *testing.T) {
	llm := &capturingLLM{}
	runtime, err := service.NewADKAgentChatRuntime(llm)
	if err != nil {
		t.Fatal(err)
	}

	runADKChatForTest(t, runtime, adkChatRequestForTest("sess-1", "我叫什麼", []domain.AgentSessionMessage{
		{Role: domain.AgentMessageRoleUser, Content: "我叫小王"},
		{Role: domain.AgentMessageRoleAssistant, Content: "你叫小王"},
		{Role: domain.AgentMessageRoleSystem, Content: "context cleared marker"},
	}))

	if len(llm.requests) != 1 {
		t.Fatalf("expected one LLM call, got %d", len(llm.requests))
	}
	got := requestTexts(llm.requests[0])
	for _, want := range []string{"我叫小王", "你叫小王", "我叫什麼"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected seeded history and current message to reach the model, missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "context cleared marker") {
		t.Fatalf("system messages must not be seeded into the ADK session: %s", got)
	}
	contents := llm.requests[0].Contents
	if len(contents) < 3 || contents[len(contents)-1].Role != genai.RoleUser {
		t.Fatalf("expected the current message to be the final user content, got %+v", contents)
	}
}

package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/llm"
	"nexus-pro-be/internal/service"
)

func TestLiteLLMGenerateContent(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = body.Model
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "hello" {
			t.Fatalf("unexpected request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}

	var response *model.LLMResponse
	for item, err := range modelClient.GenerateContent(context.Background(), userRequest("hello", ""), false) {
		if err != nil {
			t.Fatal(err)
		}
		response = item
	}
	if gotModel != "nexus-agent-fallback" {
		t.Fatalf("unexpected model: %q", gotModel)
	}
	if textFromResponse(response) != "hi" || response.Partial || !response.TurnComplete {
		t.Fatalf("unexpected response: %+v", response)
	}
}

// TestLiteLLMPing verifies the fallback route and caches repeated readiness checks.
func TestLiteLLMPing(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected probe path: %s", r.URL.Path)
		}
		var body struct {
			Model               string `json:"model"`
			MaxCompletionTokens int    `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "nexus-agent-fallback" || body.MaxCompletionTokens != 1 {
			t.Fatalf("unexpected probe payload: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-probe","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key", ProbeTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := modelClient.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := modelClient.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one cached agent probe, got %d", calls.Load())
	}
}

// TestLiteLLMPingReportsProviderFailure verifies readiness fails when the runtime route is unusable.
func TestLiteLLMPingReportsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"provider unavailable"}}`, http.StatusBadGateway)
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key", ProbeTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := modelClient.Ping(context.Background()); err == nil {
		t.Fatal("expected an unusable agent runtime route to fail readiness")
	}
}

func TestLiteLLMGenerateContentStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"he\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"llo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}

	parts := []string{}
	for item, err := range modelClient.GenerateContent(context.Background(), userRequest("hello", "test-model"), true) {
		if err != nil {
			t.Fatal(err)
		}
		if item.Partial {
			parts = append(parts, textFromResponse(item))
		}
	}
	if len(parts) != 2 || parts[0] != "he" || parts[1] != "llo" {
		t.Fatalf("unexpected stream parts: %+v", parts)
	}
}

// TestLiteLLMGenerateContentToolCall 驗證系統指令、工具聲明與函數調用不會在適配層丟失。
func TestLiteLLMGenerateContentToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name       string         `json:"name"`
					Parameters map[string]any `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[0].Content != "必須使用工具" {
			t.Fatalf("system instruction was not preserved: %+v", body.Messages)
		}
		if len(body.Tools) != 1 || body.Tools[0].Function.Name != "create_form_draft" || body.Tools[0].Function.Parameters["type"] != "object" {
			t.Fatalf("tool declaration was not preserved: %+v", body.Tools)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-tool","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"create_form_draft","arguments":"{\"template_key\":\"leave-request\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	req := userRequest("我要請假", "test-model")
	req.Config = &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText("必須使用工具", "system"),
		Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{
			Name:                 "create_form_draft",
			Description:          "建立請假草稿",
			ParametersJsonSchema: map[string]any{"type": "object", "properties": map[string]any{"template_key": map[string]any{"type": "string"}}},
		}}}},
	}

	var response *model.LLMResponse
	for item, err := range modelClient.GenerateContent(context.Background(), req, false) {
		if err != nil {
			t.Fatal(err)
		}
		response = item
	}
	if response == nil || response.Content == nil || len(response.Content.Parts) != 1 || response.Content.Parts[0].FunctionCall == nil {
		t.Fatalf("expected ADK function call response, got %+v", response)
	}
	call := response.Content.Parts[0].FunctionCall
	if call.ID != "call-1" || call.Name != "create_form_draft" || call.Args["template_key"] != "leave-request" {
		t.Fatalf("unexpected ADK function call: %+v", call)
	}
}

// TestLiteLLMGenerateContentStreamToolCall 驗證流式參數片段會聚合為一次完整 ADK 工具調用。
func TestLiteLLMGenerateContentStreamToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-stream-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call-2\",\"type\":\"function\",\"function\":{\"name\":\"create_form_draft\",\"arguments\":\"{\\\"template_key\\\":\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-stream-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"leave-request\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}

	var response *model.LLMResponse
	for item, err := range modelClient.GenerateContent(context.Background(), userRequest("我要請假", "test-model"), true) {
		if err != nil {
			t.Fatal(err)
		}
		if item != nil && item.Content != nil && len(item.Content.Parts) > 0 && item.Content.Parts[0].FunctionCall != nil {
			response = item
		}
	}
	if response == nil || response.Content.Parts[0].FunctionCall.ID != "call-2" || response.Content.Parts[0].FunctionCall.Args["template_key"] != "leave-request" {
		t.Fatalf("expected accumulated stream tool call, got %+v", response)
	}
}

// TestLiteLLMADKToolLoop 驗證流式 tool call、tool response 與下一輪生成可以完整閉環。
func TestLiteLLMADKToolLoop(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if callCount > 1 {
			last := body.Messages[len(body.Messages)-1]
			if last.Role != "tool" || last.ToolCallID == "" {
				t.Fatalf("expected tool response before model continuation, messages=%+v", body.Messages)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch callCount {
		case 1:
			writeToolCallResponse(t, w, "call-list", "list_published_form_templates", `{}`)
		case 2:
			writeToolCallResponse(t, w, "call-get", "get_published_form_template", `{"template_key":"leave-request"}`)
		case 3:
			writeToolCallResponse(t, w, "call-create", "create_form_draft", `{"template_key":"leave-request","payload":{"leave_type":"annual","hours":8}}`)
		default:
			_, _ = w.Write([]byte(`{"id":"chatcmpl-final","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"請假草稿已生成。"},"finish_reason":"stop"}]}`))
		}
	}))
	defer server.Close()
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := service.NewADKAgentChatRuntime(modelClient)
	if err != nil {
		t.Fatal(err)
	}
	executed := []string{}
	tools := map[string]service.AgentTool{
		"list_published_form_templates": func(context.Context, map[string]any) (map[string]any, error) {
			executed = append(executed, "list_published_form_templates")
			return map[string]any{"items": []map[string]any{{"key": "leave-request"}}}, nil
		},
		"get_published_form_template": func(context.Context, map[string]any) (map[string]any, error) {
			executed = append(executed, "get_published_form_template")
			return map[string]any{"template": map[string]any{"key": "leave-request", "name": "請假申請單"}}, nil
		},
		"create_form_draft": func(context.Context, map[string]any) (map[string]any, error) {
			executed = append(executed, "create_form_draft")
			return map[string]any{"draft": map[string]any{"id": "fi-1", "status": "draft"}}, nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answer := strings.Builder{}
	err = runtime.RunAgentChat(ctx, service.AgentChatRuntimeRequest{
		RequestContext: domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		SessionID:      "session-1",
		ModelName:      "test-model",
		Message:        "幫我請年假",
		Tools:          tools,
	}, func(_ context.Context, event domain.AgentChatEvent) error {
		if event.Event == domain.AgentChatEventMessageDelta {
			answer.WriteString(event.Delta)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(executed, ",") != "list_published_form_templates,get_published_form_template,create_form_draft" {
		t.Fatalf("unexpected tool execution order: %+v", executed)
	}
	if callCount != 4 || answer.String() != "請假草稿已生成。" {
		t.Fatalf("unexpected model loop result: calls=%d answer=%q", callCount, answer.String())
	}
}

// TestLiteLLMADKParallelToolLoop 驗證同一 assistant 回合的多個工具調用會緊鄰配對響應。
func TestLiteLLMADKParallelToolLoop(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
				ToolCalls  []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}

		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			response := map[string]any{
				"id": "chatcmpl-parallel", "object": "chat.completion", "created": 1, "model": "test-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{
						{"id": "call-list", "type": "function", "function": map[string]any{"name": "list_published_form_templates", "arguments": `{}`}},
						{"id": "call-balance", "type": "function", "function": map[string]any{"name": "get_leave_balance", "arguments": `{}`}},
					}},
					"finish_reason": "tool_calls",
				}},
			}
			raw, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(raw)
			return
		}

		assistantIndex := -1
		for i, message := range body.Messages {
			if message.Role == "assistant" && len(message.ToolCalls) == 2 {
				assistantIndex = i
				break
			}
		}
		if assistantIndex < 0 || len(body.Messages) < assistantIndex+3 {
			t.Fatalf("parallel assistant call was not preserved: %+v", body.Messages)
		}
		responses := body.Messages[assistantIndex+1 : assistantIndex+3]
		gotIDs := map[string]bool{}
		for _, message := range responses {
			if message.Role != "tool" || message.ToolCallID == "" {
				t.Fatalf("tool responses must immediately follow the assistant call: %+v", body.Messages)
			}
			gotIDs[message.ToolCallID] = true
		}
		if !gotIDs["call-list"] || !gotIDs["call-balance"] {
			t.Fatalf("tool response IDs do not match assistant calls: %+v", body.Messages)
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-final","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"已取得請假表單和假期餘額。"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := service.NewADKAgentChatRuntime(modelClient)
	if err != nil {
		t.Fatal(err)
	}
	executed := make(chan string, 2)
	tools := map[string]service.AgentTool{
		"list_published_form_templates": func(context.Context, map[string]any) (map[string]any, error) {
			executed <- "list_published_form_templates"
			return map[string]any{"items": []map[string]any{{"key": "leave-request"}}}, nil
		},
		"get_leave_balance": func(context.Context, map[string]any) (map[string]any, error) {
			executed <- "get_leave_balance"
			return map[string]any{"annual_hours": 40}, nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = runtime.RunAgentChat(ctx, service.AgentChatRuntimeRequest{
		RequestContext: domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"},
		SessionID:      "session-parallel",
		ModelName:      "test-model",
		Message:        "幫我確認請假表單和餘額",
		Tools:          tools,
	}, func(context.Context, domain.AgentChatEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	close(executed)
	gotTools := map[string]bool{}
	for name := range executed {
		gotTools[name] = true
	}
	if !gotTools["list_published_form_templates"] || !gotTools["get_leave_balance"] {
		t.Fatalf("parallel tools did not both execute: %+v", gotTools)
	}
	if callCount != 2 {
		t.Fatalf("unexpected model call count: %d", callCount)
	}
}

// TestLiteLLMRetryAfterUnresolvedParallelToolCalls 驗證失敗回合不會汙染同一 session 的重試消息。
func TestLiteLLMRetryAfterUnresolvedParallelToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, message := range body.Messages {
			if len(message.ToolCalls) > 0 {
				http.Error(w, "unresolved tool call history reached the provider", http.StatusBadRequest)
				return
			}
		}
		last := body.Messages[len(body.Messages)-1]
		if last.Role != "user" || last.Content != "重試請假" {
			t.Fatalf("latest user retry was not preserved: %+v", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-retry","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"可以繼續處理。"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	request := userRequest("第一次請假", "test-model")
	danglingCalls := &genai.Content{Role: "model", Parts: []*genai.Part{
		{FunctionCall: &genai.FunctionCall{ID: "call-profile", Name: "get_my_profile", Args: map[string]any{}}},
		{FunctionCall: &genai.FunctionCall{ID: "call-balance", Name: "my_leave_balances", Args: map[string]any{}}},
	}}
	request.Contents = append(request.Contents, danglingCalls, genai.NewContentFromText("重試請假", "user"))

	var response *model.LLMResponse
	for item, err := range modelClient.GenerateContent(context.Background(), request, false) {
		if err != nil {
			t.Fatal(err)
		}
		response = item
	}
	if textFromResponse(response) != "可以繼續處理。" {
		t.Fatalf("unexpected retry response: %+v", response)
	}
}

// writeToolCallResponse 輸出一個 OpenAI-compatible 函數調用回應。
func writeToolCallResponse(t *testing.T, w http.ResponseWriter, id, name, arguments string) {
	t.Helper()
	chunk := map[string]any{
		"id": "chatcmpl-" + id, "object": "chat.completion", "created": 1, "model": "test-model",
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{
				"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": arguments},
			}}},
			"finish_reason": "tool_calls",
		}},
	}
	raw, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(raw)
}

// userRequest 建立可選擇模型覆寫的 ADK 請求，覆蓋默認與租戶模型 alias 兩條路徑。
func userRequest(text, modelName string) *model.LLMRequest {
	return &model.LLMRequest{
		Model: modelName,
		Contents: []*genai.Content{{
			Role:  "user",
			Parts: []*genai.Part{genai.NewPartFromText(text)},
		}},
	}
}

// textFromResponse 讀取 ADK 回應的第一個文字片段。
func textFromResponse(resp *model.LLMResponse) string {
	if resp == nil || resp.Content == nil || len(resp.Content.Parts) == 0 || resp.Content.Parts[0] == nil {
		return ""
	}
	return resp.Content.Parts[0].Text
}

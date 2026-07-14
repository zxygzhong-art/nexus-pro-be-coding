package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestLiteLLMGenerateContentToolCall 验证系统指令、工具声明与函数调用不会在适配层丢失。
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
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[0].Content != "必须使用工具" {
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
	req := userRequest("我要请假", "test-model")
	req.Config = &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText("必须使用工具", "system"),
		Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{
			Name:                 "create_form_draft",
			Description:          "建立请假草稿",
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

// TestLiteLLMGenerateContentStreamToolCall 验证流式参数片段会聚合为一次完整 ADK 工具调用。
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
	for item, err := range modelClient.GenerateContent(context.Background(), userRequest("我要请假", "test-model"), true) {
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

// TestLiteLLMADKToolLoop 验证流式 tool call、tool response 与下一轮生成可以完整闭环。
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
			_, _ = w.Write([]byte(`{"id":"chatcmpl-final","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"请假草稿已生成。"},"finish_reason":"stop"}]}`))
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
			return map[string]any{"template": map[string]any{"key": "leave-request", "name": "请假申请单"}}, nil
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
		Message:        "帮我请年假",
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
	if callCount != 4 || answer.String() != "请假草稿已生成。" {
		t.Fatalf("unexpected model loop result: calls=%d answer=%q", callCount, answer.String())
	}
}

// writeToolCallResponse 输出一个 OpenAI-compatible 函数调用回应。
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

// userRequest 建立可选择模型覆写的 ADK 请求，覆盖默认与租户模型 alias 两条路径。
func userRequest(text, modelName string) *model.LLMRequest {
	return &model.LLMRequest{
		Model: modelName,
		Contents: []*genai.Content{{
			Role:  "user",
			Parts: []*genai.Part{genai.NewPartFromText(text)},
		}},
	}
}

// textFromResponse 读取 ADK 回应的第一个文字片段。
func textFromResponse(resp *model.LLMResponse) string {
	if resp == nil || resp.Content == nil || len(resp.Content.Parts) == 0 || resp.Content.Parts[0] == nil {
		return ""
	}
	return resp.Content.Parts[0].Text
}

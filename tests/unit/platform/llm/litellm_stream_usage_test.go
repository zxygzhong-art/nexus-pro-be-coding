package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"nexus-pro-api/internal/platform/llm"
)

// TestLiteLLMStreamingPreservesUsage verifies the OpenAI stream usage contract end to end.
func TestLiteLLMStreamingPreservesUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		streamOptions, ok := payload["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Errorf("expected stream_options.include_usage, got %#v", payload["stream_options"])
			http.Error(w, "missing stream usage", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120,\"prompt_tokens_details\":{\"cached_tokens\":40}}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	adapter, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	var final *model.LLMResponse
	for response, responseErr := range adapter.GenerateContent(context.Background(), &model.LLMRequest{
		Model:    "test",
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{genai.NewPartFromText("hello")}}},
	}, true) {
		if responseErr != nil {
			t.Fatal(responseErr)
		}
		if response != nil && response.TurnComplete {
			final = response
		}
	}
	if final == nil || final.UsageMetadata == nil {
		t.Fatalf("expected final usage metadata, got %+v", final)
	}
	usage := final.UsageMetadata
	if usage.PromptTokenCount != 100 || usage.CachedContentTokenCount != 40 || usage.CandidatesTokenCount != 20 || usage.TotalTokenCount != 120 {
		t.Fatalf("unexpected usage metadata: %+v", usage)
	}
}

// TestFunctionParametersNormalizesGenAISchemaTypes protects OpenAI-compatible task-agent tools.
func TestFunctionParametersNormalizesGenAISchemaTypes(t *testing.T) {
	var parameters map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Tools []struct {
				Function struct {
					Parameters map[string]any `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Tools) != 1 {
			t.Fatalf("tools = %#v, want one declaration", payload.Tools)
		}
		parameters = payload.Tools[0].Function.Parameters
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-schema","object":"chat.completion","created":1,"model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	declaration := &genai.FunctionDeclaration{
		Name: "sub_agent_1",
		Parameters: &genai.Schema{
			Type: "OBJECT",
			Properties: map[string]*genai.Schema{
				"request": {Type: "STRING"},
			},
			Required: []string{"request"},
		},
	}

	adapter, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	request := &model.LLMRequest{
		Model:    "test",
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{genai.NewPartFromText("hello")}}},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{declaration}}},
		},
	}
	for _, responseErr := range adapter.GenerateContent(context.Background(), request, false) {
		if responseErr != nil {
			t.Fatal(responseErr)
		}
	}
	if got := parameters["type"]; got != "object" {
		t.Fatalf("root schema type = %v, want object", got)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v, want map", parameters["properties"])
	}
	requestSchema, ok := properties["request"].(map[string]any)
	if !ok {
		t.Fatalf("request schema = %#v, want map", properties["request"])
	}
	if got := requestSchema["type"]; got != "string" {
		t.Fatalf("request schema type = %v, want string", got)
	}
}

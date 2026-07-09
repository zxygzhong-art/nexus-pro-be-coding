//go:build adk

package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"nexus-pro-be/internal/platform/llm"
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
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	var response *model.LLMResponse
	for item, err := range modelClient.GenerateContent(context.Background(), userRequest("hello"), false) {
		if err != nil {
			t.Fatal(err)
		}
		response = item
	}
	if gotModel != "test-model" {
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
	modelClient, err := llm.NewLiteLLM(llm.LiteLLMConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	parts := []string{}
	for item, err := range modelClient.GenerateContent(context.Background(), userRequest("hello"), true) {
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

func userRequest(text string) *model.LLMRequest {
	return &model.LLMRequest{
		Contents: []*genai.Content{{
			Role:  "user",
			Parts: []*genai.Part{genai.NewPartFromText(text)},
		}},
	}
}

func textFromResponse(resp *model.LLMResponse) string {
	if resp == nil || resp.Content == nil || len(resp.Content.Parts) == 0 || resp.Content.Parts[0] == nil {
		return ""
	}
	return resp.Content.Parts[0].Text
}

package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"nexus-pro-be/internal/platform/llm"
)

// TestLiteLLMEmbeddingClient verifies alias routing, authentication, and response ordering.
func TestLiteLLMEmbeddingClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected embedding request: path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "nexus-pro-embedding" || !reflect.DeepEqual(body.Input, []string{"first", "second"}) {
			t.Fatalf("unexpected embedding payload: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"upstream","data":[{"object":"embedding","index":1,"embedding":[0,1]},{"object":"embedding","index":0,"embedding":[1,0]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`))
	}))
	defer server.Close()

	client, err := llm.NewLiteLLMEmbeddingClient(llm.LiteLLMEmbeddingConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := client.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if client.Model() != "nexus-pro-embedding" || !reflect.DeepEqual(vectors, [][]float32{{1, 0}, {0, 1}}) {
		t.Fatalf("unexpected embedding result: model=%s vectors=%v", client.Model(), vectors)
	}
}

// TestLiteLLMEmbeddingClientRejectsInvalidResponse verifies dimension and cardinality checks.
func TestLiteLLMEmbeddingClientRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"upstream","data":[{"object":"embedding","index":0,"embedding":[1]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer server.Close()
	client, err := llm.NewLiteLLMEmbeddingClient(llm.LiteLLMEmbeddingConfig{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Embed(context.Background(), []string{"first", "second"}); err == nil {
		t.Fatal("expected an embedding cardinality error")
	}
}

package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"nexus-pro-api/internal/platform/llm"
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

// TestLiteLLMEmbeddingPing verifies real-route probing and bounded provider traffic.
func TestLiteLLMEmbeddingPing(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/embeddings" {
			t.Fatalf("unexpected probe path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"upstream","data":[{"object":"embedding","index":0,"embedding":[1,0]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer server.Close()

	client, err := llm.NewLiteLLMEmbeddingClient(llm.LiteLLMEmbeddingConfig{
		BaseURL: server.URL, APIKey: "test-key", ProbeTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one cached provider probe, got %d", calls.Load())
	}
}

// TestLiteLLMEmbeddingPingReportsProviderFailure verifies readiness fails on an unusable route.
func TestLiteLLMEmbeddingPingReportsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"provider unavailable"}}`, http.StatusBadGateway)
	}))
	defer server.Close()
	client, err := llm.NewLiteLLMEmbeddingClient(llm.LiteLLMEmbeddingConfig{
		BaseURL: server.URL, APIKey: "test-key", ProbeTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("expected an unusable embedding route to fail readiness")
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

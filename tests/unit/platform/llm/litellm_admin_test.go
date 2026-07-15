package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/llm"
)

// TestLiteLLMAdminSyncModelCreatesThenUpdates 驗證 stable ID 的 info/upsert 路徑與 payload。
func TestLiteLLMAdminSyncModelCreatesThenUpdates(t *testing.T) {
	exists := false
	upserts := make([]struct {
		path    string
		payload map[string]any
	}, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer master-key" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/model/info":
			if r.URL.RawQuery == "" {
				_, _ = w.Write([]byte(`{"data":[{"model_name":"nexus-agent-model-amodel-orphan","model_info":{"id":"amodel-orphan"}},{"model_name":"external-route","model_info":{"id":"external"}}]}`))
				return
			}
			if r.URL.Query().Get("litellm_model_id") != "amodel-1" {
				t.Fatalf("unexpected model info query: %s", r.URL.RawQuery)
			}
			if !exists {
				w.Header().Set("content-type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail":{"error":"Model id = amodel-1 not found on litellm proxy"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"model_name":"nexus-agent-model-amodel-1","model_info":{"id":"amodel-1"}}]}`))
		case "/model/new", "/model/update":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			upserts = append(upserts, struct {
				path    string
				payload map[string]any
			}{path: r.URL.Path, payload: payload})
			exists = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := llm.NewLiteLLMAdminClient(llm.LiteLLMAdminConfig{BaseURL: server.URL, MasterKey: "master-key", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	model := domain.AgentModel{
		ID:             "amodel-1",
		TenantID:       "tenant-1",
		Name:           "GPT Mini",
		Provider:       "openai",
		ModelName:      "gpt-4.1-mini",
		LiteLLMModel:   "user-controlled-alias",
		APIKey:         "sk-upstream",
		RateLimitRPM:   120,
		TimeoutSeconds: 45,
		Status:         domain.AgentModelStatusActive,
	}
	if _, err := client.SyncModel(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SyncModel(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	if len(upserts) != 2 || upserts[0].path != "/model/new" || upserts[1].path != "/model/update" {
		t.Fatalf("unexpected upsert paths: %+v", upserts)
	}
	if upserts[0].payload["model_name"] != domain.AgentModelLiteLLMAlias(model.ID) {
		t.Fatalf("expected stable alias, got %+v", upserts[0].payload)
	}
	params, _ := upserts[0].payload["litellm_params"].(map[string]any)
	if params["model"] != "openai/gpt-4.1-mini" || params["rpm"] != float64(120) || params["timeout"] != float64(45) {
		t.Fatalf("unexpected LiteLLM params: %+v", params)
	}
	info, _ := upserts[0].payload["model_info"].(map[string]any)
	if info["id"] != model.ID || info["tenant_id"] != model.TenantID {
		t.Fatalf("unexpected model info: %+v", info)
	}
	ids, err := client.ListManagedModelIDs(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != "amodel-orphan" {
		t.Fatalf("unexpected managed model ids: ids=%v err=%v", ids, err)
	}
}

// TestLiteLLMAdminDeleteModelTreatsMissingAsSuccess 驗證 delete 的冪等語義。
func TestLiteLLMAdminDeleteModelTreatsMissingAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/delete" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":{"error":"Model id = amodel-1 not found on litellm proxy"}}`))
	}))
	defer server.Close()
	client, err := llm.NewLiteLLMAdminClient(llm.LiteLLMAdminConfig{BaseURL: server.URL, MasterKey: "master-key", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteModel(context.Background(), "amodel-1"); err != nil {
		t.Fatalf("expected missing route deletion to be idempotent: %v", err)
	}
}

// TestLiteLLMAdminSyncModelPreservesUnrelatedBadRequest 驗證其他 400 不會被誤判為模型不存在。
func TestLiteLLMAdminSyncModelPreservesUnrelatedBadRequest(t *testing.T) {
	newCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/model/info":
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":{"error":"invalid model info request"}}`))
		case "/model/new":
			newCalls++
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := llm.NewLiteLLMAdminClient(llm.LiteLLMAdminConfig{BaseURL: server.URL, MasterKey: "master-key", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SyncModel(context.Background(), domain.AgentModel{ID: "amodel-1", Provider: "openai", ModelName: "gpt-4.1-mini"})
	if err == nil {
		t.Fatal("expected unrelated bad request to remain an error")
	}
	if newCalls != 0 {
		t.Fatalf("unexpected model creation after unrelated bad request: %d", newCalls)
	}
}

package openfga_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexus-pro-be/internal/domain"
	authzpkg "nexus-pro-be/internal/domain/authz"
	"nexus-pro-be/internal/platform/openfga"
)

func TestPingUsesHealthEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client())

	if err := checker.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/healthz" {
		t.Fatalf("path = %q, want /healthz", gotPath)
	}
}

func TestPingVerifiesConfiguredAuthorizationModel(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusNoContent)
		case "/stores/store-1/authorization-models/model-1":
			_ = json.NewEncoder(w).Encode(map[string]string{"authorization_model_id": "model-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client()).WithAuthorizationModelID("model-1")

	if err := checker.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/healthz" || paths[1] != "/stores/store-1/authorization-models/model-1" {
		t.Fatalf("unexpected readiness paths: %+v", paths)
	}
}

func TestPingFailsWhenAuthorizationModelIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusNoContent)
		case "/stores/store-1/authorization-models/model-missing":
			http.Error(w, "model missing", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client()).WithAuthorizationModelID("model-missing")

	err := checker.Ping(context.Background())
	if err == nil {
		t.Fatal("expected missing model readiness error")
	}
	for _, want := range []string{"authorization model", "model-missing", "status=404", "model missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestCheckRelationshipIncludesAuthorizationModelID(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client()).WithAuthorizationModelID("model-1")

	allowed, err := checker.CheckRelationship(context.Background(), authzpkg.RelationshipCheck{
		TenantID: "tenant-1",
		Subject:  "account:acct-1",
		Relation: "viewer",
		Object:   "agent.knowledge_article:ka-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("expected relationship check to allow")
	}
	if gotPayload["authorization_model_id"] != "model-1" {
		t.Fatalf("expected authorization_model_id in payload, got %+v", gotPayload)
	}
}

func TestWriteRelationshipTuplesPostsWritesAndDeletes(t *testing.T) {
	var gotPath string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client())

	err := checker.WriteRelationshipTuples(context.Background(), []domain.AuthzRelationshipTupleChange{
		{
			Operation: domain.AuthzRelationshipTupleWrite,
			Tuple: domain.AuthzRelationshipTuple{
				ObjectType:  "hr.employee",
				ObjectID:    "emp-1",
				Relation:    "owner",
				SubjectType: "account",
				SubjectID:   "acct-1",
			},
		},
		{
			Operation: domain.AuthzRelationshipTupleDelete,
			Tuple: domain.AuthzRelationshipTuple{
				ObjectType:  "hr.employee",
				ObjectID:    "emp-1",
				Relation:    "owner",
				SubjectType: "account",
				SubjectID:   "acct-old",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/stores/store-1/write" {
		t.Fatalf("path = %q, want /stores/store-1/write", gotPath)
	}
	writes := gotPayload["writes"].(map[string]any)["tuple_keys"].([]any)
	deletes := gotPayload["deletes"].(map[string]any)["tuple_keys"].([]any)
	if writes[0].(map[string]any)["user"] != "account:acct-1" || writes[0].(map[string]any)["object"] != "hr.employee:emp-1" {
		t.Fatalf("unexpected writes payload: %+v", gotPayload["writes"])
	}
	if deletes[0].(map[string]any)["user"] != "account:acct-old" || deletes[0].(map[string]any)["relation"] != "owner" {
		t.Fatalf("unexpected deletes payload: %+v", gotPayload["deletes"])
	}
}

func TestCheckRelationshipReturnsDetailedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client())

	_, err := checker.CheckRelationship(context.Background(), authzpkg.RelationshipCheck{
		TenantID: "tenant-1",
		Subject:  "account:acct-1",
		Relation: "viewer",
		Object:   "agent.knowledge_article:ka-1",
	})
	if err == nil {
		t.Fatal("expected OpenFGA check error")
	}
	for _, want := range []string{"openfga check failed", "status=503", "model not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestWriteRelationshipTuplesReturnsDetailedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid tuple", http.StatusBadRequest)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client())

	err := checker.WriteRelationshipTuples(context.Background(), []domain.AuthzRelationshipTupleChange{
		{
			Operation: domain.AuthzRelationshipTupleWrite,
			Tuple: domain.AuthzRelationshipTuple{
				ObjectType:  "hr.employee",
				ObjectID:    "emp-1",
				Relation:    "owner",
				SubjectType: "account",
				SubjectID:   "acct-1",
			},
		},
	})
	if err == nil {
		t.Fatal("expected OpenFGA write error")
	}
	for _, want := range []string{"openfga write failed", "status=400", "invalid tuple"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

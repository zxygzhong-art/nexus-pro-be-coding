package openfga_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/openfga"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestPingUsesHealthEndpoint 驗證 uses 健康檢查 endpoint。
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

// TestPingVerifiesConfiguredAuthorizationModel 驗證 verifies configured 授權 model。
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

// TestPingFailsWhenAuthorizationModelIsMissing 驗證 fails when 授權 model is missing。
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

// TestCheckRelationshipIncludesAuthorizationModelID 驗證關係 includes 授權 model ID。
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

	allowed, err := checker.CheckRelationship(context.Background(), domain.RelationshipCheck{
		TenantID: "tenant-1",
		Subject:  "account:acct-1",
		Relation: "viewer",
		Object:   "agent.run:run-1",
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

// TestWriteRelationshipTuplesPostsWritesAndDeletes 驗證關係 tuple posts writes and deletes。
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
		{
			Operation: domain.AuthzRelationshipTupleWrite,
			Tuple: domain.AuthzRelationshipTuple{
				ObjectType:  "assumable_role",
				ObjectID:    "role-1",
				Relation:    "trusted_group",
				SubjectType: "user_group#member",
				SubjectID:   "ug-1",
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
	if writes[1].(map[string]any)["user"] != "user_group:ug-1#member" || writes[1].(map[string]any)["relation"] != "trusted_group" {
		t.Fatalf("unexpected userset write payload: %+v", gotPayload["writes"])
	}
	if deletes[0].(map[string]any)["user"] != "account:acct-old" || deletes[0].(map[string]any)["relation"] != "owner" {
		t.Fatalf("unexpected deletes payload: %+v", gotPayload["deletes"])
	}
}

// TestWriteRelationshipTuplesFormatsUsersetSubject 驗證 userset subject 不需啟動 httptest server。
func TestWriteRelationshipTuplesFormatsUsersetSubject(t *testing.T) {
	var gotPayload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&gotPayload); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	})}
	checker := openfga.NewChecker("http://openfga.test", "store-1", client)

	err := checker.WriteRelationshipTuples(context.Background(), []domain.AuthzRelationshipTupleChange{
		{
			Operation: domain.AuthzRelationshipTupleWrite,
			Tuple: domain.AuthzRelationshipTuple{
				ObjectType:  "assumable_role",
				ObjectID:    "role-1",
				Relation:    "trusted_group",
				SubjectType: "user_group#member",
				SubjectID:   "ug-1",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writes := gotPayload["writes"].(map[string]any)["tuple_keys"].([]any)
	if writes[0].(map[string]any)["user"] != "user_group:ug-1#member" {
		t.Fatalf("unexpected userset subject payload: %+v", gotPayload)
	}
}

// TestWriteRelationshipTuplesTreatsReplayConflictsAsSuccess 驗證重放 tuple 衝突保持冪等。
func TestWriteRelationshipTuplesTreatsReplayConflictsAsSuccess(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate write": "tuple already exists",
		"missing delete":  "tuple not found",
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			})}
			checker := openfga.NewChecker("http://openfga.test", "store-1", client)

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
			if err != nil {
				t.Fatalf("expected idempotent tuple replay to succeed, got %v", err)
			}
		})
	}
}

// TestCheckRelationshipReturnsDetailedHTTPError 驗證關係 returns detailed HTTP 錯誤。
func TestCheckRelationshipReturnsDetailedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client())

	_, err := checker.CheckRelationship(context.Background(), domain.RelationshipCheck{
		TenantID: "tenant-1",
		Subject:  "account:acct-1",
		Relation: "viewer",
		Object:   "agent.run:run-1",
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

// TestWriteRelationshipTuplesReturnsDetailedHTTPError 驗證關係 tuple returns detailed HTTP 錯誤。
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

// TestCheckerSendsBearerTokenWhenConfigured 驗證設定 token 後所有 OpenFGA 請求帶 Authorization。
func TestCheckerSendsBearerTokenWhenConfigured(t *testing.T) {
	got := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got[r.Method+" "+r.URL.Path] = r.Header.Get("Authorization")
		if r.URL.Path == "/stores/store-1/check" {
			_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client()).WithAuthToken("secret-token")

	if err := checker.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := checker.CheckRelationship(context.Background(), domain.RelationshipCheck{
		TenantID: "tenant-1", Subject: "account:acct-1", Relation: "viewer", Object: "hr.employee:emp-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := checker.WriteRelationshipTuples(context.Background(), []domain.AuthzRelationshipTupleChange{{
		Operation: domain.AuthzRelationshipTupleWrite,
		Tuple: domain.AuthzRelationshipTuple{
			ObjectType: "hr.employee", ObjectID: "emp-1", Relation: "owner", SubjectType: "account", SubjectID: "acct-1",
		},
	}}); err != nil {
		t.Fatal(err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 recorded requests, got %+v", got)
	}
	for label, header := range got {
		if header != "Bearer secret-token" {
			t.Fatalf("%s: expected bearer token header, got %q", label, header)
		}
	}
}

// TestCheckerOmitsBearerTokenWhenEmpty 驗證未設定 token 時不帶 Authorization（向後相容）。
func TestCheckerOmitsBearerTokenWhenEmpty(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	checker := openfga.NewChecker(server.URL, "store-1", server.Client()).WithAuthToken("  ")

	if err := checker.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header, got %q", gotAuth)
	}
}

package openfga_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/openfga"
)

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

package v1_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

type apiRelationshipChecker struct {
	checks []domain.RelationshipCheck
	err    error
}

// CheckRelationship records unexpected API-level relationship checks and returns the configured result.
func (c *apiRelationshipChecker) CheckRelationship(_ context.Context, check domain.RelationshipCheck) (bool, error) {
	c.checks = append(c.checks, check)
	return false, c.err
}

// TestUnmodeledResourceRoutesReturnForbidden verifies the three reported APIs no longer surface OpenFGA model errors.
func TestUnmodeledResourceRoutesReturnForbidden(t *testing.T) {
	checker := &apiRelationshipChecker{err: errors.New("unmodeled relationship check")}
	handler := newRelationshipFallbackAPI(t, nil, checker, nil)

	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/v1/attendance/corrections/correction-1/approve"},
		{method: http.MethodPost, path: "/v1/workspace/agents/agent-1/unpublish"},
		{method: http.MethodGet, path: "/v1/agents/sessions/session-1/messages"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected %s %s to return 403, got %d: %s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
	}
	if len(checker.checks) != 0 {
		t.Fatalf("unmodeled route objects must not reach the relationship checker, got %+v", checker.checks)
	}
}

// TestForeignAgentSessionRemainsOwnershipSafeNotFound keeps authorized reads from exposing another account.
func TestForeignAgentSessionRemainsOwnershipSafeNotFound(t *testing.T) {
	checker := &apiRelationshipChecker{err: errors.New("relationship checker should not be needed")}
	handler := newRelationshipFallbackAPI(t, []domain.Permission{
		{Resource: "agent.run", Action: domain.ActionRead, Scope: domain.ScopeAll},
	}, checker, func(store *memory.Store, now time.Time) {
		if err := store.UpsertAgentSession(context.Background(), domain.AgentSession{
			ID:             "session-other",
			TenantID:       "demo",
			AccountID:      "acct-other",
			Title:          "private foreign session",
			Status:         domain.AgentSessionStatusActive,
			ContextVersion: 1,
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			t.Fatal(err)
		}
	})

	for _, path := range []string{
		"/v1/agents/sessions/session-other",
		"/v1/agents/sessions/session-other/messages",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected foreign session path %s to return 404, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "private foreign session") {
			t.Fatalf("foreign session details leaked from %s: %s", path, recorder.Body.String())
		}
	}
}

// newRelationshipFallbackAPI builds an authenticated API around the requested permission boundary.
func newRelationshipFallbackAPI(
	t *testing.T,
	permissions []domain.Permission,
	checker service.RelationshipChecker,
	mutateStore func(*memory.Store, time.Time),
) http.Handler {
	t.Helper()
	now := time.Date(2026, 7, 16, 9, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "demo", Name: "Demo", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	permissionSetIDs := []string{}
	if len(permissions) > 0 {
		permissionSetIDs = []string{"ps-relationship-boundary"}
		if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
			ID: "ps-relationship-boundary", TenantID: "demo", Name: "Relationship Boundary", Permissions: permissions, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-relationship-api",
		TenantID:               "demo",
		DisplayName:            "Relationship API",
		Status:                 "active",
		DirectPermissionSetIDs: permissionSetIDs,
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
		ID:        "identity-relationship-api",
		TenantID:  "demo",
		AccountID: "acct-relationship-api",
		Provider:  domain.IdentityProviderKeycloak,
		Subject:   "acct-relationship-api",
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if mutateStore != nil {
		mutateStore(store, now)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, Relationships: checker})
	return v1api.New(svc, nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider:  "keycloak",
			Subject:   "acct-relationship-api",
			TenantID:  "demo",
			AccountID: "acct-relationship-api",
		}, ok: true},
	}).Routes()
}

package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestAssumableRoleSessionLifecycleReasonsAndOwnership(t *testing.T) {
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	store, svc := newAssumableRoleSessionLifecycleFixture(t, now, nil)
	revokedAt := now.Add(-time.Minute)
	sessions := []domain.AssumableRoleSession{
		{ID: "test-active", TenantID: "tenant-1", AccountID: "acct-owner", AssumableRoleID: "role-temporary", ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "test-expired", TenantID: "tenant-1", AccountID: "acct-owner", AssumableRoleID: "role-temporary", ExpiresAt: now.Add(-time.Second), CreatedAt: now.Add(-time.Hour)},
		{ID: "test-revoked", TenantID: "tenant-1", AccountID: "acct-owner", AssumableRoleID: "role-temporary", ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt, CreatedAt: now},
		{ID: "test-foreign", TenantID: "tenant-1", AccountID: "acct-other", AssumableRoleID: "role-temporary", ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		{ID: "test-cross-tenant", TenantID: "tenant-2", AccountID: "acct-owner", AssumableRoleID: "role-temporary", ExpiresAt: now.Add(time.Hour), CreatedAt: now},
	}
	for _, session := range sessions {
		if err := store.UpsertAssumableRoleSession(context.Background(), session); err != nil {
			t.Fatal(err)
		}
	}

	for _, tt := range []struct {
		name      string
		sessionID string
		status    int
		reason    string
	}{
		{name: "invalid", sessionID: "test-missing", status: 404, reason: "assumed_role_session_invalid"},
		{name: "cross tenant", sessionID: "test-cross-tenant", status: 404, reason: "assumed_role_session_invalid"},
		{name: "expired", sessionID: "test-expired", status: 404, reason: "assumed_role_session_expired"},
		{name: "revoked", sessionID: "test-revoked", status: 404, reason: "assumed_role_session_revoked"},
		{name: "foreign", sessionID: "test-foreign", status: 403, reason: "assumed_role_session_foreign"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Me().Resolve(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-owner", AssumedRoleSessionID: tt.sessionID})
			appErr, ok := domain.AsAppError(err)
			if !ok {
				t.Fatalf("expected an application error, got %v", err)
			}
			if appErr.Status != tt.status || appErr.ReasonCode != tt.reason {
				t.Fatalf("unexpected lifecycle error classification: status=%d reason=%q", appErr.Status, appErr.ReasonCode)
			}
			if strings.Contains(appErr.Message, tt.sessionID) {
				t.Fatal("assumed-role bearer must not be reflected in an error message")
			}
		})
	}

	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-owner", AssumedRoleSessionID: "test-active"}
	if err := svc.IAM().RevokeCurrentAssumableRoleSession(ctx); err != nil {
		t.Fatal(err)
	}
	stored, ok, err := store.GetAssumableRoleSession(context.Background(), "tenant-1", "test-active")
	if err != nil || !ok || stored.RevokedAt == nil {
		t.Fatalf("expected the caller-owned session to retain a revoked record, ok=%v err=%v", ok, err)
	}
	if err := svc.IAM().RevokeCurrentAssumableRoleSession(ctx); err == nil {
		t.Fatal("expected a repeated revoke to return a stable revoked-session error")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.ReasonCode != "assumed_role_session_revoked" {
		t.Fatalf("unexpected repeated revoke error: %v", err)
	}

	foreignCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-owner", AssumedRoleSessionID: "test-foreign"}
	if err := svc.IAM().RevokeCurrentAssumableRoleSession(foreignCtx); err == nil {
		t.Fatal("expected foreign session revocation to be denied")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.ReasonCode != "assumed_role_session_foreign" {
		t.Fatalf("unexpected foreign revoke error: %v", err)
	}
	foreign, ok, err := store.GetAssumableRoleSession(context.Background(), "tenant-1", "test-foreign")
	if err != nil || !ok || foreign.RevokedAt != nil {
		t.Fatalf("foreign session must remain active, ok=%v err=%v", ok, err)
	}
}

func TestAssumableRoleBearerIsConfinedToSuccessResponse(t *testing.T) {
	now := time.Date(2026, 7, 16, 5, 0, 0, 0, time.UTC)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	store, svc := newAssumableRoleSessionLifecycleFixture(t, now, logger)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-owner", RequestID: "req-assume"}

	result, err := svc.IAM().AssumeRole(ctx, "role-temporary", domain.AssumeRoleInput{Reason: "bounded support task"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" || result.SessionToken != result.SessionID {
		t.Fatal("success response must retain the caller's temporary bearer for compatibility")
	}
	assertSerializedStateDoesNotContainBearer(t, store, logs.Bytes(), result.SessionID)

	logs.Reset()
	revokeCtx := ctx
	revokeCtx.AssumedRoleSessionID = result.SessionID
	if err := svc.IAM().RevokeCurrentAssumableRoleSession(revokeCtx); err != nil {
		t.Fatal(err)
	}
	assertSerializedStateDoesNotContainBearer(t, store, logs.Bytes(), result.SessionID)
}

func newAssumableRoleSessionLifecycleFixture(t *testing.T, now time.Time, logger *slog.Logger) (*memory.Store, *service.Service) {
	t.Helper()
	store := memory.NewStore()
	for _, item := range []domain.Tenant{{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}, {ID: "tenant-2", Name: "Tenant 2", CreatedAt: now}} {
		if err := store.UpsertTenant(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-base",
		TenantID: "tenant-1",
		Name:     "Base access",
		Permissions: []domain.Permission{
			{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
			{Resource: "iam.assumable_role", Action: domain.ActionAssume, Target: "role-temporary", Scope: domain.ScopeAll},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:          "ps-temporary",
		TenantID:    "tenant-1",
		Name:        "Temporary audit access",
		Permissions: []domain.Permission{{Resource: "audit.log", Action: domain.ActionRead, Scope: domain.ScopeAll}},
		CreatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, account := range []domain.Account{
		{ID: "acct-owner", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-base"}, CreatedAt: now},
		{ID: "acct-other", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-base"}, CreatedAt: now},
	} {
		if err := store.UpsertAccount(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "role-temporary",
		TenantID:               "tenant-1",
		Name:                   "Temporary audit role",
		PermissionSetIDs:       []string{"ps-temporary"},
		Trusted:                true,
		TrustPolicy:            map[string]any{"accounts": []string{"acct-owner"}},
		PermissionBoundary:     map[string]any{"allow": []string{"audit.log.read"}},
		SessionDurationSeconds: 1800,
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	return store, service.New(store, service.Options{Now: func() time.Time { return now }, Logger: logger})
}

func assertSerializedStateDoesNotContainBearer(t *testing.T, store *memory.Store, logBytes []byte, bearer string) {
	t.Helper()
	if bytes.Contains(logBytes, []byte(bearer)) {
		t.Fatal("application logs must not contain an assumed-role bearer")
	}
	audits, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := json.Marshal(struct {
		Audits []domain.AuditLog    `json:"audits"`
		Outbox []domain.OutboxEvent `json:"outbox"`
	}{Audits: audits, Outbox: outbox})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte(bearer)) {
		t.Fatal("audit or authz outbox details must not contain an assumed-role bearer")
	}
}

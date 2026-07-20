package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

func TestAuditReadBoundariesRedactHistoricalCredentials(t *testing.T) {
	now := time.Date(2026, 7, 16, 6, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-audit",
		TenantID: "tenant-1",
		Name:     "Audit read",
		Permissions: []domain.Permission{
			{Resource: "audit.log", Action: domain.ActionRead, Scope: domain.ScopeAll},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-audit"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}

	credential := "sess_testcredential0123456789"
	if err := store.AppendAuditLog(context.Background(), domain.AuditLog{
		ID:             "audit-historical",
		TenantID:       "tenant-1",
		ActorAccountID: "acct-1",
		Action:         "platform.me.read",
		Resource:       "me",
		Target:         credential,
		Result:         "Bearer " + credential,
		Severity:       "medium",
		Details: map[string]any{
			"assumed_role_session_id": credential,
			"safe":                    "keep-me",
			"nested": map[string]any{
				"authorization": "Bearer " + credential,
				"items": []any{
					map[string]any{"session_id": credential, "safe_nested": "keep-nested"},
				},
			},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	svc := service.New(store)
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	logs, err := svc.Audit().ListLogs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditPayloadSanitized(t, logs, credential)

	page, err := svc.Audit().ListLogPage(ctx, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertAuditPayloadSanitized(t, page, credential)

	workspacePage, err := svc.Workspace().WorkspaceAuditLogs(ctx, domain.WorkspaceAuditLogQuery{}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertAuditPayloadSanitized(t, workspacePage, credential)
	if len(workspacePage.Items) != 1 || !strings.Contains(workspacePage.Items[0].Detail, "keep-me") || !strings.Contains(workspacePage.Items[0].Detail, "keep-nested") {
		t.Fatalf("expected non-sensitive audit context to remain visible, got %+v", workspacePage.Items)
	}

	stored, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	storedRaw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(storedRaw), credential) {
		t.Fatal("read-boundary redaction must not mutate or delete retained historical data")
	}
}

func assertAuditPayloadSanitized(t *testing.T, payload any, credential string) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(raw))
	for _, forbidden := range []string{
		strings.ToLower(credential),
		"assumed_role_session_id",
		"authorization",
		"session_id",
		"bearer",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("audit read boundary exposed forbidden credential material")
		}
	}
	if !strings.Contains(text, "keep-me") {
		t.Fatalf("audit read boundary removed non-sensitive details")
	}
}

package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// newIAMOptionsFixture 建立帶完整 IAM 讀取權限的選項測試環境。
func newIAMOptionsFixture(t *testing.T) (service.IAMService, domain.RequestContext, *memory.Store) {
	t.Helper()
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam-reader",
		TenantID: "tenant-1",
		Name:     "IAM Reader",
		Permissions: []domain.Permission{
			{Resource: "iam.account", Action: "read", Scope: "all"},
			{Resource: "iam.permission_set", Action: "read", Scope: "all"},
			{Resource: "iam.user_group", Action: "read", Scope: "all"},
			{Resource: "iam.assumable_role", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		DisplayName:            "Admin",
		Email:                  "admin@example.com",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-iam-reader"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	return svc.IAM(), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}, store
}

func TestIamAccountOptionsSearchAndCursorPagination(t *testing.T) {
	svc, ctx, store := newIAMOptionsFixture(t)
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	accounts := []domain.Account{
		{ID: "acct-1", TenantID: "tenant-1", DisplayName: "Alice Chen", Email: "alice@example.com", EmployeeID: "E001", Status: "active", CreatedAt: now},
		{ID: "acct-2", TenantID: "tenant-1", DisplayName: "Bob Lin", Email: "bob@example.com", EmployeeID: "E002", Status: "suspended", CreatedAt: now},
		{ID: "acct-3", TenantID: "tenant-1", DisplayName: "Carol Wang", Email: "carol@example.com", EmployeeID: "E003", Status: "active", CreatedAt: now},
	}
	for _, account := range accounts {
		if err := store.UpsertAccount(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}

	// 模糊搜尋 email。
	page, err := svc.ListIamAccountOptions(ctx, domain.OptionQuery{Keyword: "alice@example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "acct-1" || page.Items[0].Label != "Alice Chen" {
		t.Fatalf("expected alice option, got %+v", page.Items)
	}
	meta := page.Items[0].Meta
	if meta["email"] != "alice@example.com" || meta["employee_id"] != "E001" || meta["status"] != "active" {
		t.Fatalf("unexpected account meta: %+v", meta)
	}
	if page.NextCursor != "" {
		t.Fatalf("expected no next cursor, got %q", page.NextCursor)
	}

	// 遊標分頁走完兩頁，不重不漏。
	first, err := svc.ListIamAccountOptions(ctx, domain.OptionQuery{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("expected first page of 2 with cursor, got %+v cursor=%q", first.Items, first.NextCursor)
	}
	second, err := svc.ListIamAccountOptions(ctx, domain.OptionQuery{PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.NextCursor != "" {
		t.Fatalf("expected second page of 2 without cursor, got %+v cursor=%q", second.Items, second.NextCursor)
	}
	seen := map[string]struct{}{}
	for _, p := range []domain.OptionPage{first, second} {
		for _, item := range p.Items {
			if _, dup := seen[item.ID]; dup {
				t.Fatalf("duplicate option across pages: %s", item.ID)
			}
			seen[item.ID] = struct{}{}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 distinct accounts across pages, got %d", len(seen))
	}

	// 非法遊標回 400。
	if _, err := svc.ListIamAccountOptions(ctx, domain.OptionQuery{Cursor: "not-a-cursor"}); err == nil || !strings.Contains(err.Error(), "cursor is invalid") {
		t.Fatalf("expected invalid cursor error, got %v", err)
	}
}

func TestPermissionSetOptionsIncludePermissionCount(t *testing.T) {
	svc, ctx, _ := newIAMOptionsFixture(t)

	page, err := svc.ListPermissionSetOptions(ctx, domain.OptionQuery{Keyword: "reader"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "ps-iam-reader" {
		t.Fatalf("expected reader permission set option, got %+v", page.Items)
	}
	if page.Items[0].Label != "IAM Reader" {
		t.Fatalf("expected label from name, got %q", page.Items[0].Label)
	}
	if got := page.Items[0].Meta["permission_count"]; got != 4 {
		t.Fatalf("expected permission_count 4, got %v", got)
	}
}

func TestUserGroupOptionsIncludeActiveMemberCount(t *testing.T) {
	svc, ctx, store := newIAMOptionsFixture(t)
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-1", TenantID: "tenant-1", Name: "Platform Admins", Description: "platform operators", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-2", TenantID: "tenant-1", Name: "Auditors", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// 一個有效成員 + 一個已過期成員，member_count 只算有效者。
	if err := store.UpsertGroupMembership(context.Background(), domain.GroupMembership{ID: "gm-1", TenantID: "tenant-1", UserGroupID: "ug-1", AccountID: "acct-admin", ValidFrom: now.Add(-time.Hour), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	expired := now.Add(-time.Minute)
	if err := store.UpsertGroupMembership(context.Background(), domain.GroupMembership{ID: "gm-2", TenantID: "tenant-1", UserGroupID: "ug-1", AccountID: "acct-x", ValidFrom: now.Add(-2 * time.Hour), ValidUntil: &expired, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListUserGroupOptions(ctx, domain.OptionQuery{Keyword: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "ug-1" {
		t.Fatalf("expected ug-1 option, got %+v", page.Items)
	}
	if got := page.Items[0].Meta["member_count"]; got != 1 {
		t.Fatalf("expected member_count 1 (active only), got %v", got)
	}
	if got := page.Items[0].Meta["description"]; got != "platform operators" {
		t.Fatalf("expected description meta, got %v", got)
	}

	all, err := svc.ListUserGroupOptions(ctx, domain.OptionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 2 {
		t.Fatalf("expected 2 group options, got %+v", all.Items)
	}
}

func TestAssumableRoleOptionsIncludeTrustMeta(t *testing.T) {
	svc, ctx, store := newIAMOptionsFixture(t)
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID:                     "ar-1",
		TenantID:               "tenant-1",
		Name:                   "Break Glass",
		Description:            "emergency access",
		Trusted:                true,
		SessionDurationSeconds: 3600,
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListAssumableRoleOptions(ctx, domain.OptionQuery{Keyword: "break"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "ar-1" || page.Items[0].Label != "Break Glass" {
		t.Fatalf("expected break glass role option, got %+v", page.Items)
	}
	meta := page.Items[0].Meta
	if meta["trusted"] != true || meta["session_duration_seconds"] != 3600 || meta["description"] != "emergency access" {
		t.Fatalf("unexpected role meta: %+v", meta)
	}
}

func TestIamOptionsRequireReadPermission(t *testing.T) {
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{ID: "acct-no-perm", TenantID: "tenant-1", Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }}).IAM()
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-no-perm"}

	if _, err := svc.ListIamAccountOptions(ctx, domain.OptionQuery{}); err == nil {
		t.Fatal("expected forbidden error for account options without read permission")
	}
	if _, err := svc.ListPermissionSetOptions(ctx, domain.OptionQuery{}); err == nil {
		t.Fatal("expected forbidden error for permission set options without read permission")
	}
	if _, err := svc.ListUserGroupOptions(ctx, domain.OptionQuery{}); err == nil {
		t.Fatal("expected forbidden error for user group options without read permission")
	}
	if _, err := svc.ListAssumableRoleOptions(ctx, domain.OptionQuery{}); err == nil {
		t.Fatal("expected forbidden error for assumable role options without read permission")
	}
}

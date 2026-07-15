package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestPermissionCatalogSeedIsIdempotent 驗證權限與選單 catalog seed 可重複執行。
func TestPermissionCatalogSeedIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	if err := svc.SyncPermissionCatalog(context.Background(), "tenant-1"); err != nil {
		t.Fatal(err)
	}
	firstPermissions, err := store.ListPermissionCatalogItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	firstMenus, err := store.ListMenuItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPermissions) == 0 || len(firstMenus) == 0 {
		t.Fatalf("expected seed to write permissions and menus, permissions=%d menus=%d", len(firstPermissions), len(firstMenus))
	}

	if err := svc.SyncPermissionCatalog(context.Background(), "tenant-1"); err != nil {
		t.Fatal(err)
	}
	secondPermissions, err := store.ListPermissionCatalogItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	secondMenus, err := store.ListMenuItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPermissions) != len(firstPermissions) || len(secondMenus) != len(firstMenus) {
		t.Fatalf("expected idempotent seed counts, permissions %d -> %d, menus %d -> %d", len(firstPermissions), len(secondPermissions), len(firstMenus), len(secondMenus))
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil || len(logs) != 0 {
		t.Fatalf("seed should not write audit logs, logs=%+v err=%v", logs, err)
	}
}

// TestPermissionCatalogGroupsRouteAPIsUnderCanonicalPages 驗證 route action 維持 API 型別並帶 canonical menu key。
func TestPermissionCatalogGroupsRouteAPIsUnderCanonicalPages(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	if err := service.New(store, service.Options{Now: func() time.Time { return now }}).SyncPermissionCatalog(context.Background(), "tenant-1"); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPermissionCatalogItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		resource string
		action   string
		menuKey  string
	}{
		{resource: "hr.employee", action: "create", menuKey: "hr.employees"},
		{resource: "attendance.clock", action: "read", menuKey: "attendance.clock"},
		{resource: "agent.model", action: "update", menuKey: "agents.models"},
		{resource: "audit.audit_log", action: "read", menuKey: "audit.logs"},
	} {
		found := false
		for _, item := range items {
			if item.Resource != expected.resource || item.Action != expected.action || item.PermissionType != domain.PermissionTypeAPI {
				continue
			}
			found = true
			if item.MenuKey != expected.menuKey {
				t.Fatalf("expected %s:%s under %s, got %+v", expected.resource, expected.action, expected.menuKey, item)
			}
		}
		if !found {
			t.Fatalf("expected API catalog item %s:%s", expected.resource, expected.action)
		}
	}
	for _, menuKey := range []string{"workflow.instances", "agents.runs", "agents.usage", "attendance.corrections", "attendance.worksites"} {
		found := false
		for _, item := range items {
			if item.PermissionType == domain.PermissionTypeMenu && item.MenuKey == menuKey {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected legacy business menu %s to remain in catalog", menuKey)
		}
	}
}

// TestListPermissionsUsesCatalogWhenPresent 驗證權限列表優先讀 DB catalog。
func TestListPermissionsUsesCatalogWhenPresent(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-admin",
		TenantID: "tenant-1",
		Name:     "Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.permission", Action: domain.ActionRead, Scope: domain.ScopeAll},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-admin"},
		CreatedAt:              now,
	})
	_ = store.UpsertPermissionCatalogItem(context.Background(), domain.PermissionCatalogItem{
		ID:             "perm-db-only",
		TenantID:       "tenant-1",
		Application:    "iam",
		Resource:       "iam.permission",
		Action:         "read",
		PermissionType: domain.PermissionTypeAPI,
		Name:           "DB Permission",
		Severity:       "low",
		CreatedAt:      now,
	})

	items, err := service.New(store).IAM().ListPermissions(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 ||
		items[0].ID != "perm-db-only" ||
		items[0].Application != "iam" ||
		items[0].PermissionType != domain.PermissionTypeAPI {
		t.Fatalf("expected permissions from DB catalog, got %+v", items)
	}
}

// TestListMenusUsesDBAndFallsBackToDefaultCatalog 驗證選單 DB 路徑與回退路徑。
func TestListMenusUsesDBAndFallsBackToDefaultCatalog(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)

	t.Run("fallback", func(t *testing.T) {
		store := menuFixtureStore(now, "audit")
		menus, err := service.New(store).Me().ListMenus(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
		if err != nil {
			t.Fatal(err)
		}
		if !permissionCatalogHasMenuNode(menus, "audit.logs") || !permissionCatalogHasMenuNode(menus, "audit") {
			t.Fatalf("expected fallback canonical and legacy audit menus, got %+v", menus)
		}
	})

	t.Run("database", func(t *testing.T) {
		store := menuFixtureStore(now, "custom.audit")
		_ = store.UpsertMenuItem(context.Background(), domain.MenuItem{
			ID:        "menu-custom-audit",
			TenantID:  "tenant-1",
			Key:       "custom.audit",
			Label:     "Custom Audit",
			Path:      "/custom-audit",
			SortOrder: 1,
			CreatedAt: now,
		})
		menus, err := service.New(store).Me().ListMenus(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
		if err != nil {
			t.Fatal(err)
		}
		if len(menus) != 1 || menus[0].Key != "custom.audit" || menus[0].Label != "Custom Audit" {
			t.Fatalf("expected DB menu tree, got %+v", menus)
		}
	})
}

// TestLegacyBusinessMenusRemainVisible 驗證 business workflow 與 Agent 自助入口未被 canonical workspace menu 取代。
func TestLegacyBusinessMenusRemainVisible(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	for _, menuKey := range []string{"workflow.instances", "agents.runs"} {
		t.Run(menuKey, func(t *testing.T) {
			menus, err := service.New(menuFixtureStore(now, menuKey)).Me().ListMenus(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"})
			if err != nil {
				t.Fatal(err)
			}
			if !permissionCatalogHasMenuNode(menus, menuKey) {
				t.Fatalf("expected legacy business menu %s in /me/menus, got %+v", menuKey, menus)
			}
		})
	}
}

// TestCreatePermissionSetDoubleWritesPermissionSetItems 驗證權限集合建立會雙寫關聯表。
func TestCreatePermissionSetDoubleWritesPermissionSetItems(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-admin",
		TenantID: "tenant-1",
		Name:     "Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.permission_set", Action: domain.ActionCreate, Scope: domain.ScopeAll},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-admin"},
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	set, err := svc.IAM().CreatePermissionSet(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}, domain.CreatePermissionSetInput{
		Name: "HR Reader",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "hr.employees"},
			{Resource: "audit.log", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: "audit"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPermissionSetItemsForSet(context.Background(), "tenant-1", set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two permission_set_items, got %+v", items)
	}
	catalog, err := store.ListPermissionCatalogItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) < 2 {
		t.Fatalf("expected permission catalog rows for double write, got %+v", catalog)
	}
}

// menuFixtureStore 建立 me/menu 測試資料。
func menuFixtureStore(now time.Time, menuKey string) *memory.Store {
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	menuPermission := domain.Permission{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll, MenuKey: menuKey}
	switch menuKey {
	case "audit":
		menuPermission.Resource = "audit.audit_log"
	case "workflow.instances":
		menuPermission.Resource = "workflow.form_instance"
	case "agents.runs":
		menuPermission.Resource = "agent.run"
	}
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-menu",
		TenantID: "tenant-1",
		Name:     "Menu",
		Permissions: []domain.Permission{
			{Resource: "me", Action: domain.ActionRead, Scope: domain.ScopeAll},
			menuPermission,
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-menu"},
		CreatedAt:              now,
	})
	return store
}

// permissionCatalogHasMenuNode 遞迴檢查 menu tree 是否含指定 key。
func permissionCatalogHasMenuNode(nodes []domain.MenuNode, expected string) bool {
	for _, node := range nodes {
		if node.Key == expected || permissionCatalogHasMenuNode(node.Children, expected) {
			return true
		}
	}
	return false
}

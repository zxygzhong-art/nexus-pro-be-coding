package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestProvisionTenantCreatesUsableAdmin 驗證租戶開通會建立可登入的首管理員。
func TestProvisionTenantCreatesUsableAdmin(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	result, err := svc.ProvisionTenant(context.Background(), service.TenantProvisionInput{
		TenantID:        "tenant-acme",
		TenantName:      "Acme Corp",
		AdminEmail:      "Admin@Acme.example",
		AdminName:       "Acme Admin",
		IdentitySubject: "keycloak-user-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	tenant, ok, err := store.GetTenant(context.Background(), "tenant-acme")
	if err != nil || !ok || tenant.Name != "Acme Corp" {
		t.Fatalf("expected tenant to be stored, tenant=%+v ok=%v err=%v", tenant, ok, err)
	}
	account, ok, err := store.GetAccount(context.Background(), "tenant-acme", result.AdminAccountID)
	if err != nil || !ok || account.Status != string(domain.AccountStatusActive) || account.EmployeeID != result.AdminEmployeeID {
		t.Fatalf("expected active admin account, account=%+v ok=%v err=%v", account, ok, err)
	}
	identity, ok, err := store.GetUserIdentity(context.Background(), "tenant-acme", domain.IdentityProviderKeycloak, "keycloak-user-1")
	if err != nil || !ok || identity.AccountID != result.AdminAccountID {
		t.Fatalf("expected keycloak identity binding, identity=%+v ok=%v err=%v", identity, ok, err)
	}
	employee, ok, err := store.GetEmployee(context.Background(), "tenant-acme", result.AdminEmployeeID)
	if err != nil || !ok || employee.AccountID != result.AdminAccountID || employee.OrgUnitID != result.RootOrgUnitID {
		t.Fatalf("expected admin employee linked to account and root org, employee=%+v ok=%v err=%v", employee, ok, err)
	}
	root, ok, err := store.GetOrgUnit(context.Background(), "tenant-acme", result.RootOrgUnitID)
	if err != nil || !ok || root.Code != "ROOT" || len(root.Path) != 1 || root.Path[0] != root.ID {
		t.Fatalf("expected root org unit, root=%+v ok=%v err=%v", root, ok, err)
	}
	permissionSet, ok, err := store.GetPermissionSet(context.Background(), "tenant-acme", result.AdminPermissionSetID)
	if err != nil || !ok || !hasPermission(permissionSet.Permissions, "hr.employee", domain.ActionDelete, "hr.employees") {
		t.Fatalf("expected admin permission set, permissionSet=%+v ok=%v err=%v", permissionSet, ok, err)
	}
	me, err := svc.Me().Resolve(domain.RequestContext{TenantID: "tenant-acme", AccountID: result.AdminAccountID})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"workbench", "hr.employees", "iam.permission_sets", "audit"} {
		if !hasString(me.EffectiveMenuKeys, key) {
			t.Fatalf("expected menu key %q in %+v", key, me.EffectiveMenuKeys)
		}
	}
	version, err := store.GetPermissionVersion(context.Background(), "tenant-acme")
	if err != nil || version != result.PermissionVersion || version == 0 {
		t.Fatalf("expected permission version to be bumped, version=%d result=%d err=%v", version, result.PermissionVersion, err)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-acme")
	if err != nil || len(events) != 1 || events[0].EventType != "tenant.provisioned" {
		t.Fatalf("expected tenant provisioned outbox event, events=%+v err=%v", events, err)
	}
}

// TestProvisionTenantIsIdempotent 驗證租戶開通可重複執行。
func TestProvisionTenantIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	input := service.TenantProvisionInput{
		TenantID:        "tenant-acme",
		TenantName:      "Acme Corp",
		AdminEmail:      "admin@acme.example",
		AdminName:       "Acme Admin",
		IdentitySubject: "keycloak-user-1",
	}

	first, err := svc.ProvisionTenant(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ProvisionTenant(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.AdminAccountID != second.AdminAccountID || first.AdminEmployeeID != second.AdminEmployeeID || first.AdminPermissionSetID != second.AdminPermissionSetID {
		t.Fatalf("expected stable IDs, first=%+v second=%+v", first, second)
	}
	accounts, err := store.ListAccounts(context.Background(), "tenant-acme")
	if err != nil || len(accounts) != 1 {
		t.Fatalf("expected one account after rerun, accounts=%+v err=%v", accounts, err)
	}
	identities, err := store.ListUserIdentities(context.Background(), "tenant-acme", second.AdminAccountID)
	if err != nil || len(identities) != 1 {
		t.Fatalf("expected one identity after rerun, identities=%+v err=%v", identities, err)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-acme")
	if err != nil || len(events) != 2 {
		t.Fatalf("expected one outbox event per run, events=%+v err=%v", events, err)
	}
}

// hasPermission 檢查權限集合是否包含指定權限。
func hasPermission(permissions []domain.Permission, resource string, action domain.Action, menuKey string) bool {
	for _, permission := range permissions {
		if permission.Resource == resource && permission.Action == action && permission.MenuKey == menuKey {
			return true
		}
	}
	return false
}

// hasString 檢查字串切片是否包含指定值。
func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

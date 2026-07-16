package service_test

import (
	"context"
	"encoding/json"
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
	leaveTemplate, ok, err := store.GetFormTemplateByKey(context.Background(), "tenant-acme", "leave-request")
	if err != nil || !ok || leaveTemplate.Status != "published" || leaveTemplate.CurrentVersion != 1 {
		t.Fatalf("expected published default leave template, template=%+v ok=%v err=%v", leaveTemplate, ok, err)
	}
	stages := service.ParseWorkflowStagesFromTemplate(leaveTemplate)
	if len(stages) != 1 || stages[0].Config.Role != "manager" {
		t.Fatalf("expected manager approval stage, stages=%+v", stages)
	}
	templates, err := store.ListFormTemplates(context.Background(), "tenant-acme")
	if err != nil || len(templates) != 6 {
		t.Fatalf("expected six common form templates, templates=%+v err=%v", templates, err)
	}
	for key, requiredField := range map[string]string{
		"leave-request": "leave_type", "overtime-approval": "overtime_type", "punch-fix": "correction_type",
		"job-change": "change_types", "headcount-request": "openings", "resignation": "separation_type",
	} {
		template, ok, getErr := store.GetFormTemplateByKey(context.Background(), "tenant-acme", key)
		if getErr != nil || !ok || template.Status != "published" || !templateHasBuilderField(template, requiredField) {
			t.Fatalf("expected published %s template with %s field, template=%+v ok=%v err=%v", key, requiredField, template, ok, getErr)
		}
	}
	permissionSet, ok, err := store.GetPermissionSet(context.Background(), "tenant-acme", result.AdminPermissionSetID)
	if err != nil || !ok ||
		!hasPermission(permissionSet.Permissions, "hr.employee", domain.ActionDelete, "hr.employees") ||
		!hasPermission(permissionSet.Permissions, "agent.usage", domain.ActionRead, "agents.usage") {
		t.Fatalf("expected admin permission set, permissionSet=%+v ok=%v err=%v", permissionSet, ok, err)
	}
	me, err := svc.Me().Resolve(domain.RequestContext{TenantID: "tenant-acme", AccountID: result.AdminAccountID})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"workbench", "hr.employees", "iam.permission_sets", "agents.usage", "audit"} {
		if !hasString(me.EffectiveMenuKeys, key) {
			t.Fatalf("expected menu key %q in %+v", key, me.EffectiveMenuKeys)
		}
	}
	version, err := store.GetPermissionVersion(context.Background(), "tenant-acme")
	if err != nil || version != result.PermissionVersion || version == 0 {
		t.Fatalf("expected permission version to be bumped, version=%d result=%d err=%v", version, result.PermissionVersion, err)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-acme")
	if err != nil {
		t.Fatal(err)
	}
	if countOutboxEvents(events, "tenant.provisioned") != 1 || countOutboxEvents(events, string(domain.EventOpenFGARelationshipWrite)) == 0 {
		t.Fatalf("expected tenant provisioned and OpenFGA relationship outbox events, events=%+v", events)
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
	leaveTemplate, ok, err := store.GetFormTemplateByKey(context.Background(), "tenant-acme", "leave-request")
	if err != nil || !ok {
		t.Fatalf("expected default leave template, template=%+v ok=%v err=%v", leaveTemplate, ok, err)
	}
	leaveTemplate.Name = "Acme Leave Request"
	if err := store.UpsertFormTemplate(context.Background(), leaveTemplate); err != nil {
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
	templates, err := store.ListFormTemplates(context.Background(), "tenant-acme")
	if err != nil || len(templates) != 6 {
		t.Fatalf("expected rerun to preserve six default templates, templates=%+v err=%v", templates, err)
	}
	preservedLeave, ok, err := store.GetFormTemplateByKey(context.Background(), "tenant-acme", "leave-request")
	if err != nil || !ok || preservedLeave.Name != "Acme Leave Request" {
		t.Fatalf("expected rerun to preserve customized leave template, template=%+v ok=%v err=%v", preservedLeave, ok, err)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-acme")
	if err != nil {
		t.Fatal(err)
	}
	if countOutboxEvents(events, "tenant.provisioned") != 2 {
		t.Fatalf("expected one tenant provisioned event per run, events=%+v", events)
	}
	relationshipEvents := countOutboxEvents(events, string(domain.EventOpenFGARelationshipWrite))
	if relationshipEvents == 0 || relationshipEvents != len(events)-2 {
		t.Fatalf("expected relationship events only from the first run, events=%+v", events)
	}
}

// TestEnsureTenantDefaultFormTemplatesBackfillsOnlyMissingForms 驗證補建不覆蓋既有同 key 範本。
func TestEnsureTenantDefaultFormTemplatesBackfillsOnlyMissingForms(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "demo", Name: "Demo", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID: "ft-custom-leave", TenantID: "demo", Key: "leave-request", Name: "自訂請假單",
		Schema: map[string]any{"type": "object"}, Status: "published", CurrentVersion: 3, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	created, err := svc.EnsureTenantDefaultFormTemplates(context.Background(), "demo")
	if err != nil || created != 5 {
		t.Fatalf("expected five missing templates, created=%d err=%v", created, err)
	}
	leave, ok, err := store.GetFormTemplateByKey(context.Background(), "demo", "leave-request")
	if err != nil || !ok || leave.Name != "自訂請假單" || leave.CurrentVersion != 3 {
		t.Fatalf("expected customized leave template to remain unchanged, template=%+v ok=%v err=%v", leave, ok, err)
	}
	created, err = svc.EnsureTenantDefaultFormTemplates(context.Background(), "demo")
	if err != nil || created != 0 {
		t.Fatalf("expected idempotent second backfill, created=%d err=%v", created, err)
	}
}

// templateHasBuilderField 檢查 workspace design 是否包含指定表單元件欄位。
func templateHasBuilderField(template domain.FormTemplate, fieldID string) bool {
	design, ok := template.Schema["workspace_design"].(map[string]any)
	if !ok {
		return false
	}
	raw, err := json.Marshal(design["fields"])
	if err != nil {
		return false
	}
	var fields []domain.PlatformFormBuilderField
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	for _, field := range fields {
		if field.ID == fieldID {
			return true
		}
	}
	return false
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

// countOutboxEvents 計算指定 outbox event type 數量。
func countOutboxEvents(events []domain.OutboxEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}

package service_test

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestPermissionPackageSchemaValidationRejectsMissingFieldsAndBadVersion(t *testing.T) {
	valid := service.DefaultHRPermissionPackageContent()
	if err := service.ValidatePermissionPackageContent(valid); err != nil {
		t.Fatalf("default HR package should validate: %v", err)
	}

	missingApplication := valid
	missingApplication.ApplicationCode = ""
	if err := service.ValidatePermissionPackageContent(missingApplication); err == nil {
		t.Fatal("expected missing application_code to be rejected")
	}

	badVersion := valid
	badVersion.Version = "2026"
	if err := service.ValidatePermissionPackageContent(badVersion); err == nil {
		t.Fatal("expected non-semver version to be rejected")
	}
}

func TestDefaultHRPermissionPackageJSONMatchesBuilder(t *testing.T) {
	raw, err := os.ReadFile("../../../ops/permission-packages/hr-1.0.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var fromFile domain.PermissionPackageContent
	if err := json.Unmarshal(raw, &fromFile); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidatePermissionPackageContent(fromFile); err != nil {
		t.Fatalf("HR package JSON should validate: %v", err)
	}
	fileChecksum, err := service.PermissionPackageChecksum(fromFile)
	if err != nil {
		t.Fatal(err)
	}
	builderChecksum, err := service.PermissionPackageChecksum(service.DefaultHRPermissionPackageContent())
	if err != nil {
		t.Fatal(err)
	}
	if fileChecksum != builderChecksum {
		t.Fatal("ops/permission-packages/hr-1.0.0.json drifted from DefaultHRPermissionPackageContent")
	}
}

func TestPermissionPackagePublishMakesVersionImmutable(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store, svc, ctx := permissionPackageFixture(now)
	content := testPermissionPackageContent("1.0.0")

	draft, err := svc.IAM().RegisterPermissionPackage(ctx, content)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.IAM().PublishPermissionPackage(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != domain.PermissionPackageStatusPublished || published.PublishedAt == nil {
		t.Fatalf("expected published package with published_at, got %+v", published)
	}
	if published.Checksum != draft.Checksum {
		t.Fatalf("publish should keep immutable content checksum, draft=%s published=%s", draft.Checksum, published.Checksum)
	}

	modified := content
	modified.PermissionSetTemplates = append([]domain.PermissionSetTemplateContent(nil), content.PermissionSetTemplates...)
	modified.PermissionSetTemplates[0].Name = "Changed Name"
	if _, err := svc.IAM().RegisterPermissionPackage(ctx, modified); err == nil {
		t.Fatal("expected same application/version registration to be rejected after publish")
	}
	stored, ok, err := store.GetPermissionPackage(context.Background(), draft.ID)
	if err != nil || !ok {
		t.Fatalf("expected stored package, ok=%v err=%v", ok, err)
	}
	if stored.Content.PermissionSetTemplates[0].Name != content.PermissionSetTemplates[0].Name {
		t.Fatalf("published content was overwritten: %+v", stored.Content.PermissionSetTemplates[0])
	}
}

func TestPermissionPackageImportInstantiatesArtifactsAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 30, 0, 0, time.UTC)
	store, svc, ctx := permissionPackageFixture(now)
	pkg := registerAndPublishPackage(t, svc, ctx, testPermissionPackageContent("1.0.0"))

	result, err := svc.IAM().ImportPermissionPackage(ctx, pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Imported || result.Import.Version != "1.0.0" {
		t.Fatalf("expected initial import result, got %+v", result)
	}

	sets, err := store.ListPermissionSets(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if set, ok := findPermissionSetBySource(sets, "base_reader"); !ok || set.SourcePackageVersion != "1.0.0" {
		t.Fatalf("expected permission set from template, got %+v", sets)
	}
	groups, err := store.ListUserGroups(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if group, ok := findUserGroupBySource(groups, "employees"); !ok || group.SourcePackageVersion != "1.0.0" {
		t.Fatalf("expected user group from template, got %+v", groups)
	}
	roles, err := store.ListAssumableRoles(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if role, ok := findAssumableRoleBySource(roles, "support_readonly"); !ok || role.SourcePackageVersion != "1.0.0" {
		t.Fatalf("expected assumable role from template, got %+v", roles)
	}
	catalog, err := store.ListPermissionCatalogItems(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if !catalogHasPermission(catalog, "hr.employee", "read") {
		t.Fatalf("expected imported package permission in catalog, got %+v", catalog)
	}

	repeated, err := svc.IAM().ImportPermissionPackage(ctx, pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Imported {
		t.Fatalf("repeat import should be idempotent, got %+v", repeated)
	}
	nextSets, _ := store.ListPermissionSets(context.Background(), "tenant-1")
	nextGroups, _ := store.ListUserGroups(context.Background(), "tenant-1")
	nextRoles, _ := store.ListAssumableRoles(context.Background(), "tenant-1")
	imports, err := store.ListPermissionPackageImports(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(nextSets) != len(sets) || len(nextGroups) != len(groups) || len(nextRoles) != len(roles) || len(imports) != 1 {
		t.Fatalf("repeat import created duplicates: sets %d/%d groups %d/%d roles %d/%d imports=%d", len(sets), len(nextSets), len(groups), len(nextGroups), len(roles), len(nextRoles), len(imports))
	}
}

func TestPermissionPackageUpgradeImportReportsDiffWithoutDeletingTenantCustomizations(t *testing.T) {
	now := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	store, svc, ctx := permissionPackageFixture(now)
	first := registerAndPublishPackage(t, svc, ctx, testPermissionPackageContent("1.0.0"))
	if _, err := svc.IAM().ImportPermissionPackage(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	upgraded := testPermissionPackageContent("1.1.0")
	upgraded.PermissionSetTemplates[0].Name = "Base Reader v2"
	upgraded.PermissionSetTemplates = append(upgraded.PermissionSetTemplates, domain.PermissionSetTemplateContent{
		TemplateKey: "extra_reader",
		Name:        "Extra Reader",
		Permissions: []domain.Permission{{
			ApplicationCode: "hr",
			Resource:        "hr.employee",
			Action:          domain.ActionRead,
			Scope:           domain.ScopeAll,
			PermissionType:  domain.PermissionTypeAPI,
		}},
	})
	upgraded.AssumableRoleTemplates = nil
	second := registerAndPublishPackage(t, svc, ctx, upgraded)

	result, err := svc.IAM().ImportPermissionPackage(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSorted(result.Diff.AddedTemplates, "permission_set:extra_reader") {
		t.Fatalf("expected added template diff, got %+v", result.Diff)
	}
	if !containsSorted(result.Diff.ChangedTemplates, "permission_set:base_reader") {
		t.Fatalf("expected changed template diff, got %+v", result.Diff)
	}
	if !containsSorted(result.Diff.OrphanedTemplates, "assumable_role:support_readonly") {
		t.Fatalf("expected orphaned role template diff, got %+v", result.Diff)
	}
	roles, err := store.ListAssumableRoles(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAssumableRoleBySource(roles, "support_readonly"); !ok {
		t.Fatalf("upgrade import must not delete previous tenant role, got %+v", roles)
	}
}

func permissionPackageFixture(now time.Time) (*memory.Store, *service.Service, domain.RequestContext) {
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:        "ps-package-admin",
		TenantID:  "tenant-1",
		Name:      "Package Admin",
		CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "iam.permission_package", Action: domain.ActionRead, Scope: domain.ScopeAll},
			{Resource: "iam.permission_package", Action: domain.ActionCreate, Scope: domain.ScopeAll},
			{Resource: "iam.permission_package", Action: domain.Action("publish"), Scope: domain.ScopeAll},
			{Resource: "iam.permission_package", Action: domain.ActionImport, Scope: domain.ScopeAll},
		},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-admin",
		TenantID:               "tenant-1",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-package-admin"},
		CreatedAt:              now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	return store, svc, ctx
}

func registerAndPublishPackage(t *testing.T, svc *service.Service, ctx domain.RequestContext, content domain.PermissionPackageContent) domain.PermissionPackage {
	t.Helper()
	pkg, err := svc.IAM().RegisterPermissionPackage(ctx, content)
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.IAM().PublishPermissionPackage(ctx, pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func testPermissionPackageContent(version string) domain.PermissionPackageContent {
	perm := domain.Permission{
		ApplicationCode: "hr",
		Resource:        "hr.employee",
		Action:          domain.ActionRead,
		Scope:           domain.ScopeAll,
		PermissionType:  domain.PermissionTypeAPI,
		Name:            "Read employees",
	}
	return domain.PermissionPackageContent{
		ApplicationCode: "hr",
		Version:         version,
		ResourceTypes: []domain.PermissionPackageResourceType{{
			ApplicationCode: "hr",
			ResourceType:    "employee",
			Actions:         []string{"read"},
			Name:            "Employee",
		}},
		Actions:     []domain.PermissionPackageAction{{Action: "read", Name: "Read"}},
		Permissions: []domain.Permission{perm},
		Menus: []domain.PermissionPackageMenu{{
			Key:       "hr.employees",
			Label:     "Employees",
			Path:      "/workspace/hr/employees",
			SortOrder: 1,
		}},
		DataScopes: []domain.PermissionPackageDataScope{{
			Code:      "hr_all",
			Name:      "All HR data",
			ScopeType: string(domain.ScopeAll),
		}},
		PermissionSetTemplates: []domain.PermissionSetTemplateContent{{
			TemplateKey: "base_reader",
			Name:        "Base Reader",
			Permissions: []domain.Permission{perm},
		}},
		UserGroupTemplates: []domain.UserGroupTemplateContent{{
			TemplateKey:               "employees",
			Name:                      "Employees",
			PermissionSetTemplateKeys: []string{"base_reader"},
		}},
		AssumableRoleTemplates: []domain.AssumableRoleTemplateContent{{
			TemplateKey:               "support_readonly",
			Name:                      "Support Readonly",
			PermissionSetTemplateKeys: []string{"base_reader"},
			Trusted:                   true,
			TrustPolicy:               map[string]any{"allow": []string{"iam.support"}},
			SessionDurationSeconds:    1800,
		}},
		FGAMappings: []domain.PermissionPackageFGAMapping{{
			ResourceType: "hr.employee",
			OpenFGAType:  "employee",
		}},
	}
}

func findPermissionSetBySource(items []domain.PermissionSet, source string) (domain.PermissionSet, bool) {
	for _, item := range items {
		if item.SourceTemplateKey == source {
			return item, true
		}
	}
	return domain.PermissionSet{}, false
}

func findUserGroupBySource(items []domain.UserGroup, source string) (domain.UserGroup, bool) {
	for _, item := range items {
		if item.SourceTemplateKey == source {
			return item, true
		}
	}
	return domain.UserGroup{}, false
}

func findAssumableRoleBySource(items []domain.AssumableRole, source string) (domain.AssumableRole, bool) {
	for _, item := range items {
		if item.SourceTemplateKey == source {
			return item, true
		}
	}
	return domain.AssumableRole{}, false
}

func catalogHasPermission(items []domain.PermissionCatalogItem, resource, action string) bool {
	for _, item := range items {
		if item.Resource == resource && item.Action == action && item.PermissionType == domain.PermissionTypeAPI {
			return true
		}
	}
	return false
}

func containsSorted(values []string, expected string) bool {
	sort.Strings(values)
	for _, item := range values {
		if item == expected {
			return true
		}
	}
	return false
}

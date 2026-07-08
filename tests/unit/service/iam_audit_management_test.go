package service_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestAuditDetailsIncludeRouteApplicationAndAssumedSession(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	ctx := domain.RequestContext{
		TenantID:             "tenant-1",
		AccountID:            "acct-1",
		AssumedRoleSessionID: "sess-active",
		RouteApplicationCode: "iam",
		RouteResourceType:    "field_policy",
		RouteAction:          "update",
		RoutePath:            "/v1/iam/field-policies/:id",
		RequestID:            "req-audit-context",
		TraceID:              "trace-audit-context",
		ApprovalConfirmed:    true,
		ApprovalInstanceID:   "",
	}
	if err := svc.Audit().RecordSecurityEvent(ctx, "security.cross_tenant.denied", "tenant", "tenant-2", map[string]any{"result": "denied"}); err != nil {
		t.Fatal(err)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	log, ok := findAuditLog(logs, "security.cross_tenant.denied")
	if !ok {
		t.Fatalf("expected security audit log, got %+v", logs)
	}
	if log.Details["application_code"] != "iam" || log.Details["assumed_role_session_id"] != "sess-active" || log.Details["route_path"] != "/v1/iam/field-policies/:id" {
		t.Fatalf("expected route application and assumed session details, got %+v", log.Details)
	}
}

func TestEmployeeSensitiveFieldPlainReadAuditOnlyWhenReturnedPlain(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}})
	_ = store.UpsertEmployee(context.Background(), domain.Employee{
		ID:            "emp-sensitive",
		TenantID:      "tenant-1",
		EmployeeNo:    "E9001",
		Name:          "Sensitive Person",
		PersonalEmail: "plain@example.com",
		Phone:         "0912345678",
		BasicInfo:     map[string]any{"national_id": "A123456789"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-allow-personal-email",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "personal_email",
		Effect:          "allow",
		CreatedAt:       now,
	})

	page, err := svc.HR().QueryEmployees(ctx, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	items := page.Items
	if len(items) != 1 || items[0].PersonalEmail != "plain@example.com" || items[0].Phone == "0912345678" {
		t.Fatalf("expected personal email plain and phone masked, got %+v", items)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	log, ok := findAuditLog(logs, "hr.employee.sensitive_field.read")
	if !ok {
		t.Fatalf("expected sensitive field audit, got %+v", logs)
	}
	fields, ok := log.Details["fields"].([]string)
	if !ok || !reflect.DeepEqual(fields, []string{"personal_email"}) || log.Details["count"] != 1 {
		t.Fatalf("expected personal_email sensitive audit details, got %+v", log.Details)
	}

	maskedStore, maskedSvc, maskedCtx := newEmployeeFeatureFixture(t, []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}})
	_ = maskedStore.UpsertEmployee(context.Background(), domain.Employee{
		ID:            "emp-masked",
		TenantID:      "tenant-1",
		EmployeeNo:    "E9002",
		Name:          "Masked Person",
		PersonalEmail: "masked@example.com",
		Phone:         "0987654321",
		BasicInfo:     map[string]any{"national_id": "B123456789"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	maskedPage, err := maskedSvc.HR().QueryEmployees(maskedCtx, domain.EmployeeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	maskedItems := maskedPage.Items
	if len(maskedItems) != 1 || maskedItems[0].PersonalEmail == "masked@example.com" {
		t.Fatalf("expected masked personal email, got %+v", maskedItems)
	}
	maskedLogs, err := maskedStore.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(maskedLogs, "hr.employee.sensitive_field.read"); ok {
		t.Fatalf("masked response should not write sensitive read audit, logs=%+v", maskedLogs)
	}
}

func TestIAMFieldPolicyUpdateDeleteAuditsAndInvalidatesPermissionVersion(t *testing.T) {
	now := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam",
		TenantID: "tenant-1",
		Name:     "IAM Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.field_policy", Action: "update", Scope: "all"},
			{Resource: "iam.field_policy", Action: "delete", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-iam"}, CreatedAt: now})
	_ = store.UpsertFieldPolicy(context.Background(), domain.FieldPolicy{
		ID:              "fp-1",
		TenantID:        "tenant-1",
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "phone",
		Effect:          "mask",
		CreatedAt:       now,
	})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	effect := "hide"
	updated, err := svc.IAM().UpdateFieldPolicy(ctx, "fp-1", domain.UpdateFieldPolicyInput{Effect: &effect})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Effect != "hide" {
		t.Fatalf("expected updated effect, got %+v", updated)
	}
	version, err := store.GetPermissionVersion(context.Background(), "tenant-1")
	if err != nil || version != 1 {
		t.Fatalf("expected permission version 1 after update, version=%d err=%v", version, err)
	}
	if _, err := svc.IAM().DeleteFieldPolicy(ctx, "fp-1"); err != nil {
		t.Fatal(err)
	}
	version, err = store.GetPermissionVersion(context.Background(), "tenant-1")
	if err != nil || version != 2 {
		t.Fatalf("expected permission version 2 after delete, version=%d err=%v", version, err)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "iam.field_policy.update"); !ok {
		t.Fatalf("expected field policy update audit, got %+v", logs)
	}
	if _, ok := findAuditLog(logs, "iam.field_policy.delete"); !ok {
		t.Fatalf("expected field policy delete audit, got %+v", logs)
	}
}

func TestIAMDataScopeUpdateDeleteAuditsAndInvalidatesPermissionVersion(t *testing.T) {
	now := time.Date(2026, 7, 8, 11, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam",
		TenantID: "tenant-1",
		Name:     "IAM Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.data_scope", Action: "update", Scope: "all"},
			{Resource: "iam.data_scope", Action: "delete", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-iam"}, CreatedAt: now})
	_ = store.UpsertDataScope(context.Background(), domain.DataScope{ID: "ds-1", TenantID: "tenant-1", Code: "dept", Name: "Department", ScopeType: string(domain.ScopeDepartment), CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}

	name := "Department Subtree"
	scopeType := string(domain.ScopeDepartmentSubtree)
	updated, err := svc.IAM().UpdateDataScope(ctx, "ds-1", domain.UpdateDataScopeInput{Name: &name, ScopeType: &scopeType})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.ScopeType != scopeType {
		t.Fatalf("expected updated data scope, got %+v", updated)
	}
	version, err := store.GetPermissionVersion(context.Background(), "tenant-1")
	if err != nil || version != 1 {
		t.Fatalf("expected permission version 1 after data scope update, version=%d err=%v", version, err)
	}
	if _, err := svc.IAM().DeleteDataScope(ctx, "ds-1"); err != nil {
		t.Fatal(err)
	}
	version, err = store.GetPermissionVersion(context.Background(), "tenant-1")
	if err != nil || version != 2 {
		t.Fatalf("expected permission version 2 after data scope delete, version=%d err=%v", version, err)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "iam.data_scope.update"); !ok {
		t.Fatalf("expected data scope update audit, got %+v", logs)
	}
	if _, ok := findAuditLog(logs, "iam.data_scope.delete"); !ok {
		t.Fatalf("expected data scope delete audit, got %+v", logs)
	}
}

func TestIAMOutboxEventsFilterAndRetry(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam",
		TenantID: "tenant-1",
		Name:     "IAM Outbox Admin",
		Permissions: []domain.Permission{
			{Resource: "iam.outbox_event", Action: "read", Scope: "all"},
			{Resource: "iam.outbox_event", Action: "update", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-iam"}, CreatedAt: now})
	_ = store.AppendOutboxEvent(context.Background(), domain.OutboxEvent{ID: "outbox-failed", TenantID: "tenant-1", EventType: string(domain.EventOpenFGARelationshipWrite), Status: "failed", RetryCount: 3, LastError: "openfga unavailable", CreatedAt: now})
	_ = store.AppendOutboxEvent(context.Background(), domain.OutboxEvent{ID: "outbox-pending", TenantID: "tenant-1", EventType: "tenant.provisioned", Status: "pending", CreatedAt: now.Add(time.Minute)})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", ApprovalConfirmed: true}
	hasError := true

	page, err := svc.IAM().ListOutboxEventPage(ctx, domain.OutboxEventQuery{Status: "failed", LastError: "unavailable", HasError: &hasError}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ID != "outbox-failed" {
		t.Fatalf("expected filtered failed event, got %+v", page)
	}
	retried, err := svc.IAM().RetryOutboxEvent(ctx, "outbox-failed")
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != "pending" || retried.RetryCount != 0 || retried.LastError != "" || retried.ProcessedAt != nil {
		t.Fatalf("expected retry to reset status only, got %+v", retried)
	}
	events, err := store.ListOutboxEvents(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.ID == "outbox-failed" {
			found = true
			if event.Status != "pending" || event.RetryCount != 0 || event.LastError != "" {
				t.Fatalf("expected stored outbox event reset, got %+v", event)
			}
		}
	}
	if !found {
		t.Fatal("expected stored outbox event")
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findAuditLog(logs, "iam.outbox_event.retry"); !ok {
		t.Fatalf("expected outbox retry audit, got %+v", logs)
	}
}

func TestIAMApplicationsAndResourceTypesMatchDefaultRoutePolicies(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC)
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-iam",
		TenantID: "tenant-1",
		Name:     "IAM Catalog Reader",
		Permissions: []domain.Permission{
			{Resource: "iam.application", Action: "read", Scope: "all"},
			{Resource: "iam.resource_type", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-1", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-iam"}, CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}

	apps, err := svc.IAM().ListApplications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != len(domain.DefaultApplications) {
		t.Fatalf("expected applications to match domain catalog, got %+v", apps)
	}
	for i, app := range apps {
		if app.Code != string(domain.DefaultApplications[i].Code) {
			t.Fatalf("application catalog drift at %d: %+v", i, apps)
		}
	}

	got, err := svc.IAM().ListResourceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected := expectedResourceTypesFromRoutePolicies()
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("resource type catalog drift\n got=%+v\nwant=%+v", got, expected)
	}
}

func expectedResourceTypesFromRoutePolicies() []domain.IAMResourceType {
	grouped := map[string]map[string]struct{}{}
	for _, policy := range domain.DefaultRoutePolicies {
		key := policy.ApplicationCode + "\x00" + policy.ResourceType
		if grouped[key] == nil {
			grouped[key] = map[string]struct{}{}
		}
		grouped[key][policy.Action] = struct{}{}
	}
	out := make([]domain.IAMResourceType, 0, len(grouped))
	for key, actionSet := range grouped {
		parts := strings.SplitN(key, "\x00", 2)
		actions := make([]string, 0, len(actionSet))
		for action := range actionSet {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		out = append(out, domain.IAMResourceType{ApplicationCode: parts[0], ResourceType: parts[1], Actions: actions})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ApplicationCode == out[j].ApplicationCode {
			return out[i].ResourceType < out[j].ResourceType
		}
		return out[i].ApplicationCode < out[j].ApplicationCode
	})
	return out
}

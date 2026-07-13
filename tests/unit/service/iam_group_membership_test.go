package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestUserGroupMemberAddRemoveAffectsAuthorizationAndAudits(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:        "ps-admin",
		TenantID:  "tenant-1",
		Name:      "IAM Admin",
		CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "iam.user_group", Action: "update", Scope: "all"},
		},
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:        "ps-group",
		TenantID:  "tenant-1",
		Name:      "Group Grants",
		CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-1", TenantID: "tenant-1", Name: "HR Readers", PermissionSetIDs: []string{"ps-group"}, CreatedAt: now})

	cache := &recordingAuthzSnapshot{values: map[string]domain.CheckResult{}}
	svc := service.New(store, service.Options{
		Now:           func() time.Time { return now },
		AuthzSnapshot: cache,
	})
	userCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"}
	adminCtx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
	req := domain.CheckRequest{Resource: "hr.employee", Action: "read"}

	before, err := svc.Authz().Check(userCtx, req)
	if err != nil {
		t.Fatal(err)
	}
	if before.Allowed {
		t.Fatalf("expected user to be denied before group membership, got %+v", before)
	}
	if len(cache.values) == 0 {
		t.Fatal("expected denied authz result to be cached before membership change")
	}

	validUntil := now.Add(24 * time.Hour).Format(time.RFC3339)
	membership, err := svc.IAM().AddUserGroupMember(adminCtx, "ug-1", domain.AddUserGroupMemberInput{
		AccountID:          "acct-user",
		ValidUntil:         validUntil,
		Source:             "approval",
		ApprovalInstanceID: "form-approval-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if membership.AccountID != "acct-user" || membership.Source != "approval" || membership.ValidUntil == nil {
		t.Fatalf("unexpected membership response: %+v", membership)
	}
	if len(cache.values) != 0 {
		t.Fatalf("expected authz snapshots to be invalidated after add, got %d", len(cache.values))
	}

	afterAdd, err := svc.Authz().Check(userCtx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !afterAdd.Allowed {
		t.Fatalf("expected user to receive group permission after add, got %+v", afterAdd)
	}
	group, _, _ := store.GetUserGroup(context.Background(), "tenant-1", "ug-1")
	if !containsTestString(group.MemberAccountIDs, "acct-user") {
		t.Fatalf("expected user group projection to include member, got %+v", group.MemberAccountIDs)
	}
	account, _, _ := store.GetAccount(context.Background(), "tenant-1", "acct-user")
	if !containsTestString(account.UserGroupIDs, "ug-1") {
		t.Fatalf("expected account projection to include group, got %+v", account.UserGroupIDs)
	}
	tuples, err := store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "user_group", "ug-1")
	if err != nil {
		t.Fatal(err)
	}
	if !relationshipTupleExists(tuples, "member", "account", "acct-user") {
		t.Fatalf("expected OpenFGA user_group#member tuple after add, got %+v", tuples)
	}
	logs, err := store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	addLog, ok := findAuditLog(logs, "iam.user_group.member.add")
	if !ok {
		t.Fatalf("expected add audit log, got %+v", logs)
	}
	if addLog.Severity != "high" || addLog.Details["account_id"] != "acct-user" || addLog.Details["source"] != "approval" || addLog.Details["approval_instance_id"] != "form-approval-1" {
		t.Fatalf("unexpected add audit details: %+v", addLog)
	}

	if err := svc.IAM().RemoveUserGroupMember(adminCtx, "ug-1", "acct-user"); err != nil {
		t.Fatal(err)
	}
	afterRemove, err := svc.Authz().Check(userCtx, req)
	if err != nil {
		t.Fatal(err)
	}
	if afterRemove.Allowed {
		t.Fatalf("expected user permission to be revoked after remove, got %+v", afterRemove)
	}
	tuples, err = store.ListAuthzRelationshipTuplesForObject(context.Background(), "tenant-1", "user_group", "ug-1")
	if err != nil {
		t.Fatal(err)
	}
	if relationshipTupleExists(tuples, "member", "account", "acct-user") {
		t.Fatalf("expected OpenFGA user_group#member tuple to be removed, got %+v", tuples)
	}
	logs, err = store.ListAuditLogs(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	removeLog, ok := findAuditLog(logs, "iam.user_group.member.remove")
	if !ok {
		t.Fatalf("expected remove audit log, got %+v", logs)
	}
	if removeLog.Severity != "high" || removeLog.Details["account_id"] != "acct-user" || removeLog.Details["valid_until"] == "" {
		t.Fatalf("unexpected remove audit details: %+v", removeLog)
	}
}

func TestExpiredGroupMembershipDoesNotGrantGroupPermission(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Hour)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:        "ps-group",
		TenantID:  "tenant-1",
		Name:      "Group Grants",
		CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-expired", TenantID: "tenant-1", Name: "Expired", MemberAccountIDs: []string{"acct-user"}, PermissionSetIDs: []string{"ps-group"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", Status: "active", UserGroupIDs: []string{"ug-expired"}, CreatedAt: now})
	_ = store.UpsertGroupMembership(context.Background(), domain.GroupMembership{
		ID:          "ugm-expired",
		TenantID:    "tenant-1",
		UserGroupID: "ug-expired",
		AccountID:   "acct-user",
		ValidFrom:   now.Add(-24 * time.Hour),
		ValidUntil:  &expiredAt,
		Source:      "manual",
		CreatedAt:   now.Add(-24 * time.Hour),
	})

	result, err := service.New(store, service.Options{Now: func() time.Time { return now }}).Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatalf("expected expired membership to be ignored, got %+v", result)
	}
}

func TestCreateUserGroupExpandsMemberAccountIDsToMemberships(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:        "ps-admin",
		TenantID:  "tenant-1",
		Name:      "IAM Admin",
		CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "iam.user_group", Action: "create", Scope: "all"},
		},
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:        "ps-group",
		TenantID:  "tenant-1",
		Name:      "Group Grants",
		CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
		},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})

	group, err := svc.IAM().CreateUserGroup(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"},
		domain.CreateUserGroupInput{
			Name:             "Created Group",
			PermissionSetIDs: []string{"ps-group"},
			MemberAccountIDs: []string{"acct-user"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	membership, ok, err := store.GetGroupMembership(context.Background(), "tenant-1", group.ID, "acct-user")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || membership.Source != "manual" || membership.ValidUntil != nil {
		t.Fatalf("expected create to materialize manual membership, got ok=%v membership=%+v", ok, membership)
	}
	account, _, _ := store.GetAccount(context.Background(), "tenant-1", "acct-user")
	if !containsTestString(account.UserGroupIDs, group.ID) {
		t.Fatalf("expected account projection to include created group, got %+v", account.UserGroupIDs)
	}
	result, err := svc.Authz().Check(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"},
		domain.CheckRequest{Resource: "hr.employee", Action: "read"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatalf("expected created membership to grant group permission, got %+v", result)
	}
}

func containsTestString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

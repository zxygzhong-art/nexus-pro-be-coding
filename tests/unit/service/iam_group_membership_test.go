package service_test

import (
	"context"
	"strings"
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
	if len(cache.values) != 0 {
		t.Fatal("expected denied authz result not to be cached")
	}

	validUntil := now.Add(24 * time.Hour).Format(time.RFC3339)
	if _, err := svc.IAM().AddUserGroupMember(adminCtx, "ug-1", domain.AddUserGroupMemberInput{
		AccountID: "acct-user", Source: "approval", ApprovalInstanceID: "client-claimed-approval",
	}); err == nil || !strings.Contains(err.Error(), "verified workflow") && !strings.Contains(err.Error(), "source manual") {
		t.Fatalf("expected client-supplied approval provenance to be rejected, got %v", err)
	}
	membership, err := svc.IAM().AddUserGroupMember(adminCtx, "ug-1", domain.AddUserGroupMemberInput{
		AccountID:  "acct-user",
		ValidUntil: validUntil,
		Source:     "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if membership.AccountID != "acct-user" || membership.Source != "manual" || membership.ApprovalInstanceID != "" || membership.ValidUntil == nil {
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
	if addLog.Severity != "high" || addLog.Details["account_id"] != "acct-user" || addLog.Details["source"] != "manual" || addLog.Details["approval_instance_id"] != "" {
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
	account, _, _ = store.GetAccount(context.Background(), "tenant-1", "acct-user")
	if containsTestString(account.UserGroupIDs, "ug-1") {
		t.Fatalf("expected authoritative membership close to rebuild account projection, got %+v", account.UserGroupIDs)
	}
	history, err := store.ListGroupMembershipsForGroup(context.Background(), "tenant-1", "ug-1")
	if err != nil || len(history) != 1 || history[0].ValidUntil == nil {
		t.Fatalf("expected membership removal to preserve a closed history row, history=%+v err=%v", history, err)
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

	now = now.Add(time.Minute)
	if _, err := svc.IAM().AddUserGroupMember(adminCtx, "ug-1", domain.AddUserGroupMemberInput{AccountID: "acct-user"}); err != nil {
		t.Fatal(err)
	}
	history, err = store.ListGroupMembershipsForGroup(context.Background(), "tenant-1", "ug-1")
	if err != nil || len(history) != 2 {
		t.Fatalf("expected rejoin to append a non-overlapping history row, history=%+v err=%v", history, err)
	}
}

// TestGroupMembershipExpiryCapsAllowSnapshot 驗證羣組授權快照不會活得比成員關係更久。
func TestGroupMembershipExpiryCapsAllowSnapshot(t *testing.T) {
	now := time.Now().UTC()
	validUntil := now.Add(30 * time.Second)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-group", TenantID: "tenant-1", Name: "Group Grants", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}},
	})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-admin", TenantID: "tenant-1", Name: "IAM Reader", CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "iam.user_group", Action: "read", Scope: "all"},
			{Resource: "iam.account", Action: "read", Scope: "all"},
		},
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{
		ID: "ug-1", TenantID: "tenant-1", Name: "Temporary Readers", PermissionSetIDs: []string{"ps-group"}, CreatedAt: now,
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-admin"}, CreatedAt: now})
	_ = store.UpsertGroupMembership(context.Background(), domain.GroupMembership{
		ID: "ugm-1", TenantID: "tenant-1", UserGroupID: "ug-1", AccountID: "acct-user",
		ValidFrom: now.Add(-time.Hour), ValidUntil: &validUntil, Source: "manual", CreatedAt: now,
	})
	cache := &recordingAuthzSnapshot{values: map[string]domain.CheckResult{}, now: func() time.Time { return now }}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AuthzSnapshot: cache})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"}
	req := domain.CheckRequest{Resource: "hr.employee", Action: "read"}

	allowed, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected active temporary membership to grant access, got %+v", allowed)
	}
	if len(cache.ttls) != 1 || cache.ttls[0] > 30*time.Second {
		t.Fatalf("expected snapshot TTL capped by membership expiry, got %+v", cache.ttls)
	}

	now = validUntil.Add(time.Second)
	denied, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed {
		t.Fatalf("expected expired membership not to survive cached allow, got %+v", denied)
	}
	groups, err := svc.IAM().ListUserGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].MemberAccountIDs) != 0 {
		t.Fatalf("expected group read projection to exclude expired memberships, got %+v", groups)
	}
	accounts, err := svc.IAM().ListIamAccountPage(ctx, "", domain.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts.Items) != 1 || len(accounts.Items[0].UserGroupIDs) != 0 {
		t.Fatalf("expected account read projection to exclude expired memberships, got %+v", accounts.Items)
	}
}

// TestPermissionAssignmentExpiryCapsAllowSnapshot 驗證臨時指派到期後不會沿用既有 allow 快照。
func TestPermissionAssignmentExpiryCapsAllowSnapshot(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Second)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-temporary", TenantID: "tenant-1", Name: "Temporary Grants", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "hr.employee", Action: "read", Scope: "all"}},
	})
	_ = store.UpsertAccount(context.Background(), domain.Account{ID: "acct-user", TenantID: "tenant-1", Status: "active", CreatedAt: now})
	_ = store.UpsertPermissionSetAssignment(context.Background(), domain.PermissionSetAssignment{
		ID: "psa-temporary", TenantID: "tenant-1", PrincipalType: "account", PrincipalID: "acct-user",
		PermissionSetID: "ps-temporary", Effect: "allow", ExpiresAt: &expiresAt, CreatedAt: now,
	})
	cache := &recordingAuthzSnapshot{values: map[string]domain.CheckResult{}, now: func() time.Time { return now }}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, AuthzSnapshot: cache})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"}
	req := domain.CheckRequest{Resource: "hr.employee", Action: "read"}

	allowed, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatalf("expected active temporary assignment to grant access, got %+v", allowed)
	}
	if len(cache.ttls) != 1 || cache.ttls[0] > 30*time.Second {
		t.Fatalf("expected snapshot TTL capped by assignment expiry, got %+v", cache.ttls)
	}

	now = expiresAt.Add(time.Second)
	denied, err := svc.Authz().Check(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed {
		t.Fatalf("expected expired assignment not to survive cached allow, got %+v", denied)
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

// TestExpiredGroupMembershipDoesNotSatisfyAssumableRoleTrust 驗證角色 trust 不採用帳號上的過期羣組投影。
func TestExpiredGroupMembershipDoesNotSatisfyAssumableRoleTrust(t *testing.T) {
	now := time.Now().UTC()
	expiredAt := now.Add(-time.Hour)
	store := memory.NewStore()
	_ = store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now})
	_ = store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-assume", TenantID: "tenant-1", Name: "Assume", CreatedAt: now,
		Permissions: []domain.Permission{{Resource: "iam.assumable_role", Action: "assume", Target: "role-expired", Scope: "all"}},
	})
	_ = store.UpsertUserGroup(context.Background(), domain.UserGroup{ID: "ug-expired", TenantID: "tenant-1", Name: "Expired", CreatedAt: now})
	_ = store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-user", TenantID: "tenant-1", Status: "active", UserGroupIDs: []string{"ug-expired"},
		DirectPermissionSetIDs: []string{"ps-assume"}, CreatedAt: now,
	})
	_ = store.UpsertGroupMembership(context.Background(), domain.GroupMembership{
		ID: "ugm-expired", TenantID: "tenant-1", UserGroupID: "ug-expired", AccountID: "acct-user",
		ValidFrom: now.Add(-24 * time.Hour), ValidUntil: &expiredAt, Source: "manual", CreatedAt: now,
	})
	_ = store.UpsertAssumableRole(context.Background(), domain.AssumableRole{
		ID: "role-expired", TenantID: "tenant-1", Name: "Expired Group Role", Trusted: true,
		TrustPolicy:        map[string]any{"user_groups": []string{"ug-expired"}},
		PermissionBoundary: map[string]any{"allow": []string{"hr.employee.read"}}, SessionDurationSeconds: 900, CreatedAt: now,
	})

	_, err := service.New(store, service.Options{Now: func() time.Time { return now }}).IAM().AssumeRole(
		domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-user"},
		"role-expired",
		domain.AssumeRoleInput{Reason: "expired trust test"},
	)
	if err == nil || !strings.Contains(err.Error(), "trust policy") {
		t.Fatalf("expected expired group not to satisfy role trust, got %v", err)
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

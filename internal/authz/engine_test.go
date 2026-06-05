package authz

import (
	"context"
	"testing"
	"time"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/principal"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"gorm.io/datatypes"
)

// fakeSource is an in-memory DataSource for engine unit tests (no DB).
type fakeSource struct {
	assignments map[string][]AssignmentInfo // key "type:id"
	setPerms    map[string][]models.Permission
	fieldByRes  map[string][]models.FieldPolicy
	session     *models.AssumableRoleSession
	boundary    *models.PermissionBoundary
}

func (f *fakeSource) Assignments(_ context.Context, subjects []SubjectRef) ([]AssignmentInfo, error) {
	var out []AssignmentInfo
	for _, s := range subjects {
		out = append(out, f.assignments[s.Type+":"+s.ID]...)
	}
	return out, nil
}
func (f *fakeSource) PermissionsForSets(_ context.Context, ids []string) (map[string][]models.Permission, error) {
	out := map[string][]models.Permission{}
	for _, id := range ids {
		if p, ok := f.setPerms[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}
func (f *fakeSource) FieldPolicies(_ context.Context, _, resourceType string) ([]models.FieldPolicy, error) {
	return f.fieldByRes[resourceType], nil
}
func (f *fakeSource) Menus(context.Context, string) ([]models.MenuItem, error) { return nil, nil }
func (f *fakeSource) PermissionsByIDs(context.Context, []string) (map[string]models.Permission, error) {
	return nil, nil
}
func (f *fakeSource) ActiveSession(context.Context, string) (*models.AssumableRoleSession, error) {
	return f.session, nil
}
func (f *fakeSource) Boundary(context.Context, string) (*models.PermissionBoundary, error) {
	return f.boundary, nil
}

func perm(id, res, act, risk string, high bool) models.Permission {
	return models.Permission{
		TenantModel:  models.TenantModel{BaseModel: models.BaseModel{ID: id}},
		ResourceType: res, Action: act, RiskLevel: risk, HighRisk: high,
	}
}

func ctxWith(p principal.Principal) context.Context {
	return reqctx.WithPrincipal(context.Background(), p)
}

func TestMultiGroupUnion(t *testing.T) {
	f := &fakeSource{
		assignments: map[string][]AssignmentInfo{
			"group:g1": {{PermissionSetID: "ps1", SubjectType: "group", Effect: "allow", DataScopeType: "department", SourceLabel: "group.g1"}},
			"group:g2": {{PermissionSetID: "ps2", SubjectType: "group", Effect: "allow", DataScopeType: "tenant", SourceLabel: "group.g2"}},
		},
		setPerms: map[string][]models.Permission{
			"ps1": {perm("hr.employee.read", "employee", "read", "normal", false)},
			"ps2": {perm("hr.employee.export", "employee", "export", "high", true)},
		},
	}
	e := NewLocalEngine(f)
	p := principal.Principal{TenantID: "t", AccountID: "a", GroupIDs: []string{"g1", "g2"}}

	// read from g1
	d, _ := e.Check(ctxWith(p), Request{ResourceType: "employee", Action: "read"})
	if !d.Allowed {
		t.Fatalf("expected read allowed via group union")
	}
	// export from g2 — union grants it; scope should be widest (tenant)
	d, _ = e.Check(ctxWith(p), Request{ResourceType: "employee", Action: "export"})
	if !d.Allowed || d.Scope != "tenant" {
		t.Fatalf("expected export allowed scope=tenant, got allowed=%v scope=%s", d.Allowed, d.Scope)
	}
	if !d.RequiresApproval {
		t.Fatalf("export is high_risk, expected RequiresApproval=true")
	}
}

func TestExplicitDenyWins(t *testing.T) {
	f := &fakeSource{
		assignments: map[string][]AssignmentInfo{
			"group:g1": {{PermissionSetID: "ps1", SubjectType: "group", Effect: "allow", SourceLabel: "group.g1"}},
			"user:a":   {{PermissionSetID: "psDeny", SubjectType: "user", Effect: "deny", SourceLabel: "user.a"}},
		},
		setPerms: map[string][]models.Permission{
			"ps1":    {perm("hr.employee.read", "employee", "read", "normal", false)},
			"psDeny": {perm("hr.employee.read", "employee", "read", "normal", false)},
		},
	}
	e := NewLocalEngine(f)
	p := principal.Principal{TenantID: "t", AccountID: "a", GroupIDs: []string{"g1"}}
	d, _ := e.Check(ctxWith(p), Request{ResourceType: "employee", Action: "read"})
	if d.Allowed {
		t.Fatalf("explicit deny must win over allow")
	}
	if len(d.MissingPermissions) == 0 {
		t.Fatalf("denied decision should list missing permission")
	}
}

func TestBoundaryShrink(t *testing.T) {
	f := &fakeSource{
		assignments: map[string][]AssignmentInfo{
			"group:g1":                         {{PermissionSetID: "ps1", SubjectType: "group", Effect: "allow", SourceLabel: "group.g1"}},
			"assumable_role:assumable.support": {{PermissionSetID: "ps1", SubjectType: "assumable_role", Effect: "allow", DataScopeType: "tenant", SourceLabel: "assumable_role.support"}},
		},
		setPerms: map[string][]models.Permission{
			"ps1": {
				perm("hr.employee.read", "employee", "read", "normal", false),
				perm("hr.employee.export", "employee", "export", "high", true),
			},
		},
		session: &models.AssumableRoleSession{
			ID:                   "ars_1",
			AssumableRoleID:      "assumable.support",
			PermissionBoundaryID: "boundary.ro",
			ExpiresAt:            time.Now().Add(time.Hour),
			Status:               "active",
		},
		boundary: &models.PermissionBoundary{
			TenantModel:        models.TenantModel{BaseModel: models.BaseModel{ID: "boundary.ro"}},
			AllowedPermissions: datatypes.JSON(`["hr.employee.read"]`),
			ScopeType:          "tenant",
		},
	}
	e := NewLocalEngine(f)
	p := principal.Principal{TenantID: "t", AccountID: "a", GroupIDs: []string{"g1"}, AssumedRoleSessionID: "ars_1"}

	// read is within the boundary
	if d, _ := e.Check(ctxWith(p), Request{ResourceType: "employee", Action: "read"}); !d.Allowed {
		t.Fatalf("read should be allowed within boundary")
	}
	// export is excluded by the boundary even though the permission set grants it
	if d, _ := e.Check(ctxWith(p), Request{ResourceType: "employee", Action: "export"}); d.Allowed {
		t.Fatalf("export must be denied by permission boundary")
	}
}

func TestFieldPoliciesReturned(t *testing.T) {
	f := &fakeSource{
		assignments: map[string][]AssignmentInfo{
			"group:g1": {{PermissionSetID: "ps1", SubjectType: "group", Effect: "allow", SourceLabel: "group.g1"}},
		},
		setPerms: map[string][]models.Permission{
			"ps1": {perm("hr.employee.read", "employee", "read", "normal", false)},
		},
		fieldByRes: map[string][]models.FieldPolicy{
			"employee": {
				{Field: "salary", Effect: "masked"},
				{Field: "email", Effect: "visible"},
			},
		},
	}
	e := NewLocalEngine(f)
	p := principal.Principal{TenantID: "t", AccountID: "a", GroupIDs: []string{"g1"}}
	d, _ := e.Check(ctxWith(p), Request{ApplicationCode: "hr", ResourceType: "employee", Action: "read"})
	if d.FieldPolicies["salary"] != "masked" || d.FieldPolicies["email"] != "visible" {
		t.Fatalf("expected field policies, got %#v", d.FieldPolicies)
	}
}

func TestCrossTenantHasNoGrants(t *testing.T) {
	// A principal whose groups have no assignments gets denied (RLS would also
	// prevent cross-tenant rows from ever reaching the engine).
	f := &fakeSource{assignments: map[string][]AssignmentInfo{}, setPerms: map[string][]models.Permission{}}
	e := NewLocalEngine(f)
	p := principal.Principal{TenantID: "t", AccountID: "a", GroupIDs: []string{"g1"}}
	if d, _ := e.Check(ctxWith(p), Request{ResourceType: "employee", Action: "read"}); d.Allowed {
		t.Fatalf("expected deny when no assignments exist")
	}
}

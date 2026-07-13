package service_test

import (
	"context"
	"testing"

	"nexus-pro-be/internal/domain"
)

// TestClosingOrgUnitClosesDescendants 验证父组织关闭会递归关闭全部后代。
func TestClosingOrgUnitClosesDescendants(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
		{Resource: "hr.org_unit", Action: "read", Scope: "all"},
	})
	root, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Child", ParentID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Grandchild", ParentID: child.ID})
	if err != nil {
		t.Fatal(err)
	}
	closed := true
	if _, err := svc.HR().UpdateOrgUnit(ctx, root.ID, domain.UpdateOrgUnitInput{Closed: &closed}); err != nil {
		t.Fatal(err)
	}
	units, err := svc.HR().ListOrgUnits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]domain.OrgUnit{}
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	for _, id := range []string{root.ID, child.ID, grandchild.ID} {
		if !byID[id].Closed {
			t.Fatalf("expected org unit %s to be closed, got %+v", id, byID[id])
		}
	}
}

// TestClosedAncestorBlocksChildReopen 验证父组织关闭时子组织不能单独重新启用。
func TestClosedAncestorBlocksChildReopen(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
	})
	root, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Child", ParentID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	closed := true
	if _, err := svc.HR().UpdateOrgUnit(ctx, root.ID, domain.UpdateOrgUnitInput{Closed: &closed}); err != nil {
		t.Fatal(err)
	}
	open := false
	if _, err := svc.HR().UpdateOrgUnit(ctx, child.ID, domain.UpdateOrgUnitInput{Closed: &open}); err == nil {
		t.Fatal("expected reopening child under closed parent to fail")
	}
}

// TestCreateOrgUnitUnderClosedParentInheritsClosed 验证关闭父级下新建的组织单元自动关闭。
func TestCreateOrgUnitUnderClosedParentInheritsClosed(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
	})
	root, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	closed := true
	if _, err := svc.HR().UpdateOrgUnit(ctx, root.ID, domain.UpdateOrgUnitInput{Closed: &closed}); err != nil {
		t.Fatal(err)
	}
	child, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Child", ParentID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !child.Closed {
		t.Fatalf("expected child to inherit closed state, got %+v", child)
	}
}

// TestAssignManagerPositionUpdatesPositionOrgUnit 验证主管岗位绑定会同步岗位所属组织单元。
func TestAssignManagerPositionUpdatesPositionOrgUnit(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "read", Scope: "all"},
	})
	unit, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "CEO"})
	if err != nil {
		t.Fatal(err)
	}
	position, err := svc.HR().CreatePosition(ctx, domain.CreatePositionInput{Code: "MANAGER", Name: "Manager"})
	if err != nil {
		t.Fatal(err)
	}
	positionID := position.ID
	if _, err := svc.HR().UpdateOrgUnit(ctx, unit.ID, domain.UpdateOrgUnitInput{ManagerPositionID: &positionID}); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.HR().GetPosition(ctx, position.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.OrgUnitID != unit.ID {
		t.Fatalf("expected position org unit %s, got %+v", unit.ID, updated)
	}
}

// TestCreateOrgUnitWithManagerPositionUpdatesPositionOrgUnit 验证创建组织时绑定主管岗位也会同步归属。
func TestCreateOrgUnitWithManagerPositionUpdatesPositionOrgUnit(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "read", Scope: "all"},
	})
	position, err := svc.HR().CreatePosition(ctx, domain.CreatePositionInput{Code: "DIRECTOR", Name: "Director"})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "R&D", ManagerPositionID: position.ID})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.HR().GetPosition(ctx, position.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.OrgUnitID != unit.ID {
		t.Fatalf("expected position org unit %s, got %+v", unit.ID, updated)
	}
}

// TestAssignSharedPositionAsManagerIsRejected 验证跨组织共用岗位不能被收窄为单一组织的主管岗。
func TestAssignSharedPositionAsManagerIsRejected(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
	})
	target, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	position, err := svc.HR().CreatePosition(ctx, domain.CreatePositionInput{Code: "SHARED", Name: "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(context.Background(), domain.Employee{
		ID: "emp-other", TenantID: ctx.TenantID, Name: "Other Employee", OrgUnitID: other.ID,
		PositionID: position.ID, Position: position.Name, Status: string(domain.EmployeeStatusActive),
	}); err != nil {
		t.Fatal(err)
	}
	positionID := position.ID
	if _, err := svc.HR().UpdateOrgUnit(ctx, target.ID, domain.UpdateOrgUnitInput{ManagerPositionID: &positionID}); err == nil {
		t.Fatal("expected shared position manager assignment to fail")
	}
}

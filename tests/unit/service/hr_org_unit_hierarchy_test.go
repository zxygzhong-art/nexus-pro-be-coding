package service_test

import (
	"testing"

	"nexus-pro-api/internal/domain"
)

// TestClosingOrgUnitClosesDescendants 驗證父組織關閉會遞歸關閉全部後代。
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

// TestClosedAncestorBlocksChildReopen 驗證父組織關閉時子組織不能單獨重新啟用。
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

// TestCreateOrgUnitUnderClosedParentInheritsClosed 驗證關閉父級下新建的組織單元自動關閉。
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

// TestUpdateOrgUnitChartVisibility 驗證部門層級的樹狀圖展示設定會持久化。
func TestUpdateOrgUnitChartVisibility(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
		{Resource: "hr.org_unit", Action: "read", Scope: "all"},
	})
	unit, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{Name: "Hidden Department"})
	if err != nil {
		t.Fatal(err)
	}
	if !unit.ShowInOrgChart {
		t.Fatalf("expected a new org unit to be visible by default, got %+v", unit)
	}

	hidden := false
	updated, err := svc.HR().UpdateOrgUnit(ctx, unit.ID, domain.UpdateOrgUnitInput{ShowInOrgChart: &hidden})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ShowInOrgChart {
		t.Fatalf("expected updated org unit to be hidden, got %+v", updated)
	}

	units, err := svc.HR().ListOrgUnits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range units {
		if item.ID == unit.ID && item.ShowInOrgChart {
			t.Fatalf("expected persisted org unit visibility to be false, got %+v", item)
		}
	}
}

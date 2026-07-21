package service_test

import (
	"testing"

	"nexus-pro-api/internal/domain"
)

func TestOrgUnitManagerPositionBindAndPositionInUse(t *testing.T) {
	svc, ctx := newServiceFixture([]domain.Permission{
		{Resource: "hr.org_unit", Action: "create", Scope: "all"},
		{Resource: "hr.org_unit", Action: "update", Scope: "all"},
		{Resource: "hr.org_unit", Action: "read", Scope: "all"},
		{Resource: "hr.position", Action: "create", Scope: "all"},
		{Resource: "hr.position", Action: "update", Scope: "all"},
		{Resource: "hr.position", Action: "delete", Scope: "all"},
		{Resource: "hr.position", Action: "read", Scope: "all"},
	})

	position, err := svc.HR().CreatePosition(ctx, domain.CreatePositionInput{
		Code: "DIR", Name: "部門總監", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{
		Name: "研發部", ManagerPositionID: position.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.ManagerPositionID != position.ID {
		t.Fatalf("expected manager_position_id=%s, got %q", position.ID, root.ManagerPositionID)
	}

	child, err := svc.HR().CreateOrgUnit(ctx, domain.CreateOrgUnitInput{
		Name: "後端組", ParentID: root.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ManagerPositionID != "" {
		t.Fatalf("child should leave manager position empty for inheritance, got %q", child.ManagerPositionID)
	}

	disabled := "disabled"
	if _, err := svc.HR().UpdatePosition(ctx, position.ID, domain.UpdatePositionInput{Status: &disabled}); err == nil {
		t.Fatal("expected disable to fail while used as manager position")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "validation_failed" {
		t.Fatalf("expected validation error, got %#v", err)
	} else {
		found := false
		for _, field := range appErr.FieldErrors {
			if field.Field == "status" && field.Code == "in_use" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected status.in_use field error, got %#v", appErr.FieldErrors)
		}
	}

	clear := ""
	if _, err := svc.HR().UpdateOrgUnit(ctx, root.ID, domain.UpdateOrgUnitInput{ManagerPositionID: &clear}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.HR().UpdatePosition(ctx, position.ID, domain.UpdatePositionInput{Status: &disabled}); err != nil {
		t.Fatalf("expected disable after unbind, got %v", err)
	}
}

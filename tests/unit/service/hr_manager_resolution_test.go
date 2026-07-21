package service_test

import (
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

func TestResolveEffectiveManagerInheritsParentPosition(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	units := []domain.OrgUnit{
		{ID: "ou-root", Name: "公司", ManagerPositionID: "pos-dir", Path: []string{"ou-root"}},
		{ID: "ou-child", Name: "後端組", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}},
	}
	employees := []domain.Employee{
		{ID: "emp-dir", EmployeeNo: "E001", Name: "總監", OrgUnitID: "ou-root", PositionID: "pos-dir", Status: "active", EmploymentStatus: "active"},
		{ID: "emp-dev", EmployeeNo: "E002", Name: "工程師", OrgUnitID: "ou-child", PositionID: "pos-eng", Status: "active", EmploymentStatus: "active"},
	}

	got := service.ResolveEffectiveManager(employees[1], employees, units, now)
	if got.ManagerEmployeeID != "emp-dir" || got.Source != "position" || got.Issue != "" {
		t.Fatalf("expected inherited position manager, got %#v", got)
	}
	if got.DefiningOrgUnitID != "ou-root" || got.PositionID != "pos-dir" {
		t.Fatalf("expected defining org/position from parent, got %#v", got)
	}
}

func TestResolveEffectiveManagerOverrideWins(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	units := []domain.OrgUnit{
		{ID: "ou-root", ManagerPositionID: "pos-dir", Path: []string{"ou-root"}},
	}
	employees := []domain.Employee{
		{ID: "emp-dir", EmployeeNo: "E001", OrgUnitID: "ou-root", PositionID: "pos-dir", Status: "active", EmploymentStatus: "active"},
		{ID: "emp-dev", EmployeeNo: "E002", OrgUnitID: "ou-root", PositionID: "pos-eng", ManagerEmployeeID: "emp-other", Status: "active", EmploymentStatus: "active"},
		{ID: "emp-other", EmployeeNo: "E003", OrgUnitID: "ou-root", Status: "active", EmploymentStatus: "active"},
	}

	got := service.ResolveEffectiveManager(employees[1], employees, units, now)
	if got.ManagerEmployeeID != "emp-other" || got.Source != "override" {
		t.Fatalf("expected override, got %#v", got)
	}
}

func TestResolveEffectiveManagerHolderClimbsParent(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	units := []domain.OrgUnit{
		{ID: "ou-corp", ManagerPositionID: "pos-ceo", Path: []string{"ou-corp"}},
		{ID: "ou-eng", ParentID: "ou-corp", ManagerPositionID: "pos-dir", Path: []string{"ou-corp", "ou-eng"}},
	}
	employees := []domain.Employee{
		{ID: "emp-ceo", EmployeeNo: "E001", OrgUnitID: "ou-corp", PositionID: "pos-ceo", Status: "active", EmploymentStatus: "active"},
		{ID: "emp-dir", EmployeeNo: "E002", OrgUnitID: "ou-eng", PositionID: "pos-dir", Status: "active", EmploymentStatus: "active"},
	}

	got := service.ResolveEffectiveManager(employees[1], employees, units, now)
	if got.ManagerEmployeeID != "emp-ceo" || got.Source != "position" {
		t.Fatalf("expected holder to climb to parent manager, got %#v", got)
	}
}

func TestResolveEffectiveManagerMissingPosition(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	units := []domain.OrgUnit{
		{ID: "ou-root", Path: []string{"ou-root"}},
		{ID: "ou-child", ParentID: "ou-root", Path: []string{"ou-root", "ou-child"}},
	}
	employees := []domain.Employee{
		{ID: "emp-dev", OrgUnitID: "ou-child", Status: "active", EmploymentStatus: "active"},
	}

	got := service.ResolveEffectiveManager(employees[0], employees, units, now)
	if got.Source != "none" || got.Issue != "manager_position_missing" || got.ManagerEmployeeID != "" {
		t.Fatalf("expected manager_position_missing, got %#v", got)
	}
}

func TestResolveEffectiveManagerUnfilled(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	units := []domain.OrgUnit{
		{ID: "ou-root", ManagerPositionID: "pos-dir", Path: []string{"ou-root"}},
	}
	employees := []domain.Employee{
		{ID: "emp-dev", OrgUnitID: "ou-root", PositionID: "pos-eng", Status: "active", EmploymentStatus: "active"},
	}

	got := service.ResolveEffectiveManager(employees[0], employees, units, now)
	if got.Issue != "manager_unfilled" || got.ManagerEmployeeID != "" {
		t.Fatalf("expected manager_unfilled, got %#v", got)
	}
}

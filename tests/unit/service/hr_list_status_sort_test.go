package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

func TestQueryEmployeesSortsByStatusFirst(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	})
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for _, employee := range []domain.Employee{
		{ID: "emp-resigned", TenantID: "tenant-1", Name: "Resigned", Status: "resigned", EmploymentStatus: "resigned", HireDate: ptrTime(now.Add(72 * time.Hour)), CreatedAt: now},
		{ID: "emp-active", TenantID: "tenant-1", Name: "Active", Status: "active", EmploymentStatus: "active", HireDate: ptrTime(now), CreatedAt: now.Add(time.Minute)},
		{ID: "emp-onboarding", TenantID: "tenant-1", Name: "Onboarding", Status: "onboarding", EmploymentStatus: "onboarding", HireDate: ptrTime(now.Add(48 * time.Hour)), CreatedAt: now.Add(2 * time.Minute)},
		{ID: "emp-probation", TenantID: "tenant-1", Name: "Probation", Status: "probation", EmploymentStatus: "probation", HireDate: ptrTime(now.Add(24 * time.Hour)), CreatedAt: now.Add(3 * time.Minute)},
	} {
		if err := store.UpsertEmployee(context.Background(), employee); err != nil {
			t.Fatal(err)
		}
	}

	page, err := svc.HR().QueryEmployees(ctx, domain.EmployeeQuery{Page: 1, PageSize: 20, Sort: "hire_date_desc"})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		got = append(got, item.ID)
	}
	want := []string{"emp-active", "emp-probation", "emp-onboarding", "emp-resigned"}
	if len(got) != len(want) {
		t.Fatalf("unexpected employee page size: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected status-first order %v, got %v", want, got)
		}
	}
}

func TestListOrgUnitPageSortsOpenBeforeClosed(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.org_unit", Action: "read", Scope: "all"},
	})
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for _, unit := range []domain.OrgUnit{
		{ID: "ou-closed", TenantID: "tenant-1", Name: "Closed Unit", Closed: true, CreatedAt: now},
		{ID: "ou-open", TenantID: "tenant-1", Name: "Open Unit", Closed: false, CreatedAt: now.Add(time.Minute)},
	} {
		if err := store.UpsertOrgUnit(context.Background(), unit); err != nil {
			t.Fatal(err)
		}
	}

	page, err := svc.HR().ListOrgUnitPage(ctx, domain.PageRequest{Page: 1, PageSize: 20, Sort: "created_at_asc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) < 2 {
		t.Fatalf("expected at least 2 org units, got %+v", page.Items)
	}
	var openIdx, closedIdx = -1, -1
	for i, item := range page.Items {
		switch item.ID {
		case "ou-open":
			openIdx = i
		case "ou-closed":
			closedIdx = i
		}
	}
	if openIdx < 0 || closedIdx < 0 {
		t.Fatalf("missing expected org units in page: %+v", page.Items)
	}
	if openIdx > closedIdx {
		t.Fatalf("expected open org unit before closed, got open=%d closed=%d items=%+v", openIdx, closedIdx, page.Items)
	}

	sorted := utils.SortOrgUnits([]domain.OrgUnit{
		{ID: "ou-closed", Name: "Closed", Code: "Z01", Closed: true, CreatedAt: now},
		{ID: "ou-open", Name: "Open", Code: "A01", Closed: false, CreatedAt: now.Add(time.Hour)},
	}, "code_asc")
	if sorted[0].ID != "ou-open" || sorted[1].ID != "ou-closed" {
		t.Fatalf("expected SortOrgUnits to prefer open units, got %+v", sorted)
	}
}

func TestListPositionPageSortsActiveBeforeDisabled(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, hrPositionContractPermissions())
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for _, position := range []domain.Position{
		{ID: "pos-disabled", TenantID: "tenant-1", Code: "DIS", Name: "Disabled Role", Status: string(domain.PositionStatusDisabled), CreatedAt: now, UpdatedAt: now},
		{ID: "pos-active", TenantID: "tenant-1", Code: "ACT", Name: "Active Role", Status: string(domain.PositionStatusActive), CreatedAt: now.Add(time.Minute), UpdatedAt: now},
	} {
		if err := store.UpsertPosition(context.Background(), position); err != nil {
			t.Fatal(err)
		}
	}

	page, err := svc.HR().ListPositionPage(ctx, domain.PageRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) < 2 || page.Items[0].ID != "pos-active" {
		t.Fatalf("expected active position first, got %+v", page.Items)
	}
}

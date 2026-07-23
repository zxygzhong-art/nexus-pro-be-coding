package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

func TestSystemLeaveTypeCatalogDefaultsAndTenantOverride(t *testing.T) {
	base, ctx := newServiceFixture([]domain.Permission{
		{Resource: "attendance.leave", Action: "read", Scope: "all"},
		{Resource: "attendance.leave", Action: "update", Scope: "all"},
	})
	svc := service.New(base.Store())

	catalog, err := svc.Attendance().ListLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Total != 15 || catalog.Enabled != 15 || len(catalog.Items) != 15 {
		t.Fatalf("unexpected default leave catalog: %+v", catalog)
	}
	if catalog.Items[0].Code != "sick_full" || catalog.Items[0].NameZH != "全薪病假" || catalog.Items[0].NameEN != "Full Pay Sick Leave" {
		t.Fatalf("unexpected first system leave type: %+v", catalog.Items[0])
	}
	last := catalog.Items[len(catalog.Items)-1]
	if last.Code != "business_trip" || last.NameZH != "外勤" || last.NameEN != "Business Trip" {
		t.Fatalf("unexpected last system leave type: %+v", last)
	}

	updated, err := svc.Attendance().SetLeaveTypeEnabled(ctx, "annual", domain.SetLeaveTypeEnabledInput{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled {
		t.Fatalf("expected annual leave to be disabled: %+v", updated)
	}

	catalog, err = svc.Attendance().ListLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Enabled != 14 {
		t.Fatalf("expected tenant override to reduce enabled count: %+v", catalog)
	}
}

func TestLeaveTypeParentAndChildEnablementAreIndependentByID(t *testing.T) {
	base, ctx := newServiceFixture([]domain.Permission{
		{Resource: "attendance.leave", Action: "read", Scope: "all"},
		{Resource: "attendance.leave", Action: "update", Scope: "all"},
	})
	store := base.Store()
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertLeaveType(context.Background(), domain.LeaveType{
		ID: "category:s0020", TenantID: "tenant-1", Code: "s0020", Kind: "category",
		NameZH: "病假分類", Enabled: true, DisplayOrder: 20, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLeaveType(context.Background(), domain.LeaveType{
		ID: "s0020", TenantID: "tenant-1", Code: "s0020", Kind: "item",
		ParentID: "category:s0020", ParentCode: "s0020", NameZH: "全薪病假",
		Enabled: true, DisplayOrder: 21, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store)

	if _, err := svc.Attendance().SetLeaveTypeEnabled(ctx, "category:s0020", domain.SetLeaveTypeEnabledInput{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	catalog, err := svc.Attendance().ListLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]bool{}
	for _, item := range catalog.Items {
		states[item.ID] = item.Enabled
	}
	if states["category:s0020"] || !states["s0020"] {
		t.Fatalf("parent update must not change child state: %+v", states)
	}

	if _, err := svc.Attendance().SetLeaveTypeEnabled(ctx, "s0020", domain.SetLeaveTypeEnabledInput{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	catalog, err = svc.Attendance().ListLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	states = map[string]bool{}
	for _, item := range catalog.Items {
		states[item.ID] = item.Enabled
	}
	if states["category:s0020"] || states["s0020"] {
		t.Fatalf("child update must remain independent from parent: %+v", states)
	}
}

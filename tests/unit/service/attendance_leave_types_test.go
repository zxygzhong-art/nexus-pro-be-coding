package service_test

import (
	"testing"

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

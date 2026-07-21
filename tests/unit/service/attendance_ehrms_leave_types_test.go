package service_test

import (
	"encoding/json"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

func TestEHRMSLeaveTypeCatalogPersistsCompleteSnapshot(t *testing.T) {
	base, ctx := newServiceFixture([]domain.Permission{
		{Resource: "attendance.leave", Action: "read", Scope: "all"},
		{Resource: "attendance.leave", Action: "update", Scope: "all"},
	})
	syncedAt := time.Date(2026, 7, 20, 9, 30, 0, 0, time.UTC)
	items := []domain.EHRMSLeaveType{
		{
			Code: "I001", Kind: "category", Unit: "小時", NameZH: "全薪病假", MinUnit: "0.5",
			Raw: json.RawMessage(`{"code":"I001","kind":"category","future_field":"preserved"}`),
		},
		{
			Code: "I001-1", Kind: "item", ParentCode: "I001", NameZH: "Full Pay Sick Leave",
			InclHoliday: "否", InclFestival: "否", Raw: json.RawMessage(`{"code":"I001-1","kind":"item"}`),
		},
		{
			Code: "I001", Kind: "item", ParentCode: "I001", NameZH: "Full Pay Sick Leave (same code)",
			Raw: json.RawMessage(`{"code":"I001","kind":"item","parent_code":"I001"}`),
		},
	}
	svc := service.New(base.Store(), service.Options{
		Now:         func() time.Time { return syncedAt },
		EHRMSClient: fakeEHRMSClient{leaveTypes: items},
	})

	empty, err := svc.Attendance().ListEHRMSLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Total != 0 || empty.SyncedAt != nil {
		t.Fatalf("read must not initialize an empty snapshot: %+v", empty)
	}

	catalog, err := svc.Attendance().SyncEHRMSLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Total != 3 || catalog.Categories != 1 || catalog.LeaveItems != 2 || catalog.SyncedAt == nil || !catalog.SyncedAt.Equal(syncedAt) {
		t.Fatalf("unexpected synced catalog: %+v", catalog)
	}

	// A new service without an upstream client must still serve the persisted snapshot.
	persisted, err := service.New(base.Store()).Attendance().ListEHRMSLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Items) != 3 || persisted.Items[2].Code != persisted.Items[0].Code || string(persisted.Items[0].Raw) != string(items[0].Raw) {
		t.Fatalf("expected complete persisted payload, got %+v", persisted.Items)
	}
}

func TestEHRMSLeaveTypeSyncRequiresUpdateAndTenantWideScope(t *testing.T) {
	items := []domain.EHRMSLeaveType{{Code: "I001", Kind: "item", NameZH: "病假"}}
	tests := []struct {
		name        string
		permissions []domain.Permission
		reasonCode  string
	}{
		{
			name:        "read only",
			permissions: []domain.Permission{{Resource: "attendance.leave", Action: "read", Scope: "all"}},
			reasonCode:  "button_denied",
		},
		{
			name:        "scoped update",
			permissions: []domain.Permission{{Resource: "attendance.leave", Action: "update", Scope: "self"}},
			reasonCode:  "data_scope_denied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, ctx := newServiceFixture(tt.permissions)
			svc := service.New(base.Store(), service.Options{EHRMSClient: fakeEHRMSClient{leaveTypes: items}})
			_, err := svc.Attendance().SyncEHRMSLeaveTypes(ctx)
			appErr, ok := domain.AsAppError(err)
			if !ok || appErr.Status != 403 || appErr.ReasonCode != tt.reasonCode {
				t.Fatalf("expected forbidden %s, got %v", tt.reasonCode, err)
			}
		})
	}
}

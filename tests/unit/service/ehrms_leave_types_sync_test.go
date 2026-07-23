package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

func TestSyncEHRMSAttendanceSyncsLeaveTypesAndConvertsMaxValueToBalanceMinutes(t *testing.T) {
	syncNow := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
		{Resource: "attendance.leave", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		leaveTypes: []domain.EHRMSLeaveTypeRecord{
			{
				"code":        "S0001",
				"kind":        "category",
				"name_zh":     "事假",
				"max_value":   "112小時(1年)",
				"unit":        "小時",
				"parent_code": "",
			},
			{
				"code":        "S0001-1",
				"kind":        "item",
				"name_zh":     "Personal Leave",
				"name_en":     "事假",
				"category":    "法定假別",
				"max_value":   "112小時(後1年)",
				"parent_code": "S0001",
			},
			{
				"leave_code": "personal",
				"leave_type": "事假",
				"max_value":  "0",
				"unit":       "days",
			},
		},
	}, Now: func() time.Time { return syncNow }})

	result, err := svc.Attendance().SyncEHRMSAttendance(ctx, domain.EHRMSAttendanceSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.LeaveTypesFetched != 3 || result.LeaveTypesUpserted != 3 {
		t.Fatalf("unexpected leave type sync counts: %+v", result)
	}

	items, err := store.ListLeaveTypes(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]domain.LeaveType{}
	for _, item := range items {
		byCode[item.Code] = item
	}
	category, ok := byCode["s0001"]
	if !ok {
		t.Fatalf("category leave type missing: %+v", items)
	}
	if category.ID != category.Code || category.Kind != "category" || category.ParentCode != "" {
		t.Fatalf("unexpected category identity or hierarchy: %+v", category)
	}
	personalItem, ok := byCode["s0001-1"]
	if !ok {
		t.Fatalf("child leave type missing: %+v", items)
	}
	if personalItem.ID != personalItem.Code || personalItem.Kind != "item" || personalItem.ParentCode != category.Code {
		t.Fatalf("unexpected child identity or hierarchy: %+v", personalItem)
	}
	if personalItem.Unit != "小時" || !personalItem.RequiresBalance || personalItem.MaxBalanceMinutes != 112*60 {
		t.Fatalf("child should inherit parent unit before max_value conversion: %+v", personalItem)
	}

	personal, ok := byCode["personal"]
	if !ok {
		t.Fatalf("personal leave type missing: %+v", items)
	}
	if personal.RequiresBalance || personal.MaxBalanceMinutes != 0 {
		t.Fatalf("zero max_value should not require balance: %+v", personal)
	}
}

func TestSyncEHRMSLeaveTypesDoesNotFetchEmployeeAttendance(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		leaveTypes:    []domain.EHRMSLeaveTypeRecord{{"code": "annual", "name": "特休假", "name_en": "Annual Leave"}},
		attendanceErr: context.Canceled,
	}})

	result, err := svc.Attendance().SyncEHRMSLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 1 || result.Upserted != 1 {
		t.Fatalf("unexpected leave type sync result: %+v", result)
	}
	items, err := store.ListLeaveTypes(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Code == "annual" && item.NameZH == "特休假" {
			found = true
		}
	}
	if !found {
		t.Fatalf("synced leave type missing: %+v", items)
	}
}

func TestSyncEHRMSLeaveTypesHandlesSameCodeCategoryItemAndSpecialGroup(t *testing.T) {
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "attendance.clock", Action: "import", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{
		leaveTypes: []domain.EHRMSLeaveTypeRecord{
			{"code": "S0020", "kind": "category", "name_zh": "祭儀放假", "unit": "小時"},
			{"code": "S0020", "kind": "item", "name_zh": "祭儀放假", "parent_code": "S0020", "max_value": "21小時(後1年)"},
			{"code": "001", "kind": "special_group", "name_zh": "特休假"},
		},
	}})

	result, err := svc.Attendance().SyncEHRMSLeaveTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 3 || result.Upserted != 3 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	items, err := store.ListLeaveTypes(context.Background(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]domain.LeaveType{}
	for _, item := range items {
		byID[item.ID] = item
	}
	category := byID["category:s0020"]
	child := byID["s0020"]
	group := byID["001"]
	if category.Kind != "category" || child.Kind != "item" || child.ParentID != category.ID || child.ParentCode != category.Code {
		t.Fatalf("same-code category/item hierarchy was not preserved: category=%+v child=%+v", category, child)
	}
	if group.Kind != "special_group" || group.ParentID != "" || group.ParentCode != "" {
		t.Fatalf("special group must remain a root node: %+v", group)
	}
}

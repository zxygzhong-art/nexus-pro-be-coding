package service_test

import (
	"context"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestEhrmsInferParentDeptCodeUsesLongestPrefix(t *testing.T) {
	t.Parallel()
	codes := map[string]struct{}{"C01": {}, "C0101": {}, "C0105": {}, "C010501": {}}
	if got := service.EHRMSInferParentDeptCode("C010501", codes); got != "C0105" {
		t.Fatalf("expected parent C0105, got %q", got)
	}
	if got := service.EHRMSInferParentDeptCode("C0101", codes); got != "C01" {
		t.Fatalf("expected parent C01, got %q", got)
	}
	if got := service.EHRMSInferParentDeptCode("C01", codes); got != "" {
		t.Fatalf("expected root without parent, got %q", got)
	}
}

func TestEhrmsOrgUnitsFromDepartmentsUsesParentCodeAndClosed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	units := service.EHRMSOrgUnitsFromDepartments("tenant-1", []domain.EHRMSDepartmentRecord{
		{"部門代碼": "C01", "部門中文名稱": "Corporate", "部門英文名稱": "Corporate EN"},
		{"部門代碼": "C0101", "部門中文名稱": "Sales(已關閉)", "上級部門代碼": "C01", "部門已關閉": "true"},
		{"部門代碼": "C0102", "部門中文名稱": "Ops（已關閉）", "上級部門代碼": "C01"},
	}, now)
	byID := map[string]domain.OrgUnit{}
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	if byID["C0101"].ParentID != "C01" || !byID["C0101"].Closed {
		t.Fatalf("unexpected department mapping: %+v", byID["C0101"])
	}
	if byID["C0101"].Name != "Sales" {
		t.Fatalf("expected closed suffix stripped from name, got %q", byID["C0101"].Name)
	}
	if !byID["C0102"].Closed || byID["C0102"].Name != "Ops" {
		t.Fatalf("expected name suffix to mark closed and strip label, got %+v", byID["C0102"])
	}
	if len(byID["C0101"].Path) != 2 || byID["C0101"].Path[0] != "C01" {
		t.Fatalf("unexpected path: %+v", byID["C0101"].Path)
	}
}

func TestEhrmsOrgUnitsFromDepartmentsInheritsClosedAncestor(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	units := service.EHRMSOrgUnitsFromDepartments("tenant-1", []domain.EHRMSDepartmentRecord{
		{"部門代碼": "C01", "部門中文名稱": "Corporate", "部門已關閉": "true"},
		{"部門代碼": "C0101", "部門中文名稱": "Sales", "上級部門代碼": "C01", "部門已關閉": "false"},
		{"部門代碼": "C010101", "部門中文名稱": "Sales Ops", "上級部門代碼": "C0101", "部門已關閉": "false"},
	}, now)
	for _, unit := range units {
		if !unit.Closed {
			t.Fatalf("expected %s to inherit closed state, got %+v", unit.ID, unit)
		}
	}
}

func TestEhrmsCleanDepartmentName(t *testing.T) {
	t.Parallel()
	name, closed := service.EHRMSCleanDepartmentName("COO Office(已關閉)")
	if name != "COO Office" || !closed {
		t.Fatalf("got name=%q closed=%v", name, closed)
	}
	name, closed = service.EHRMSCleanDepartmentName("Active Dept")
	if name != "Active Dept" || closed {
		t.Fatalf("got name=%q closed=%v", name, closed)
	}
}

func TestEhrmsPositionsFromRecordsDedupesByJobCode(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	positions := service.EHRMSPositionsFromRecords("tenant-1", []domain.EHRMSPositionRecord{
		{"職務代碼": "0704", "職務中文名稱": "工程師", "職務英文名稱": "Engineer"},
		{"職務代碼": "0704", "職務中文名稱": "Engineer Alt"},
	}, now)
	if len(positions) != 1 || positions[0].Name != "工程師" || positions[0].NameEN != "Engineer" {
		t.Fatalf("unexpected positions: %+v", positions)
	}
}

func TestUpsertEHRMSPositionsPreservesOrgUnitAssignment(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertPosition(context.Background(), domain.Position{
		ID: "0901", TenantID: "tenant-1", Code: "0901", Name: "Manager", OrgUnitID: "ou-ceo",
		Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	hr := service.New(store).HR()
	if _, err := hr.UpsertEHRMSPositions(service.RequestContext{TenantID: "tenant-1"}, []domain.Position{{
		ID: "0901", TenantID: "tenant-1", Code: "0901", Name: "Manager", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	updated, ok, err := store.GetPosition(context.Background(), "tenant-1", "0901")
	if err != nil || !ok {
		t.Fatalf("expected position after sync, ok=%v err=%v", ok, err)
	}
	if updated.OrgUnitID != "ou-ceo" {
		t.Fatalf("expected eHRMS sync to preserve org unit, got %+v", updated)
	}
}

func TestUpsertEHRMSOrgUnitsPreservesManagerPosition(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "ou-ceo", TenantID: "tenant-1", Code: "CEO", Name: "CEO", Path: []string{"ou-ceo"},
		ManagerPositionID: "0901", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	hr := service.New(store).HR()
	if _, err := hr.UpsertEHRMSOrgUnits(service.RequestContext{TenantID: "tenant-1"}, []domain.OrgUnit{{
		ID: "ou-ceo", TenantID: "tenant-1", Code: "CEO", Name: "CEO", Path: []string{"ou-ceo"}, CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	updated, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "ou-ceo")
	if err != nil || !ok {
		t.Fatalf("expected org unit after sync, ok=%v err=%v", ok, err)
	}
	if updated.ManagerPositionID != "0901" {
		t.Fatalf("expected eHRMS sync to preserve manager position, got %+v", updated)
	}
}

// TestUpsertEHRMSOrgUnitsAttachesRootsToCanonicalRoot 驗證同步根部門會掛到既有頂層組織下。
func TestUpsertEHRMSOrgUnitsAttachesRootsToCanonicalRoot(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertOrgUnit(context.Background(), domain.OrgUnit{
		ID: "ou-root", TenantID: "tenant-1", Code: "ROOT", Name: "Company", Path: []string{"ou-root"}, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	hr := service.New(store).HR()
	departments := []domain.OrgUnit{
		{ID: "C01", TenantID: "tenant-1", Code: "C01", Name: "Cloud", Path: []string{"C01"}, Source: "ehrms", CreatedAt: now, UpdatedAt: now},
		{ID: "C0101", TenantID: "tenant-1", Code: "C0101", Name: "Sales", ParentID: "C01", Path: []string{"C01", "C0101"}, Source: "ehrms", CreatedAt: now, UpdatedAt: now},
	}
	if _, err := hr.UpsertEHRMSOrgUnits(service.RequestContext{TenantID: "tenant-1"}, departments); err != nil {
		t.Fatal(err)
	}
	root, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "C01")
	if err != nil || !ok {
		t.Fatalf("expected synced root, ok=%v err=%v", ok, err)
	}
	child, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "C0101")
	if err != nil || !ok {
		t.Fatalf("expected synced child, ok=%v err=%v", ok, err)
	}
	if root.ParentID != "ou-root" || len(root.Path) != 2 || root.Path[0] != "ou-root" {
		t.Fatalf("expected synced root under canonical root, got %+v", root)
	}
	if child.ParentID != "C01" || len(child.Path) != 3 || child.Path[0] != "ou-root" {
		t.Fatalf("expected child path under canonical root, got %+v", child)
	}
}

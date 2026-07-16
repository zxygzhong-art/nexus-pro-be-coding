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
	byCode := map[string]domain.OrgUnit{}
	for _, unit := range units {
		byCode[unit.Code] = unit
	}
	if byCode["C0101"].ParentID != byCode["C01"].ID || !byCode["C0101"].Closed {
		t.Fatalf("unexpected department mapping: %+v", byCode["C0101"])
	}
	if byCode["C0101"].Name != "Sales" {
		t.Fatalf("expected closed suffix stripped from name, got %q", byCode["C0101"].Name)
	}
	if !byCode["C0102"].Closed || byCode["C0102"].Name != "Ops" {
		t.Fatalf("expected name suffix to mark closed and strip label, got %+v", byCode["C0102"])
	}
	if len(byCode["C0101"].Path) != 2 || byCode["C0101"].Path[0] != byCode["C01"].ID {
		t.Fatalf("unexpected path: %+v", byCode["C0101"].Path)
	}
	if byCode["C01"].ID == "C01" || byCode["C0101"].ID == "C0101" {
		t.Fatalf("expected opaque tenant-scoped IDs, got %+v", byCode)
	}
}

// TestEHRMSCatalogIDsAreTenantScoped verifies identical external codes never share global IDs.
func TestEHRMSCatalogIDsAreTenantScoped(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	departments := []domain.EHRMSDepartmentRecord{{"部門代碼": "C01", "部門中文名稱": "Corporate"}}
	positions := []domain.EHRMSPositionRecord{{"職務代碼": "0704", "職務中文名稱": "Engineer"}}

	tenantAUnits := service.EHRMSOrgUnitsFromDepartments("tenant-a", departments, now)
	tenantBUnits := service.EHRMSOrgUnitsFromDepartments("tenant-b", departments, now)
	tenantAPositions := service.EHRMSPositionsFromRecords("tenant-a", positions, now)
	tenantBPositions := service.EHRMSPositionsFromRecords("tenant-b", positions, now)

	if tenantAUnits[0].ID == tenantBUnits[0].ID || tenantAUnits[0].Code != tenantBUnits[0].Code {
		t.Fatalf("expected tenant-scoped org IDs with the same business code, a=%+v b=%+v", tenantAUnits[0], tenantBUnits[0])
	}
	if tenantAPositions[0].ID == tenantBPositions[0].ID || tenantAPositions[0].Code != tenantBPositions[0].Code {
		t.Fatalf("expected tenant-scoped position IDs with the same business code, a=%+v b=%+v", tenantAPositions[0], tenantBPositions[0])
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
	if positions[0].ID == positions[0].Code {
		t.Fatalf("expected opaque position ID, got %+v", positions[0])
	}
}

// mustOrgUnitByCode resolves a tenant-local business code in memory-backed tests.
func mustOrgUnitByCode(t *testing.T, store *memory.Store, tenantID, code string) domain.OrgUnit {
	t.Helper()
	units, err := store.ListOrgUnits(context.Background(), tenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range units {
		if unit.Code == code {
			return unit
		}
	}
	t.Fatalf("expected org unit code %q for tenant %q", code, tenantID)
	return domain.OrgUnit{}
}

// mustPositionByCode resolves a tenant-local position business code in memory-backed tests.
func mustPositionByCode(t *testing.T, store *memory.Store, tenantID, code string) domain.Position {
	t.Helper()
	position, ok, err := store.GetPositionByCode(context.Background(), tenantID, code)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected position code %q for tenant %q", code, tenantID)
	}
	return position
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
		ID: "ehrms-pos-new", TenantID: "tenant-1", Code: "0901", Name: "Manager", Status: string(domain.PositionStatusActive), CreatedAt: now, UpdatedAt: now,
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
	if _, ok, err := store.GetPosition(context.Background(), "tenant-1", "ehrms-pos-new"); err != nil || ok {
		t.Fatalf("expected business-code reconciliation to preserve the legacy ID, ok=%v err=%v", ok, err)
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
		ID: "ehrms-ou-new", TenantID: "tenant-1", Code: "CEO", Name: "CEO", Path: []string{"ehrms-ou-new"}, CreatedAt: now, UpdatedAt: now,
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
	if _, ok, err := store.GetOrgUnit(context.Background(), "tenant-1", "ehrms-ou-new"); err != nil || ok {
		t.Fatalf("expected business-code reconciliation to preserve the legacy ID, ok=%v err=%v", ok, err)
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

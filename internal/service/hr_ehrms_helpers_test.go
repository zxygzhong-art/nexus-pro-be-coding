package service

import (
	"testing"
	"time"
)

func TestEhrmsInferParentDeptCodeUsesLongestPrefix(t *testing.T) {
	t.Parallel()
	codes := map[string]struct{}{"C01": {}, "C0101": {}, "C0105": {}, "C010501": {}}
	if got := ehrmsInferParentDeptCode("C010501", codes); got != "C0105" {
		t.Fatalf("expected parent C0105, got %q", got)
	}
	if got := ehrmsInferParentDeptCode("C0101", codes); got != "C01" {
		t.Fatalf("expected parent C01, got %q", got)
	}
	if got := ehrmsInferParentDeptCode("C01", codes); got != "" {
		t.Fatalf("expected root without parent, got %q", got)
	}
}

func TestEhrmsOrgUnitsBuildHierarchy(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	records := []EHRMSEmployeeRecord{
		{"部門代碼": "C01", "部門中文名稱": "Corporate"},
		{"部門代碼": "C0101", "部門中文名稱": "Sales"},
	}
	units := ehrmsOrgUnits("tenant-1", records, now)
	byID := map[string]OrgUnit{}
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	if byID["C0101"].ParentID != "C01" {
		t.Fatalf("unexpected parent: %+v", byID["C0101"])
	}
	if len(byID["C0101"].Path) != 2 || byID["C0101"].Path[0] != "C01" || byID["C0101"].Path[1] != "C0101" {
		t.Fatalf("unexpected path: %+v", byID["C0101"].Path)
	}
}

func TestEhrmsPositionsDedupesByJobCode(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	records := []EHRMSEmployeeRecord{
		{"職務代碼": "0704", "職務中文名稱": "工程師"},
		{"職務代碼": "0704", "職務中文名稱": "Engineer"},
	}
	positions := ehrmsPositions("tenant-1", records, now)
	if len(positions) != 1 || positions[0].ID != "0704" || positions[0].Name != "工程師" {
		t.Fatalf("unexpected positions: %+v", positions)
	}
}

package ehrms

import (
	"testing"

	"nexus-pro-be/internal/domain"
)

func TestNormalizeEmployeeRecordsMapsEnglishAliases(t *testing.T) {
	t.Parallel()
	rows := normalizeEmployeeRecords([]domain.EHRMSEmployeeRecord{{
		"emp_id":      "IKM001",
		"name_zh":     "測試員工",
		"work_status": "在職",
		"dept_code":   "M0101",
		"hire_date":   "2026/06/01",
		"national_id": "A123456789",
		"leave_group": "-",
	}})
	if rows[0]["員工編號"] != "IKM001" || rows[0]["中文姓名"] != "測試員工" || rows[0]["在職狀態"] != "在職" {
		t.Fatalf("unexpected normalized record: %+v", rows[0])
	}
	if rows[0]["dept_code"] != "M0101" {
		t.Fatalf("expected original keys to remain, got %+v", rows[0])
	}
	if rows[0]["休假群組"] != "-" {
		t.Fatalf("expected leave_group alias, got %+v", rows[0])
	}
}

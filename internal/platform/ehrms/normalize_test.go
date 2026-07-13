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
		"school_zh":   "Nexus University",
		"email":       "test@ikala.ai",
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
	if rows[0]["學校名稱(中文)"] != "Nexus University" {
		t.Fatalf("expected school_zh alias, got %+v", rows[0])
	}
	if rows[0]["公司信箱"] != "test@ikala.ai" {
		t.Fatalf("expected email alias, got %+v", rows[0])
	}
}

func TestNormalizeDepartmentAndPositionRecords(t *testing.T) {
	t.Parallel()
	departments := normalizeDepartmentRecords([]domain.EHRMSDepartmentRecord{{
		"code":        "C0101",
		"name":        "Sales",
		"parent_code": "C01",
		"closed":      "true",
	}})
	if departments[0]["部門代碼"] != "C0101" || departments[0]["上級部門代碼"] != "C01" || departments[0]["部門已關閉"] != "true" {
		t.Fatalf("unexpected department normalize: %+v", departments[0])
	}
	positions := normalizePositionRecords([]domain.EHRMSPositionRecord{{
		"job_code":     "0704",
		"job_title_zh": "工程師",
		"job_title_en": "Engineer",
	}})
	if positions[0]["職務代碼"] != "0704" || positions[0]["職務中文名稱"] != "工程師" || positions[0]["職務英文名稱"] != "Engineer" {
		t.Fatalf("unexpected position normalize: %+v", positions[0])
	}
}

func TestNormalizeLeaveRecords(t *testing.T) {
	t.Parallel()
	balances := normalizeLeaveBalanceRecords([]domain.EHRMSLeaveBalanceRecord{{
		"emp_id":      "IKM017",
		"leave_type":  "annual",
		"remaining":   "8",
		"expire_date": "2026-12-31",
	}})
	if balances[0]["員工編號"] != "IKM017" || balances[0]["假別"] != "annual" || balances[0]["餘額"] != "8" || balances[0]["到期日"] != "2026-12-31" {
		t.Fatalf("unexpected leave balance normalize: %+v", balances[0])
	}
	details := normalizeLeaveDetailRecords([]domain.EHRMSLeaveDetailRecord{{
		"emp_id":     "IKM017",
		"date":       "2026-06-11",
		"leave_type": "annual",
		"start":      "09:00",
		"end":        "13:00",
		"hours":      "4",
	}})
	if details[0]["員工編號"] != "IKM017" || details[0]["日期"] != "2026-06-11" || details[0]["開始時間"] != "09:00" || details[0]["時數"] != "4" {
		t.Fatalf("unexpected leave detail normalize: %+v", details[0])
	}
}

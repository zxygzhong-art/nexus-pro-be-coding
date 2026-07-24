package ehrms_test

import (
	"testing"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/ehrms"
)

func TestNormalizeEmployeeRecordsMapsEnglishAliases(t *testing.T) {
	t.Parallel()
	rows := ehrms.NormalizeEmployeeRecords([]domain.EHRMSEmployeeRecord{{
		"emp_id":                  "IKM001",
		"name_zh":                 "測試員工",
		"work_status":             "在職",
		"dept_code":               "M0101",
		"hire_date":               "2026/06/01",
		"quit_date":               "2026/06/30",
		"seniority_start":         "2026/06/01",
		"probation_end":           "2026/09/01",
		"national_id":             "A123456789",
		"passport_name":           "TEST EMPLOYEE",
		"entry_date":              "2026/01/02",
		"arc_no":                  "A800000001",
		"arc_expiry_date":         "2027/01/02",
		"tax_id":                  "TW-001",
		"work_permit_no":          "WP-001",
		"work_permit_expiry_date": "2027/01/01",
		"contract_expiry_date":    "2028/01/01",
		"broker":                  "Nexus Agency",
		"emergency_phone":         "0912-345-678",
		"emergency_contact":       "王小明",
		"emergency_relation":      "配偶",
		"leave_group":             "-",
		"school_zh":               "Nexus University",
		"email":                   "test@ikala.ai",
	}})
	if rows[0]["員工編號"] != "IKM001" || rows[0]["中文姓名"] != "測試員工" || rows[0]["在職狀態"] != "在職" {
		t.Fatalf("unexpected normalized record: %+v", rows[0])
	}
	if rows[0]["dept_code"] != "M0101" {
		t.Fatalf("expected original keys to remain, got %+v", rows[0])
	}
	if rows[0]["休假羣組"] != "-" {
		t.Fatalf("expected leave_group alias, got %+v", rows[0])
	}
	if rows[0]["學校名稱(中文)"] != "Nexus University" {
		t.Fatalf("expected school_zh alias, got %+v", rows[0])
	}
	if rows[0]["公司信箱"] != "test@ikala.ai" {
		t.Fatalf("expected email alias, got %+v", rows[0])
	}
	if rows[0]["離職日期"] != "2026/06/30" {
		t.Fatalf("expected quit_date alias, got %+v", rows[0])
	}
	if rows[0]["年資起始日"] != "2026/06/01" || rows[0]["試用期滿日"] != "2026/09/01" {
		t.Fatalf("expected employment date aliases, got %+v", rows[0])
	}
	if rows[0]["護照姓名"] != "TEST EMPLOYEE" || rows[0]["居留證號"] != "A800000001" || rows[0]["仲介單位"] != "Nexus Agency" {
		t.Fatalf("expected foreign employee basic-info aliases, got %+v", rows[0])
	}
	if rows[0]["緊急聯絡人電話"] != "0912-345-678" || rows[0]["緊急聯絡人姓名"] != "王小明" || rows[0]["緊急聯絡人關係"] != "配偶" {
		t.Fatalf("expected emergency contact aliases, got %+v", rows[0])
	}
}

func TestNormalizeDepartmentAndPositionRecords(t *testing.T) {
	t.Parallel()
	departments := ehrms.NormalizeDepartmentRecords([]domain.EHRMSDepartmentRecord{{
		"code":        "C0101",
		"name":        "Sales",
		"parent_code": "C01",
		"closed":      "true",
	}})
	if departments[0]["部門代碼"] != "C0101" || departments[0]["上級部門代碼"] != "C01" || departments[0]["部門已關閉"] != "true" {
		t.Fatalf("unexpected department normalize: %+v", departments[0])
	}
	positions := ehrms.NormalizePositionRecords([]domain.EHRMSPositionRecord{{
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
	balances := ehrms.NormalizeLeaveBalanceRecords([]domain.EHRMSLeaveBalanceRecord{{
		"emp_id":              "IKM017",
		"year":                "2026",
		"leave_code":          "I001-1",
		"leave_category_code": "I001",
		"name_zh":             "Full Pay Sick Leave",
		"remaining":           "40",
		"quota":               "40",
		"used":                "0",
		"unit":                "hours",
	}})
	if balances[0]["員工編號"] != "IKM017" || balances[0]["假別"] != "Full Pay Sick Leave" ||
		balances[0]["假別代碼"] != "I001-1" || balances[0]["餘額"] != "40" {
		t.Fatalf("unexpected leave balance normalize: %+v", balances[0])
	}
	details := ehrms.NormalizeLeaveDetailRecords([]domain.EHRMSLeaveDetailRecord{{
		"emp_id":              "IKM017",
		"date":                "2026-01-30",
		"leave_type":          "外勤",
		"leave_code":          "S0013-1",
		"leave_category_code": "S0013",
		"start":               "2026-01-30 09:00:00",
		"end":                 "2026-01-30 17:00:00",
		"hours":               "7",
	}})
	if details[0]["員工編號"] != "IKM017" || details[0]["日期"] != "2026-01-30" ||
		details[0]["開始時間"] != "2026-01-30 09:00:00" || details[0]["時數"] != "7" {
		t.Fatalf("unexpected leave detail normalize: %+v", details[0])
	}
}

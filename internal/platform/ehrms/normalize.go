package ehrms

import (
	"encoding/json"
	"fmt"
	"strings"

	"nexus-pro-api/internal/domain"
)

var employeeFieldAliases = map[string]string{
	"emp_id":                  "員工編號",
	"name_zh":                 "中文姓名",
	"name_en":                 "英文姓名",
	"first_name":              "First Name",
	"last_name":               "Last Name",
	"gender":                  "性別",
	"birthday":                "生日",
	"hire_date":               "到職日期",
	"quit_date":               "離職日期",
	"seniority_start":         "年資起始日",
	"probation_end":           "試用期滿日",
	"work_status":             "在職狀態",
	"nationality":             "國籍名稱",
	"national_id":             "身份證號",
	"passport_no":             "護照號碼",
	"passport_name":           "護照姓名",
	"entry_date":              "入境日期",
	"arc_no":                  "居留證號",
	"arc_expiry_date":         "居留證到期日",
	"tax_id":                  "稅籍編號",
	"work_permit_no":          "工作證號",
	"work_permit_expiry_date": "工作證到期日",
	"contract_expiry_date":    "契約到期日",
	"broker":                  "仲介單位",
	"emergency_phone":         "緊急聯絡人電話",
	"emergency_contact":       "緊急聯絡人姓名",
	"emergency_relation":      "緊急聯絡人關係",
	"identity_type":           "身份類別名稱",
	"education":               "最高學歷",
	"dept_code":               "部門代碼",
	"dept_name_zh":            "部門中文名稱",
	"dept_name_en":            "部門英文名稱",
	"job_code":                "職務代碼",
	"job_title_zh":            "職務中文名稱",
	"job_title_en":            "職務英文名稱",
	"card_no":                 "卡號",
	"clock_required":          "上下班刷卡",
	"shift_name":              "員工班別名稱",
	"shift_attr":              "員工班別屬性",
	"direct_indirect":         "直接/間接員工",
	"leave_group":             "休假羣組",
	"school_name":             "學校名稱(中文)",
	"school_zh":               "學校名稱(中文)",
	"email":                   "公司信箱",
}

var departmentFieldAliases = map[string]string{
	"code":                 "部門代碼",
	"name":                 "部門中文名稱",
	"name_zh":              "部門中文名稱",
	"name_en":              "部門英文名稱",
	"parent_code":          "上級部門代碼",
	"depth":                "部門層級",
	"closed":               "部門已關閉",
	"headcount":            "部門人數",
	"manager_job_code":     "主管職務代碼",
	"manager_job_title":    "主管職務中文名稱",
	"manager_job_title_en": "主管職務英文名稱",
	"manager_emp_id":       "主管員工編號",
	"manager":              "主管姓名",
}

var positionFieldAliases = map[string]string{
	"job_code":     "職務代碼",
	"job_title_zh": "職務中文名稱",
	"job_title_en": "職務英文名稱",
	"headcount":    "職務人數",
}

var attendanceFieldAliases = map[string]string{
	"emp_id":      "員工編號",
	"date":        "日期",
	"shift_start": "班別開始",
	"shift_end":   "班別結束",
	"shift_hours": "班別工時",
	"daily_hours": "應出勤工時",
	"clock_hours": "刷卡工時",
	"clock_start": "clock_start",
	"clock_end":   "clock_end",
}

var leaveBalanceFieldAliases = map[string]string{
	"emp_id":              "員工編號",
	"year":                "年度",
	"leave_type":          "假別",
	"name_zh":             "假別",
	"name_en":             "假別",
	"unit":                "單位",
	"quota":               "額度",
	"used":                "已使用",
	"remaining":           "餘額",
	"grant_start":         "發放起始日",
	"expire_date":         "到期日",
	"carry_in":            "遞延餘額",
	"carry_expire":        "遞延到期日",
	"leave_code":          "假別代碼",
	"leave_category_code": "假別類別代碼",
}

var leaveDetailFieldAliases = map[string]string{
	"emp_id":              "員工編號",
	"date":                "日期",
	"leave_type":          "假別",
	"start":               "開始時間",
	"end":                 "結束時間",
	"hours":               "時數",
	"leave_code":          "假別代碼",
	"leave_category_code": "假別類別代碼",
	"leave_item":          "假勤項目",
	"remark":              "備註",
	"source":              "資料來源",
	"deduct_item":         "扣除項目",
	"deduct_hours":        "扣除時間",
}

// flattenLeaveEntitlementRows expands /leave-entitlement nested payloads into flat
// balance rows expected by SyncEHRMSAttendance leave-balance upsert.
//
// Upstream shape:
//
//	[{emp_id, year, unit, entitlements: {categoryCode: [{leave_code, quota, used, remaining, name_zh, ...}]}}]
func flattenLeaveEntitlementRows(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		// Already-flat legacy/test rows (no entitlements object).
		if _, hasNested := row["entitlements"]; !hasNested {
			out = append(out, row)
			continue
		}
		entitlements, _ := row["entitlements"].(map[string]any)
		if len(entitlements) == 0 {
			continue
		}
		for categoryCode, rawItems := range entitlements {
			items, _ := rawItems.([]any)
			for _, rawItem := range items {
				item, ok := rawItem.(map[string]any)
				if !ok || item == nil {
					continue
				}
				flat := make(map[string]any, len(row)+len(item))
				for key, value := range row {
					if key != "entitlements" {
						flat[key] = value
					}
				}
				for key, value := range item {
					flat[key] = value
				}
				flat["leave_category_code"] = categoryCode
				if leaveType := firstNonEmptyJSONString(flat["leave_type"], flat["name_zh"], flat["name_en"], flat["leave_code"]); leaveType != "" {
					flat["leave_type"] = leaveType
				}
				out = append(out, flat)
			}
		}
	}
	return out
}

// flattenLeaveDetailRows expands /leave nested payloads into flat detail rows.
// Parent balances are ignored; only details[] are synced as leave records.
//
// Upstream shape:
//
//	[{emp_id, leave_type, leave_code, leave_category_code, balances: [...], details: [{date, start, end, hours, ...}]}]
func flattenLeaveDetailRows(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		rawDetails, hasDetails := row["details"]
		if !hasDetails {
			// Already-flat legacy/test rows.
			out = append(out, row)
			continue
		}
		details, _ := rawDetails.([]any)
		if len(details) == 0 {
			continue
		}
		parent := make(map[string]any, len(row))
		for key, value := range row {
			if key != "balances" && key != "details" {
				parent[key] = value
			}
		}
		for _, rawDetail := range details {
			detail, ok := rawDetail.(map[string]any)
			if !ok || detail == nil {
				continue
			}
			flat := make(map[string]any, len(detail)+4)
			for key, value := range parent {
				if value != nil {
					flat[key] = value
				}
			}
			for key, value := range detail {
				flat[key] = value
			}
			out = append(out, flat)
		}
	}
	return out
}

func firstNonEmptyJSONString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringValueFromJSON(value)); text != "" {
			return text
		}
	}
	return ""
}

var leaveTypeFieldAliases = map[string]string{
	"code":        "假別代碼",
	"leave_code":  "假別代碼",
	"kind":        "節點類型",
	"parent_code": "上級假別代碼",
	"name":        "假別名稱",
	"name_zh":     "假別名稱",
	"leave_type":  "假別名稱",
	"name_en":     "英文名稱",
	"max_value":   "最大值",
	"maxValue":    "最大值",
	"unit":        "單位",
	"category":    "假別類別",
}

// NormalizeEmployeeRecords 將上游 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizeEmployeeRecords(rows []domain.EHRMSEmployeeRecord) []domain.EHRMSEmployeeRecord {
	return normalizeEmployeeRecords(rows)
}

func normalizeEmployeeRecords(rows []domain.EHRMSEmployeeRecord) []domain.EHRMSEmployeeRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSEmployeeRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizeEmployeeRecord(row))
	}
	return out
}

func normalizeEmployeeRecord(row domain.EHRMSEmployeeRecord) domain.EHRMSEmployeeRecord {
	return domain.EHRMSEmployeeRecord(applyFieldAliases(map[string]string(row), employeeFieldAliases))
}

// NormalizeDepartmentRecords 將上游部門 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizeDepartmentRecords(rows []domain.EHRMSDepartmentRecord) []domain.EHRMSDepartmentRecord {
	return normalizeDepartmentRecords(rows)
}

// NormalizePositionRecords 將上游崗位 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizePositionRecords(rows []domain.EHRMSPositionRecord) []domain.EHRMSPositionRecord {
	return normalizePositionRecords(rows)
}

// NormalizeAttendanceRecords 將上游考勤 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizeAttendanceRecords(rows []domain.EHRMSAttendanceRecord) []domain.EHRMSAttendanceRecord {
	return normalizeAttendanceRecords(rows)
}

// NormalizeLeaveBalanceRecords 將上游假別餘額 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizeLeaveBalanceRecords(rows []domain.EHRMSLeaveBalanceRecord) []domain.EHRMSLeaveBalanceRecord {
	return normalizeLeaveBalanceRecords(rows)
}

// NormalizeLeaveDetailRecords 將上游已休明細 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizeLeaveDetailRecords(rows []domain.EHRMSLeaveDetailRecord) []domain.EHRMSLeaveDetailRecord {
	return normalizeLeaveDetailRecords(rows)
}

// NormalizeLeaveTypeRecords 將上游假別目錄 JSON 欄位別名合併為服務層使用的 canonical key。
func NormalizeLeaveTypeRecords(rows []domain.EHRMSLeaveTypeRecord) []domain.EHRMSLeaveTypeRecord {
	return normalizeLeaveTypeRecords(rows)
}

func normalizeDepartmentRecords(rows []domain.EHRMSDepartmentRecord) []domain.EHRMSDepartmentRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSDepartmentRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizeDepartmentRecord(row))
	}
	return out
}

func normalizeDepartmentRecord(row domain.EHRMSDepartmentRecord) domain.EHRMSDepartmentRecord {
	return domain.EHRMSDepartmentRecord(applyFieldAliases(map[string]string(row), departmentFieldAliases))
}

func normalizePositionRecords(rows []domain.EHRMSPositionRecord) []domain.EHRMSPositionRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSPositionRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizePositionRecord(row))
	}
	return out
}

func normalizePositionRecord(row domain.EHRMSPositionRecord) domain.EHRMSPositionRecord {
	return domain.EHRMSPositionRecord(applyFieldAliases(map[string]string(row), positionFieldAliases))
}

func normalizeAttendanceRecords(rows []domain.EHRMSAttendanceRecord) []domain.EHRMSAttendanceRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSAttendanceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizeAttendanceRecord(row))
	}
	return out
}

func normalizeAttendanceRecord(row domain.EHRMSAttendanceRecord) domain.EHRMSAttendanceRecord {
	return domain.EHRMSAttendanceRecord(applyFieldAliases(map[string]string(row), attendanceFieldAliases))
}

func normalizeLeaveBalanceRecords(rows []domain.EHRMSLeaveBalanceRecord) []domain.EHRMSLeaveBalanceRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSLeaveBalanceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizeLeaveBalanceRecord(row))
	}
	return out
}

func normalizeLeaveBalanceRecord(row domain.EHRMSLeaveBalanceRecord) domain.EHRMSLeaveBalanceRecord {
	values := map[string]string(row)
	if strings.TrimSpace(values["假別"]) == "" {
		values["假別"] = firstNonEmptyString(
			values["leave_type"], values["name_zh"], values["name_en"], values["leave_code"],
		)
	}
	return domain.EHRMSLeaveBalanceRecord(applyFieldAliases(values, leaveBalanceFieldAliases))
}

func normalizeLeaveDetailRecords(rows []domain.EHRMSLeaveDetailRecord) []domain.EHRMSLeaveDetailRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSLeaveDetailRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizeLeaveDetailRecord(row))
	}
	return out
}

func normalizeLeaveDetailRecord(row domain.EHRMSLeaveDetailRecord) domain.EHRMSLeaveDetailRecord {
	values := map[string]string(row)
	if strings.TrimSpace(values["假別"]) == "" {
		values["假別"] = firstNonEmptyString(values["leave_type"], values["leave_code"])
	}
	return domain.EHRMSLeaveDetailRecord(applyFieldAliases(values, leaveDetailFieldAliases))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func normalizeLeaveTypeRecords(rows []domain.EHRMSLeaveTypeRecord) []domain.EHRMSLeaveTypeRecord {
	if len(rows) == 0 {
		return rows
	}
	out := make([]domain.EHRMSLeaveTypeRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, normalizeLeaveTypeRecord(row))
	}
	return out
}

func normalizeLeaveTypeRecord(row domain.EHRMSLeaveTypeRecord) domain.EHRMSLeaveTypeRecord {
	return domain.EHRMSLeaveTypeRecord(applyFieldAliases(map[string]string(row), leaveTypeFieldAliases))
}

func applyFieldAliases(row map[string]string, aliases map[string]string) map[string]string {
	if len(row) == 0 {
		return row
	}
	normalized := make(map[string]string, len(row)+len(aliases))
	for key, value := range row {
		normalized[key] = value
	}
	for alias, canonical := range aliases {
		if strings.TrimSpace(normalized[canonical]) != "" {
			continue
		}
		if value := strings.TrimSpace(normalized[alias]); value != "" {
			normalized[canonical] = value
		}
	}
	return normalized
}

func stringRecordFromJSON(values map[string]any) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = stringValueFromJSON(value)
	}
	return out
}

// stringValueFromJSON coerces upstream JSON scalars and arrays into sync-friendly strings.
// Arrays are joined with ", " so multi-value fields (e.g. tags/roles) do not fail decode.
func stringValueFromJSON(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if part := stringValueFromJSON(item); part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

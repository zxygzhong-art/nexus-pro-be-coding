package ehrms

import (
	"encoding/json"
	"fmt"
	"strings"

	"nexus-pro-be/internal/domain"
)

var employeeFieldAliases = map[string]string{
	"emp_id":          "員工編號",
	"name_zh":         "中文姓名",
	"name_en":         "英文姓名",
	"first_name":      "First Name",
	"last_name":       "Last Name",
	"gender":          "性別",
	"birthday":        "生日",
	"hire_date":       "到職日期",
	"seniority_start": "年資起始日",
	"probation_end":   "試用期滿日",
	"work_status":     "在職狀態",
	"nationality":     "國籍名稱",
	"national_id":     "身份證號",
	"passport_no":     "護照號碼",
	"identity_type":   "身份類別名稱",
	"education":       "最高學歷",
	"dept_code":       "部門代碼",
	"dept_name_zh":    "部門中文名稱",
	"dept_name_en":    "部門英文名稱",
	"job_code":        "職務代碼",
	"job_title_zh":    "職務中文名稱",
	"job_title_en":    "職務英文名稱",
	"card_no":         "卡號",
	"clock_required":  "上下班刷卡",
	"shift_name":      "員工班別名稱",
	"shift_attr":      "員工班別屬性",
	"direct_indirect": "直接/間接員工",
	"leave_group":     "休假群組",
	"school_name":     "學校名稱(中文)",
	"school_zh":       "學校名稱(中文)",
	"email":           "公司信箱",
}

var departmentFieldAliases = map[string]string{
	"code":        "部門代碼",
	"name":        "部門中文名稱",
	"name_zh":     "部門中文名稱",
	"name_en":     "部門英文名稱",
	"parent_code": "上級部門代碼",
	"depth":       "部門層級",
	"closed":      "部門已關閉",
	"headcount":   "部門人數",
}

var positionFieldAliases = map[string]string{
	"job_code":     "職務代碼",
	"job_title_zh": "職務中文名稱",
	"job_title_en": "職務英文名稱",
	"headcount":    "職務人數",
}

var attendanceFieldAliases = map[string]string{
	"emp_id":           "員工編號",
	"date":             "日期",
	"shift_start":      "班別開始",
	"shift_end":        "班別結束",
	"shift_hours":      "班別工時",
	"daily_hours":      "應出勤工時",
	"clock_hours":      "刷卡工時",
	"clock_start":      "clock_start",
	"clock_end":        "clock_end",
	"attend_start":     "attend_start",
	"attend_end":       "attend_end",
	"attend_hours":     "attend_hours",
	"attend_counted":   "attend_counted",
	"leave_type":       "leave_type",
	"leave_start":      "leave_start",
	"leave_end":        "leave_end",
	"leave_hours":      "leave_hours",
	"leave_counted":    "leave_counted",
	"leave2_type":      "leave2_type",
	"leave2_start":     "leave2_start",
	"leave2_end":       "leave2_end",
	"leave2_hours":     "leave2_hours",
	"leave2_counted":   "leave2_counted",
	"overtime_start":   "overtime_start",
	"overtime_end":     "overtime_end",
	"overtime_hours":   "overtime_hours",
	"overtime_counted": "overtime_counted",
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
		switch v := value.(type) {
		case nil:
			out[key] = ""
		case string:
			out[key] = strings.TrimSpace(v)
		case json.Number:
			out[key] = v.String()
		default:
			out[key] = strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return out
}

package domain

import "strings"

// agent_catalog.go — Agent 工具目錄與在地化輔助（純函式，service 與 service/agent 共用）。

// AgentToolCatalog 是 Agent 工具的靜態目錄（授權 tuple 開通與後台清單的共同真相）。
func AgentToolCatalog() []AgentToolMeta {
	return []AgentToolMeta{
		{Value: "knowledge.search", Label: "Knowledge Search", Description: "Search tenant knowledge content.", DescriptionZhTW: "搜尋目前租戶已綁定的知識庫內容。", Category: "knowledge", Readonly: true, RequiredPermission: "agent.tool.call:knowledge.search"},
		{Value: "get_my_profile", Label: "My Profile", Description: "Read current account profile.", DescriptionZhTW: "讀取目前登入帳號與員工基本資料。", Category: "profile", Readonly: true, RequiredPermission: "agent.tool.call:get_my_profile"},
		{Value: "list_employees", Label: "List Employees", Description: "Read employee summaries.", DescriptionZhTW: "依權限範圍列出員工摘要。", Category: "employee", Readonly: true, RequiredPermission: "agent.tool.call:list_employees"},
		{Value: "get_employee", Label: "Get Employee", Description: "Read one employee summary.", DescriptionZhTW: "依員工 ID 讀取單一員工的受限摘要。", Category: "employee", Readonly: true, RequiredPermission: "agent.tool.call:get_employee"},
		{Value: "my_leave_balances", Label: "My Leave Balances", Description: "Read current account leave balances.", DescriptionZhTW: "讀取目前員工的各類假期餘額。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:my_leave_balances"},
		{Value: "check_leave_eligibility", Label: "Check Leave Eligibility", Description: "Check leave policy and choose balance reservation or the non-blocking no-balance fallback.", DescriptionZhTW: "依現行假勤政策、日期、時數與餘額檢查請假資格。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:check_leave_eligibility"},
		{Value: "my_clock_records", Label: "My Clock Records", Description: "Read current account clock records.", DescriptionZhTW: "讀取目前員工近期的上下班打卡紀錄。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:my_clock_records"},
		{Value: "my_attendance_summary", Label: "My Attendance Summary", Description: "Read the current employee's monthly attendance summary.", DescriptionZhTW: "讀取目前員工本月出勤、工時、請假與加班摘要。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:my_attendance_summary"},
		{Value: "clock_in_or_out", Label: "Clock In Or Out", Description: "Prepare a geofenced clock-in or clock-out for explicit confirmation.", DescriptionZhTW: "依定位資料準備上下班打卡確認卡。", Category: "attendance", Readonly: false, RequiredPermission: "agent.tool.call:clock_in_or_out"},
		{Value: "my_attendance_anomalies", Label: "My Attendance Anomalies", Description: "Read current employee attendance anomaly days for a month.", DescriptionZhTW: "讀取目前員工指定月份的遲到、早退與缺卡異常。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:my_attendance_anomalies"},
		{Value: "team_attendance_today", Label: "Team Attendance Today", Description: "Read the authorized team's attendance state for today.", DescriptionZhTW: "讀取目前權限範圍內團隊今日出勤狀態。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:team_attendance_today"},
		{Value: "team_attendance_anomalies", Label: "Team Attendance Anomalies", Description: "Read authorized team attendance anomaly rows.", DescriptionZhTW: "讀取目前權限範圍內團隊異常員工明細。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:team_attendance_anomalies"},
		{Value: "schedule_coverage_gaps", Label: "Schedule Coverage Gaps", Description: "Find authorized employees with scheduled attendance coverage gaps.", DescriptionZhTW: "找出目前權限範圍內有排班出勤缺口的人員。", Category: "attendance", Readonly: true, RequiredPermission: "agent.tool.call:schedule_coverage_gaps"},
		{Value: "schedule_notification", Label: "Schedule Notification", Description: "Prepare an attendance or schedule reminder for explicit confirmation.", DescriptionZhTW: "準備考勤或排班提醒通知確認卡。", Category: "notification", Readonly: false, RequiredPermission: "agent.tool.call:schedule_notification"},
		{Value: "my_form_history", Label: "My Form History", Description: "Read the current account's own form application history.", DescriptionZhTW: "依表單類型與狀態讀取目前帳號的申請紀錄。", Category: "form_application", Readonly: true, RequiredPermission: "agent.tool.call:my_form_history"},
		{Value: "withdraw_or_cancel_leave_request", Label: "Withdraw Leave Request", Description: "Prepare withdrawal of the current employee's submitted leave request.", DescriptionZhTW: "準備撤回或銷假目前員工已送出的假單。", Category: "form_application", Readonly: false, RequiredPermission: "agent.tool.call:withdraw_or_cancel_leave_request"},
		{Value: "my_pending_reviews", Label: "My Pending Reviews", Description: "Read pending workflow reviews.", DescriptionZhTW: "讀取目前帳號可以處理的待審核表單。", Category: "approval", Readonly: true, RequiredPermission: "agent.tool.call:my_pending_reviews"},
		{Value: "workspace_insights", Label: "Workspace Insights", Description: "Read workspace insight reports.", DescriptionZhTW: "讀取指定月份的工作區洞察指標與報表摘要。", Category: "analytics", Readonly: true, RequiredPermission: "agent.tool.call:workspace_insights"},
		{Value: "hr_attendance_anomaly_report", Label: "HR Attendance Anomaly Report", Description: "Read a bounded HR attendance anomaly report.", DescriptionZhTW: "讀取具分頁與嚴重度篩選的 HR 考勤異常報表。", Category: "analytics", Readonly: true, RequiredPermission: "agent.tool.call:hr_attendance_anomaly_report"},
		{Value: "payroll_attendance_export", Label: "Payroll Attendance Export", Description: "Generate a bounded payroll attendance CSV export.", DescriptionZhTW: "產生薪資結算前使用的考勤 CSV。", Category: "analytics", Readonly: true, RequiredPermission: "agent.tool.call:payroll_attendance_export"},
		{Value: "hr_faq_insights", Label: "HR FAQ Insights", Description: "Read recent high-frequency prompts recorded for employee-service agents.", DescriptionZhTW: "讀取員工服務 Agent 已記錄的熱門問題。", Category: "analytics", Readonly: true, RequiredPermission: "agent.tool.call:hr_faq_insights"},
		{Value: "employee_bulk_import", Label: "Employee Bulk Import", Description: "Prepare a bounded employee list import for explicit confirmation.", DescriptionZhTW: "準備批次匯入員工名單確認卡。", Category: "employee", Readonly: false, RequiredPermission: "agent.tool.call:employee_bulk_import"},
		{Value: "employee_lifecycle_change", Label: "Employee Lifecycle Change", Description: "Prepare leave suspension, reinstatement, or another employee status transition.", DescriptionZhTW: "準備留職停薪、復職或其他員工狀態異動確認卡。", Category: "employee", Readonly: false, RequiredPermission: "agent.tool.call:employee_lifecycle_change"},
		{Value: "employee_change_history", Label: "Employee Change History", Description: "Read authorized employee lifecycle and internal experience history.", DescriptionZhTW: "讀取授權範圍內員工的生命週期與內部異動歷程。", Category: "employee", Readonly: true, RequiredPermission: "agent.tool.call:employee_change_history"},
		{Value: "create_hr_handoff_ticket", Label: "Create HR Handoff Ticket", Description: "Create an HR handoff form draft and prepare it for confirmation.", DescriptionZhTW: "建立 HR 轉交單草稿並產生送出確認卡。", Category: "form_application", Readonly: false, RequiredPermission: "agent.tool.call:create_hr_handoff_ticket"},
		{Value: "list_published_form_templates", Label: "Published Forms", Description: "List published forms available to the current account.", DescriptionZhTW: "列出目前帳號可以使用的已發布表單。", Category: "form_application", Readonly: true, RequiredPermission: "agent.tool.call:list_published_form_templates"},
		{Value: "get_published_form_template", Label: "Form Schema", Description: "Read an Agent-safe form schema and data sources.", DescriptionZhTW: "讀取已發布表單的欄位、資料來源與簽核路徑。", Category: "form_application", Readonly: true, RequiredPermission: "agent.tool.call:get_published_form_template"},
		{Value: "create_form_draft", Label: "Create Form Draft", Description: "Create a reversible form draft for the current account.", DescriptionZhTW: "為目前帳號建立可撤銷的表單草稿。", Category: "form_application", Readonly: false, RequiredPermission: "agent.tool.call:create_form_draft"},
		{Value: "update_form_draft", Label: "Update Form Draft", Description: "Update a reversible form draft owned by the current account.", DescriptionZhTW: "更新目前帳號擁有的可撤銷表單草稿。", Category: "form_application", Readonly: false, RequiredPermission: "agent.tool.call:update_form_draft"},
		{Value: "preview_form_submission", Label: "Preview Form Submission", Description: "Validate a draft and prepare explicit user confirmation.", DescriptionZhTW: "驗證表單草稿並產生送出前的使用者確認卡。", Category: "form_application", Readonly: true, RequiredPermission: "agent.tool.call:preview_form_submission"},
		{Value: "prepare_bulk_review", Label: "Prepare Bulk Review", Description: "Prepare a fixed review batch for explicit user confirmation.", DescriptionZhTW: "固定一批待審項目並產生批次審核確認卡。", Category: "approval", Readonly: true, RequiredPermission: "agent.tool.call:prepare_bulk_review"},
		{Value: "form.get_capabilities", Label: "Form Builder Capabilities", Description: "Read controlled form schema, widgets, data-source metadata, and workflow roles.", DescriptionZhTW: "讀取表單設計可用的欄位、元件、資料來源與流程角色。", Category: "form_design", Readonly: true, RequiredPermission: "agent.tool.call:form.get_capabilities"},
		{Value: "form.get_data_source_schema", Label: "Form Data Source Schema", Description: "Read metadata-only data-source fields for form authoring.", DescriptionZhTW: "讀取表單設計可用的資料來源欄位中繼資料。", Category: "form_design", Readonly: true, RequiredPermission: "agent.tool.call:form.get_data_source_schema"},
		{Value: "form.create_draft", Label: "Create Form Definition Draft", Description: "Create a reversible Agent-authored form definition draft.", DescriptionZhTW: "建立可撤銷的 Agent 表單定義草稿。", Category: "form_design", Readonly: false, RequiredPermission: "agent.tool.call:form.create_draft"},
		{Value: "form.update_draft", Label: "Update Form Definition Draft", Description: "Update an Agent-authored form definition draft with revision protection.", DescriptionZhTW: "在版本保護下更新 Agent 表單定義草稿。", Category: "form_design", Readonly: false, RequiredPermission: "agent.tool.call:form.update_draft"},
		{Value: "form.validate_draft", Label: "Validate Form Definition Draft", Description: "Validate and compile a controlled form definition draft.", DescriptionZhTW: "驗證並編譯受控的表單定義草稿。", Category: "form_design", Readonly: true, RequiredPermission: "agent.tool.call:form.validate_draft"},
		{Value: "form.preview_draft", Label: "Preview Form Definition Draft", Description: "Preview a controlled form definition draft.", DescriptionZhTW: "預覽表單定義草稿，不產生業務副作用。", Category: "form_design", Readonly: true, RequiredPermission: "agent.tool.call:form.preview_draft"},
		{Value: "form.simulate_workflow", Label: "Simulate Form Workflow", Description: "Simulate the form approval path without starting a real workflow.", DescriptionZhTW: "模擬表單簽核路徑，但不啟動真實流程。", Category: "form_design", Readonly: true, RequiredPermission: "agent.tool.call:form.simulate_workflow"},
	}
}

// LocalizedSuggestedQuestions 依 locale 選擇建議問題文案。
func LocalizedSuggestedQuestions(
	items []LocalizedAgentSuggestedQuestion,
	locale string,
	fallback []string,
) []string {
	locale = PreferredLocaleWithDefault(locale)
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item.Translations[locale])
		if value == "" {
			value = strings.TrimSpace(item.Translations[DefaultPreferredLocale])
		}
		if value == "" {
			value = strings.TrimSpace(item.Translations[PreferredLocaleENUS])
		}
		if value != "" {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return uniqueStrings(fallback)
	}
	return result
}

// uniqueStrings 去重去空（domain 內部小工具，與 service 同名函式相同語意）。
func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

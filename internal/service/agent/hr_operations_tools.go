package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

const (
	maxAgentPayrollExportBytes = 2 << 20
	maxAgentEmployeeImportRows = 50
)

func (c AgentService) toolMyAttendanceAnomalies(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	summary, err := c.Attendance().AttendanceMonthlySummary(ctx, strings.TrimSpace(stringFromAny(args["month"])))
	if err != nil {
		return nil, err
	}
	items := make([]AttendanceMonthlyDaySummary, 0, summary.AbnormalDays)
	for _, day := range summary.Days {
		if day.DayStatus == "abnormal" || len(day.AnomalyReasons) > 0 {
			items = append(items, day)
		}
	}
	return map[string]any{"employee_id": summary.EmployeeID, "month": summary.Month, "items": items, "total": len(items)}, nil
}

func (c AgentService) toolTeamAttendanceToday(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	now := c.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	period, err := c.Workspace().WorkspaceAttendance(ctx, WorkspaceAttendanceQuery{
		Year:         now.Year(),
		Month:        int(now.Month()),
		Projection:   "clock",
		DepartmentID: strings.TrimSpace(stringFromAny(args["department_id"])),
		Keyword:      strings.TrimSpace(stringFromAny(args["keyword"])),
		Page:         1,
		PageSize:     intFromToolArgs(args, "limit", 50, 100),
		Paginated:    true,
	})
	if err != nil {
		return nil, err
	}
	dateIndex := -1
	today := now.Format(time.DateOnly)
	for index, date := range period.Dates {
		if fmt.Sprintf("%04d-%02d-%02d", date.Y, date.M, date.D) == today {
			dateIndex = index
			break
		}
	}
	items := make([]map[string]any, 0, len(period.Clock.Rows))
	for _, row := range period.Clock.Rows {
		if dateIndex < 0 || dateIndex >= len(row.Cells) {
			continue
		}
		cell := row.Cells[dateIndex]
		items = append(items, map[string]any{
			"employee": row.Employee, "date": today, "clock_in": cell.In, "clock_out": cell.Out,
			"recorded": cell.Recorded, "abnormal": cell.Abnormal, "reason": cell.Reason,
		})
	}
	return map[string]any{"date": today, "items": items, "pagination": period.Pagination}, nil
}

func (c AgentService) toolTeamAttendanceAnomalies(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	report, err := c.Workspace().WorkspaceClockAbnormals(ctx, workspaceAbnormalQueryFromToolArgs(args, c.Now()))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"items": report.Items, "pagination": report.Pagination,
		"employee_pagination": report.EmployeePagination, "summary_scope": report.SummaryScope,
	}, nil
}

func (c AgentService) toolScheduleCoverageGaps(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	result, err := c.toolTeamAttendanceToday(ctx, args)
	if err != nil {
		return nil, err
	}
	rows, _ := result["items"].([]map[string]any)
	gaps := make([]map[string]any, 0)
	for _, row := range rows {
		recorded, _ := row["recorded"].(bool)
		abnormal, _ := row["abnormal"].(bool)
		if recorded && !abnormal {
			continue
		}
		reason := strings.TrimSpace(stringFromAny(row["reason"]))
		if reason == "" {
			reason = "no accepted attendance record for the scheduled day"
		}
		gaps = append(gaps, map[string]any{"employee": row["employee"], "date": row["date"], "reason": reason})
	}
	return map[string]any{
		"date": result["date"], "items": gaps, "total": len(gaps),
		"definition": "coverage gaps are scheduled employees without a complete normal attendance record; staffing demand targets are not inferred",
	}, nil
}

func (c AgentService) toolHRAttendanceAnomalyReport(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	result, err := c.toolTeamAttendanceAnomalies(ctx, args)
	if err != nil {
		return nil, err
	}
	threshold := floatFromToolArgs(args, "overtime_threshold_hours")
	if threshold <= 0 {
		result["overtime_alerts"] = []map[string]any{}
		result["overtime_threshold_hours"] = nil
		return result, nil
	}
	year, month := yearMonthFromToolArgs(args, c.Now())
	attendance, err := c.Workspace().WorkspaceAttendance(ctx, WorkspaceAttendanceQuery{
		Year: year, Month: month, Projection: "attendance",
		DepartmentID: strings.TrimSpace(stringFromAny(args["department_id"])),
		Page:         1, PageSize: intFromToolArgs(args, "employee_page_size", 50, 100), Paginated: true,
	})
	if err != nil {
		return nil, err
	}
	alerts := make([]map[string]any, 0)
	for _, row := range attendance.Attendance.Rows {
		if row.Summary.OvertimeHours > threshold {
			alerts = append(alerts, map[string]any{
				"employee": row.Employee, "overtime_hours": row.Summary.OvertimeHours,
				"threshold_hours": threshold, "excess_hours": row.Summary.OvertimeHours - threshold,
			})
		}
	}
	result["overtime_alerts"] = alerts
	result["overtime_threshold_hours"] = threshold
	return result, nil
}

func (c AgentService) toolPayrollAttendanceExport(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	year, month := yearMonthFromToolArgs(args, c.Now())
	kind := strings.TrimSpace(stringFromAny(args["kind"]))
	if kind == "" {
		kind = "attendance"
	}
	raw, filename, err := c.Workspace().ExportWorkspaceAttendanceCSV(ctx, WorkspaceAttendanceQuery{
		Year: year, Month: month, DepartmentID: strings.TrimSpace(stringFromAny(args["department_id"])),
	}, kind)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxAgentPayrollExportBytes {
		return nil, BadRequest("attendance export exceeds the 2 MiB Agent download limit; use the workspace export endpoint")
	}
	return map[string]any{
		"filename": filename, "content_type": "text/csv; charset=utf-8",
		"encoding": "base64", "content_base64": base64.StdEncoding.EncodeToString(raw), "size_bytes": len(raw),
	}, nil
}

func (c AgentService) toolEmployeeChangeHistory(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	employeeID := strings.TrimSpace(stringFromAny(args["employee_id"]))
	if employeeID == "" {
		return nil, BadRequest("employee_id is required")
	}
	employee, err := c.HR().GetEmployee(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"employee": map[string]any{
			"id": employee.ID, "employee_no": employee.EmployeeNo, "name": employee.Name,
			"status": employee.Status, "employment_status": employee.EmploymentStatus,
			"org_unit_id": employee.OrgUnitID, "position_id": employee.PositionID, "position": employee.Position,
			"updated_at": employee.UpdatedAt,
		},
		"internal_experiences": employee.InternalExperiences,
		"transition": map[string]any{
			"type": employee.EmploymentInfo["transition_type"], "reason": employee.EmploymentInfo["transition_reason"],
			"start_date": employee.EmploymentInfo["transition_start_date"], "end_date": employee.EmploymentInfo["transition_end_date"],
		},
	}, nil
}

func (c AgentService) toolHRFAQInsights(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceType("usage"), ActionRead, ""); err != nil {
		return nil, err
	}
	definitions, err := c.store.ListAgentDefinitions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0)
	for _, definition := range definitions {
		if !strings.Contains(strings.ToLower(definition.Name+" "+definition.Description), "員工服務") &&
			!strings.Contains(strings.ToLower(definition.Name+" "+definition.Description), "employee service") {
			continue
		}
		items = append(items, map[string]any{
			"agent_id": definition.ID, "agent_name": definition.Name,
			"top_prompts": definition.Usage.TopPrompts, "run_count": definition.Usage.TotalRuns,
		})
	}
	return map[string]any{"items": items, "definition": "top_prompts are the recent prompts retained by Agent usage statistics"}, nil
}

func (c AgentService) toolClockInOrOut(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	status, err := c.Attendance().AttendanceClockStatus(ctx)
	if err != nil {
		return nil, err
	}
	direction := strings.ToLower(strings.TrimSpace(stringFromAny(args["direction"])))
	if direction == "" {
		direction = status.NextAction
	}
	if direction != "in" && direction != "out" {
		return nil, BadRequest("direction must be in or out")
	}
	arguments := make(map[string]any, len(args)+1)
	for key, value := range args {
		arguments[key] = value
	}
	arguments["direction"] = direction
	return c.prepareInternalAction(ctx, "clock_in_or_out", "確認考勤打卡", "請確認定位與打卡方向後執行。", "確認打卡", arguments, []domain.AgentAnalysisRow{
		{Label: "方向", Value: direction}, {Label: "工作日", Value: status.WorkDate},
	})
}

func (c AgentService) toolWithdrawLeaveRequest(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringFromAny(args["form_instance_id"]))
	if id == "" {
		return nil, BadRequest("form_instance_id is required")
	}
	detail, err := c.Workflow().GetFormInstanceDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail.TemplateKey != "leave-request" && detail.TemplateKey != "leave-cancel" {
		return nil, BadRequest("form instance is not a leave request")
	}
	return c.prepareInternalAction(ctx, "withdraw_or_cancel_leave_request", "確認撤回假單", "撤回後將停止目前簽核流程。", "確認撤回", args, []domain.AgentAnalysisRow{
		{Label: "假單", Value: detail.TemplateName}, {Label: "狀態", Value: detail.Status},
	})
}

func (c AgentService) toolScheduleNotification(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	employeeIDs := stringSliceFromToolArgs(args["employee_ids"])
	if len(employeeIDs) == 0 {
		return nil, BadRequest("employee_ids is required")
	}
	if strings.TrimSpace(stringFromAny(args["title"])) == "" || strings.TrimSpace(stringFromAny(args["body"])) == "" {
		return nil, BadRequest("title and body are required")
	}
	for _, employeeID := range employeeIDs {
		if _, err := c.HR().GetEmployee(ctx, employeeID); err != nil {
			return nil, err
		}
	}
	return c.prepareInternalAction(ctx, "schedule_notification", "確認發送排班提醒", "通知只會送給目前權限範圍內且已綁定帳號的員工。", "確認發送", args, []domain.AgentAnalysisRow{
		{Label: "收件人數", Value: fmt.Sprintf("%d", len(employeeIDs))},
		{Label: "標題", Value: strings.TrimSpace(stringFromAny(args["title"]))},
	})
}

func (c AgentService) toolEmployeeBulkImport(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	var employees []CreateEmployeeInput
	if err := decodeToolValue(args["employees"], &employees); err != nil {
		return nil, BadRequest("employees must be an array of employee objects")
	}
	if len(employees) == 0 || len(employees) > maxAgentEmployeeImportRows {
		return nil, BadRequest("employees must contain between 1 and 50 rows")
	}
	rows := make([]domain.AgentAnalysisRow, 0, len(employees))
	for index, employee := range employees {
		if _, err := c.HR().PreviewCreateEmployee(ctx, employee); err != nil {
			return nil, fmt.Errorf("employee row %d: %w", index+1, err)
		}
		rows = append(rows, domain.AgentAnalysisRow{Label: fmt.Sprintf("第 %d 筆", index+1), Value: firstNonEmptyToolValue(employee.Name, employee.CompanyEmail)})
	}
	return c.prepareInternalAction(ctx, "employee_bulk_import", "確認批次匯入員工", "所有資料已通過建立前驗證。", "確認匯入", args, rows)
}

func (c AgentService) toolEmployeeLifecycleChange(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	employeeID := strings.TrimSpace(stringFromAny(args["employee_id"]))
	status := strings.TrimSpace(stringFromAny(args["status"]))
	if employeeID == "" || status == "" {
		return nil, BadRequest("employee_id and status are required")
	}
	employee, err := c.HR().GetEmployee(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	return c.prepareInternalAction(ctx, "employee_lifecycle_change", "確認員工狀態異動", "此操作會更新員工生命週期與帳號狀態。", "確認異動", args, []domain.AgentAnalysisRow{
		{Label: "員工", Value: employee.Name}, {Label: "目前狀態", Value: employee.EmploymentStatus}, {Label: "新狀態", Value: status},
	})
}

func (c AgentService) toolCreateHRHandoffTicket(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"category":            strings.TrimSpace(stringFromAny(args["category"])),
		"question":            strings.TrimSpace(stringFromAny(args["question"])),
		"checked_sources":     args["checked_sources"],
		"missing_information": strings.TrimSpace(stringFromAny(args["missing_information"])),
	}
	created, err := c.Service.ToolCreateFormDraft(ctx, map[string]any{"template_key": "hr-handoff", "payload": payload})
	if err != nil {
		return nil, err
	}
	draft, ok := created["draft"].(domain.FormInstance)
	if !ok {
		return nil, domain.E(500, "agent_hr_handoff_invalid", "HR handoff draft response is invalid")
	}
	return c.Service.ToolPreviewFormSubmission(ctx, map[string]any{"draft_id": draft.ID})
}

func (c AgentService) prepareInternalAction(ctx domain.RequestContext, action, title, description, label string, args map[string]any, rows []domain.AgentAnalysisRow) (map[string]any, error) {
	confirmation, err := c.Service.PrepareAgentInternalActionConfirmation(ctx, action, title, description, label, args, rows)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "confirmation_required", "confirmation": confirmation}, nil
}

func workspaceAbnormalQueryFromToolArgs(args map[string]any, now time.Time) domain.WorkspaceClockAbnormalQuery {
	year, month := yearMonthFromToolArgs(args, now)
	return domain.WorkspaceClockAbnormalQuery{
		Year: year, Month: month,
		BaseDepartmentID: strings.TrimSpace(stringFromAny(args["base_department_id"])),
		DepartmentID:     strings.TrimSpace(stringFromAny(args["department_id"])),
		Keyword:          strings.TrimSpace(stringFromAny(args["keyword"])),
		Severity:         strings.TrimSpace(stringFromAny(args["severity"])),
		Page:             intFromToolArgs(args, "page", 1, 1000),
		PageSize:         intFromToolArgs(args, "page_size", 20, 100),
		EmployeePage:     intFromToolArgs(args, "employee_page", 1, 1000),
		EmployeePageSize: intFromToolArgs(args, "employee_page_size", 50, 100),
	}
}

func yearMonthFromToolArgs(args map[string]any, now time.Time) (int, int) {
	month := strings.TrimSpace(stringFromAny(args["month"]))
	if parsed, err := time.Parse("2006-01", month); err == nil {
		return parsed.Year(), int(parsed.Month())
	}
	return now.Year(), int(now.Month())
}

func stringSliceFromToolArgs(value any) []string {
	var values []string
	if decodeToolValue(value, &values) != nil {
		return nil
	}
	return uniqueStrings(values)
}

func decodeToolValue(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func firstNonEmptyToolValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

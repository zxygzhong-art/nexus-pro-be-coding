package service

import (
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
	"sort"
	"strings"
	"time"
)

// workspaceFormTemplateStatus 將設計器啟用狀態投影成模板生命週期。
func workspaceFormTemplateStatus(enabled, deleted bool) string {
	if deleted {
		return "archived"
	}
	if enabled {
		return "published"
	}
	return "draft"
}

func (c WorkspaceService) Workspace(ctx RequestContext) (PlatformWorkspaceResponse, error) {
	auditLogs, err := c.workspaceAuditLogsForAggregate(ctx)
	if err != nil {
		return PlatformWorkspaceResponse{}, err
	}
	leavePolicy, err := c.Service.Attendance().CurrentAttendancePolicy(ctx)
	if err != nil {
		return PlatformWorkspaceResponse{}, err
	}
	formDesign, err := c.formDesign(ctx)
	if err != nil {
		return PlatformWorkspaceResponse{}, err
	}
	return PlatformWorkspaceResponse{
		AuditLogs:   auditLogs,
		FormDesign:  formDesign,
		LeavePolicy: leavePolicy,
	}, nil
}

// workspaceAuditLogsForAggregate 處理工作區稽核 logs for aggregate 的服務流程。
func (c WorkspaceService) workspaceAuditLogsForAggregate(ctx RequestContext) ([]WorkspaceAuditLog, error) {
	auditLogs, err := c.WorkspaceAuditLogs(ctx, WorkspaceAuditLogQuery{}, PageRequest{Page: 1, PageSize: 50, Sort: "created_at_desc"})
	if err != nil {
		return nil, err
	}
	return auditLogs.Items, nil
}

// WorkspaceEmployees 處理工作區員工的服務流程。
func (c WorkspaceService) WorkspaceEmployees(ctx RequestContext, query PlatformWorkspaceEmployeesQuery) (PlatformWorkspaceEmployeesResponse, error) {
	employees, err := c.visibleWorkspaceEmployees(ctx, "platform.workspace.employees")
	if err != nil {
		return PlatformWorkspaceEmployeesResponse{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PlatformWorkspaceEmployeesResponse{}, err
	}
	cardsByID := workspaceEmployeeCards(employees, workspaceOrgNames(units))
	items := make([]WorkspaceEmployeeCard, 0, len(employees))
	for _, employee := range employees {
		if card, ok := cardsByID[employee.ID]; ok {
			if !platformWorkspaceEmployeeMatches(query, employee, card) {
				continue
			}
			items = append(items, card)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return PlatformWorkspaceEmployeesResponse{
		Employees:  items,
		CSVHeaders: []string{"員工編號", "姓名", "Email", "部門", "職位", "類別", "電話", "狀態", "到職日期"},
	}, nil
}

// WorkspaceFormDesign 處理工作區表單 design 讀取的服務流程。
func (c WorkspaceService) WorkspaceFormDesign(ctx RequestContext) (PlatformFormDesign, error) {
	return c.formDesign(ctx)
}

// CreateWorkspaceFormDesign 建立工作區表單 design 的服務流程。
func (c WorkspaceService) CreateWorkspaceFormDesign(ctx RequestContext, input SaveWorkspaceFormDesignInput) (PlatformFormDesign, error) {
	key := workspaceFormDesignKey(input.ID, input.Name, c.Now())
	if strings.TrimSpace(input.Name) == "" {
		return PlatformFormDesign{}, BadRequest("name is required")
	}
	if err := validateWorkspaceFormDesignInput(input.Fields, input.Stages); err != nil {
		return PlatformFormDesign{}, err
	}
	if err := validateSystemFormFieldLocks(key, input.FormKind, input.Fields); err != nil {
		return PlatformFormDesign{}, err
	}
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceType("form_template"), ResourceID: key, Action: ActionCreate},
		AuditTarget{Event: "platform.workspace.form_design.create", Resource: "form_template", Target: key},
	)
	if err != nil {
		return PlatformFormDesign{}, err
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		workspace := tx.Workspace()
		current, exists, err := workspace.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, key)
		if err != nil {
			return err
		}
		if exists && !platformTemplateDeleted(current.Schema) {
			return domain.Conflict("form template key already exists")
		}
		now := workspace.Now()
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		templateID := utils.NewID("ft")
		createdAt := now
		currentVersion := 1
		if exists {
			templateID = current.ID
			createdAt = current.CreatedAt
			currentVersion = max(current.CurrentVersion, 1) + 1
			if createdAt.IsZero() {
				createdAt = now
			}
		}
		template := FormTemplate{
			ID:             templateID,
			TenantID:       ctx.TenantID,
			Key:            key,
			Name:           strings.TrimSpace(input.Name),
			Description:    strings.TrimSpace(input.Desc),
			Schema:         workspaceFormDesignSchema(nil, input, enabled, false, now),
			Status:         workspaceFormTemplateStatus(enabled, false),
			CurrentVersion: currentVersion,
			CreatedAt:      createdAt,
			UpdatedAt:      now,
		}
		if err := workspace.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
			return err
		}
		if err := tx.audit(ctx, "platform.workspace.form_design.create", "form_template", template.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"template_key": template.Key,
			"name":         template.Name,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx)
	}); err != nil {
		return PlatformFormDesign{}, err
	}
	return c.formDesign(ctx)
}

// UpdateWorkspaceFormDesign 更新工作區表單 design 的服務流程。
func (c WorkspaceService) UpdateWorkspaceFormDesign(ctx RequestContext, id string, input UpdateWorkspaceFormDesignInput) (PlatformFormDesign, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PlatformFormDesign{}, BadRequest("id is required")
	}
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceType("form_template"), ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "platform.workspace.form_design.update", Resource: "form_template", Target: id},
	)
	if err != nil {
		return PlatformFormDesign{}, err
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		workspace := tx.Workspace()
		template, err := workspace.currentWorkspaceFormTemplate(ctx, id)
		if err != nil {
			return err
		}
		if platformTemplateDeleted(template.Schema) {
			return NotFound("form template", id)
		}
		next := workspaceFormDesignInputFromTemplate(template)
		if input.Icon != nil {
			next.Icon = strings.TrimSpace(*input.Icon)
		}
		if input.Name != nil {
			next.Name = strings.TrimSpace(*input.Name)
		}
		if input.Category != nil {
			next.Category = strings.TrimSpace(*input.Category)
		}
		if input.Desc != nil {
			next.Desc = strings.TrimSpace(*input.Desc)
		}
		if input.Enabled != nil {
			next.Enabled = input.Enabled
		}
		if input.FormKind != nil {
			next.FormKind = strings.TrimSpace(*input.FormKind)
		}
		if input.Fields != nil {
			next.Fields = *input.Fields
		}
		if input.Stages != nil {
			next.Stages = *input.Stages
		}
		if strings.TrimSpace(next.Name) == "" {
			return BadRequest("name is required")
		}
		// Only enforce design contract when fields/stages are explicitly patched.
		// Enable/disable and metadata-only updates should not block on legacy templates.
		if input.Fields != nil || input.Stages != nil {
			if err := validateWorkspaceFormDesignInput(next.Fields, next.Stages); err != nil {
				return err
			}
			if err := validateSystemFormFieldLocks(template.Key, next.FormKind, next.Fields); err != nil {
				return err
			}
			if input.Fields != nil && template.Status == "published" {
				if err := ValidatePublishedFormFieldIdentity(platformTemplateFields(template.Key, template.Schema), next.Fields); err != nil {
					return err
				}
			}
		}
		enabled := true
		if next.Enabled != nil {
			enabled = *next.Enabled
		}
		now := workspace.Now()
		template.Name = strings.TrimSpace(next.Name)
		template.Description = strings.TrimSpace(next.Desc)
		template.Schema = workspaceFormDesignSchema(template.Schema, next, enabled, false, now)
		template.Status = workspaceFormTemplateStatus(enabled, false)
		template.CurrentVersion = max(template.CurrentVersion, 1) + 1
		template.UpdatedAt = now
		template.DeletedAt = nil
		if err := workspace.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
			return err
		}
		if err := tx.audit(ctx, "platform.workspace.form_design.update", "form_template", template.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"template_key": template.Key,
			"name":         template.Name,
			"enabled":      enabled,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx)
	}); err != nil {
		return PlatformFormDesign{}, err
	}
	return c.formDesign(ctx)
}

// DeleteWorkspaceFormDesign 刪除工作區表單 design 的服務流程。
func (c WorkspaceService) DeleteWorkspaceFormDesign(ctx RequestContext, id string) (PlatformFormDesign, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PlatformFormDesign{}, BadRequest("id is required")
	}
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceType("form_template"), ResourceID: id, Action: ActionDelete},
		AuditTarget{Event: "platform.workspace.form_design.delete", Resource: "form_template", Target: id},
	)
	if err != nil {
		return PlatformFormDesign{}, err
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		workspace := tx.Workspace()
		template, err := workspace.currentWorkspaceFormTemplate(ctx, id)
		if err != nil {
			return err
		}
		if platformTemplateDeleted(template.Schema) {
			return NotFound("form template", id)
		}
		next := workspaceFormDesignInputFromTemplate(template)
		disabled := false
		next.Enabled = &disabled
		now := workspace.Now()
		template.Schema = workspaceFormDesignSchema(template.Schema, next, false, true, now)
		template.Status = workspaceFormTemplateStatus(false, true)
		template.CurrentVersion = max(template.CurrentVersion, 1) + 1
		template.UpdatedAt = now
		template.DeletedAt = &now
		if err := workspace.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
			return err
		}
		if err := tx.audit(ctx, "platform.workspace.form_design.delete", "form_template", template.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"template_key": template.Key,
			"name":         template.Name,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx)
	}); err != nil {
		return PlatformFormDesign{}, err
	}
	return c.formDesign(ctx)
}

// formDesign 處理表單 design 的服務流程。
func (c WorkspaceService) formDesign(ctx RequestContext) (PlatformFormDesign, error) {
	templates, err := c.Service.Workflow().ListFormTemplates(ctx)
	if err != nil {
		return PlatformFormDesign{}, err
	}
	forms := make([]PlatformFormDesignForm, 0, len(templates))
	hasTemplates := len(templates) > 0
	for _, template := range templates {
		if platformTemplateDeleted(template.Schema) {
			continue
		}
		forms = append(forms, PlatformFormDesignForm{
			ID:             template.Key,
			Icon:           platformTemplateIcon(template),
			Name:           template.Name,
			Category:       platformTemplateCategory(template),
			Desc:           platformTemplateDesc(template),
			Flow:           platformTemplateFlow(template.Schema),
			Enabled:        platformTemplateEnabled(template.Schema),
			AddedThisMonth: sameYearMonth(template.CreatedAt, c.Now()),
			UpdatedAt:      platformTemplateUpdatedAt(template.Schema, template.CreatedAt),
			FormKind:       firstNonEmpty(platformTemplateFormKind(template.Schema), defaultFormKindForTemplateKey(template.Key)),
			Fields:         platformTemplateFields(template.Key, template.Schema),
			Stages:         platformTemplateStages(template.Schema),
		})
	}
	if len(forms) == 0 && !hasTemplates {
		for _, column := range platformFormColumns() {
			for _, item := range column.Items {
				forms = append(forms, PlatformFormDesignForm{
					ID:        item.ID,
					Icon:      item.Emoji,
					Name:      item.Title,
					Category:  column.Title,
					Flow:      "直屬主管 → HR",
					Enabled:   true,
					UpdatedAt: platformTime(c.Now()),
				})
			}
		}
	}
	return PlatformFormDesign{
		Categories: platformFormCategoryNames(),
		Forms:      forms,
		Builder:    platformFormBuilderContract(),
	}, nil
}

// currentWorkspaceFormTemplate 處理目前工作區表單範本的服務流程。
func (c WorkspaceService) currentWorkspaceFormTemplate(ctx RequestContext, id string) (FormTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FormTemplate{}, BadRequest("id is required")
	}
	if template, ok, err := c.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, id); err != nil {
		return FormTemplate{}, err
	} else if ok {
		return template, nil
	}
	if template, ok, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, id); err != nil {
		return FormTemplate{}, err
	} else if ok {
		return template, nil
	}
	return FormTemplate{}, NotFound("form template", id)
}

// Insights 處理工作區洞察的服務流程。
func (c WorkspaceService) Insights(ctx RequestContext, query PlatformInsightsQuery) (PlatformInsightsResponse, error) {
	month := strings.TrimSpace(query.Month)
	if month == "" {
		month = c.Now().Format("2006-01")
	}
	period := workspaceOverviewQueryFromInsightMonth(month)
	overview, err := c.WorkspaceOverview(ctx, period)
	if err != nil {
		return PlatformInsightsResponse{}, err
	}
	attendance, err := c.WorkspaceAttendance(ctx, WorkspaceAttendanceQuery{Year: period.Year, Month: period.Month})
	if err != nil {
		return PlatformInsightsResponse{}, err
	}
	return PlatformInsightsResponse{
		Month:   overview.Month,
		Reports: c.insightReports(overview, attendance),
		AIPanel: PlatformInsightsAIPanel{
			Messages: []PlatformChatMessage{
				{ID: "im1", Role: "assistant", Avatar: "🤖", Content: "已根據目前後端資料產生人力與出勤報表摘要；業務與財務資料源尚未接入。"},
			},
			QuickPrompts: []string{"本月重點", "異常部門", "請假排行", "資料源狀態"},
		},
	}, nil
}

// workspaceOverviewQueryFromInsightMonth 將報表月份轉成共用工作區查詢期間。
func workspaceOverviewQueryFromInsightMonth(month string) WorkspaceOverviewQuery {
	parsed, err := time.Parse("2006-01", strings.TrimSpace(month))
	if err != nil {
		return WorkspaceOverviewQuery{}
	}
	return WorkspaceOverviewQuery{Year: parsed.Year(), Month: int(parsed.Month())}
}

// insightReports 處理 insight reports 的服務流程。
func (c WorkspaceService) insightReports(overview WorkspaceOverviewResponse, attendance WorkspaceAttendanceResponse) map[string]any {
	members, memberHours, leaveChart, totalHours := insightAttendanceMembers(attendance)
	memberCount := len(members)
	leaveDays := attendance.Attendance.Summary.LeaveHours / workspaceDayHours
	hires := overview.HRSummary.Hires
	separations := overview.HRSummary.Separations
	return map[string]any{
		"dept_tasks": map[string]any{
			"title": "部門工時與出勤摘要",
			"metrics": []map[string]any{
				{"id": "dept-total-hours", "label": "本月工時", "value": totalHours, "unit": "h", "variant": "primary"},
				{"id": "leave-days", "label": "本月請假", "value": leaveDays, "unit": "天", "variant": "warning"},
				{"id": "members", "label": "成員數", "value": memberCount, "unit": "人"},
				{"id": "hires", "label": "本月新進", "value": hires, "unit": "人", "variant": "success"},
				{"id": "separations", "label": "本月離職", "value": separations, "unit": "人", "variant": "warning"},
			},
			"leave_chart":           leaveChart,
			"member_hours":          memberHours,
			"product_distribution":  []map[string]any{},
			"category_distribution": []map[string]any{},
			"members":               members,
		},
		"sales": map[string]any{
			"title":         "業務摘要",
			"source_status": "not_connected",
			"caveat":        "業務 CRM / 銷售資料源尚未接入，暫不展示 pipeline、成交金額或客戶列表。",
			"metrics":       []map[string]any{},
			"trend_bars":    []map[string]any{},
			"clients":       []map[string]any{},
		},
		"finance": map[string]any{
			"title":         "財務摘要",
			"source_status": "not_connected",
			"caveat":        "財務系統資料源尚未接入，暫不展示收入、支出、淨利或部門收支。",
			"metrics":       []map[string]any{},
			"monthly_bars":  []map[string]any{},
			"departments":   []map[string]any{},
		},
	}
}

type insightMemberBar struct {
	ID     string
	Label  string
	Avatar string
	Value  float64
}

// insightAttendanceMembers 將既有月度出勤矩陣投影成洞察成員、工時與請假圖表。
func insightAttendanceMembers(attendance WorkspaceAttendanceResponse) ([]map[string]any, []map[string]any, []map[string]any, float64) {
	members := make([]map[string]any, 0, len(attendance.Attendance.Rows))
	hourBars := make([]insightMemberBar, 0, len(attendance.Attendance.Rows))
	leaveBars := make([]insightMemberBar, 0, len(attendance.Attendance.Rows))
	totalHours := 0.0
	for _, row := range attendance.Attendance.Rows {
		name := utils.FirstNonEmpty(row.Employee.NameZH, row.Employee.NameEN, row.Employee.ID)
		leaveDays := row.Summary.LeaveHours / workspaceDayHours
		member := map[string]any{
			"id":              row.Employee.ID,
			"name":            name,
			"avatar":          row.Employee.Avatar,
			"hours":           row.Summary.AttendedHours,
			"leave_days":      leaveDays,
			"primary_product": "—",
			"task_count":      0,
			"products":        0,
			"tasks":           []map[string]any{},
		}
		if leaveType := insightMemberLeaveType(row.Summary.LeaveByType, attendance.LeaveLegend); leaveType != "" {
			member["leave_type"] = leaveType
		}
		members = append(members, member)
		hourBars = append(hourBars, insightMemberBar{ID: row.Employee.ID, Label: name, Avatar: row.Employee.Avatar, Value: row.Summary.AttendedHours})
		if leaveDays > 0 {
			leaveBars = append(leaveBars, insightMemberBar{ID: row.Employee.ID, Label: name, Avatar: row.Employee.Avatar, Value: leaveDays})
		}
		totalHours += row.Summary.AttendedHours
	}
	sort.SliceStable(members, func(i, j int) bool { return members[i]["id"].(string) < members[j]["id"].(string) })
	return members, insightTopMemberBars(hourBars, "primary", "h"), insightTopMemberBars(leaveBars, "warning", " 天"), totalHours
}

// insightTopMemberBars 取前八名成員並補齊一致的圖表比例基準。
func insightTopMemberBars(items []insightMemberBar, tone string, unit string) []map[string]any {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Value != items[j].Value {
			return items[i].Value > items[j].Value
		}
		return items[i].ID < items[j].ID
	})
	if len(items) > 8 {
		items = items[:8]
	}
	maxValue := 1.0
	if len(items) > 0 && items[0].Value > maxValue {
		maxValue = items[0].Value
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id": item.ID, "label": item.Label, "avatar": item.Avatar,
			"value": item.Value, "max": maxValue, "meta": workspaceFloat(item.Value) + unit, "tone": tone,
		})
	}
	return out
}

// insightMemberLeaveType 將成員假別時數轉成穩定、可讀的假別摘要。
func insightMemberLeaveType(leaveByType map[string]float64, legend []WorkspaceLeaveLegendItem) string {
	labels := make(map[string]string, len(legend))
	for _, item := range legend {
		labels[item.Code] = item.Label
	}
	codes := make([]string, 0, len(leaveByType))
	for code, hours := range leaveByType {
		if hours > 0 {
			codes = append(codes, code)
		}
	}
	sort.SliceStable(codes, func(i, j int) bool {
		if leaveByType[codes[i]] != leaveByType[codes[j]] {
			return leaveByType[codes[i]] > leaveByType[codes[j]]
		}
		return codes[i] < codes[j]
	})
	for i, code := range codes {
		codes[i] = firstNonEmpty(labels[code], code)
	}
	return strings.Join(codes, "、")
}

// UpdateWorkspaceOrganizationManager 更新工作區 organization 主管的服務流程。
func (c WorkspaceService) UpdateWorkspaceOrganizationManager(ctx RequestContext, displayID string, input UpdateWorkspaceOrganizationManagerInput) (WorkspaceOrganizationResponse, error) {
	if input.ParentID == nil {
		return WorkspaceOrganizationResponse{}, BadRequest("parent_id is required")
	}
	visibleEmployees, err := c.visibleWorkspaceEmployees(ctx, "workspace.organization")
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	visibleEmployees = workspaceOrganizationEmployees(visibleEmployees)
	employee, ok := workspaceEmployeeByDisplayID(visibleEmployees, displayID)
	if !ok {
		return WorkspaceOrganizationResponse{}, NotFound("employee", strings.TrimSpace(displayID))
	}
	parentDisplayID := strings.TrimSpace(*input.ParentID)
	managerEmployeeID := ""
	if parentDisplayID != "" && parentDisplayID != workspaceParentNone {
		manager, ok := workspaceEmployeeByDisplayID(visibleEmployees, parentDisplayID)
		if !ok {
			return WorkspaceOrganizationResponse{}, NotFound("employee", parentDisplayID)
		}
		managerEmployeeID = manager.ID
	}
	if workspaceManagerCycle(visibleEmployees, employee.ID, managerEmployeeID) {
		return WorkspaceOrganizationResponse{}, domainValidation("manager relationship would create a cycle", FieldError{Tab: "employment_info", Field: "manager_employee_id", Code: "invalid", Message: "manager relationship would create a cycle"})
	}
	account, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: employee.ID, Action: ActionUpdate},
		AuditTarget{Event: "platform.workspace.organization.manager.update", Resource: string(ResourceEmployee), Target: employee.ID},
	)
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	if err := c.Service.HR().withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, employee.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", employee.ID)
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		before := next
		next.ManagerEmployeeID = managerEmployeeID
		next.UpdatedAt = tx.Now()
		next = tx.appendHistoryForChangedEmployment(before, next, "主管調整")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, before, next, string(EventEmployeeAuthzSubjectUpdate)); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeUpdated), next.ID, map[string]any{"employee_id": next.ID, "manager_employee_id": next.ManagerEmployeeID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "platform.workspace.organization.manager.update", string(ResourceEmployee), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"before_manager_employee_id": before.ManagerEmployeeID,
			"after_manager_employee_id":  next.ManagerEmployeeID,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	return c.WorkspaceOrganization(ctx)
}

// UpdateWorkspaceOrganizationVisibility 更新員工在組織圖預覽中的可見性。
func (c WorkspaceService) UpdateWorkspaceOrganizationVisibility(ctx RequestContext, displayID string, input UpdateWorkspaceOrganizationVisibilityInput) (WorkspaceOrganizationResponse, error) {
	if input.ShowInOrgChart == nil {
		return WorkspaceOrganizationResponse{}, BadRequest("show_in_org_chart is required")
	}
	visibleEmployees, err := c.visibleWorkspaceEmployees(ctx, "workspace.organization")
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	visibleEmployees = workspaceOrganizationEmployees(visibleEmployees)
	employee, ok := workspaceEmployeeByDisplayID(visibleEmployees, displayID)
	if !ok {
		return WorkspaceOrganizationResponse{}, NotFound("employee", strings.TrimSpace(displayID))
	}
	account, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: employee.ID, Action: ActionUpdate},
		AuditTarget{Event: "platform.workspace.organization.visibility.update", Resource: string(ResourceEmployee), Target: employee.ID},
	)
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	showInOrgChart := *input.ShowInOrgChart
	if err := c.Service.HR().withTransaction(ctx, func(tx HRService) error {
		current, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, employee.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", employee.ID)
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{current}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		if err := tx.store.UpdateEmployeeOrgChartVisibility(goContext(ctx), ctx.TenantID, current.ID, showInOrgChart, tx.Now()); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeUpdated), current.ID, map[string]any{
			"employee_id":       current.ID,
			"show_in_org_chart": showInOrgChart,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "platform.workspace.organization.visibility.update", string(ResourceEmployee), current.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"before_show_in_org_chart": current.ShowInOrgChart,
			"after_show_in_org_chart":  showInOrgChart,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	return c.WorkspaceOrganization(ctx)
}

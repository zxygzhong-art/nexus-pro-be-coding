package service

import (
	"nexus-pro-be/internal/domain"
	"sort"
	"strings"
)

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
		if appErr, ok := domain.AsAppError(err); ok && appErr.ReasonCode == "approval_required" {
			return []WorkspaceAuditLog{}, nil
		}
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
		templateID := "ft-" + key
		createdAt := now
		if exists {
			templateID = current.ID
			createdAt = current.CreatedAt
			if createdAt.IsZero() {
				createdAt = now
			}
		}
		template := FormTemplate{
			ID:          templateID,
			TenantID:    ctx.TenantID,
			Key:         key,
			Name:        strings.TrimSpace(input.Name),
			Description: strings.TrimSpace(input.Desc),
			Schema:      workspaceFormDesignSchema(nil, input, enabled, false, now),
			CreatedAt:   createdAt,
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
		if input.Fields != nil {
			next.Fields = *input.Fields
		}
		if input.Stages != nil {
			next.Stages = *input.Stages
		}
		if strings.TrimSpace(next.Name) == "" {
			return BadRequest("name is required")
		}
		enabled := true
		if next.Enabled != nil {
			enabled = *next.Enabled
		}
		template.Name = strings.TrimSpace(next.Name)
		template.Description = strings.TrimSpace(next.Desc)
		template.Schema = workspaceFormDesignSchema(template.Schema, next, enabled, false, workspace.Now())
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
		template.Schema = workspaceFormDesignSchema(template.Schema, next, false, true, workspace.Now())
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
			Fields:         platformTemplateFields(template.Schema),
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
	overview, err := c.WorkspaceOverview(ctx, WorkspaceOverviewQuery{})
	if err != nil {
		return PlatformInsightsResponse{}, err
	}
	return PlatformInsightsResponse{
		Month:   month,
		Reports: c.insightReports(month, overview),
		AIPanel: PlatformInsightsAIPanel{
			Messages: []PlatformChatMessage{
				{ID: "im1", Role: "assistant", Avatar: "🤖", Content: "已根據目前後端資料產生人力、業務與財務報表摘要。"},
			},
			QuickPrompts: []string{"本月重點", "異常部門", "請假排行", "生成摘要"},
		},
	}, nil
}

// insightReports 處理 insight reports 的服務流程。
func (c WorkspaceService) insightReports(month string, overview WorkspaceOverviewResponse) map[string]any {
	active := overview.HRSummary.Active
	hires := overview.HRSummary.Hires
	separations := overview.HRSummary.Separations
	return map[string]any{
		"dept_tasks": map[string]any{
			"title": "部門任務與出勤摘要",
			"metrics": []map[string]any{
				{"id": "dept-total-hours", "label": "估算工時", "value": active * 8, "unit": "h", "variant": "primary"},
				{"id": "leave-days", "label": "今日請假", "value": overview.Attendance.Leave, "unit": "人", "variant": "warning"},
				{"id": "checked-in", "label": "已上班", "value": overview.Attendance.CheckedIn, "unit": "人", "variant": "success"},
			},
			"leave_chart": []map[string]any{
				{"id": "leave", "label": "請假", "value": overview.Attendance.Leave, "max": maxInt(active, 1), "tone": "warning", "active": true},
				{"id": "absent", "label": "未到", "value": overview.Attendance.Absent, "max": maxInt(active, 1), "tone": "danger"},
			},
			"member_hours": []map[string]any{
				{"id": "team", "label": "全公司", "value": active * 8, "max": maxInt(active*8, 1), "tone": "primary"},
			},
			"product_distribution": []map[string]any{{
				"id":       "nexus",
				"label":    "Nexus",
				"total":    active * 8,
				"segments": []map[string]any{{"id": "oa", "label": "OA", "value": active * 8, "tone": "primary"}},
			}},
			"category_distribution": []map[string]any{{
				"id":       "work",
				"label":    "工作類型",
				"total":    active * 8,
				"segments": []map[string]any{{"id": "operation", "label": "營運", "value": active * 8, "tone": "info"}},
			}},
			"members": []map[string]any{},
		},
		"sales": map[string]any{
			"title": "業務摘要",
			"metrics": []map[string]any{
				{"id": "pipeline", "label": "Pipeline", "value": "NT$ 0", "variant": "primary"},
				{"id": "won", "label": "已成交", "value": "NT$ 0", "variant": "success"},
			},
			"trend_bars": []map[string]any{{"id": month, "label": month, "value": 0, "max": 1, "tone": "primary", "active": true}},
			"clients":    []map[string]any{},
		},
		"finance": map[string]any{
			"title": "財務摘要",
			"metrics": []map[string]any{
				{"id": "hires", "label": "本月新進", "value": hires, "unit": "人", "variant": "success"},
				{"id": "separations", "label": "本月離職", "value": separations, "unit": "人", "variant": "warning"},
			},
			"monthly_bars": []map[string]any{{"id": month, "label": month, "income": hires, "expense": separations, "active": true}},
			"departments":  []map[string]any{},
		},
	}
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

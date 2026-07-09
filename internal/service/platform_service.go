package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

const (
	platformDateLayout          = "2006/01/02"
	platformFormDesignSchemaKey = "workspace_design"
)

// PlatformService 定義平台服務的資料結構。
type PlatformService struct {
	*Service
}

// Platform 處理平台的服務流程。
func (c *Service) Platform() PlatformService {
	return PlatformService{Service: c}
}

// Home 處理首頁的服務流程。
func (c PlatformService) Home(ctx RequestContext) (PlatformHomeResponse, error) {
	clockSummary, err := c.clockSummary(ctx)
	if err != nil {
		return PlatformHomeResponse{}, err
	}
	assistants, err := c.publishedPlatformAssistants(ctx, 6, PlatformAssistantsQuery{})
	if err != nil {
		return PlatformHomeResponse{}, err
	}
	if len(assistants) == 0 {
		assistants = firstPlatformAssistants(6)
	}
	return PlatformHomeResponse{
		Assistants:   assistants,
		FormColumns:  platformHomeFormColumns(),
		ClockSummary: clockSummary,
	}, nil
}

// ListAssistants 列出助理的服務流程。
func (c PlatformService) ListAssistants(ctx RequestContext, query PlatformAssistantsQuery) (PlatformAssistantsResponse, error) {
	items, err := c.publishedPlatformAssistants(ctx, 0, query)
	if err != nil {
		return PlatformAssistantsResponse{}, err
	}
	if len(items) > 0 {
		return PlatformAssistantsResponse{
			Data:         items,
			Total:        len(items),
			ChatMessages: platformAssistantMessages(),
			QuickPrompts: []string{"推薦客訴處理助理", "月度業績分析", "幫我擬週報", "新人 onboarding", "出差交通建議"},
		}, nil
	}
	tag := strings.ToLower(strings.TrimSpace(query.Tag))
	search := strings.ToLower(strings.TrimSpace(query.Search))
	items = make([]PlatformAssistant, 0)
	for _, assistant := range platformAssistantCatalog() {
		if tag != "" && tag != "all" && assistant.Tag != tag {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(assistant.Title+" "+assistant.Desc+" "+assistant.Tag), search) {
			continue
		}
		items = append(items, assistant)
	}
	return PlatformAssistantsResponse{
		Data:         items,
		Total:        len(items),
		ChatMessages: platformAssistantMessages(),
		QuickPrompts: []string{"推薦客訴處理助理", "月度業績分析", "幫我擬週報", "新人 onboarding", "出差交通建議"},
	}, nil
}

func (c PlatformService) publishedPlatformAssistants(ctx RequestContext, limit int, query PlatformAssistantsQuery) ([]PlatformAssistant, error) {
	agents, err := c.store.ListPublishedAgentDefinitions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	tag := strings.ToLower(strings.TrimSpace(query.Tag))
	search := strings.ToLower(strings.TrimSpace(query.Search))
	items := make([]PlatformAssistant, 0, len(agents))
	for _, agent := range agents {
		assistant := PlatformAssistant{
			ID:    agent.ID,
			Emoji: agent.Emoji,
			Title: agent.Name,
			Desc:  agent.Description,
			Tag:   string(agent.Category),
		}
		if tag != "" && tag != "all" && assistant.Tag != tag {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(assistant.Title+" "+assistant.Desc+" "+assistant.Tag), search) {
			continue
		}
		items = append(items, assistant)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

// Forms 處理表單的服務流程。
func (c PlatformService) Forms(ctx RequestContext) (PlatformFormsResponse, error) {
	applications, drafts, err := c.formInstances(ctx)
	if err != nil {
		return PlatformFormsResponse{}, err
	}
	categories, err := c.platformFormCategories(ctx)
	if err != nil {
		return PlatformFormsResponse{}, err
	}
	return PlatformFormsResponse{
		Categories:   categories,
		Applications: applications,
		Drafts:       drafts,
		AIMessages: []PlatformChatMessage{
			{ID: "fm1", Role: "assistant", Avatar: "📋", Content: "哈囉！我是 AI 表單助理。告訴我你想處理什麼事情，我能推薦最適合的表單並協助你填寫。"},
		},
		QuickPrompts: []string{"我要請假", "我要報銷", "我要用印", "不確定用哪張", "我要領用", "設備報修"},
	}, nil
}

// Tasks 處理任務的服務流程。
func (c PlatformService) Tasks(ctx RequestContext) (PlatformTasksResponse, error) {
	clockSummary, err := c.clockSummary(ctx)
	if err != nil {
		return PlatformTasksResponse{}, err
	}
	records, todos, err := c.taskProjection(ctx)
	if err != nil {
		return PlatformTasksResponse{}, err
	}
	return PlatformTasksResponse{
		Records:      records,
		Todos:        todos,
		ClockSummary: clockSummary,
		AIMessages: []PlatformChatMessage{
			{ID: "tm1", Role: "assistant", Avatar: "🤖", Content: "今日工時與任務已整理完成，可以繼續追問本週投入分佈或待辦重點。"},
		},
		QuickPrompts: []string{"本月工時統計", "本週任務重點", "今日剩餘工時", "類別分佈"},
	}, nil
}

// CreateTaskItem 建立任務項目的服務流程。
func (c PlatformService) CreateTaskItem(ctx RequestContext, input CreatePlatformTaskItemInput) (PlatformTaskItem, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	record, err := c.platformTaskItemRecord(ctx, account.ID, PlatformTaskRecordItem{
		ID:        utils.NewID("pti"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		WorkDate:  input.WorkDate,
		Title:     input.Title,
		Category:  input.Category,
		Product:   input.Product,
		Hours:     input.Hours,
		Note:      input.Note,
	})
	if err != nil {
		return PlatformTaskItem{}, err
	}
	if err := c.store.UpsertPlatformTaskItem(goContext(ctx), record); err != nil {
		return PlatformTaskItem{}, err
	}
	return platformTaskItemFromRecord(record), nil
}

// UpdateTaskItem 更新任務項目的服務流程。
func (c PlatformService) UpdateTaskItem(ctx RequestContext, id string, input UpdatePlatformTaskItemInput) (PlatformTaskItem, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	record, err := c.currentPlatformTaskItem(ctx, account.ID, id)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	if input.WorkDate != nil {
		record.WorkDate = *input.WorkDate
	}
	if input.Title != nil {
		record.Title = *input.Title
	}
	if input.Category != nil {
		record.Category = *input.Category
	}
	if input.Product != nil {
		record.Product = *input.Product
	}
	if input.Hours != nil {
		record.Hours = *input.Hours
	}
	if input.Note != nil {
		record.Note = *input.Note
	}
	record, err = c.platformTaskItemRecord(ctx, account.ID, record)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	if err := c.store.UpsertPlatformTaskItem(goContext(ctx), record); err != nil {
		return PlatformTaskItem{}, err
	}
	return platformTaskItemFromRecord(record), nil
}

// DeleteTaskItem 刪除任務項目的服務流程。
func (c PlatformService) DeleteTaskItem(ctx RequestContext, id string) (PlatformTaskItem, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	record, err := c.currentPlatformTaskItem(ctx, account.ID, id)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	if err := c.store.DeletePlatformTaskItem(goContext(ctx), ctx.TenantID, account.ID, strings.TrimSpace(id)); err != nil {
		return PlatformTaskItem{}, err
	}
	return platformTaskItemFromRecord(record), nil
}

// CreateTaskTodo 建立任務待辦的服務流程。
func (c PlatformService) CreateTaskTodo(ctx RequestContext, input CreatePlatformTaskTodoInput) (PlatformTaskTodo, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	record, err := c.platformTaskTodoRecord(ctx, account.ID, PlatformTaskTodoRecord{
		ID:        utils.NewID("ptd"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		Text:      input.Text,
		DueDate:   input.DueDate,
		Status:    "open",
	})
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	if err := c.store.UpsertPlatformTaskTodo(goContext(ctx), record); err != nil {
		return PlatformTaskTodo{}, err
	}
	return platformTaskTodoFromRecord(record), nil
}

// UpdateTaskTodo 更新任務待辦的服務流程。
func (c PlatformService) UpdateTaskTodo(ctx RequestContext, id string, input UpdatePlatformTaskTodoInput) (PlatformTaskTodo, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	record, err := c.currentPlatformTaskTodo(ctx, account.ID, id)
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	if input.Text != nil {
		record.Text = *input.Text
	}
	if input.DueDate != nil {
		record.DueDate = *input.DueDate
	}
	if input.Done != nil {
		if *input.Done {
			record.Status = "done"
		} else {
			record.Status = "open"
		}
	}
	record, err = c.platformTaskTodoRecord(ctx, account.ID, record)
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	if err := c.store.UpsertPlatformTaskTodo(goContext(ctx), record); err != nil {
		return PlatformTaskTodo{}, err
	}
	return platformTaskTodoFromRecord(record), nil
}

// DeleteTaskTodo 刪除任務待辦的服務流程。
func (c PlatformService) DeleteTaskTodo(ctx RequestContext, id string) (PlatformTaskTodo, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	record, err := c.currentPlatformTaskTodo(ctx, account.ID, id)
	if err != nil {
		return PlatformTaskTodo{}, err
	}
	if err := c.store.DeletePlatformTaskTodo(goContext(ctx), ctx.TenantID, account.ID, strings.TrimSpace(id)); err != nil {
		return PlatformTaskTodo{}, err
	}
	return platformTaskTodoFromRecord(record), nil
}

// ConvertTaskTodo 處理 convert 任務待辦的服務流程。
func (c PlatformService) ConvertTaskTodo(ctx RequestContext, id string, input ConvertPlatformTaskTodoInput) (PlatformTaskItem, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PlatformTaskItem{}, err
	}
	var out PlatformTaskItem
	err = c.withTenantTransaction(ctx, func(tx *Service) error {
		platform := tx.Platform()
		todo, err := platform.currentPlatformTaskTodo(ctx, account.ID, id)
		if err != nil {
			return err
		}
		title := strings.TrimSpace(input.Title)
		if title == "" {
			title = todo.Text
		}
		hours := input.Hours
		if hours == 0 {
			hours = 0.5
		}
		item, err := platform.platformTaskItemRecord(ctx, account.ID, PlatformTaskRecordItem{
			ID:        utils.NewID("pti"),
			TenantID:  ctx.TenantID,
			AccountID: account.ID,
			WorkDate:  input.WorkDate,
			Title:     title,
			Category:  firstNonEmpty(input.Category, "待辦"),
			Product:   firstNonEmpty(input.Product, "Nexus"),
			Hours:     hours,
			Note:      input.Note,
		})
		if err != nil {
			return err
		}
		if err := platform.store.UpsertPlatformTaskItem(goContext(ctx), item); err != nil {
			return err
		}
		todo.Status = "done"
		todo.ConvertedTaskItemID = item.ID
		todo.UpdatedAt = platform.Now()
		if err := platform.store.UpsertPlatformTaskTodo(goContext(ctx), todo); err != nil {
			return err
		}
		out = platformTaskItemFromRecord(item)
		return nil
	})
	if err != nil {
		return PlatformTaskItem{}, err
	}
	return out, nil
}

// platformWorkspaceEmployeeMatches 處理平台工作區員工 matches。
func platformWorkspaceEmployeeMatches(query PlatformWorkspaceEmployeesQuery, employee Employee, card WorkspaceEmployeeCard) bool {
	departmentID := strings.TrimSpace(query.DepartmentID)
	if departmentID != "" && employee.OrgUnitID != departmentID {
		return false
	}
	department := strings.ToLower(strings.TrimSpace(query.Department))
	if department != "" && !strings.Contains(strings.ToLower(card.Dept+" "+employee.OrgUnitID), department) {
		return false
	}
	if status := strings.TrimSpace(query.Status); status != "" && card.Status != workspaceStatusLabel(NormalizeEmployeeStatus(status)) {
		return false
	}
	if employmentStatus := strings.TrimSpace(query.EmploymentStatus); employmentStatus != "" {
		if workspaceEmployeeStatus(employee) != strings.ToLower(NormalizeEmployeeStatus(employmentStatus)) {
			return false
		}
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	if keyword == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		card.ID,
		card.NameZH,
		card.NameEN,
		card.Email,
		card.Dept,
		card.Title,
		card.Type,
		card.Phone,
	}, " "))
	return strings.Contains(haystack, keyword)
}

// clockSummary 處理打卡摘要的服務流程。
func (c PlatformService) clockSummary(ctx RequestContext) (PlatformClockSummary, error) {
	status, err := c.Service.Attendance().AttendanceClockStatus(ctx)
	if err != nil {
		return PlatformClockSummary{}, err
	}
	now := c.Now()
	checkedInAt := clockTime(status.ClockIn)
	checkedOutAt := clockTime(status.ClockOut)
	location := "未設定"
	if status.Worksite != nil && strings.TrimSpace(status.Worksite.Name) != "" {
		location = status.Worksite.Name
	}
	monthlyDays, monthlyHours, leaveDays, overtimeHours := c.monthlyClockAndLeaveSummary(ctx, status.EmployeeID, now)
	return PlatformClockSummary{
		DateLabel:             platformDateLabel(now),
		CheckedInAt:           checkedInAt,
		CheckedOutAt:          checkedOutAt,
		Location:              location,
		MonthlyAttendanceDays: monthlyDays,
		MonthlyHours:          monthlyHours,
		MonthlyOvertimeHours:  overtimeHours,
		LeaveDays:             leaveDays,
	}, nil
}

// monthlyClockAndLeaveSummary 處理每月打卡 and 請假摘要的服務流程。請假與加班只計已核准的申請。
func (c PlatformService) monthlyClockAndLeaveSummary(ctx RequestContext, employeeID string, now time.Time) (int, float64, float64, float64) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, 0)
	records, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, AttendanceClockRecordQuery{
		EmployeeID: employeeID,
		FromDate:   start.Format(time.DateOnly),
		ToDate:     end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return 0, 0, 0, 0
	}
	days := map[string]struct{}{}
	for _, record := range records {
		if record.EmployeeID != employeeID || record.Direction != clockDirectionIn || record.RecordStatus != clockRecordStatusAccepted {
			continue
		}
		if !record.ClockedAt.Before(start) && record.ClockedAt.Before(end) {
			days[record.WorkDate] = struct{}{}
		}
	}
	leaveHours := 0.0
	leaves, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: []string{employeeID},
		Status:      "approved",
		FromDate:    start.Format(time.DateOnly),
		ToDate:      end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err == nil {
		for _, leave := range leaves {
			if leave.EmployeeID != employeeID || leave.EndAt.Before(start) || !leave.StartAt.Before(end) {
				continue
			}
			leaveHours += leave.Hours
		}
	}
	overtimeHours := 0.0
	overtimes, err := c.store.ListOvertimeRequestsByQuery(goContext(ctx), ctx.TenantID, OvertimeRequestQuery{
		EmployeeIDs: []string{employeeID},
		Status:      "approved",
		FromDate:    start.Format(time.DateOnly),
		ToDate:      end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err == nil {
		for _, overtime := range overtimes {
			if overtime.EmployeeID != employeeID || overtime.EndAt.Before(start) || !overtime.StartAt.Before(end) {
				continue
			}
			overtimeHours += overtime.Hours
		}
	}
	return len(days), float64(len(days))*workspaceDayHours + overtimeHours, leaveHours / workspaceDayHours, overtimeHours
}

// formInstances 處理表單實例的服務流程。
func (c PlatformService) formInstances(ctx RequestContext) ([]PlatformFormApplication, []PlatformFormDraft, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, nil, err
	}
	instances, err := c.store.ListFormInstancesByQuery(goContext(ctx), ctx.TenantID, FormInstanceQuery{ApplicantAccountID: account.ID})
	if err != nil {
		return nil, nil, err
	}
	templates, err := c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, nil, err
	}
	templateByID := map[string]FormTemplate{}
	for _, template := range templates {
		templateByID[template.ID] = template
	}
	applications := make([]PlatformFormApplication, 0)
	drafts := make([]PlatformFormDraft, 0)
	for _, instance := range instances {
		if instance.ApplicantAccountID != account.ID {
			continue
		}
		template := templateByID[instance.TemplateID]
		title := template.Name
		if title == "" {
			title = template.Key
		}
		if strings.EqualFold(instance.Status, workflowFormStatusDraft) {
			drafts = append(drafts, PlatformFormDraft{
				ID:          instance.ID,
				TemplateKey: template.Key,
				Title:       title,
				UpdatedAt:   platformTime(instance.UpdatedAt),
				Summary:     platformFormSummary(instance.Payload),
				Payload:     utils.CopyStringMap(instance.Payload),
			})
			continue
		}
		applications = append(applications, PlatformFormApplication{
			ID:          instance.ID,
			TemplateKey: template.Key,
			Title:       title,
			Applicant:   account.DisplayName,
			SubmittedAt: platformTime(instance.SubmittedAt),
			Status:      platformFormStatus(instance.Status),
			Summary:     platformFormSummary(instance.Payload),
			Payload:     utils.CopyStringMap(instance.Payload),
		})
	}
	sort.SliceStable(applications, func(i, j int) bool {
		return applications[i].SubmittedAt > applications[j].SubmittedAt
	})
	sort.SliceStable(drafts, func(i, j int) bool {
		return drafts[i].UpdatedAt > drafts[j].UpdatedAt
	})
	return applications, drafts, nil
}

// taskProjection 處理任務 projection 的服務流程。
func (c PlatformService) taskProjection(ctx RequestContext) ([]PlatformTaskRecord, []PlatformTaskTodo, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, nil, err
	}
	taskItems, err := c.store.ListPlatformTaskItems(goContext(ctx), ctx.TenantID, account.ID)
	if err != nil {
		return nil, nil, err
	}
	taskTodos, err := c.store.ListPlatformTaskTodos(goContext(ctx), ctx.TenantID, account.ID)
	if err != nil {
		return nil, nil, err
	}
	runs, err := c.store.ListAgentRunsByAccount(goContext(ctx), ctx.TenantID, account.ID)
	if err != nil {
		return nil, nil, err
	}
	recordsByDate := map[string]*PlatformTaskRecord{}
	todos := make([]PlatformTaskTodo, 0)
	for _, item := range taskItems {
		record := recordsByDate[item.WorkDate]
		if record == nil {
			record = &PlatformTaskRecord{Date: item.WorkDate, Weekday: platformWeekdayFromDate(item.WorkDate), Items: []PlatformTaskItem{}}
			recordsByDate[item.WorkDate] = record
		}
		record.Items = append(record.Items, platformTaskItemFromRecord(item))
		record.TotalHours += item.Hours
	}
	for _, todo := range taskTodos {
		todos = append(todos, platformTaskTodoFromRecord(todo))
	}
	for _, run := range runs {
		if run.AccountID != account.ID {
			continue
		}
		date := run.CreatedAt.Format(platformDateLayout)
		record := recordsByDate[date]
		if record == nil {
			record = &PlatformTaskRecord{Date: date, Weekday: platformWeekday(run.CreatedAt), Items: []PlatformTaskItem{}}
			recordsByDate[date] = record
		}
		hours := 1.0
		item := PlatformTaskItem{ID: run.ID, Title: run.Prompt, Category: "AI", Product: "Nexus", Hours: hours, Note: run.Status}
		record.Items = append(record.Items, item)
		record.TotalHours += hours
		todos = append(todos, PlatformTaskTodo{ID: "todo-" + run.ID, Text: run.Prompt, Done: run.Status == string(AgentRunStatusCompleted), Date: run.CreatedAt.Format("01/02")})
	}
	records := make([]PlatformTaskRecord, 0, len(recordsByDate))
	for _, record := range recordsByDate {
		records = append(records, *record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Date > records[j].Date
	})
	if len(records) == 0 {
		records = []PlatformTaskRecord{{
			Date:       c.Now().Format(platformDateLayout),
			Weekday:    platformWeekday(c.Now()),
			TotalHours: 0,
			Items:      []PlatformTaskItem{},
		}}
	}
	return records, todos, nil
}

// currentPlatformTaskItem 處理目前平台任務項目的服務流程。
func (c PlatformService) currentPlatformTaskItem(ctx RequestContext, accountID string, id string) (PlatformTaskRecordItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PlatformTaskRecordItem{}, BadRequest("id is required")
	}
	item, ok, err := c.store.GetPlatformTaskItem(goContext(ctx), ctx.TenantID, accountID, id)
	if err != nil {
		return PlatformTaskRecordItem{}, err
	}
	if !ok || item.AccountID != accountID {
		return PlatformTaskRecordItem{}, NotFound("platform task item", id)
	}
	return item, nil
}

// currentPlatformTaskTodo 處理目前平台任務待辦的服務流程。
func (c PlatformService) currentPlatformTaskTodo(ctx RequestContext, accountID string, id string) (PlatformTaskTodoRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PlatformTaskTodoRecord{}, BadRequest("id is required")
	}
	todo, ok, err := c.store.GetPlatformTaskTodo(goContext(ctx), ctx.TenantID, accountID, id)
	if err != nil {
		return PlatformTaskTodoRecord{}, err
	}
	if !ok || todo.AccountID != accountID {
		return PlatformTaskTodoRecord{}, NotFound("platform task todo", id)
	}
	return todo, nil
}

// platformTaskItemRecord 處理平台任務項目 record 的服務流程。
func (c PlatformService) platformTaskItemRecord(ctx RequestContext, accountID string, item PlatformTaskRecordItem) (PlatformTaskRecordItem, error) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return PlatformTaskRecordItem{}, BadRequest("title is required")
	}
	if err := validatePlatformTaskHours(item.Hours); err != nil {
		return PlatformTaskRecordItem{}, err
	}
	workDate, err := normalizePlatformWorkDate(item.WorkDate, c.Now())
	if err != nil {
		return PlatformTaskRecordItem{}, err
	}
	now := c.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.ID = strings.TrimSpace(item.ID)
	item.TenantID = ctx.TenantID
	item.AccountID = accountID
	item.WorkDate = workDate
	item.Title = title
	item.Category = firstNonEmpty(item.Category, "一般")
	item.Product = firstNonEmpty(item.Product, "Nexus")
	item.Note = strings.TrimSpace(item.Note)
	item.UpdatedAt = now
	return item, nil
}

// platformTaskTodoRecord 處理平台任務待辦 record 的服務流程。
func (c PlatformService) platformTaskTodoRecord(ctx RequestContext, accountID string, todo PlatformTaskTodoRecord) (PlatformTaskTodoRecord, error) {
	text := strings.TrimSpace(todo.Text)
	if text == "" {
		return PlatformTaskTodoRecord{}, BadRequest("text is required")
	}
	dueDate, err := normalizeOptionalPlatformDate(todo.DueDate)
	if err != nil {
		return PlatformTaskTodoRecord{}, err
	}
	status := strings.TrimSpace(todo.Status)
	if status == "" {
		status = "open"
	}
	if status != "open" && status != "done" {
		return PlatformTaskTodoRecord{}, BadRequest("status must be open or done")
	}
	now := c.Now()
	if todo.CreatedAt.IsZero() {
		todo.CreatedAt = now
	}
	todo.ID = strings.TrimSpace(todo.ID)
	todo.TenantID = ctx.TenantID
	todo.AccountID = accountID
	todo.Text = text
	todo.DueDate = dueDate
	todo.Status = status
	todo.UpdatedAt = now
	return todo, nil
}

// platformTaskItemFromRecord 處理平台任務項目 來源 record。
func platformTaskItemFromRecord(item PlatformTaskRecordItem) PlatformTaskItem {
	return PlatformTaskItem{
		ID:       item.ID,
		Title:    item.Title,
		Category: item.Category,
		Product:  item.Product,
		Hours:    item.Hours,
		Note:     item.Note,
	}
}

// platformTaskTodoFromRecord 處理平台任務待辦 來源 record。
func platformTaskTodoFromRecord(todo PlatformTaskTodoRecord) PlatformTaskTodo {
	return PlatformTaskTodo{
		ID:   todo.ID,
		Text: todo.Text,
		Done: todo.Status == "done",
		Date: platformTaskTodoDate(todo),
	}
}

// platformTaskTodoDate 處理平台任務待辦日期。
func platformTaskTodoDate(todo PlatformTaskTodoRecord) string {
	if todo.DueDate != "" {
		if parsed, err := time.Parse(platformDateLayout, todo.DueDate); err == nil {
			return parsed.Format("01/02")
		}
		return todo.DueDate
	}
	if !todo.CreatedAt.IsZero() {
		return todo.CreatedAt.Format("01/02")
	}
	return ""
}

// platformWeekdayFromDate 處理平台星期 來源 日期。
func platformWeekdayFromDate(date string) string {
	parsed, err := time.Parse(platformDateLayout, date)
	if err != nil {
		return ""
	}
	return platformWeekday(parsed)
}

// normalizePlatformWorkDate 正規化平台 work 日期。
func normalizePlatformWorkDate(value string, fallback time.Time) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback.Format(platformDateLayout), nil
	}
	for _, layout := range []string{platformDateLayout, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format(platformDateLayout), nil
		}
	}
	return "", BadRequest("work_date must be YYYY/MM/DD or YYYY-MM-DD")
}

// normalizeOptionalPlatformDate 正規化可選平台日期。
func normalizeOptionalPlatformDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return normalizePlatformWorkDate(value, time.Time{})
}

// validatePlatformTaskHours 驗證平台任務小時。
func validatePlatformTaskHours(hours float64) error {
	if hours <= 0 || hours > 24 {
		return BadRequest("hours must be greater than zero and no more than 24")
	}
	if math.Abs(hours*2-math.Round(hours*2)) > 0.000001 {
		return BadRequest("hours must use 0.5 hour increments")
	}
	return nil
}

// firstNonEmpty 取得第一個non 空值。
func firstNonEmpty(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

// workspaceFormDesignKey 處理工作區表單 design key。
func workspaceFormDesignKey(id string, name string, now time.Time) string {
	id = workspaceFormDesignSlug(id)
	if id != "" {
		return id
	}
	id = workspaceFormDesignSlug(name)
	if id != "" {
		return "custom-" + id
	}
	return fmt.Sprintf("custom-%d", now.UnixNano())
}

// workspaceFormDesignSlug 處理工作區表單 design slug。
func workspaceFormDesignSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// workspaceFormDesignInputFromTemplate 處理工作區表單 design 輸入 來源 範本。
func workspaceFormDesignInputFromTemplate(template FormTemplate) SaveWorkspaceFormDesignInput {
	enabled := platformTemplateEnabled(template.Schema)
	return SaveWorkspaceFormDesignInput{
		ID:       template.Key,
		Icon:     platformTemplateIcon(template),
		Name:     template.Name,
		Category: platformTemplateCategory(template),
		Desc:     platformTemplateDesc(template),
		Enabled:  &enabled,
		FormKind: firstNonEmpty(platformTemplateFormKind(template.Schema), defaultFormKindForTemplateKey(template.Key)),
		Fields:   platformTemplateFields(template.Schema),
		Stages:   platformTemplateStages(template.Schema),
	}
}

// workspaceFormDesignSchema 處理工作區表單 design schema。
func workspaceFormDesignSchema(base map[string]any, input SaveWorkspaceFormDesignInput, enabled bool, deleted bool, updatedAt time.Time) map[string]any {
	schema := utils.CopyStringMap(base)
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	fields := input.Fields
	stages := input.Stages
	// Soft-delete / incomplete legacy rows may still rely on contract defaults.
	// Create/Update paths validate non-empty stages before calling this helper.
	if len(fields) == 0 {
		fields = platformFormBuilderContract().Fields
	}
	if len(stages) == 0 {
		stages = platformFormBuilderContract().Stages
	}
	schema[platformFormDesignSchemaKey] = map[string]any{
		"icon":       firstNonEmpty(input.Icon, "📋"),
		"category":   firstNonEmpty(input.Category, "其他"),
		"desc":       strings.TrimSpace(input.Desc),
		"enabled":    enabled,
		"deleted":    deleted,
		"form_kind":  firstNonEmpty(strings.TrimSpace(input.FormKind), platformTemplateFormKind(base)),
		"updated_at": updatedAt.UTC().Format(time.RFC3339),
		"fields":     fields,
		"stages":     stages,
	}
	schema["flow"] = platformStageFlow(stages)
	return schema
}

// platformTemplateDesign 處理平台範本 design。
func platformTemplateDesign(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	if value, ok := schema[platformFormDesignSchemaKey].(map[string]any); ok {
		return value
	}
	return nil
}

// platformTemplateDeleted 處理平台範本 deleted。
func platformTemplateDeleted(schema map[string]any) bool {
	return platformDesignBool(platformTemplateDesign(schema), "deleted", false)
}

// platformTemplateEnabled 處理平台範本 enabled。
func platformTemplateEnabled(schema map[string]any) bool {
	return platformDesignBool(platformTemplateDesign(schema), "enabled", true)
}

// platformTemplateIcon 處理平台範本 icon。
func platformTemplateIcon(template FormTemplate) string {
	if icon := platformDesignString(platformTemplateDesign(template.Schema), "icon"); icon != "" {
		return icon
	}
	for _, column := range platformFormColumns() {
		for _, item := range column.Items {
			if item.ID == template.Key && item.Emoji != "" {
				return item.Emoji
			}
		}
	}
	return "📋"
}

// platformTemplateCategory 處理平台範本分類。
func platformTemplateCategory(template FormTemplate) string {
	if category := platformDesignString(platformTemplateDesign(template.Schema), "category"); category != "" {
		return category
	}
	return platformFormCategory(template.Key)
}

// platformTemplateFormKind 處理平台範本 form kind。
func platformTemplateFormKind(schema map[string]any) string {
	if kind := platformDesignString(platformTemplateDesign(schema), "form_kind"); kind != "" {
		return kind
	}
	return ""
}

func defaultFormKindForTemplateKey(key string) string {
	switch strings.TrimSpace(key) {
	case "leave-request", "field-leave":
		return "hybrid"
	case "leave-cancel":
		return "system"
	default:
		return "custom"
	}
}

var lockedLeaveCoreFieldIDs = map[string]struct{}{
	"leave_type": {},
	"start_at":   {},
	"end_at":     {},
	"hours":      {},
	"proxy":      {},
	"reason":     {},
}

func lockedFieldIDsForTemplate(templateKey, formKind string) map[string]struct{} {
	kind := strings.TrimSpace(formKind)
	if kind == "" {
		kind = defaultFormKindForTemplateKey(templateKey)
	}
	if kind != "system" && kind != "hybrid" {
		return nil
	}
	switch strings.TrimSpace(templateKey) {
	case "leave-request", "field-leave", "leave-cancel":
		return lockedLeaveCoreFieldIDs
	default:
		return nil
	}
}

// platformTemplateDesc 處理平台範本 desc。
func platformTemplateDesc(template FormTemplate) string {
	if desc := platformDesignString(platformTemplateDesign(template.Schema), "desc"); desc != "" {
		return desc
	}
	return template.Description
}

// platformTemplateUpdatedAt 處理平台範本 updated at。
func platformTemplateUpdatedAt(schema map[string]any, fallback time.Time) string {
	if raw := platformDesignString(platformTemplateDesign(schema), "updated_at"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return platformTime(parsed)
		}
	}
	return platformTime(fallback)
}

// platformTemplateFields 處理平台範本欄位。
func platformTemplateFields(schema map[string]any) []PlatformFormBuilderField {
	if fields, ok := platformDecodeSlice[PlatformFormBuilderField](platformTemplateDesign(schema)["fields"]); ok {
		return fields
	}
	return platformFormBuilderContract().Fields
}

// platformTemplateStages 處理平台範本 stages。
func platformTemplateStages(schema map[string]any) []PlatformFormBuilderStage {
	if stages, ok := platformDecodeSlice[PlatformFormBuilderStage](platformTemplateDesign(schema)["stages"]); ok {
		return stages
	}
	return platformFormBuilderContract().Stages
}

// platformDecodeSlice 處理平台 decode slice。
func platformDecodeSlice[T any](value any) ([]T, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.([]T); ok {
		return typed, true
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var out []T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

// platformDesignString 處理平台 design 字串。
func platformDesignString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// platformDesignBool 處理平台 design 布林值。
func platformDesignBool(values map[string]any, key string, fallback bool) bool {
	if len(values) == 0 {
		return fallback
	}
	if value, ok := values[key].(bool); ok {
		return value
	}
	return fallback
}

// platformFormBuilderContract 處理平台表單 builder contract。
func platformFormBuilderContract() PlatformFormBuilderContract {
	return PlatformFormBuilderContract{
		Layouts: []PlatformFormBuilderLayout{
			{Key: "single", Label: "單欄", Columns: []string{"100%"}},
			{Key: "two", Label: "雙欄", Columns: []string{"50%", "50%"}},
		},
		FieldTypes: []PlatformFormBuilderFieldType{
			{Key: "text", Label: "文字", Icon: "type"},
			{Key: "textarea", Label: "多行文字", Icon: "align-left"},
			{Key: "date", Label: "日期", Icon: "calendar"},
			{Key: "number", Label: "數字", Icon: "hash"},
			{Key: "upload", Label: "附件", Icon: "paperclip"},
		},
		Fields: []PlatformFormBuilderField{
			{ID: "subject", Type: "text", Label: "申請主旨", Placeholder: "請填寫申請主旨", Required: true},
			{ID: "needed_at", Type: "datetime", Label: "需求日期", Placeholder: "選擇日期", Required: true},
			{ID: "description", Type: "textarea", Label: "申請說明", Placeholder: "請填寫申請內容", Required: true},
		},
		Stages: []PlatformFormBuilderStage{
			{ID: "stage-manager", Type: "approver", Label: "直屬主管", Detail: "依員工主管關係自動帶入", Config: map[string]any{"role": "manager"}},
			{ID: "stage-hr", Type: "approver", Label: "HR 複核", Detail: "高風險表單需 HR 確認", Config: map[string]any{"role": "hr"}},
			{ID: "stage-notify", Type: "notify", Label: "通知申請人", Detail: "簽核完成後發送通知", Config: map[string]any{"role": "applicant"}},
		},
	}
}

// platformAssistantCatalog 處理平台助理目錄。
func platformAssistantCatalog() []PlatformAssistant {
	return []PlatformAssistant{
		{ID: "employee-care", Emoji: "🙋", Title: "員工疑難雜症助理", Desc: "提供員工諮詢、申訴與 IT 報修引導。", Tag: "workflow"},
		{ID: "sales-analytics", Emoji: "📈", Title: "業務報表分析", Desc: "解讀銷售數據並抓取成長或衰退訊號。", Tag: "analytics"},
		{ID: "product-catalog", Emoji: "📖", Title: "產品目錄助理", Desc: "查詢產品規格並產出提案摘要。", Tag: "doc"},
		{ID: "recruiting", Emoji: "🤝", Title: "招聘獵頭助理", Desc: "協助產生 JD、篩選履歷與安排面試。", Tag: "workflow"},
		{ID: "training-mentor", Emoji: "🎓", Title: "培訓學長姐", Desc: "推薦入職訓練清單並解答內部作業規範。", Tag: "doc"},
		{ID: "legal-contract", Emoji: "⚖️", Title: "合約法務顧問", Desc: "掃描合約風險並比對版本差異。", Tag: "doc"},
		{ID: "project-manager", Emoji: "🏗️", Title: "專案進度管家", Desc: "彙整專案里程碑並產出週報。", Tag: "workflow"},
		{ID: "security", Emoji: "🛡️", Title: "資安風控官", Desc: "提醒異常登入與資安宣導。", Tag: "it"},
	}
}

// firstPlatformAssistants 取得第一個平台助理。
func firstPlatformAssistants(limit int) []PlatformAssistant {
	items := platformAssistantCatalog()
	if limit > 0 && len(items) > limit {
		return append([]PlatformAssistant(nil), items[:limit]...)
	}
	return append([]PlatformAssistant(nil), items...)
}

// platformAssistantMessages 處理平台助理 messages。
func platformAssistantMessages() []PlatformChatMessage {
	return []PlatformChatMessage{
		{ID: "m1", Role: "assistant", Avatar: "🤖", Content: "哈囉！告訴我你想處理的事情，我能幫你挑選最合適的助理。"},
	}
}

// platformHomeFormColumns 處理平台首頁表單 columns。
func platformHomeFormColumns() []PlatformFormColumn {
	columns := platformFormColumns()
	if len(columns) > 2 {
		return append([]PlatformFormColumn(nil), columns[:2]...)
	}
	return columns
}

// platformFormCategories 依已啟用範本組裝表單分類；無範本時回退靜態清單。
func (c PlatformService) platformFormCategories(ctx RequestContext) ([]PlatformFormColumn, error) {
	templates, err := c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	return platformFormColumnsFromTemplates(templates), nil
}

// platformFormColumnsFromTemplates 將啟用範本分組為表單入口欄位。
func platformFormColumnsFromTemplates(templates []FormTemplate) []PlatformFormColumn {
	enabled := make([]FormTemplate, 0, len(templates))
	for _, template := range templates {
		if platformTemplateDeleted(template.Schema) || !platformTemplateEnabled(template.Schema) {
			continue
		}
		if strings.TrimSpace(template.Key) == "" {
			continue
		}
		enabled = append(enabled, template)
	}
	if len(enabled) == 0 {
		return platformFormColumns()
	}
	grouped := map[string][]PlatformFormItem{}
	for _, template := range enabled {
		category := platformTemplateCategory(template)
		title := strings.TrimSpace(template.Name)
		if title == "" {
			title = template.Key
		}
		grouped[category] = append(grouped[category], PlatformFormItem{
			ID:    template.Key,
			Emoji: platformTemplateIcon(template),
			Title: title,
			Desc:  platformTemplateDesc(template),
		})
	}
	ordered := make([]PlatformFormColumn, 0, len(grouped))
	seen := map[string]struct{}{}
	for _, column := range platformFormColumns() {
		items, ok := grouped[column.Title]
		if !ok || len(items) == 0 {
			continue
		}
		ordered = append(ordered, PlatformFormColumn{
			Title: column.Title,
			Emoji: column.Emoji,
			Items: items,
		})
		seen[column.Title] = struct{}{}
	}
	for category, items := range grouped {
		if _, ok := seen[category]; ok {
			continue
		}
		ordered = append(ordered, PlatformFormColumn{
			Title: category,
			Emoji: "📋",
			Items: items,
		})
	}
	return ordered
}

// platformFormColumns 處理平台表單 columns。
func platformFormColumns() []PlatformFormColumn {
	return []PlatformFormColumn{
		{Title: "人事考勤類", Emoji: "👥", Items: []PlatformFormItem{
			{ID: "leave-request", Emoji: "🗓️", Title: "請假申請單", Desc: "特休 / 事假 / 病假 / 公假"},
			{ID: "overtime-approval", Emoji: "⏰", Title: "加班核准申請單", Desc: "平日延時、假日加班皆可使用"},
			{ID: "punch-fix", Emoji: "🕒", Title: "HR-005 補卡單", Desc: "漏打卡或打卡異常補登"},
		}},
		{Title: "人資相關", Emoji: "👥", Items: []PlatformFormItem{
			{ID: "job-change", Emoji: "📋", Title: "人事/職務/薪資異動單", Desc: "異動職務、調薪、調動"},
			{ID: "headcount-request", Emoji: "➕", Title: "iKala 人員增補申請單", Desc: "新增職缺與招募"},
			{ID: "resignation", Emoji: "👋", Title: "離職及退休申請單", Desc: "離職、退休手續辦理"},
		}},
		{Title: "財會相關", Emoji: "💰", Items: []PlatformFormItem{
			{ID: "expense-claim", Emoji: "💸", Title: "費用報支申請單", Desc: "日常費用核銷"},
			{ID: "prepayment", Emoji: "💵", Title: "預支款申請單", Desc: "出差或專案預支款"},
			{ID: "advance-reimburse", Emoji: "💳", Title: "員工代墊款請領清單", Desc: "員工代墊費用請領"},
		}},
		{Title: "行政相關", Emoji: "🧾", Items: []PlatformFormItem{
			{ID: "travel-request", Emoji: "🛫", Title: "國內外出差申請表", Desc: "出差行程預先申請"},
			{ID: "business-card", Emoji: "📇", Title: "名片申請單", Desc: "新印或補印名片"},
			{ID: "memo", Emoji: "📝", Title: "簽呈", Desc: "通用簽呈"},
		}},
	}
}

// platformFormCategoryNames 處理平台表單分類 names。
func platformFormCategoryNames() []string {
	columns := platformFormColumns()
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		out = append(out, column.Title)
	}
	return out
}

// platformFormCategory 處理平台表單分類。
func platformFormCategory(templateKey string) string {
	for _, column := range platformFormColumns() {
		for _, item := range column.Items {
			if item.ID == templateKey {
				return column.Title
			}
		}
	}
	return "其他"
}

// platformTemplateFlow 處理平台範本 flow。
func platformTemplateFlow(schema map[string]any) string {
	if stages, ok := platformDecodeSlice[PlatformFormBuilderStage](platformTemplateDesign(schema)["stages"]); ok {
		return platformStageFlow(stages)
	}
	if flow, ok := schema["flow"].(string); ok && strings.TrimSpace(flow) != "" {
		return flow
	}
	return "直屬主管 → HR"
}

// platformStageFlow 處理平台 stage flow。
func platformStageFlow(stages []PlatformFormBuilderStage) string {
	labels := make([]string, 0, len(stages))
	for _, stage := range stages {
		if label := strings.TrimSpace(stage.Label); label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		return "—"
	}
	return strings.Join(labels, " → ")
}

// platformFormStatus 處理平台表單狀態。
func platformFormStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		return "approved"
	case "rejected":
		return "rejected"
	case "returned":
		return "returned"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return "reviewing"
	}
}

// platformFormSummary 處理平台表單摘要。
func platformFormSummary(payload map[string]any) string {
	if len(payload) == 0 {
		return "已送出"
	}
	for _, key := range []string{"desc", "description", "subject", "reason"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if leaveType, ok := payload["leave_type"].(string); ok {
		return "假別 " + leaveType
	}
	if leaveType, ok := payload["leaveType"].(string); ok {
		return "假別 " + leaveType
	}
	return "已送出"
}

// clockTime 處理打卡時間。
func clockTime(record *AttendanceClockRecord) *string {
	if record == nil {
		return nil
	}
	text := record.ClockedAt.Format("15:04")
	return &text
}

// platformDateLabel 處理平台日期 label。
func platformDateLabel(t time.Time) string {
	return fmt.Sprintf("%s %s", t.Format(platformDateLayout), platformWeekday(t))
}

// platformWeekday 處理平台星期。
func platformWeekday(t time.Time) string {
	names := []string{"週日", "週一", "週二", "週三", "週四", "週五", "週六"}
	return names[int(t.Weekday())]
}

// platformTime 處理平台時間。
func platformTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04")
}

// sameYearMonth 處理 same year 月份。
func sameYearMonth(left, right time.Time) bool {
	return !left.IsZero() && left.Year() == right.Year() && left.Month() == right.Month()
}

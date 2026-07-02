package domain

import "time"

// PlatformAssistant describes one assistant card shown in the OA workbench.
type PlatformAssistant struct {
	ID    string `json:"id"`
	Emoji string `json:"emoji"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Tag   string `json:"tag,omitempty"`
}

// PlatformFormItem describes one form shortcut in a category column.
type PlatformFormItem struct {
	ID    string `json:"id"`
	Emoji string `json:"emoji"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// PlatformFormColumn groups form shortcuts by business category.
type PlatformFormColumn struct {
	Title string             `json:"title"`
	Emoji string             `json:"emoji"`
	Items []PlatformFormItem `json:"items"`
}

// PlatformClockSummary is the compact home-page attendance card contract.
type PlatformClockSummary struct {
	DateLabel             string  `json:"date_label"`
	CheckedInAt           *string `json:"checked_in_at"`
	CheckedOutAt          *string `json:"checked_out_at"`
	Location              string  `json:"location"`
	MonthlyAttendanceDays int     `json:"monthly_attendance_days"`
	MonthlyHours          float64 `json:"monthly_hours"`
	LeaveDays             float64 `json:"leave_days"`
}

// PlatformHomeResponse aggregates the first-screen workbench widgets.
type PlatformHomeResponse struct {
	Assistants   []PlatformAssistant  `json:"assistants"`
	FormColumns  []PlatformFormColumn `json:"form_columns"`
	ClockSummary PlatformClockSummary `json:"clock_summary"`
}

// PlatformChatMessage is one lightweight AI panel message.
type PlatformChatMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Avatar  string `json:"avatar"`
	Content string `json:"content"`
}

// PlatformAssistantsQuery filters assistant cards.
type PlatformAssistantsQuery struct {
	Tag    string `json:"tag,omitempty"`
	Search string `json:"search,omitempty"`
}

// PlatformAssistantsResponse lists assistant cards plus the sidebar chat seed.
type PlatformAssistantsResponse struct {
	Data         []PlatformAssistant   `json:"data"`
	Total        int                   `json:"total"`
	ChatMessages []PlatformChatMessage `json:"chat_messages"`
	QuickPrompts []string              `json:"quick_prompts"`
}

// PlatformFormApplication is one submitted workflow form row for the forms page.
type PlatformFormApplication struct {
	ID          string         `json:"id"`
	TemplateKey string         `json:"template_key,omitempty"`
	Title       string         `json:"title"`
	Applicant   string         `json:"applicant"`
	SubmittedAt string         `json:"submitted_at"`
	Status      string         `json:"status"`
	Summary     string         `json:"summary"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// PlatformFormDraft is one locally resumable form draft placeholder.
type PlatformFormDraft struct {
	ID          string         `json:"id"`
	TemplateKey string         `json:"template_key,omitempty"`
	Title       string         `json:"title"`
	UpdatedAt   string         `json:"updated_at"`
	Summary     string         `json:"summary"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// PlatformFormsResponse powers the employee self-service forms page.
type PlatformFormsResponse struct {
	Categories   []PlatformFormColumn      `json:"categories"`
	Applications []PlatformFormApplication `json:"applications"`
	Drafts       []PlatformFormDraft       `json:"drafts"`
	AIMessages   []PlatformChatMessage     `json:"ai_messages"`
	QuickPrompts []string                  `json:"quick_prompts"`
}

// PlatformTaskItem describes one task record line.
type PlatformTaskItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Category string  `json:"category"`
	Product  string  `json:"product"`
	Hours    float64 `json:"hours"`
	Note     string  `json:"note"`
}

// PlatformTaskRecord groups one day's task lines.
type PlatformTaskRecord struct {
	Date       string             `json:"date"`
	Weekday    string             `json:"weekday"`
	TotalHours float64            `json:"total_hours"`
	Items      []PlatformTaskItem `json:"items"`
}

// PlatformTaskTodo is one task sidebar checklist item.
type PlatformTaskTodo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
	Date string `json:"date"`
}

// PlatformTaskRecordItem persists one manually entered work-log item.
type PlatformTaskRecordItem struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AccountID string    `json:"account_id"`
	WorkDate  string    `json:"work_date"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Product   string    `json:"product"`
	Hours     float64   `json:"hours"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlatformTaskTodoRecord persists one current-account task-page todo.
type PlatformTaskTodoRecord struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	AccountID           string    `json:"account_id"`
	Text                string    `json:"text"`
	DueDate             string    `json:"due_date"`
	Status              string    `json:"status"`
	ConvertedTaskItemID string    `json:"converted_task_item_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CreatePlatformTaskItemInput carries task-page work-log create fields.
type CreatePlatformTaskItemInput struct {
	WorkDate string  `json:"work_date,omitempty"`
	Title    string  `json:"title"`
	Category string  `json:"category,omitempty"`
	Product  string  `json:"product,omitempty"`
	Hours    float64 `json:"hours"`
	Note     string  `json:"note,omitempty"`
}

// UpdatePlatformTaskItemInput carries task-page work-log patch fields.
type UpdatePlatformTaskItemInput struct {
	WorkDate *string  `json:"work_date,omitempty"`
	Title    *string  `json:"title,omitempty"`
	Category *string  `json:"category,omitempty"`
	Product  *string  `json:"product,omitempty"`
	Hours    *float64 `json:"hours,omitempty"`
	Note     *string  `json:"note,omitempty"`
}

// CreatePlatformTaskTodoInput carries task-page todo create fields.
type CreatePlatformTaskTodoInput struct {
	Text    string `json:"text"`
	DueDate string `json:"due_date,omitempty"`
}

// UpdatePlatformTaskTodoInput carries task-page todo patch fields.
type UpdatePlatformTaskTodoInput struct {
	Text    *string `json:"text,omitempty"`
	DueDate *string `json:"due_date,omitempty"`
	Done    *bool   `json:"done,omitempty"`
}

// ConvertPlatformTaskTodoInput controls how a todo becomes a work-log item.
type ConvertPlatformTaskTodoInput struct {
	WorkDate string  `json:"work_date,omitempty"`
	Title    string  `json:"title,omitempty"`
	Category string  `json:"category,omitempty"`
	Product  string  `json:"product,omitempty"`
	Hours    float64 `json:"hours,omitempty"`
	Note     string  `json:"note,omitempty"`
}

// PlatformTasksResponse powers the employee task page.
type PlatformTasksResponse struct {
	Records      []PlatformTaskRecord  `json:"records"`
	Todos        []PlatformTaskTodo    `json:"todos"`
	ClockSummary PlatformClockSummary  `json:"clock_summary"`
	AIMessages   []PlatformChatMessage `json:"ai_messages"`
	QuickPrompts []string              `json:"quick_prompts"`
}

// PlatformFormDesign describes the workspace form-builder seed data.
type PlatformFormDesign struct {
	Categories []string                    `json:"categories"`
	Forms      []PlatformFormDesignForm    `json:"forms"`
	Builder    PlatformFormBuilderContract `json:"builder"`
}

// PlatformFormDesignForm is one form template row in workspace settings.
type PlatformFormDesignForm struct {
	ID             string                     `json:"id"`
	Icon           string                     `json:"icon"`
	Name           string                     `json:"name"`
	Category       string                     `json:"category"`
	Desc           string                     `json:"desc,omitempty"`
	Flow           string                     `json:"flow"`
	Enabled        bool                       `json:"enabled"`
	AddedThisMonth bool                       `json:"added_this_month"`
	UpdatedAt      string                     `json:"updated_at"`
	Fields         []PlatformFormBuilderField `json:"fields,omitempty"`
	Stages         []PlatformFormBuilderStage `json:"stages,omitempty"`
}

// PlatformFormBuilderContract contains reusable form-builder palettes.
type PlatformFormBuilderContract struct {
	Layouts    []PlatformFormBuilderLayout    `json:"layouts"`
	FieldTypes []PlatformFormBuilderFieldType `json:"field_types"`
	Fields     []PlatformFormBuilderField     `json:"fields"`
	Stages     []PlatformFormBuilderStage     `json:"stages"`
}

// PlatformFormBuilderLayout describes a form-builder layout choice.
type PlatformFormBuilderLayout struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Columns []string `json:"columns"`
}

// PlatformFormBuilderFieldType describes a field palette item.
type PlatformFormBuilderFieldType struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

// PlatformFormBuilderField is one default field in the builder canvas.
type PlatformFormBuilderField struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
}

// PlatformFormBuilderStage is one approval-flow node in the builder canvas.
type PlatformFormBuilderStage struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// PlatformWorkspaceResponse aggregates workspace settings panes used by the FE.
type PlatformWorkspaceResponse struct {
	AdminSettings WorkspaceAdminsResponse  `json:"admin_settings"`
	AuditLogs     []WorkspaceAuditLog      `json:"audit_logs"`
	FormDesign    PlatformFormDesign       `json:"form_design"`
	LeavePolicy   AttendancePolicyResponse `json:"leave_policy"`
}

// PlatformWorkspaceEmployeesResponse powers the workspace employee table export contract.
type PlatformWorkspaceEmployeesResponse struct {
	Employees  []WorkspaceEmployeeCard `json:"employees"`
	CSVHeaders []string                `json:"csv_headers"`
}

// PlatformWorkspaceEmployeesQuery filters the workspace employee table payload.
type PlatformWorkspaceEmployeesQuery struct {
	DepartmentID     string `json:"department_id,omitempty"`
	Department       string `json:"department,omitempty"`
	Status           string `json:"status,omitempty"`
	EmploymentStatus string `json:"employment_status,omitempty"`
	Keyword          string `json:"keyword,omitempty"`
}

// UpdateWorkspaceOrganizationManagerInput updates one employee's manager link from the workspace organization view.
type UpdateWorkspaceOrganizationManagerInput struct {
	ParentID *string `json:"parent_id,omitempty"`
}

// CreateWorkspaceAdminInput grants workspace-admin permissions to an account-bound employee.
type CreateWorkspaceAdminInput struct {
	EmployeeID  string            `json:"employee_id"`
	Permissions map[string]string `json:"permissions"`
}

// UpdateWorkspaceAdminPermissionsInput replaces one workspace admin's permission matrix.
type UpdateWorkspaceAdminPermissionsInput struct {
	Permissions map[string]string `json:"permissions"`
}

// SaveWorkspaceFormDesignInput carries one workspace form-builder template write.
type SaveWorkspaceFormDesignInput struct {
	ID       string                     `json:"id,omitempty"`
	Icon     string                     `json:"icon,omitempty"`
	Name     string                     `json:"name"`
	Category string                     `json:"category,omitempty"`
	Desc     string                     `json:"desc,omitempty"`
	Enabled  *bool                      `json:"enabled,omitempty"`
	Fields   []PlatformFormBuilderField `json:"fields,omitempty"`
	Stages   []PlatformFormBuilderStage `json:"stages,omitempty"`
}

// UpdateWorkspaceFormDesignInput carries partial workspace form-builder updates.
type UpdateWorkspaceFormDesignInput struct {
	Icon     *string                     `json:"icon,omitempty"`
	Name     *string                     `json:"name,omitempty"`
	Category *string                     `json:"category,omitempty"`
	Desc     *string                     `json:"desc,omitempty"`
	Enabled  *bool                       `json:"enabled,omitempty"`
	Fields   *[]PlatformFormBuilderField `json:"fields,omitempty"`
	Stages   *[]PlatformFormBuilderStage `json:"stages,omitempty"`
}

// PlatformInsightsQuery selects the report month.
type PlatformInsightsQuery struct {
	Month string `json:"month,omitempty"`
}

// PlatformInsightsResponse returns answer-ready report payloads for the insights page.
type PlatformInsightsResponse struct {
	Month   string                  `json:"month"`
	Reports map[string]any          `json:"reports"`
	AIPanel PlatformInsightsAIPanel `json:"ai_panel"`
}

// PlatformInsightsAIPanel contains chat seed messages and quick prompts.
type PlatformInsightsAIPanel struct {
	Messages     []PlatformChatMessage `json:"messages"`
	QuickPrompts []string              `json:"quick_prompts"`
}

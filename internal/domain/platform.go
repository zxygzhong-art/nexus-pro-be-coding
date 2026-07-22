package domain

import "time"

// PlatformAssistant 定義平臺助理的資料結構。
type PlatformAssistant struct {
	ID                 string   `json:"id"`
	Emoji              string   `json:"emoji"`
	Title              string   `json:"title"`
	Desc               string   `json:"desc"`
	Tag                string   `json:"tag,omitempty"`
	WelcomeMessage     string   `json:"welcome_message"`
	SuggestedQuestions []string `json:"suggested_questions"`
	Runnable           bool     `json:"runnable"`
}

// PlatformFormItem 定義平臺表單項目的資料結構。
type PlatformFormItem struct {
	ID    string `json:"id"`
	Emoji string `json:"emoji"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// PlatformFormColumn 定義平臺表單 column 的資料結構。
type PlatformFormColumn struct {
	Title string             `json:"title"`
	Emoji string             `json:"emoji"`
	Items []PlatformFormItem `json:"items"`
}

// PlatformClockSummary 定義平臺打卡摘要的資料結構。
type PlatformClockSummary struct {
	DateLabel             string  `json:"date_label"`
	CheckedInAt           *string `json:"checked_in_at"`
	CheckedOutAt          *string `json:"checked_out_at"`
	Location              string  `json:"location"`
	MonthlyAttendanceDays int     `json:"monthly_attendance_days"`
	MonthlyHours          float64 `json:"monthly_hours"`
	MonthlyOvertimeHours  float64 `json:"monthly_overtime_hours"`
	LeaveDays             float64 `json:"leave_days"`
}

// PlatformHomeResponse 定義平臺首頁回應的資料結構。
type PlatformHomeResponse struct {
	Assistants   []PlatformAssistant   `json:"assistants"`
	FormColumns  []PlatformFormColumn  `json:"form_columns"`
	ClockSummary *PlatformClockSummary `json:"clock_summary,omitempty"`
}

// PlatformChatMessage 定義平臺 chat message 的資料結構。
type PlatformChatMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Avatar  string `json:"avatar"`
	Content string `json:"content"`
}

// PlatformAssistantsQuery 定義平臺助理查詢的資料結構。
type PlatformAssistantsQuery struct {
	Tag    string `json:"tag,omitempty"`
	Search string `json:"search,omitempty"`
}

// PlatformAssistantsResponse 定義平臺助理回應的資料結構。
type PlatformAssistantsResponse struct {
	Data         []PlatformAssistant   `json:"data"`
	Total        int                   `json:"total"`
	ChatMessages []PlatformChatMessage `json:"chat_messages"`
	QuickPrompts []string              `json:"quick_prompts"`
}

// PlatformFormApplication 定義平臺表單 application 的資料結構。
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

// PlatformFormDraft 定義平臺表單草稿的資料結構。
type PlatformFormDraft struct {
	ID          string         `json:"id"`
	TemplateKey string         `json:"template_key,omitempty"`
	Title       string         `json:"title"`
	UpdatedAt   string         `json:"updated_at"`
	Summary     string         `json:"summary"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// PlatformFormsQuery 定義平臺表單查詢的資料結構。
type PlatformFormsQuery struct {
	Status   string `json:"status,omitempty"`
	Template string `json:"template,omitempty"`
	Search   string `json:"search,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// PlatformFormApplicationPage 定義平臺表單 application 分頁回應的資料結構。
type PlatformFormApplicationPage struct {
	Items    []PlatformFormApplication `json:"items"`
	Total    int                       `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

// PlatformFormDraftPage 定義平臺表單草稿分頁回應的資料結構。
type PlatformFormDraftPage struct {
	Items    []PlatformFormDraft `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// PlatformFormsResponse 定義平臺表單回應的資料結構。
type PlatformFormsResponse struct {
	Categories   []PlatformFormColumn       `json:"categories"`
	Applications PlatformFormApplicationPage `json:"applications"`
	Drafts       PlatformFormDraftPage       `json:"drafts"`
	AIMessages   []PlatformChatMessage      `json:"ai_messages"`
	QuickPrompts []string                   `json:"quick_prompts"`
}

// PlatformTaskItem 定義平臺任務項目的資料結構。
type PlatformTaskItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Category string  `json:"category"`
	Product  string  `json:"product"`
	Hours    float64 `json:"hours"`
	Note     string  `json:"note"`
	WorkDate string  `json:"work_date,omitempty"`
	Source   string  `json:"source,omitempty"`
	ReadOnly bool    `json:"read_only,omitempty"`
}

// PlatformTaskRecord 定義平臺任務 record 的資料結構。
type PlatformTaskRecord struct {
	Date       string             `json:"date"`
	Weekday    string             `json:"weekday"`
	TotalHours float64            `json:"total_hours"`
	Items      []PlatformTaskItem `json:"items"`
}

// PlatformTaskTodo 定義平臺任務待辦的資料結構。
type PlatformTaskTodo struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Done     bool   `json:"done"`
	Date     string `json:"date"`
	WorkDate string `json:"work_date,omitempty"`
	DueDate  string `json:"due_date,omitempty"`
	Source   string `json:"source,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// PlatformTaskRecordItem 定義平臺任務 record 項目的資料結構。
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

// PlatformTaskTodoRecord 定義平臺任務待辦 record 的資料結構。
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

// CreatePlatformTaskItemInput 定義平臺任務項目輸入的資料結構。
type CreatePlatformTaskItemInput struct {
	WorkDate string  `json:"work_date,omitempty"`
	Title    string  `json:"title"`
	Category string  `json:"category,omitempty"`
	Product  string  `json:"product,omitempty"`
	Hours    float64 `json:"hours"`
	Note     string  `json:"note,omitempty"`
}

// UpdatePlatformTaskItemInput 定義平臺任務項目輸入的資料結構。
type UpdatePlatformTaskItemInput struct {
	WorkDate *string  `json:"work_date,omitempty"`
	Title    *string  `json:"title,omitempty"`
	Category *string  `json:"category,omitempty"`
	Product  *string  `json:"product,omitempty"`
	Hours    *float64 `json:"hours,omitempty"`
	Note     *string  `json:"note,omitempty"`
}

// CreatePlatformTaskTodoInput 定義平臺任務待辦輸入的資料結構。
type CreatePlatformTaskTodoInput struct {
	Text    string `json:"text"`
	DueDate string `json:"due_date,omitempty"`
}

// UpdatePlatformTaskTodoInput 定義平臺任務待辦輸入的資料結構。
type UpdatePlatformTaskTodoInput struct {
	Text    *string `json:"text,omitempty"`
	DueDate *string `json:"due_date,omitempty"`
	Done    *bool   `json:"done,omitempty"`
}

// ConvertPlatformTaskTodoInput 定義 convert 平臺任務待辦輸入的資料結構。
type ConvertPlatformTaskTodoInput struct {
	WorkDate string  `json:"work_date,omitempty"`
	Title    string  `json:"title,omitempty"`
	Category string  `json:"category,omitempty"`
	Product  string  `json:"product,omitempty"`
	Hours    float64 `json:"hours,omitempty"`
	Note     string  `json:"note,omitempty"`
}

// PlatformTasksDefaultWindowDays 是平臺任務查詢的預設時間窗天數。
const PlatformTasksDefaultWindowDays = 90

// PlatformTasksQuery 定義平臺任務 cursor 分頁與時間窗查詢的資料結構。
type PlatformTasksQuery struct {
	Cursor          string    `json:"cursor,omitempty"`
	PageSize        int       `json:"page_size,omitempty"`
	From            time.Time `json:"from,omitempty"`
	To              time.Time `json:"to,omitempty"`
	HasCursor       bool      `json:"-"`
	CursorCreatedAt time.Time `json:"-"`
	CursorID        string    `json:"-"`
}

// PlatformTasksResponse 定義平臺任務回應的資料結構。
type PlatformTasksResponse struct {
	Records      []PlatformTaskRecord  `json:"records"`
	Items        []PlatformTaskItem    `json:"items"`
	Todos        []PlatformTaskTodo    `json:"todos"`
	NextCursor   string                `json:"next_cursor,omitempty"`
	ClockSummary *PlatformClockSummary `json:"clock_summary,omitempty"`
	AIMessages   []PlatformChatMessage `json:"ai_messages"`
	QuickPrompts []string              `json:"quick_prompts"`
}

// PlatformFormDesign 定義平臺表單 design 的資料結構。
type PlatformFormDesign struct {
	Categories []string                    `json:"categories"`
	Forms      []PlatformFormDesignForm    `json:"forms"`
	Builder    PlatformFormBuilderContract `json:"builder"`
}

// PlatformFormDesignForm 定義平臺表單 design 表單的資料結構。
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
	UpdatedBy      string                     `json:"updated_by,omitempty"`
	FormKind       string                     `json:"form_kind,omitempty"`
	Fields         []PlatformFormBuilderField `json:"fields,omitempty"`
	Stages         []PlatformFormBuilderStage `json:"stages,omitempty"`
}

// PlatformFormBuilderContract 定義平臺表單 builder contract 的資料結構。
type PlatformFormBuilderContract struct {
	Layouts    []PlatformFormBuilderLayout    `json:"layouts"`
	FieldTypes []PlatformFormBuilderFieldType `json:"field_types"`
	Fields     []PlatformFormBuilderField     `json:"fields"`
	Stages     []PlatformFormBuilderStage     `json:"stages"`
}

// PlatformFormBuilderLayout 定義平臺表單 builder layout 的資料結構。
type PlatformFormBuilderLayout struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Columns []string `json:"columns"`
}

// PlatformFormBuilderFieldType 定義平臺表單 builder 欄位 type 的資料結構。
type PlatformFormBuilderFieldType struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

// PlatformFormBuilderFieldOption 定義表單欄位選項。
type PlatformFormBuilderFieldOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PlatformFormBuilderFieldBinding 定義欄位與受控資料源的持久化綁定。
type PlatformFormBuilderFieldBinding struct {
	SourceID   string `json:"source_id"`
	ValueField string `json:"value_field"`
	LabelField string `json:"label_field,omitempty"`
}

// PlatformFormBuilderFieldAnalytics 定義欄位可用的統計語意。
type PlatformFormBuilderFieldAnalytics struct {
	Reportable   bool     `json:"reportable"`
	Role         string   `json:"role,omitempty"`
	Aggregations []string `json:"aggregations,omitempty"`
	Filterable   bool     `json:"filterable,omitempty"`
	Groupable    bool     `json:"groupable,omitempty"`
}

// PlatformFormBuilderFieldSecurity 定義欄位敏感度與 Agent 可見性。
type PlatformFormBuilderFieldSecurity struct {
	Classification string `json:"classification,omitempty"`
	Masking        string `json:"masking,omitempty"`
	AgentAccess    bool   `json:"agent_access,omitempty"`
}

// PlatformFormBuilderField 定義平臺表單 builder 欄位的資料結構。
type PlatformFormBuilderField struct {
	ID             string                             `json:"id"`
	Type           string                             `json:"type"`
	Label          string                             `json:"label"`
	Placeholder    string                             `json:"placeholder"`
	Required       bool                               `json:"required"`
	DefaultValue   any                                `json:"default_value,omitempty"`
	LayoutColumns  []string                           `json:"layout_columns,omitempty"`
	Options        []PlatformFormBuilderFieldOption   `json:"options,omitempty"`
	Binding        *PlatformFormBuilderFieldBinding   `json:"binding,omitempty"`
	Analytics      *PlatformFormBuilderFieldAnalytics `json:"analytics,omitempty"`
	Security       *PlatformFormBuilderFieldSecurity  `json:"security,omitempty"`
	ParentLayoutID string                             `json:"parent_layout_id,omitempty"`
	SlotIndex      *int                               `json:"slot_index,omitempty"`
}

// PlatformFormBuilderStage 定義平臺表單 builder stage 的資料結構。
type PlatformFormBuilderStage struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Label  string         `json:"label"`
	Detail string         `json:"detail"`
	Config map[string]any `json:"config,omitempty"`
}

// PlatformWorkspaceResponse 定義平臺工作區回應的資料結構。
type PlatformWorkspaceResponse struct {
	AuditLogs   []WorkspaceAuditLog      `json:"audit_logs"`
	FormDesign  PlatformFormDesign       `json:"form_design"`
	LeavePolicy AttendancePolicyResponse `json:"leave_policy"`
}

// PlatformWorkspaceEmployeesResponse 定義平臺工作區員工回應的資料結構。
type PlatformWorkspaceEmployeesResponse struct {
	Employees  []WorkspaceEmployeeCard `json:"employees"`
	CSVHeaders []string                `json:"csv_headers"`
}

// PlatformWorkspaceEmployeesQuery 定義平臺工作區員工查詢的資料結構。
type PlatformWorkspaceEmployeesQuery struct {
	DepartmentID     string `json:"department_id,omitempty"`
	Department       string `json:"department,omitempty"`
	Status           string `json:"status,omitempty"`
	EmploymentStatus string `json:"employment_status,omitempty"`
	Keyword          string `json:"keyword,omitempty"`
}

// UpdateWorkspaceOrganizationManagerInput 定義工作區 organization 主管輸入的資料結構。
type UpdateWorkspaceOrganizationManagerInput struct {
	ParentID *string `json:"parent_id,omitempty"`
}

// UpdateWorkspaceOrganizationVisibilityInput 定義組織圖預覽可見性輸入。
type UpdateWorkspaceOrganizationVisibilityInput struct {
	ShowInOrgChart *bool `json:"show_in_org_chart,omitempty"`
}

// SaveWorkspaceFormDesignInput 定義工作區表單 design 輸入的資料結構。
type SaveWorkspaceFormDesignInput struct {
	ID       string                     `json:"id,omitempty"`
	Icon     string                     `json:"icon,omitempty"`
	Name     string                     `json:"name"`
	Category string                     `json:"category,omitempty"`
	Desc     string                     `json:"desc,omitempty"`
	Enabled  *bool                      `json:"enabled,omitempty"`
	FormKind string                     `json:"form_kind,omitempty"`
	Fields   []PlatformFormBuilderField `json:"fields,omitempty"`
	Stages   []PlatformFormBuilderStage `json:"stages,omitempty"`
}

// UpdateWorkspaceFormDesignInput 定義工作區表單 design 輸入的資料結構。
type UpdateWorkspaceFormDesignInput struct {
	Icon     *string                     `json:"icon,omitempty"`
	Name     *string                     `json:"name,omitempty"`
	Category *string                     `json:"category,omitempty"`
	Desc     *string                     `json:"desc,omitempty"`
	Enabled  *bool                       `json:"enabled,omitempty"`
	FormKind *string                     `json:"form_kind,omitempty"`
	Fields   *[]PlatformFormBuilderField `json:"fields,omitempty"`
	Stages   *[]PlatformFormBuilderStage `json:"stages,omitempty"`
}

// PlatformInsightsQuery 定義平臺洞察查詢的資料結構。
type PlatformInsightsQuery struct {
	Month string `json:"month,omitempty"`
}

// PlatformInsightsResponse 定義平臺洞察回應的資料結構。
type PlatformInsightsResponse struct {
	Month   string                  `json:"month"`
	Reports map[string]any          `json:"reports"`
	AIPanel PlatformInsightsAIPanel `json:"ai_panel"`
}

// PlatformInsightsAIPanel 定義平臺洞察 ai panel 的資料結構。
type PlatformInsightsAIPanel struct {
	Messages     []PlatformChatMessage `json:"messages"`
	QuickPrompts []string              `json:"quick_prompts"`
}

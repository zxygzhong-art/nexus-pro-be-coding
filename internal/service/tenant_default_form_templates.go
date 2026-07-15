package service

import (
	"fmt"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

type tenantDefaultFormDefinition struct {
	Key         string
	Name        string
	Description string
	Category    string
	Icon        string
	FormKind    string
	Fields      []domain.PlatformFormBuilderField
	Stages      []domain.PlatformFormBuilderStage
}

// tenantDefaultFormTemplates 建立表單中心的六份常用申請表單。
func tenantDefaultFormTemplates(tenantID string, now time.Time) []domain.FormTemplate {
	definitions := tenantDefaultFormDefinitions()
	templates := make([]domain.FormTemplate, 0, len(definitions))
	for _, definition := range definitions {
		templates = append(templates, tenantDefaultFormTemplate(tenantID, now, definition))
	}
	return templates
}

// tenantDefaultFormDefinitions 集中管理內建表單的元件、分類與簽核流程。
func tenantDefaultFormDefinitions() []tenantDefaultFormDefinition {
	return []tenantDefaultFormDefinition{
		{
			Key: "leave-request", Name: "請假申請單", Description: "特休 / 事假 / 病假 / 公假",
			Category: "人事考勤類", Icon: "🗓️", FormKind: "hybrid",
			Fields: platformLeaveRequestBuilderFields(), Stages: tenantManagerApprovalStages(),
		},
		{
			Key: "overtime-approval", Name: "加班核准申請單", Description: "平日延時、假日加班皆可使用",
			Category: "人事考勤類", Icon: "⏰", FormKind: "hybrid",
			Fields: tenantOvertimeFormFields(), Stages: tenantManagerApprovalStages(),
		},
		{
			Key: "punch-fix", Name: "HR-005 補卡單", Description: "漏打卡或打卡異常補登",
			Category: "人事考勤類", Icon: "🕒", FormKind: "hybrid",
			Fields: tenantPunchFixFormFields(), Stages: tenantManagerHRApprovalStages(),
		},
		{
			Key: "job-change", Name: "人事/職務/薪資異動單", Description: "異動職務、調薪、調動",
			Category: "人資相關", Icon: "📋", FormKind: "custom",
			Fields: tenantJobChangeFormFields(), Stages: tenantManagerHRApprovalStages(),
		},
		{
			Key: "headcount-request", Name: "iKala 人員增補申請單", Description: "新增職缺與招募",
			Category: "人資相關", Icon: "➕", FormKind: "custom",
			Fields: tenantHeadcountFormFields(), Stages: tenantHeadcountApprovalStages(),
		},
		{
			Key: "resignation", Name: "離職及退休申請單", Description: "離職、退休手續辦理",
			Category: "人資相關", Icon: "👋", FormKind: "custom",
			Fields: tenantResignationFormFields(), Stages: tenantManagerHRApprovalStages(),
		},
	}
}

// tenantDefaultFormTemplate 將 builder 元件定義編譯成可發布的表單範本。
func tenantDefaultFormTemplate(tenantID string, now time.Time, definition tenantDefaultFormDefinition) domain.FormTemplate {
	return domain.FormTemplate{
		ID:             fmt.Sprintf("ft-%s-%s-%s", safeTenantProvisionSlug(tenantID), shortTenantProvisionHash(tenantID), definition.Key),
		TenantID:       tenantID,
		Key:            definition.Key,
		Name:           definition.Name,
		Description:    definition.Description,
		Schema:         tenantDefaultFormSchema(definition),
		Status:         "published",
		CurrentVersion: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// tenantDefaultFormSchema 同步產生 JSON Schema 與 workspace builder 契約。
func tenantDefaultFormSchema(definition tenantDefaultFormDefinition) map[string]any {
	properties := make(map[string]any)
	required := make([]string, 0)
	for _, field := range definition.Fields {
		property, ok := tenantDefaultFormProperty(field)
		if !ok {
			continue
		}
		properties[field.ID] = property
		if field.Required {
			required = append(required, field.ID)
		}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
		platformFormDesignSchemaKey: map[string]any{
			"enabled":   true,
			"form_kind": definition.FormKind,
			"category":  definition.Category,
			"icon":      definition.Icon,
			"desc":      definition.Description,
			"fields":    definition.Fields,
			"stages":    definition.Stages,
		},
	}
}

// tenantDefaultFormProperty 將輸入元件映射成基本 JSON Schema 型別。
func tenantDefaultFormProperty(field domain.PlatformFormBuilderField) (map[string]any, bool) {
	switch strings.TrimSpace(field.Type) {
	case "layout", "section-title", "divider", "html":
		return nil, false
	case "number":
		return map[string]any{"type": "number"}, true
	case "checkbox":
		return map[string]any{"type": "boolean"}, true
	case "multilist", "file", "image":
		return map[string]any{"type": "array"}, true
	case "datetime":
		return map[string]any{"type": "string", "format": "date-time"}, true
	case "date":
		return map[string]any{"type": "string", "format": "date"}, true
	case "email":
		return map[string]any{"type": "string", "format": "email"}, true
	case "url":
		return map[string]any{"type": "string", "format": "uri"}, true
	default:
		return map[string]any{"type": "string"}, true
	}
}

// tenantApplicantFields 建立各表單共用的申請人與組織自動帶入欄位。
func tenantApplicantFields() []domain.PlatformFormBuilderField {
	return []domain.PlatformFormBuilderField{
		{ID: "section-applicant", Type: "section-title", Label: "申請人資料"},
		{ID: "layout-applicant", Type: "layout", Label: "申請人列", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		{ID: "applicant_name", Type: "autofill", Label: "申請人", Placeholder: "登入者自動帶入", Binding: tenantFormBinding("current_user", "display_name", ""), ParentLayoutID: "layout-applicant", SlotIndex: tenantFormSlot(0)},
		{ID: "applicant_employee_no", Type: "autofill", Label: "員工編號", Placeholder: "依申請人自動帶入", Binding: tenantFormBinding("current_user", "employee_no", ""), ParentLayoutID: "layout-applicant", SlotIndex: tenantFormSlot(1)},
		{ID: "layout-applicant-org", Type: "layout", Label: "組織列", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		{ID: "applicant_department", Type: "autofill", Label: "部門", Placeholder: "依申請人自動帶入", Binding: tenantFormBinding("current_user", "department_name", ""), ParentLayoutID: "layout-applicant-org", SlotIndex: tenantFormSlot(0)},
		{ID: "applicant_position", Type: "autofill", Label: "職稱", Placeholder: "依申請人自動帶入", Binding: tenantFormBinding("current_user", "position_name", ""), ParentLayoutID: "layout-applicant-org", SlotIndex: tenantFormSlot(1)},
	}
}

// tenantOvertimeFormFields 建立加班核准表單元件。
func tenantOvertimeFormFields() []domain.PlatformFormBuilderField {
	fields := tenantApplicantFields()
	return append(fields,
		domain.PlatformFormBuilderField{ID: "section-overtime", Type: "section-title", Label: "加班內容"},
		domain.PlatformFormBuilderField{ID: "overtime_type", Type: "radio", Label: "加班類型", Placeholder: "請選擇", Required: true, DefaultValue: "weekday", Options: []domain.PlatformFormBuilderFieldOption{{Label: "平日延時", Value: "weekday"}, {Label: "假日加班", Value: "holiday"}}, Analytics: tenantDimensionAnalytics()},
		domain.PlatformFormBuilderField{ID: "layout-overtime-range", Type: "layout", Label: "加班時段", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "start_at", Type: "datetime", Label: "開始時間", Placeholder: "選擇日期時間", Required: true, ParentLayoutID: "layout-overtime-range", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "end_at", Type: "datetime", Label: "結束時間", Placeholder: "選擇日期時間", Required: true, ParentLayoutID: "layout-overtime-range", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "layout-overtime-result", Type: "layout", Label: "時數與補償", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "hours", Type: "number", Label: "加班時數", Placeholder: "0", Required: true, Analytics: tenantMeasureAnalytics(), ParentLayoutID: "layout-overtime-result", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "compensation_type", Type: "radio", Label: "補償方式", Placeholder: "請選擇", Required: true, DefaultValue: "leave", Options: []domain.PlatformFormBuilderFieldOption{{Label: "補休", Value: "leave"}, {Label: "加班費", Value: "pay"}}, Analytics: tenantDimensionAnalytics(), ParentLayoutID: "layout-overtime-result", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "reason", Type: "textarea", Label: "加班事由", Placeholder: "請說明工作內容與必要性", Required: true},
		domain.PlatformFormBuilderField{ID: "attachment", Type: "file", Label: "附件", Placeholder: "可上傳工作安排或佐證資料"},
	)
}

// tenantPunchFixFormFields 建立補卡申請表單元件。
func tenantPunchFixFormFields() []domain.PlatformFormBuilderField {
	fields := tenantApplicantFields()
	return append(fields,
		domain.PlatformFormBuilderField{ID: "section-punch-fix", Type: "section-title", Label: "補卡內容"},
		domain.PlatformFormBuilderField{ID: "correction_type", Type: "radio", Label: "補登類型", Placeholder: "請選擇", Required: true, DefaultValue: "add_record", Options: []domain.PlatformFormBuilderFieldOption{{Label: "新增打卡", Value: "add_record"}, {Label: "更正打卡", Value: "replace_record"}, {Label: "作廢打卡", Value: "void_record"}}, Analytics: tenantDimensionAnalytics()},
		domain.PlatformFormBuilderField{ID: "layout-punch", Type: "layout", Label: "打卡資料", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "direction", Type: "radio", Label: "打卡方向", Placeholder: "請選擇", Required: true, DefaultValue: "clock_in", Options: []domain.PlatformFormBuilderFieldOption{{Label: "上班", Value: "clock_in"}, {Label: "下班", Value: "clock_out"}}, Analytics: tenantDimensionAnalytics(), ParentLayoutID: "layout-punch", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "requested_clocked_at", Type: "datetime", Label: "應打卡時間", Placeholder: "選擇日期時間", Required: true, ParentLayoutID: "layout-punch", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "target_clock_record_id", Type: "text", Label: "原打卡紀錄 ID", Placeholder: "更正或作廢時填寫"},
		domain.PlatformFormBuilderField{ID: "reason", Type: "textarea", Label: "補卡原因", Placeholder: "請說明漏打卡或異常原因", Required: true},
		domain.PlatformFormBuilderField{ID: "attachment", Type: "image", Label: "佐證圖片", Placeholder: "可上傳打卡或出勤佐證"},
	)
}

// tenantJobChangeFormFields 建立人事、職務與薪資異動表單元件。
func tenantJobChangeFormFields() []domain.PlatformFormBuilderField {
	fields := tenantApplicantFields()
	return append(fields,
		domain.PlatformFormBuilderField{ID: "section-change-target", Type: "section-title", Label: "異動對象"},
		domain.PlatformFormBuilderField{ID: "subject_employee_id", Type: "select", Label: "異動員工", Placeholder: "請選擇員工", Required: true, Binding: tenantFormBinding("employees", "id", "name")},
		domain.PlatformFormBuilderField{ID: "change_types", Type: "multilist", Label: "異動項目", Placeholder: "可複選", Required: true, Options: []domain.PlatformFormBuilderFieldOption{{Label: "部門調動", Value: "department"}, {Label: "職務異動", Value: "position"}, {Label: "主管異動", Value: "manager"}, {Label: "薪資調整", Value: "salary"}, {Label: "聘僱類型", Value: "employment_type"}}, Analytics: tenantDimensionAnalytics()},
		domain.PlatformFormBuilderField{ID: "effective_date", Type: "date", Label: "生效日期", Placeholder: "選擇日期", Required: true},
		domain.PlatformFormBuilderField{ID: "section-change-detail", Type: "section-title", Label: "異動內容"},
		domain.PlatformFormBuilderField{ID: "layout-change-org", Type: "layout", Label: "組織與職務", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "new_department_id", Type: "select", Label: "新部門", Placeholder: "如適用請選擇", Binding: tenantFormBinding("departments", "id", "name"), ParentLayoutID: "layout-change-org", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "new_position_id", Type: "select", Label: "新職務", Placeholder: "如適用請選擇", Binding: tenantFormBinding("positions", "id", "name"), ParentLayoutID: "layout-change-org", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "new_manager_id", Type: "select", Label: "新直屬主管", Placeholder: "如適用請選擇", Binding: tenantFormBinding("employees", "id", "name")},
		domain.PlatformFormBuilderField{ID: "layout-change-salary", Type: "layout", Label: "薪資異動", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "current_salary", Type: "number", Label: "目前月薪", Placeholder: "僅薪資調整時填寫", Security: tenantRestrictedSecurity(), ParentLayoutID: "layout-change-salary", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "proposed_salary", Type: "number", Label: "建議月薪", Placeholder: "僅薪資調整時填寫", Security: tenantRestrictedSecurity(), ParentLayoutID: "layout-change-salary", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "reason", Type: "textarea", Label: "異動原因", Placeholder: "請說明異動背景與依據", Required: true},
		domain.PlatformFormBuilderField{ID: "attachment", Type: "file", Label: "附件", Placeholder: "可上傳核薪或異動佐證"},
	)
}

// tenantHeadcountFormFields 建立人員增補申請表單元件。
func tenantHeadcountFormFields() []domain.PlatformFormBuilderField {
	fields := tenantApplicantFields()
	return append(fields,
		domain.PlatformFormBuilderField{ID: "section-headcount-role", Type: "section-title", Label: "職缺資料"},
		domain.PlatformFormBuilderField{ID: "layout-headcount-org", Type: "layout", Label: "部門與職務", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "request_department_id", Type: "select", Label: "需求部門", Placeholder: "請選擇部門", Required: true, Binding: tenantFormBinding("departments", "id", "name"), ParentLayoutID: "layout-headcount-org", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "position_id", Type: "select", Label: "既有職務", Placeholder: "如適用請選擇", Binding: tenantFormBinding("positions", "id", "name"), ParentLayoutID: "layout-headcount-org", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "position_title", Type: "text", Label: "職缺名稱", Placeholder: "請填寫對外招募職稱", Required: true},
		domain.PlatformFormBuilderField{ID: "layout-headcount-plan", Type: "layout", Label: "招募規劃", Placeholder: "分欄容器：1fr / 1fr / 1fr", LayoutColumns: []string{"1fr", "1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "employment_type", Type: "radio", Label: "聘僱類型", Placeholder: "請選擇", Required: true, DefaultValue: "full_time", Options: []domain.PlatformFormBuilderFieldOption{{Label: "正職", Value: "full_time"}, {Label: "約聘", Value: "contract"}, {Label: "實習", Value: "intern"}}, Analytics: tenantDimensionAnalytics(), ParentLayoutID: "layout-headcount-plan", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "openings", Type: "number", Label: "需求人數", Placeholder: "1", Required: true, DefaultValue: 1, Analytics: tenantMeasureAnalytics(), ParentLayoutID: "layout-headcount-plan", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "desired_start_date", Type: "date", Label: "期望到職日", Placeholder: "選擇日期", Required: true, ParentLayoutID: "layout-headcount-plan", SlotIndex: tenantFormSlot(2)},
		domain.PlatformFormBuilderField{ID: "job_level", Type: "select", Label: "職等", Placeholder: "請選擇", Options: []domain.PlatformFormBuilderFieldOption{{Label: "初階", Value: "junior"}, {Label: "中階", Value: "mid"}, {Label: "資深", Value: "senior"}, {Label: "主管", Value: "manager"}}, Analytics: tenantDimensionAnalytics()},
		domain.PlatformFormBuilderField{ID: "replacement", Type: "checkbox", Label: "此職缺為離職遞補", Placeholder: ""},
		domain.PlatformFormBuilderField{ID: "replaced_employee_id", Type: "select", Label: "被遞補員工", Placeholder: "遞補職缺時選擇", Binding: tenantFormBinding("employees", "id", "name")},
		domain.PlatformFormBuilderField{ID: "section-headcount-detail", Type: "section-title", Label: "招募說明"},
		domain.PlatformFormBuilderField{ID: "responsibilities", Type: "textarea", Label: "工作職責", Placeholder: "請列出主要工作內容", Required: true},
		domain.PlatformFormBuilderField{ID: "qualifications", Type: "textarea", Label: "資格條件", Placeholder: "請列出必要能力與經驗", Required: true},
		domain.PlatformFormBuilderField{ID: "business_reason", Type: "textarea", Label: "增補原因", Placeholder: "請說明人力缺口與業務影響", Required: true},
		domain.PlatformFormBuilderField{ID: "budget_range", Type: "text", Label: "預算範圍", Placeholder: "例：月薪 60,000–80,000", Security: tenantConfidentialSecurity()},
		domain.PlatformFormBuilderField{ID: "attachment", Type: "file", Label: "附件", Placeholder: "可上傳 JD 或人力規劃"},
	)
}

// tenantResignationFormFields 建立離職及退休申請表單元件。
func tenantResignationFormFields() []domain.PlatformFormBuilderField {
	fields := tenantApplicantFields()
	return append(fields,
		domain.PlatformFormBuilderField{ID: "section-separation", Type: "section-title", Label: "離職與退休資料"},
		domain.PlatformFormBuilderField{ID: "separation_type", Type: "radio", Label: "申請類型", Placeholder: "請選擇", Required: true, DefaultValue: "resignation", Options: []domain.PlatformFormBuilderFieldOption{{Label: "離職", Value: "resignation"}, {Label: "退休", Value: "retirement"}}, Analytics: tenantDimensionAnalytics()},
		domain.PlatformFormBuilderField{ID: "layout-separation-date", Type: "layout", Label: "日期", Placeholder: "分欄容器：1fr / 1fr", LayoutColumns: []string{"1fr", "1fr"}},
		domain.PlatformFormBuilderField{ID: "notice_date", Type: "date", Label: "提出日期", Placeholder: "選擇日期", Required: true, ParentLayoutID: "layout-separation-date", SlotIndex: tenantFormSlot(0)},
		domain.PlatformFormBuilderField{ID: "last_working_date", Type: "date", Label: "最後工作日", Placeholder: "選擇日期", Required: true, ParentLayoutID: "layout-separation-date", SlotIndex: tenantFormSlot(1)},
		domain.PlatformFormBuilderField{ID: "reason", Type: "textarea", Label: "離職或退休原因", Placeholder: "請說明原因", Required: true, Security: tenantConfidentialSecurity()},
		domain.PlatformFormBuilderField{ID: "section-handover", Type: "section-title", Label: "交接安排"},
		domain.PlatformFormBuilderField{ID: "handover_employee_id", Type: "select", Label: "交接人", Placeholder: "請選擇交接人", Required: true, Binding: tenantFormBinding("employees", "id", "name")},
		domain.PlatformFormBuilderField{ID: "handover_summary", Type: "textarea", Label: "交接事項", Placeholder: "請列出專案、文件與待辦事項", Required: true},
		domain.PlatformFormBuilderField{ID: "asset_return_confirmed", Type: "checkbox", Label: "已確認公司資產歸還安排", Placeholder: ""},
		domain.PlatformFormBuilderField{ID: "attachment", Type: "file", Label: "附件", Placeholder: "可上傳交接清單或相關文件"},
	)
}

// tenantManagerApprovalStages 建立直屬主管簽核流程。
func tenantManagerApprovalStages() []domain.PlatformFormBuilderStage {
	return []domain.PlatformFormBuilderStage{{ID: "stage-manager", Type: "approver", Label: "直屬主管", Detail: "依員工主管關係自動帶入", Config: map[string]any{"role": "manager"}}}
}

// tenantManagerHRApprovalStages 建立主管與 HR 的兩階段簽核流程。
func tenantManagerHRApprovalStages() []domain.PlatformFormBuilderStage {
	return []domain.PlatformFormBuilderStage{
		{ID: "stage-manager", Type: "approver", Label: "直屬主管", Detail: "依員工主管關係自動帶入", Config: map[string]any{"role": "manager"}},
		{ID: "stage-hr", Type: "approver", Label: "HR 複核", Detail: "由 HR 確認人事與出勤資料", Config: map[string]any{"role": "hr"}},
	}
}

// tenantHeadcountApprovalStages 建立部門主管、HR 與總經理的人力增補流程。
func tenantHeadcountApprovalStages() []domain.PlatformFormBuilderStage {
	return []domain.PlatformFormBuilderStage{
		{ID: "stage-dept-head", Type: "approver", Label: "部門主管", Detail: "確認人力需求與部門編制", Config: map[string]any{"role": "dept-head"}},
		{ID: "stage-hr", Type: "approver", Label: "HR 複核", Detail: "確認職缺與招募條件", Config: map[string]any{"role": "hr"}},
		{ID: "stage-ceo", Type: "approver", Label: "總經理核准", Detail: "核准新增編制", Config: map[string]any{"role": "ceo"}},
	}
}

// tenantFormBinding 建立受控資料來源綁定。
func tenantFormBinding(sourceID, valueField, labelField string) *domain.PlatformFormBuilderFieldBinding {
	return &domain.PlatformFormBuilderFieldBinding{SourceID: sourceID, ValueField: valueField, LabelField: labelField}
}

// tenantFormSlot 建立分欄位置指標。
func tenantFormSlot(value int) *int { return &value }

// tenantDimensionAnalytics 標記可分組與篩選的統計維度。
func tenantDimensionAnalytics() *domain.PlatformFormBuilderFieldAnalytics {
	return &domain.PlatformFormBuilderFieldAnalytics{Reportable: true, Role: "dimension", Aggregations: []string{"count"}, Filterable: true, Groupable: true}
}

// tenantMeasureAnalytics 標記可加總與平均的統計指標。
func tenantMeasureAnalytics() *domain.PlatformFormBuilderFieldAnalytics {
	return &domain.PlatformFormBuilderFieldAnalytics{Reportable: true, Role: "measure", Aggregations: []string{"sum", "avg"}, Filterable: true}
}

// tenantRestrictedSecurity 標記不可供 Agent 讀取的受限薪資欄位。
func tenantRestrictedSecurity() *domain.PlatformFormBuilderFieldSecurity {
	return &domain.PlatformFormBuilderFieldSecurity{Classification: "restricted", Masking: "full", AgentAccess: false}
}

// tenantConfidentialSecurity 標記需遮罩的機密人事欄位。
func tenantConfidentialSecurity() *domain.PlatformFormBuilderFieldSecurity {
	return &domain.PlatformFormBuilderFieldSecurity{Classification: "confidential", Masking: "partial", AgentAccess: false}
}

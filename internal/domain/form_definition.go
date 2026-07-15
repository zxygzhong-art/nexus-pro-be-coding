package domain

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const FormDefinitionSchemaVersion2 = 2

// FormDefinitionDraftStatus 描述表單定義草稿的受控生命週期。
type FormDefinitionDraftStatus string

const (
	FormDefinitionDraftStatusDraft         FormDefinitionDraftStatus = "draft"
	FormDefinitionDraftStatusReviewPending FormDefinitionDraftStatus = "review_pending"
	FormDefinitionDraftStatusRejected      FormDefinitionDraftStatus = "rejected"
	FormDefinitionDraftStatusPublished     FormDefinitionDraftStatus = "published"
)

// FormDefinitionDraft 保存 Agent 或管理員產生的 authoring schema 與編譯快照。
type FormDefinitionDraft struct {
	ID                  string                    `json:"id"`
	TenantID            string                    `json:"tenant_id"`
	OwnerAccountID      string                    `json:"owner_account_id"`
	BaseTemplateID      string                    `json:"base_template_id,omitempty"`
	SchemaVersion       int                       `json:"schema_version"`
	AuthoringSchema     FormDefinitionSchemaV2    `json:"authoring_schema"`
	CompiledSchema      map[string]any            `json:"compiled_schema,omitempty"`
	Status              FormDefinitionDraftStatus `json:"status"`
	Revision            int64                     `json:"revision"`
	Source              string                    `json:"source"`
	AgentID             string                    `json:"agent_id,omitempty"`
	AgentRunID          string                    `json:"agent_run_id,omitempty"`
	AgentSessionID      string                    `json:"agent_session_id,omitempty"`
	ToolCallID          string                    `json:"tool_call_id,omitempty"`
	ValidationResult    FormDefinitionValidation  `json:"validation_result"`
	SubmittedAt         *time.Time                `json:"submitted_at,omitempty"`
	PublishedTemplateID string                    `json:"published_template_id,omitempty"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

// FormDefinitionSchemaV2 是低代碼表單的穩定 authoring contract；它不直接等於 runtime schema。
type FormDefinitionSchemaV2 struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description,omitempty"`
	Category      string                  `json:"category,omitempty"`
	Fields        []FormFieldDefinitionV2 `json:"fields"`
	Layout        FormLayoutV2            `json:"layout"`
	Workflow      FormWorkflowV2          `json:"workflow"`
}

// FormFieldDefinitionV2 描述一個與 runtime widget 解耦的字段。
type FormFieldDefinitionV2 struct {
	ID           string                `json:"id"`
	Label        string                `json:"label"`
	Description  string                `json:"description,omitempty"`
	DataType     string                `json:"dataType"`
	Widget       string                `json:"widget"`
	Required     bool                  `json:"required,omitempty"`
	Placeholder  string                `json:"placeholder,omitempty"`
	DefaultValue any                   `json:"defaultValue,omitempty"`
	Options      []FormFieldOptionV2   `json:"options,omitempty"`
	Binding      *FormFieldBindingV2   `json:"binding,omitempty"`
	Validation   FormFieldValidationV2 `json:"validation,omitempty"`
	Analytics    FormFieldAnalyticsV2  `json:"analytics,omitempty"`
	Security     FormFieldSecurityV2   `json:"security,omitempty"`
}

// FormFieldOptionV2 描述靜態選項。
type FormFieldOptionV2 struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// FormFieldBindingV2 描述受控數據源綁定，僅允許 capability catalog 中的字段。
type FormFieldBindingV2 struct {
	SourceID   string `json:"sourceId"`
	ValueField string `json:"valueField"`
	LabelField string `json:"labelField,omitempty"`
}

// FormFieldValidationV2 描述聲明式校驗規則；MVP 不執行任意表達式。
type FormFieldValidationV2 struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
}

// FormFieldAnalyticsV2 描述可報表化的字段語意。
type FormFieldAnalyticsV2 struct {
	Reportable   bool     `json:"reportable,omitempty"`
	Role         string   `json:"role,omitempty"`
	Aggregations []string `json:"aggregations,omitempty"`
	Filterable   bool     `json:"filterable,omitempty"`
	Groupable    bool     `json:"groupable,omitempty"`
}

// FormFieldSecurityV2 描述字段敏感度與 Agent 可見性。
type FormFieldSecurityV2 struct {
	Classification string `json:"classification,omitempty"`
	Masking        string `json:"masking,omitempty"`
	AgentAccess    bool   `json:"agentAccess,omitempty"`
}

// FormLayoutV2 保存佈局順序，不讓 Agent 直接拼接 HTML/CSS。
type FormLayoutV2 struct {
	Rows []FormLayoutRowV2 `json:"rows"`
}

// FormLayoutRowV2 描述一行中的字段順序。
type FormLayoutRowV2 struct {
	ID       string   `json:"id,omitempty"`
	FieldIDs []string `json:"fieldIds"`
}

// FormWorkflowV2 描述有限狀態審批流程。
type FormWorkflowV2 struct {
	Stages []FormWorkflowStageV2 `json:"stages"`
}

// FormWorkflowStageV2 描述一個受控流程節點。
type FormWorkflowStageV2 struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Label  string         `json:"label"`
	Detail string         `json:"detail,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

// FormDefinitionValidation 是可持久化、可被 Agent 讀取的確定性驗證結果。
type FormDefinitionValidation struct {
	Valid  bool         `json:"valid"`
	Errors []FieldError `json:"errors,omitempty"`
}

// FormBuilderDataSourceMetadata 是不含業務記錄的 Agent-safe 數據源能力描述。
type FormBuilderDataSourceMetadata struct {
	ID     string                `json:"id"`
	Label  string                `json:"label"`
	Kind   string                `json:"kind"`
	Fields []FormDataSourceField `json:"fields"`
}

// FormBuilderWorkflowTarget 描述可配置的審批目標角色。
type FormBuilderWorkflowTarget struct {
	Role        string `json:"role"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// FormBuilderCapabilitiesResponse 是表單 Agent 創作所需的最小能力目錄。
type FormBuilderCapabilitiesResponse struct {
	SchemaVersion   int                             `json:"schema_version"`
	FieldTypes      []string                        `json:"field_types"`
	Widgets         []string                        `json:"widgets"`
	DataSources     []FormBuilderDataSourceMetadata `json:"data_sources"`
	WorkflowTargets []FormBuilderWorkflowTarget     `json:"workflow_targets"`
}

// CreateFormDefinitionDraftInput 建立受控表單定義草稿。
type CreateFormDefinitionDraftInput struct {
	BaseTemplateID string                 `json:"base_template_id,omitempty"`
	Schema         FormDefinitionSchemaV2 `json:"schema"`
	Source         string                 `json:"source,omitempty"`
	AgentID        string                 `json:"agent_id,omitempty"`
	AgentRunID     string                 `json:"agent_run_id,omitempty"`
	AgentSessionID string                 `json:"agent_session_id,omitempty"`
	ToolCallID     string                 `json:"tool_call_id,omitempty"`
}

// UpdateFormDefinitionDraftInput 更新草稿並要求調用方攜帶當前 revision。
type UpdateFormDefinitionDraftInput struct {
	Revision       int64                  `json:"revision"`
	Schema         FormDefinitionSchemaV2 `json:"schema"`
	Source         string                 `json:"source,omitempty"`
	AgentRunID     string                 `json:"agent_run_id,omitempty"`
	AgentSessionID string                 `json:"agent_session_id,omitempty"`
	ToolCallID     string                 `json:"tool_call_id,omitempty"`
}

// FormDefinitionPreview 返回校驗、編譯結果與有限流程模擬結果。
type FormDefinitionPreview struct {
	Draft          FormDefinitionDraft      `json:"draft"`
	Validation     FormDefinitionValidation `json:"validation"`
	CompiledSchema map[string]any           `json:"compiled_schema,omitempty"`
}

// FormWorkflowSimulation 是不寫入運行時流程的靜態模擬結果。
type FormWorkflowSimulation struct {
	Stages []FormWorkflowSimulationStage `json:"stages"`
}

// FormWorkflowSimulationStage 描述模擬時的審批角色與順序。
type FormWorkflowSimulationStage struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	TargetRoles []string `json:"target_roles,omitempty"`
}

var formDefinitionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var formDefinitionBindingFields = map[string]map[string]struct{}{
	"current_user": {"display_name": {}, "email": {}, "employee_id": {}, "employee_no": {}, "department_id": {}, "department_name": {}, "position_id": {}, "position_name": {}},
	"departments":  {"id": {}, "name": {}, "code": {}},
	"employees":    {"id": {}, "name": {}, "employee_no": {}, "email": {}, "department_id": {}, "department_name": {}, "position_id": {}, "position_name": {}},
	"positions":    {"id": {}, "name": {}, "code": {}, "department_id": {}},
	"leave_types":  {"code": {}, "name": {}, "unit": {}},
}

// ValidateFormDefinitionSchemaV2 執行不依賴數據庫狀態的結構校驗。
func ValidateFormDefinitionSchemaV2(schema FormDefinitionSchemaV2) FormDefinitionValidation {
	result := FormDefinitionValidation{Valid: true}
	add := func(field, code, message string) {
		result.Valid = false
		result.Errors = append(result.Errors, FieldError{Field: field, Code: code, Message: message})
	}
	if schema.SchemaVersion != FormDefinitionSchemaVersion2 {
		add("schemaVersion", "unsupported_version", "schemaVersion must be 2")
	}
	if strings.TrimSpace(schema.Name) == "" {
		add("name", "required", "form name is required")
	}
	fieldIDs := make(map[string]struct{}, len(schema.Fields))
	for index, field := range schema.Fields {
		path := "fields[" + itoa(index) + "]"
		id := strings.TrimSpace(field.ID)
		if !formDefinitionIDPattern.MatchString(id) {
			add(path+".id", "invalid_id", "field id must start with a lowercase letter and contain only lowercase letters, numbers, _ or -")
		}
		if _, exists := fieldIDs[id]; exists {
			add(path+".id", "duplicate", "field id must be unique")
		}
		fieldIDs[id] = struct{}{}
		if strings.TrimSpace(field.Label) == "" {
			add(path+".label", "required", "field label is required")
		}
		if !validFormDataType(field.DataType) {
			add(path+".dataType", "unsupported", "unsupported field data type")
		}
		if !validFormWidget(field.Widget) {
			add(path+".widget", "unsupported", "unsupported field widget")
		}
		if field.Validation.Pattern != "" {
			if _, err := regexp.Compile(field.Validation.Pattern); err != nil {
				add(path+".validation.pattern", "invalid_pattern", "validation pattern is invalid")
			}
		}
		if field.Validation.MinLength != nil && field.Validation.MaxLength != nil && *field.Validation.MinLength > *field.Validation.MaxLength {
			add(path+".validation", "invalid_range", "minLength cannot exceed maxLength")
		}
		if field.Validation.Min != nil && field.Validation.Max != nil && *field.Validation.Min > *field.Validation.Max {
			add(path+".validation", "invalid_range", "min cannot exceed max")
		}
		if isOptionWidget(field.Widget) && len(field.Options) == 0 && field.Binding == nil {
			add(path+".options", "required", "option widgets require options or a data source binding")
		}
		if field.Binding != nil {
			validateFormDefinitionBinding(add, path+".binding", field)
		}
	}
	seenLayout := make(map[string]struct{}, len(fieldIDs))
	for rowIndex, row := range schema.Layout.Rows {
		if len(row.FieldIDs) == 0 {
			add("layout.rows["+itoa(rowIndex)+"].fieldIds", "required", "layout row cannot be empty")
			continue
		}
		for _, fieldID := range row.FieldIDs {
			if _, ok := fieldIDs[fieldID]; !ok {
				add("layout.rows["+itoa(rowIndex)+"].fieldIds", "unknown_field", "layout references an unknown field")
			}
			if _, ok := seenLayout[fieldID]; ok {
				add("layout.rows["+itoa(rowIndex)+"].fieldIds", "duplicate_field", "a field can appear in layout only once")
			}
			seenLayout[fieldID] = struct{}{}
		}
	}
	for fieldID := range fieldIDs {
		if _, ok := seenLayout[fieldID]; !ok {
			add("layout", "missing_field", "every field must appear in layout: "+fieldID)
		}
	}
	stageIDs := map[string]struct{}{}
	for index, stage := range schema.Workflow.Stages {
		path := "workflow.stages[" + itoa(index) + "]"
		if !formDefinitionIDPattern.MatchString(strings.TrimSpace(stage.ID)) {
			add(path+".id", "invalid_id", "stage id is invalid")
		}
		if _, ok := stageIDs[stage.ID]; ok {
			add(path+".id", "duplicate", "stage id must be unique")
		}
		stageIDs[stage.ID] = struct{}{}
		if !validWorkflowStageType(stage.Type) {
			add(path+".type", "unsupported", "unsupported workflow stage type")
		}
		if strings.TrimSpace(stage.Label) == "" {
			add(path+".label", "required", "stage label is required")
		}
		if stage.Type == "approver" && strings.TrimSpace(stringFromAny(stage.Config["role"])) == "" && len(stringSliceFromAny(stage.Config["account_ids"])) == 0 && len(stringSliceFromAny(stage.Config["user_group_ids"])) == 0 {
			add(path+".config", "approver_target_required", "approver stage requires role, account_ids, or user_group_ids")
		}
	}
	if len(schema.Workflow.Stages) == 0 {
		add("workflow.stages", "required", "at least one workflow stage is required")
	}
	sort.SliceStable(result.Errors, func(i, j int) bool { return result.Errors[i].Field < result.Errors[j].Field })
	return result
}

// validateFormDefinitionBinding 限制 Agent 只能引用 capability catalog 暴露的資料源字段。
func validateFormDefinitionBinding(add func(string, string, string), path string, field FormFieldDefinitionV2) {
	sourceID := strings.TrimSpace(field.Binding.SourceID)
	allowedFields, sourceOK := formDefinitionBindingFields[sourceID]
	if !sourceOK {
		add(path+".sourceId", "unsupported_source", "unsupported form data source")
		return
	}
	if _, ok := allowedFields[strings.TrimSpace(field.Binding.ValueField)]; !ok {
		add(path+".valueField", "unsupported_field", "unsupported data source value field")
	}
	if sourceID == "current_user" {
		if field.Widget != "autofill" && field.Widget != "readonly" {
			add(path, "invalid_widget", "current_user bindings require an autofill or readonly field")
		}
		return
	}
	if _, ok := allowedFields[strings.TrimSpace(field.Binding.LabelField)]; !ok {
		add(path+".labelField", "unsupported_field", "collection binding requires a valid label field")
	}
	if !isOptionWidget(field.Widget) {
		add(path, "invalid_widget", "collection bindings require an option widget")
	}
}

// CompileFormDefinitionSchemaV2 轉換為現有 runtime 所需的 workspace_design schema。
func CompileFormDefinitionSchemaV2(schema FormDefinitionSchemaV2) (map[string]any, FormDefinitionValidation) {
	validation := ValidateFormDefinitionSchemaV2(schema)
	if !validation.Valid {
		return nil, validation
	}
	fields := make([]map[string]any, 0, len(schema.Fields)+len(schema.Layout.Rows))
	for rowIndex, row := range schema.Layout.Rows {
		layoutID := row.ID
		if layoutID == "" {
			layoutID = "layout-" + itoa(rowIndex+1)
		}
		fields = append(fields, map[string]any{"id": layoutID, "type": "layout", "label": "row", "layout_columns": repeatColumns(len(row.FieldIDs))})
		for slot, fieldID := range row.FieldIDs {
			for _, field := range schema.Fields {
				if field.ID != fieldID {
					continue
				}
				compiled := map[string]any{"id": field.ID, "type": runtimeWidget(field.DataType, field.Widget), "label": field.Label, "placeholder": field.Placeholder, "required": field.Required, "default_value": field.DefaultValue, "parent_layout_id": layoutID, "slot_index": slot}
				if len(field.Options) > 0 {
					options := make([]map[string]any, 0, len(field.Options))
					for _, option := range field.Options {
						options = append(options, map[string]any{"label": option.Label, "value": option.Value})
					}
					compiled["options"] = options
				}
				if field.Binding != nil {
					compiled["binding"] = map[string]any{"source_id": field.Binding.SourceID, "value_field": field.Binding.ValueField, "label_field": field.Binding.LabelField}
				}
				if field.Analytics.Reportable || field.Analytics.Filterable || field.Analytics.Groupable || field.Analytics.Role != "" {
					compiled["analytics"] = field.Analytics
				}
				if field.Security.Classification != "" || field.Security.Masking != "" || field.Security.AgentAccess {
					compiled["security"] = field.Security
				}
				fields = append(fields, compiled)
			}
		}
	}
	stages := make([]map[string]any, 0, len(schema.Workflow.Stages))
	for _, stage := range schema.Workflow.Stages {
		stages = append(stages, map[string]any{"id": stage.ID, "type": stage.Type, "label": stage.Label, "detail": stage.Detail, "config": stage.Config})
	}
	return map[string]any{"type": "object", "schema_version": FormDefinitionSchemaVersion2, "workspace_design": map[string]any{"category": schema.Category, "desc": schema.Description, "enabled": true, "deleted": false, "form_kind": "custom", "fields": fields, "stages": stages}, "flow": "custom"}, validation
}

func validFormDataType(value string) bool {
	switch strings.TrimSpace(value) {
	case "string", "number", "boolean", "date", "datetime", "string_array", "object":
		return true
	}
	return false
}
func validFormWidget(value string) bool {
	switch strings.TrimSpace(value) {
	case "input", "textarea", "number", "checkbox", "date", "datetime", "select", "radio", "multilist", "autofill", "readonly":
		return true
	}
	return false
}
func isOptionWidget(value string) bool {
	switch strings.TrimSpace(value) {
	case "select", "radio", "multilist":
		return true
	}
	return false
}
func validWorkflowStageType(value string) bool {
	switch strings.TrimSpace(value) {
	case "approver", "condition", "parallel", "notify":
		return true
	}
	return false
}
func runtimeWidget(dataType, widget string) string {
	if widget == "input" && dataType == "string" {
		return "text"
	}
	if widget == "readonly" {
		return "autofill"
	}
	return widget
}
func repeatColumns(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "1fr"
	}
	return out
}
func itoa(value int) string          { return strconv.Itoa(value) }
func stringFromAny(value any) string { s, _ := value.(string); return strings.TrimSpace(s) }
func stringSliceFromAny(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		if typed, ok2 := value.([]string); ok2 {
			return typed
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s := stringFromAny(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

package service

import (
	"fmt"
	"strconv"
	"strings"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

var allowedWorkspaceFormStageTypes = map[string]struct{}{
	"approver":  {},
	"condition": {},
	"parallel":  {},
	"notify":    {},
}

var allowedWorkspaceFormRoles = map[string]struct{}{
	"applicant": {},
	"manager":   {},
	"relative":  {},
	"dept-head": {},
	"hr":        {},
	"finance":   {},
	"ceo":       {},
}

var allowedFormAnalyticsRoles = map[string]struct{}{"dimension": {}, "measure": {}}
var allowedFormAggregations = map[string]struct{}{"count": {}, "sum": {}, "avg": {}, "min": {}, "max": {}}
var allowedFormSecurityClassifications = map[string]struct{}{"public": {}, "internal": {}, "confidential": {}, "restricted": {}}
var allowedFormSecurityMasking = map[string]struct{}{"none": {}, "partial": {}, "full": {}}

// validateWorkspaceFormDesignInput 驗證自訂表單設計的欄位與流程節點。
func validateWorkspaceFormDesignInput(fields []domain.PlatformFormBuilderField, stages []domain.PlatformFormBuilderStage) error {
	fieldErrors := make([]domain.FieldError, 0)
	seenFieldIDs := map[string]struct{}{}
	for i, field := range fields {
		id := strings.TrimSpace(field.ID)
		if id == "" {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   fmt.Sprintf("fields[%d].id", i),
				Code:    "required",
				Message: "field id is required",
			})
			continue
		}
		if _, exists := seenFieldIDs[id]; exists {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   "fields." + id,
				Code:    "duplicate",
				Message: "field id must be unique",
			})
		}
		seenFieldIDs[id] = struct{}{}
		if strings.TrimSpace(field.Type) == "" {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   "fields." + id + ".type",
				Code:    "required",
				Message: "field type is required",
			})
		}
		if strings.TrimSpace(field.Label) == "" {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   "fields." + id + ".label",
				Code:    "required",
				Message: "field label is required",
			})
		}
		if binding := field.Binding; binding != nil {
			fieldErrors = append(fieldErrors, ValidateFormFieldBinding(id, field.Type, *binding)...)
		}
		fieldErrors = append(fieldErrors, ValidateFormFieldAnalyticsAndSecurity(id, field)...)
	}

	if len(stages) == 0 {
		fieldErrors = append(fieldErrors, domain.FieldError{
			Field:   "stages",
			Code:    "required",
			Message: "at least one workflow stage is required",
		})
	}

	seenStageIDs := map[string]struct{}{}
	hasApproverStage := false
	for i, stage := range stages {
		id := strings.TrimSpace(stage.ID)
		stageType := strings.TrimSpace(stage.Type)
		prefix := fmt.Sprintf("stages[%d]", i)
		if id == "" {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   prefix + ".id",
				Code:    "required",
				Message: "stage id is required",
			})
		} else {
			prefix = "stages." + id
			if _, exists := seenStageIDs[id]; exists {
				fieldErrors = append(fieldErrors, domain.FieldError{
					Field:   prefix,
					Code:    "duplicate",
					Message: "stage id must be unique",
				})
			}
			seenStageIDs[id] = struct{}{}
		}
		if stageType == "" {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   prefix + ".type",
				Code:    "required",
				Message: "stage type is required",
			})
			continue
		}
		if _, ok := allowedWorkspaceFormStageTypes[stageType]; !ok {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   prefix + ".type",
				Code:    "invalid",
				Message: "stage type must be one of approver, condition, parallel, notify",
			})
			continue
		}
		config := workflowStageConfigFromMap(stage.Config)
		switch stageType {
		case "approver", "parallel":
			hasApproverStage = true
			fallthrough
		case "notify":
			if len(config.AccountIDs) == 0 && len(config.UserGroupIDs) == 0 && strings.TrimSpace(config.Role) == "" {
				fieldErrors = append(fieldErrors, domain.FieldError{
					Field:   prefix + ".config",
					Code:    "required",
					Message: "stage config must include role, account_ids, or user_group_ids",
				})
			}
			if role := strings.TrimSpace(config.Role); role != "" {
				if _, ok := allowedWorkspaceFormRoles[role]; !ok {
					fieldErrors = append(fieldErrors, domain.FieldError{
						Field:   prefix + ".config.role",
						Code:    "invalid",
						Message: "unsupported workflow role",
					})
				}
			}
		case "condition":
			field := strings.TrimSpace(config.Field)
			value := strings.TrimSpace(config.Value)
			if field == "" || strings.TrimSpace(config.Operator) == "" || value == "" {
				fieldErrors = append(fieldErrors, domain.FieldError{
					Field:   prefix + ".config",
					Code:    "required",
					Message: "condition stage requires field, operator, and value",
				})
			} else if field != "level" {
				// Runtime evaluates every non-level condition field numerically;
				// reject values that would otherwise degrade to a silent zero.
				if _, err := strconv.ParseFloat(value, 64); err != nil {
					fieldErrors = append(fieldErrors, domain.FieldError{
						Field:   prefix + ".config.value",
						Code:    "invalid",
						Message: "condition value must be a number",
					})
				}
			}
		}
	}
	if len(stages) > 0 && !hasApproverStage {
		fieldErrors = append(fieldErrors, domain.FieldError{
			Field:   "stages",
			Code:    "invalid",
			Message: "workflow must include at least one approver or parallel stage",
		})
	}

	if len(fieldErrors) > 0 {
		return ValidationFailed("form design validation failed", fieldErrors)
	}
	return nil
}

// ValidatePublishedFormFieldIdentity prevents published field IDs and types from being removed or changed.
func ValidatePublishedFormFieldIdentity(previous, next []domain.PlatformFormBuilderField) error {
	nextByID := make(map[string]domain.PlatformFormBuilderField, len(next))
	for _, field := range next {
		nextByID[strings.TrimSpace(field.ID)] = field
	}
	errors := make([]domain.FieldError, 0)
	for _, field := range previous {
		id := strings.TrimSpace(field.ID)
		nextField, exists := nextByID[id]
		if !exists {
			errors = append(errors, domain.FieldError{Field: "fields." + id + ".id", Code: "locked", Message: "published field id cannot be removed or changed"})
			continue
		}
		if strings.TrimSpace(nextField.Type) != strings.TrimSpace(field.Type) {
			errors = append(errors, domain.FieldError{Field: "fields." + id + ".type", Code: "locked", Message: "published field type cannot be changed"})
		}
	}
	if len(errors) > 0 {
		return ValidationFailed("published form field identity is immutable", errors)
	}
	return nil
}

// ValidateFormFieldAnalyticsAndSecurity validates reportability, aggregation, and field-security settings.
func ValidateFormFieldAnalyticsAndSecurity(fieldID string, field domain.PlatformFormBuilderField) []domain.FieldError {
	errors := make([]domain.FieldError, 0)
	if analytics := field.Analytics; analytics != nil {
		role := strings.TrimSpace(analytics.Role)
		if analytics.Reportable && role == "" {
			errors = append(errors, domain.FieldError{Field: "fields." + fieldID + ".analytics.role", Code: "required", Message: "reportable field must declare dimension or measure role"})
		} else if role != "" {
			if _, ok := allowedFormAnalyticsRoles[role]; !ok {
				errors = append(errors, domain.FieldError{Field: "fields." + fieldID + ".analytics.role", Code: "invalid", Message: "analytics role must be dimension or measure"})
			}
		}
		for _, aggregation := range analytics.Aggregations {
			aggregation = strings.TrimSpace(aggregation)
			if _, ok := allowedFormAggregations[aggregation]; !ok {
				errors = append(errors, domain.FieldError{Field: "fields." + fieldID + ".analytics.aggregations", Code: "invalid", Message: "unsupported analytics aggregation"})
				continue
			}
			if (aggregation == "sum" || aggregation == "avg") && strings.TrimSpace(field.Type) != "number" {
				errors = append(errors, domain.FieldError{Field: "fields." + fieldID + ".analytics.aggregations", Code: "invalid", Message: "sum and avg require a number field"})
			}
		}
	}
	if security := field.Security; security != nil {
		classification := strings.TrimSpace(security.Classification)
		if classification != "" {
			if _, ok := allowedFormSecurityClassifications[classification]; !ok {
				errors = append(errors, domain.FieldError{Field: "fields." + fieldID + ".security.classification", Code: "invalid", Message: "unsupported security classification"})
			}
		}
		masking := strings.TrimSpace(security.Masking)
		if masking != "" {
			if _, ok := allowedFormSecurityMasking[masking]; !ok {
				errors = append(errors, domain.FieldError{Field: "fields." + fieldID + ".security.masking", Code: "invalid", Message: "unsupported masking mode"})
			}
		}
	}
	return errors
}

// validateSystemFormFieldLocks 確保系統/半系統表單的核心欄位 ID 不被刪除。
func validateSystemFormFieldLocks(templateKey, formKind string, fields []domain.PlatformFormBuilderField) error {
	locked := lockedFieldIDsForTemplate(templateKey, formKind)
	if len(locked) == 0 {
		return nil
	}
	present := map[string]struct{}{}
	for _, field := range fields {
		id := strings.TrimSpace(field.ID)
		if id == "" {
			continue
		}
		present[id] = struct{}{}
	}
	fieldErrors := make([]domain.FieldError, 0)
	for id := range locked {
		if _, ok := present[id]; !ok {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   "fields." + id,
				Code:    "locked",
				Message: "system/hybrid core field cannot be removed: " + id,
			})
		}
	}
	if len(fieldErrors) > 0 {
		return ValidationFailed("system form field lock validation failed", fieldErrors)
	}
	return nil
}

// validateFormSubmissionPayload strips server metadata before validating the frozen template contract.
func (c WorkflowService) validateFormSubmissionPayload(ctx RequestContext, template domain.FormTemplate, payload map[string]any) (map[string]any, error) {
	sanitized := workflowPayloadForNewInstance(template, workflowPayload(payload))
	normalized, err := c.normalizeLeaveSubmissionHours(ctx, template.Key, sanitized)
	if err != nil {
		return nil, err
	}
	fields, hasExplicitFields := platformExplicitTemplateFields(template.Schema)
	if !hasExplicitFields {
		return normalized, nil
	}
	var catalog domain.FormDataSourceCatalogResponse
	if formFieldsHaveBindings(fields) {
		var err error
		catalog, err = c.loadFormDataSources(ctx)
		if err != nil {
			return nil, err
		}
	}
	fieldErrors := make([]domain.FieldError, 0)
	for _, field := range fields {
		id := strings.TrimSpace(field.ID)
		if id == "" || strings.EqualFold(strings.TrimSpace(field.Type), "layout") {
			continue
		}
		value, exists := normalized[id]
		if field.Binding != nil {
			boundValue, boundExists, bindingError := ValidateAndResolveBoundSubmissionValue(catalog, *field.Binding, value, exists)
			if bindingError != "" {
				fieldErrors = append(fieldErrors, domain.FieldError{Field: id, Code: "invalid_binding_value", Message: bindingError})
				continue
			}
			if boundExists {
				normalized[id] = boundValue
				value, exists = boundValue, true
			}
		}
		if !exists || isEmptyFormPayloadValue(value) {
			if field.Required {
				fieldErrors = append(fieldErrors, domain.FieldError{
					Field:   id,
					Code:    "required",
					Message: utils.FirstNonEmpty(strings.TrimSpace(field.Label), id) + " is required",
				})
			}
			continue
		}
		if errCode, message := validateFormPayloadFieldType(field, value); errCode != "" {
			fieldErrors = append(fieldErrors, domain.FieldError{
				Field:   id,
				Code:    errCode,
				Message: message,
			})
		}
	}
	if len(fieldErrors) > 0 {
		return nil, ValidationFailed("form submission validation failed", fieldErrors)
	}
	return normalized, nil
}

// normalizeLeaveSubmissionHours makes attendance policy the server-side source of truth for leave hours.
func (c WorkflowService) normalizeLeaveSubmissionHours(ctx RequestContext, templateKey string, payload map[string]any) (map[string]any, error) {
	if _, linked := leaveLinkedTemplateKeys[strings.TrimSpace(templateKey)]; !linked || !workflowLeavePayloadHasLinkedFields(payload) {
		return payload, nil
	}

	startRaw := utils.FirstNonEmpty(stringFromAny(payload["start_at"]), stringFromAny(payload["startAt"]))
	endRaw := utils.FirstNonEmpty(stringFromAny(payload["end_at"]), stringFromAny(payload["endAt"]))
	fieldErrors := make([]domain.FieldError, 0, 2)
	if startRaw == "" {
		fieldErrors = append(fieldErrors, domain.FieldError{Field: "start_at", Code: "required", Message: "start time is required"})
	}
	if endRaw == "" {
		fieldErrors = append(fieldErrors, domain.FieldError{Field: "end_at", Code: "required", Message: "end time is required"})
	}
	if len(fieldErrors) > 0 {
		return nil, ValidationFailed("leave time validation failed", fieldErrors)
	}

	startAt, startErr := utils.ParseDateTime(startRaw)
	endAt, endErr := utils.ParseDateTime(endRaw)
	if startErr != nil {
		fieldErrors = append(fieldErrors, domain.FieldError{Field: "start_at", Code: "invalid", Message: "start time must be RFC3339"})
	}
	if endErr != nil {
		fieldErrors = append(fieldErrors, domain.FieldError{Field: "end_at", Code: "invalid", Message: "end time must be RFC3339"})
	}
	if len(fieldErrors) > 0 {
		return nil, ValidationFailed("leave time validation failed", fieldErrors)
	}
	if !endAt.After(startAt) {
		return nil, ValidationFailed("leave time validation failed", []domain.FieldError{{
			Field: "end_at", Code: "invalid_range", Message: "end time must be after start time",
		}})
	}

	policy, err := c.Service.Attendance().loadAttendancePolicyResponse(ctx)
	if err != nil {
		return nil, err
	}
	hours := CalculateLeaveHoursWithinPolicy(startAt, endAt, policy.WorkTime)
	if hours <= 0 {
		return nil, ValidationFailed("leave time validation failed", []domain.FieldError{{
			Field: "hours", Code: "outside_work_time", Message: "selected time does not include working hours",
		}})
	}
	payload["hours"] = hours
	return payload, nil
}

// formFieldsHaveBindings 避免沒有資料綁定的舊表單產生額外查詢。
func formFieldsHaveBindings(fields []domain.PlatformFormBuilderField) bool {
	for _, field := range fields {
		if field.Binding != nil {
			return true
		}
	}
	return false
}

// ValidateAndResolveBoundSubmissionValue resolves server-owned values and rejects unknown collection selections.
func ValidateAndResolveBoundSubmissionValue(catalog domain.FormDataSourceCatalogResponse, binding domain.PlatformFormBuilderFieldBinding, value any, exists bool) (any, bool, string) {
	source, ok := formDataSourceByID(catalog, strings.TrimSpace(binding.SourceID))
	if !ok {
		return nil, false, "bound data source is unavailable"
	}
	valueField := strings.TrimSpace(binding.ValueField)
	if source.Kind == "object" {
		if len(source.Records) == 0 {
			return nil, false, "bound data source has no current record"
		}
		resolved, ok := source.Records[0][valueField]
		if !ok {
			return nil, false, "bound field is unavailable"
		}
		return resolved, true, ""
	}
	if !exists || isEmptyFormPayloadValue(value) {
		return value, exists, ""
	}
	allowed := make(map[string]struct{}, len(source.Records))
	for _, record := range source.Records {
		allowed[dataSourceString(record[valueField])] = struct{}{}
	}
	values := []string{dataSourceString(value)}
	switch typed := value.(type) {
	case []string:
		values = typed
	case []any:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, dataSourceString(item))
		}
	}
	for _, item := range values {
		if _, ok := allowed[item]; !ok {
			return nil, false, "selected value is not present in the bound data source"
		}
	}
	return value, true, ""
}

func isEmptyFormPayloadValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	default:
		return false
	}
}

func validateFormPayloadFieldType(field domain.PlatformFormBuilderField, value any) (code, message string) {
	label := utils.FirstNonEmpty(strings.TrimSpace(field.Label), strings.TrimSpace(field.ID))
	switch strings.TrimSpace(field.Type) {
	case "number":
		switch typed := value.(type) {
		case float64, float32, int, int64, int32:
			return "", ""
		case string:
			if strings.TrimSpace(typed) == "" {
				return "invalid", label + " must be a number"
			}
			if _, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err != nil {
				return "invalid", label + " must be a number"
			}
			return "", ""
		default:
			return "invalid", label + " must be a number"
		}
	case "checkbox":
		if _, ok := value.(bool); !ok {
			return "invalid", label + " must be a boolean"
		}
	case "multilist":
		switch value.(type) {
		case []any, []string:
			return "", ""
		default:
			return "invalid", label + " must be an array"
		}
	}
	return "", ""
}

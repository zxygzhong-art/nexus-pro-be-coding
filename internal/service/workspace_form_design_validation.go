package service

import (
	"fmt"
	"strconv"
	"strings"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
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
			if len(config.AccountIDs) == 0 && strings.TrimSpace(config.Role) == "" {
				fieldErrors = append(fieldErrors, domain.FieldError{
					Field:   prefix + ".config",
					Code:    "required",
					Message: "stage config must include role or account_ids",
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
			if strings.TrimSpace(config.Field) == "" || strings.TrimSpace(config.Operator) == "" || strings.TrimSpace(config.Value) == "" {
				fieldErrors = append(fieldErrors, domain.FieldError{
					Field:   prefix + ".config",
					Code:    "required",
					Message: "condition stage requires field, operator, and value",
				})
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

// validateFormSubmissionPayload 依 template fields 驗證提交 payload。
// 僅在 schema 明確宣告 fields 時校驗；未宣告時不套用 builder contract 預設欄位，避免誤傷舊模板。
func validateFormSubmissionPayload(template domain.FormTemplate, payload map[string]any) error {
	design := platformTemplateDesign(template.Schema)
	if design == nil {
		return nil
	}
	rawFields, hasFields := design["fields"]
	if !hasFields {
		return nil
	}
	fields, ok := platformDecodeSlice[domain.PlatformFormBuilderField](rawFields)
	if !ok || len(fields) == 0 {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	fieldErrors := make([]domain.FieldError, 0)
	for _, field := range fields {
		id := strings.TrimSpace(field.ID)
		if id == "" || strings.EqualFold(strings.TrimSpace(field.Type), "layout") {
			continue
		}
		value, exists := payload[id]
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
		return ValidationFailed("form submission validation failed", fieldErrors)
	}
	return nil
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

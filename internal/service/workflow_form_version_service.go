package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

// currentFormTemplateVersion 取得模板目前指向的不可變版本。
func (c WorkflowService) currentFormTemplateVersion(ctx RequestContext, template domain.FormTemplate) (domain.FormTemplateVersion, error) {
	versionNumber := max(template.CurrentVersion, 1)
	version, ok, err := c.store.GetFormTemplateVersionByNumber(goContext(ctx), ctx.TenantID, template.ID, versionNumber)
	if err != nil {
		return domain.FormTemplateVersion{}, err
	}
	if !ok {
		return domain.FormTemplateVersion{}, NotFound("form template version", fmt.Sprintf("%s:%d", template.ID, versionNumber))
	}
	return version, nil
}

// formTemplateVersionForInstance 取得表單實例提交時綁定的模板版本。
func (c WorkflowService) formTemplateVersionForInstance(ctx RequestContext, template domain.FormTemplate, instance domain.FormInstance) (domain.FormTemplateVersion, error) {
	if strings.TrimSpace(instance.TemplateVersionID) == "" {
		return c.currentFormTemplateVersion(ctx, template)
	}
	version, ok, err := c.store.GetFormTemplateVersion(goContext(ctx), ctx.TenantID, instance.TemplateVersionID)
	if err != nil {
		return domain.FormTemplateVersion{}, err
	}
	if !ok || version.TemplateID != template.ID {
		return domain.FormTemplateVersion{}, NotFound("form template version", instance.TemplateVersionID)
	}
	return version, nil
}

// formTemplateAtVersion 將模板身份與不可變 schema 快照組合成執行期模板。
func formTemplateAtVersion(template domain.FormTemplate, version domain.FormTemplateVersion) domain.FormTemplate {
	template.Schema = version.Schema
	template.Status = version.Status
	template.CurrentVersion = version.Version
	template.UpdatedAt = version.CreatedAt
	return template
}

// replaceFormInstanceFieldProjection 將可報表欄位同步成類型化投影。
func (c WorkflowService) replaceFormInstanceFieldProjection(ctx RequestContext, template domain.FormTemplate, instance domain.FormInstance) error {
	fields := platformTemplateFields(template.Key, template.Schema)
	values := make([]domain.FormInstanceFieldValue, 0, len(fields))
	for _, field := range fields {
		if field.Analytics == nil || !field.Analytics.Reportable || isStructuralFormFieldType(field.Type) {
			continue
		}
		rawValue, exists := instance.Payload[field.ID]
		if !exists || rawValue == nil {
			continue
		}
		value, ok := projectFormInstanceFieldValue(instance, field, rawValue)
		if ok {
			values = append(values, value)
		}
	}
	return c.store.ReplaceFormInstanceFieldValues(goContext(ctx), ctx.TenantID, instance.ID, values)
}

// projectFormInstanceFieldValue 將單一 payload 值轉為可索引型別。
func projectFormInstanceFieldValue(instance domain.FormInstance, field domain.PlatformFormBuilderField, rawValue any) (domain.FormInstanceFieldValue, bool) {
	createdAt := instance.UpdatedAt
	if createdAt.IsZero() {
		createdAt = instance.SubmittedAt
	}
	value := domain.FormInstanceFieldValue{
		TenantID: instance.TenantID, FormInstanceID: instance.ID, TemplateID: instance.TemplateID,
		TemplateVersionID: instance.TemplateVersionID, FieldID: field.ID, CreatedAt: createdAt,
	}
	switch strings.TrimSpace(field.Type) {
	case "number":
		number, ok := formProjectionNumber(rawValue)
		if !ok {
			return domain.FormInstanceFieldValue{}, false
		}
		value.ValueType = "number"
		value.ValueNumber = number
	case "checkbox":
		booleanValue, ok := rawValue.(bool)
		if !ok {
			return formProjectionJSON(value, rawValue)
		}
		value.ValueType = "boolean"
		value.ValueBoolean = &booleanValue
	case "date":
		text, ok := rawValue.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return domain.FormInstanceFieldValue{}, false
		}
		value.ValueType = "date"
		value.ValueDate = text
	case "datetime":
		text, ok := rawValue.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return domain.FormInstanceFieldValue{}, false
		}
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return domain.FormInstanceFieldValue{}, false
		}
		value.ValueType = "timestamp"
		value.ValueTimestamp = text
	default:
		if text, ok := rawValue.(string); ok {
			value.ValueType = "text"
			value.ValueText = text
			return value, true
		}
		return formProjectionJSON(value, rawValue)
	}
	return value, true
}

// formProjectionNumber 將 JSON number 或數字字串正規化為十進位字串。
func formProjectionNumber(value any) (string, bool) {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case json.Number:
		if _, err := typed.Float64(); err == nil {
			return typed.String(), true
		}
	case string:
		if _, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
			return strings.TrimSpace(typed), true
		}
	}
	return "", false
}

// formProjectionJSON 保存多選、物件與其他複合值。
func formProjectionJSON(value domain.FormInstanceFieldValue, rawValue any) (domain.FormInstanceFieldValue, bool) {
	raw, err := json.Marshal(rawValue)
	if err != nil {
		return domain.FormInstanceFieldValue{}, false
	}
	value.ValueType = "json"
	value.ValueJSON = raw
	return value, true
}

// isStructuralFormFieldType 判斷不會出現在提交 payload 的佈局欄位。
func isStructuralFormFieldType(fieldType string) bool {
	switch strings.TrimSpace(fieldType) {
	case "layout", "section-title", "divider", "html":
		return true
	default:
		return false
	}
}

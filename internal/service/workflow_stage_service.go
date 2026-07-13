package service

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"nexus-pro-be/internal/domain"
)

var workflowConditionNumberPattern = regexp.MustCompile(`(?:≥|>|=|<|≤|>=|<=)\s*([0-9]+)`)

// ParseWorkflowStagesFromTemplate 從 template schema 解析可執行流程節點。
func ParseWorkflowStagesFromTemplate(template domain.FormTemplate) []domain.WorkflowStageDefinition {
	stages := platformTemplateStages(template.Schema)
	out := make([]domain.WorkflowStageDefinition, 0, len(stages))
	for _, stage := range stages {
		if strings.TrimSpace(stage.ID) == "" || strings.TrimSpace(stage.Type) == "" {
			continue
		}
		out = append(out, normalizeWorkflowStageDefinition(stage))
	}
	return out
}

// SerializeWorkflowStages 序列化流程節點快照。
func SerializeWorkflowStages(stages []domain.WorkflowStageDefinition) string {
	raw, err := json.Marshal(stages)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// DeserializeWorkflowStages 還原流程節點快照。
func DeserializeWorkflowStages(raw string) []domain.WorkflowStageDefinition {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []domain.WorkflowStageDefinition
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func normalizeWorkflowStageDefinition(stage domain.PlatformFormBuilderStage) domain.WorkflowStageDefinition {
	config := workflowStageConfigFromMap(stage.Config)
	if config.Role == "" && len(config.AccountIDs) == 0 && len(config.UserGroupIDs) == 0 && config.Field == "" {
		config = inferWorkflowStageConfig(stage)
	}
	return domain.WorkflowStageDefinition{
		ID:     strings.TrimSpace(stage.ID),
		Type:   strings.TrimSpace(stage.Type),
		Label:  strings.TrimSpace(stage.Label),
		Detail: strings.TrimSpace(stage.Detail),
		Config: config,
	}
}

func workflowStageConfigFromMap(values map[string]any) domain.WorkflowStageConfig {
	if len(values) == 0 {
		return domain.WorkflowStageConfig{}
	}
	config := domain.WorkflowStageConfig{
		Role:                    stringFromAny(values["role"]),
		Mode:                    stringFromAny(values["mode"]),
		Field:                   stringFromAny(values["field"]),
		Operator:                stringFromAny(values["operator"]),
		Value:                   stringFromAny(values["value"]),
		TrueNextStageID:         stringFromAny(values["true_next_stage_id"]),
		FalseNextStageID:        stringFromAny(values["false_next_stage_id"]),
		ExcludeApplicant:        workflowBoolFromAny(values["exclude_applicant"]),
		RequireDistinctApprover: workflowBoolFromAny(values["require_distinct_approver"]),
	}
	if level := workflowIntFromAny(values["relative_level"]); level > 0 {
		config.RelativeLevel = level
	}
	if hours := workflowIntFromAny(values["remind_after_hours"]); hours > 0 {
		config.RemindAfterHours = hours
	} else if hours := workflowIntFromAny(values["remindAfterHours"]); hours > 0 {
		config.RemindAfterHours = hours
	}
	config.AccountIDs = uniqueWorkflowRecipientIDs(stringSliceFromAny(values["account_ids"]))
	config.UserGroupIDs = uniqueWorkflowRecipientIDs(stringSliceFromAny(values["user_group_ids"]))
	if levels, ok := values["levels"].([]any); ok {
		for _, item := range levels {
			if level := workflowIntFromAny(item); level > 0 {
				config.Levels = append(config.Levels, level)
			}
		}
	}
	return config
}

// workflowBoolFromAny normalizes persisted workflow flags from JSON-compatible values.
func workflowBoolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func inferWorkflowStageConfig(stage domain.PlatformFormBuilderStage) domain.WorkflowStageConfig {
	text := strings.TrimSpace(stage.Label + " " + stage.Detail)
	stageType := strings.TrimSpace(stage.Type)
	switch stageType {
	case "notify":
		return domain.WorkflowStageConfig{Role: inferWorkflowRole(text)}
	case "parallel":
		return domain.WorkflowStageConfig{Role: inferWorkflowRole(text), Mode: "all"}
	case "condition":
		return inferWorkflowConditionConfig(stage)
	default:
		role := inferWorkflowRole(text)
		config := domain.WorkflowStageConfig{Role: role}
		if strings.Contains(text, "+2") {
			config.Role = "relative"
			config.RelativeLevel = 2
		} else if strings.Contains(text, "+1") || strings.Contains(text, "+N") {
			config.Role = "relative"
			config.RelativeLevel = 1
		}
		return config
	}
}

func inferWorkflowRole(text string) string {
	switch {
	case strings.Contains(text, "部門主管"):
		return "dept-head"
	case strings.Contains(text, "HR"):
		return "hr"
	case strings.Contains(text, "財務"):
		return "finance"
	case strings.Contains(text, "總經理"):
		return "ceo"
	case strings.Contains(text, "申請者本人"):
		return "applicant"
	case strings.Contains(text, "+2"):
		return "relative"
	case strings.Contains(text, "+1") || strings.Contains(text, "+N"):
		return "relative"
	default:
		return "manager"
	}
}

func inferWorkflowConditionConfig(stage domain.PlatformFormBuilderStage) domain.WorkflowStageConfig {
	label := strings.TrimSpace(stage.Label)
	field := "hours"
	switch {
	case strings.Contains(label, "金額"):
		field = "amount"
	case strings.Contains(label, "職等"):
		field = "level"
	}
	operator := ">="
	switch {
	case strings.Contains(label, "≤") || strings.Contains(label, "<="):
		operator = "<="
	case strings.Contains(label, "<"):
		operator = "<"
	case strings.Contains(label, ">") && !strings.Contains(label, "≥"):
		operator = ">"
	case strings.Contains(label, "="):
		operator = "=="
	}
	value := ""
	if match := workflowConditionNumberPattern.FindStringSubmatch(label); len(match) > 1 {
		value = match[1]
	}
	levels := make([]int, 0)
	if field == "level" {
		for _, token := range regexp.MustCompile(`[0-9]+`).FindAllString(label, -1) {
			if level, err := strconv.Atoi(token); err == nil {
				levels = append(levels, level)
			}
		}
	}
	return domain.WorkflowStageConfig{
		Field:    field,
		Operator: operator,
		Value:    value,
		Levels:   levels,
	}
}

func workflowIntFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func validateWorkflowTemplateSubmittable(template domain.FormTemplate) error {
	if platformTemplateDeleted(template.Schema) {
		return BadRequest("form template is deleted")
	}
	if !platformTemplateEnabled(template.Schema) {
		return BadRequest("form template is disabled")
	}
	if len(ParseWorkflowStagesFromTemplate(template)) == 0 {
		return BadRequest("form template has no workflow stages")
	}
	return nil
}

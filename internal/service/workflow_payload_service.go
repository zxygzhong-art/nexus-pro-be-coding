package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

// withWorkflowReview 附加流程審核。
func withWorkflowReview(payload map[string]any, kind, accountID, comment string, at time.Time) map[string]any {
	next := utils.CopyStringMap(payload)
	if next == nil {
		next = map[string]any{}
	}
	next["_review"] = map[string]any{
		"type":       kind,
		"account_id": accountID,
		"comment":    strings.TrimSpace(comment),
		"time":       apiTimestamp(at),
	}
	return next
}

// workflowPayload 處理流程 payload。
func workflowPayload(payload map[string]any) map[string]any {
	next := utils.CopyStringMap(payload)
	if next == nil {
		return map[string]any{}
	}
	return next
}

// requireFormInstanceVisible 處理 require 表單實例可見。
func requireFormInstanceVisible(instance FormInstance, account Account, decision CheckResult) error {
	switch decision.Scope {
	case ScopeSelf, ScopeOwn:
		if instance.ApplicantAccountID != account.ID {
			return NotFound("form instance", instance.ID)
		}
	}
	return nil
}

// safeWorkflowFileName 處理 safe 流程檔案名稱。
func safeWorkflowFileName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "workflow-form"
	}
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "workflow-form"
	}
	return value
}

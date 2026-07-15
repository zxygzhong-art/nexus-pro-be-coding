package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

var workflowServerOwnedPayloadTokens = map[string]struct{}{
	"applicantaccountid":    {},
	"approvalstatus":        {},
	"approvedby":            {},
	"currentrunid":          {},
	"employeeid":            {},
	"forminstanceid":        {},
	"formstatus":            {},
	"linkedresourceid":      {},
	"linkedresourcestatus":  {},
	"linkedresourcetype":    {},
	"leaverequestid":        {},
	"leaverequeststatus":    {},
	"overtimerequestid":     {},
	"overtimerequeststatus": {},
	"reviewstatus":          {},
	"serverstatus":          {},
	"submittedat":           {},
	"templateid":            {},
	"templateversionid":     {},
	"tenantid":              {},
	"updatedat":             {},
	"workflowstatus":        {},
}

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

// workflowPayloadForNewInstance applies explicit frozen fields or legacy passthrough without server metadata.
func workflowPayloadForNewInstance(template FormTemplate, payload map[string]any) map[string]any {
	fields, hasExplicitFields := platformExplicitTemplateFields(template.Schema)
	allowed := make(map[string]struct{})
	for _, field := range fields {
		id := strings.TrimSpace(field.ID)
		if id == "" || isStructuralFormFieldType(field.Type) {
			continue
		}
		allowed[id] = struct{}{}
	}
	for _, key := range workflowNotificationRecipientPayloadKeys {
		allowed[key] = struct{}{}
	}

	next := make(map[string]any, len(allowed))
	for key, value := range payload {
		if workflowServerOwnsPayloadKey(key) {
			continue
		}
		if hasExplicitFields {
			if _, ok := allowed[key]; !ok {
				continue
			}
		}
		next[key] = value
	}
	return workflowPayload(next)
}

// workflowServerOwnsPayloadKey classifies linkage and workflow metadata that must never seed a new instance.
func workflowServerOwnsPayloadKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if strings.HasPrefix(key, "_") {
		return true
	}
	token := strings.ToLower(key)
	token = strings.NewReplacer("_", "", "-", "", ".", "").Replace(token)
	if strings.HasPrefix(token, "linkedresource") {
		return true
	}
	_, owned := workflowServerOwnedPayloadTokens[token]
	return owned
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

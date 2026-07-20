package service

import (
	"encoding/json"
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

// workflowReviewPayloadKey 是流程 payload 中存放最近一次審批動作的私有鍵。
const workflowReviewPayloadKey = "_review"

// workflowReviewPayload 定義 _review 的穩定結構；寫入與讀取都經 JSON marshal/unmarshal，不再裸斷言。
type workflowReviewPayload struct {
	Type      string `json:"type"`
	AccountID string `json:"account_id"`
	Comment   string `json:"comment"`
	Time      string `json:"time"`
}

// toMap 以 JSON 往返產生與既有儲存格式一致的 map[string]any。
func (r workflowReviewPayload) toMap() map[string]any {
	raw, err := json.Marshal(r)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

// workflowReviewFromPayload 取出 typed 審批記錄；鍵缺失、型別不符或 type 為空皆視為不存在。
func workflowReviewFromPayload(payload map[string]any) (workflowReviewPayload, bool) {
	raw, ok := payload[workflowReviewPayloadKey]
	if !ok {
		return workflowReviewPayload{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return workflowReviewPayload{}, false
	}
	review := workflowReviewPayload{}
	if err := json.Unmarshal(encoded, &review); err != nil {
		return workflowReviewPayload{}, false
	}
	if strings.TrimSpace(review.Type) == "" {
		return workflowReviewPayload{}, false
	}
	return review, true
}

// withWorkflowReview 附加流程審核。
func withWorkflowReview(payload map[string]any, kind, accountID, comment string, at time.Time) map[string]any {
	next := utils.CopyStringMap(payload)
	if next == nil {
		next = map[string]any{}
	}
	next[workflowReviewPayloadKey] = workflowReviewPayload{
		Type:      kind,
		AccountID: accountID,
		Comment:   strings.TrimSpace(comment),
		Time:      apiTimestamp(at),
	}.toMap()
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

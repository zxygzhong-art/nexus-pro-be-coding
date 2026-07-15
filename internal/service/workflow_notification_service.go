package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

// notifyWorkflowFormReviewed 投遞表單審核結果給申請人。
func (c WorkflowService) notifyWorkflowFormReviewed(ctx RequestContext, instance FormInstance, template FormTemplate, reviewer Account, kind, reason string) error {
	if strings.TrimSpace(instance.ApplicantAccountID) == "" {
		return nil
	}
	title := workflowNotificationTemplateTitle(template, instance)
	tone, statusText, notificationTitle, actionText := workflowReviewNotificationCopy(kind, title)
	body := "由 " + workflowAccountLabel(reviewer) + actionText + "。"
	if reason = strings.TrimSpace(reason); reason != "" {
		body += " 審核意見：" + reason
	}
	return c.deliverWorkflowNotification(ctx, Notification{
		ID:                 workflowNotificationID("review-"+kind, instance.ID),
		TenantID:           ctx.TenantID,
		Tone:               tone,
		Category:           "workflow",
		Title:              notificationTitle,
		Body:               body,
		StatusText:         statusText,
		LinkURL:            "/forms?applicationId=" + instance.ID,
		SourceType:         "workflow.form.review_result",
		SourceID:           instance.ID + ":" + kind,
		CreatedByAccountID: reviewer.ID,
		CreatedAt:          workflowNotificationTime(instance, c.Now()),
	}, []string{instance.ApplicantAccountID})
}

// deliverWorkflowNotification 將一筆工作流通知寫入內容與收件者狀態。
func (c WorkflowService) deliverWorkflowNotification(ctx RequestContext, notification Notification, recipientIDs []string) error {
	recipients, err := c.validWorkflowNotificationRecipients(ctx, recipientIDs)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}
	if notification.ID == "" {
		notification.ID = utils.NewID("notif")
	}
	if notification.TenantID == "" {
		notification.TenantID = ctx.TenantID
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = c.Now()
	}
	notification.CreatedAt = notification.CreatedAt.UTC()
	if err := c.store.UpsertNotification(goContext(ctx), notification); err != nil {
		return err
	}
	for _, accountID := range recipients {
		if err := c.store.UpsertNotificationRecipient(goContext(ctx), NotificationRecipient{
			NotificationID: notification.ID,
			TenantID:       notification.TenantID,
			AccountID:      accountID,
			CreatedAt:      notification.CreatedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

// validWorkflowNotificationRecipients 過濾不存在或停用的通知收件帳號。
func (c WorkflowService) validWorkflowNotificationRecipients(ctx RequestContext, recipientIDs []string) ([]string, error) {
	ids := uniqueWorkflowRecipientIDs(recipientIDs)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return nil, err
		}
		if !ok || account.Status == string(AccountStatusDisabled) || account.Status == string(AccountStatusPendingInvite) {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// workflowReviewNotificationCopy 產生審核結果通知文案。
func workflowReviewNotificationCopy(kind, title string) (tone, statusText, notificationTitle, actionText string) {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "approve":
		return "success", "已核准", "你的「" + title + "」已核准", "已核准這筆申請"
	case "return":
		return "warning", "已退回", "你的「" + title + "」已退回補件", "已退回這筆申請"
	default:
		return "warning", "不通過", "你的「" + title + "」未通過", "未通過這筆申請"
	}
}

// uniqueWorkflowRecipientIDs 正規化並去重通知收件帳號。
func uniqueWorkflowRecipientIDs(values []string, excluded ...string) []string {
	excludedSet := map[string]struct{}{}
	for _, id := range excluded {
		if id = strings.TrimSpace(id); id != "" {
			excludedSet[id] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, id := range values {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, skip := excludedSet[id]; skip {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// workflowNotificationTemplateTitle 取通知中使用的表單名稱。
func workflowNotificationTemplateTitle(template FormTemplate, instance FormInstance) string {
	return utils.FirstNonEmpty(template.Name, template.Key, template.ID, instance.TemplateID, "表單申請")
}

// workflowNotificationID 建立可重試的工作流通知 ID。
func workflowNotificationID(kind, instanceID string) string {
	return safeWorkflowFileName("notif-workflow-" + kind + "-" + instanceID)
}

// workflowNotificationTime 取通知時間並保證 UTC。
func workflowNotificationTime(instance FormInstance, fallback time.Time) time.Time {
	if !instance.UpdatedAt.IsZero() {
		return instance.UpdatedAt.UTC()
	}
	if !instance.SubmittedAt.IsZero() {
		return instance.SubmittedAt.UTC()
	}
	return fallback.UTC()
}

// workflowAccountLabel 取通知文案中的帳號顯示名稱。
func workflowAccountLabel(account Account) string {
	return utils.FirstNonEmpty(account.DisplayName, account.Email, account.ID, "系統")
}

// workflowPayloadMentionsAccount 處理流程 payload mentions 帳號。
func workflowPayloadMentionsAccount(payload map[string]any, accountID string) bool {
	if accountID == "" {
		return false
	}
	for _, key := range workflowNotificationRecipientPayloadKeys {
		if stringSliceContains(payload[key], accountID) {
			return true
		}
	}
	return false
}

// stringSliceContains 處理字串 slice contains。
func stringSliceContains(value any, target string) bool {
	switch v := value.(type) {
	case []string:
		for _, item := range v {
			if item == target {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if stringFromAny(item) == target {
				return true
			}
		}
	}
	return false
}

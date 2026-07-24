package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	identityProvisioningFastPathBatch      = 25
	identityProvisioningDefaultMaxRetries  = 5
	identityProvisioningMaxErrorLength     = 500
	identityProvisioningRetryBackoffBase   = 30 * time.Second
	identityProvisioningRetryBackoffMaxCap = 10 * time.Minute
	identityProvisioningClaimLease         = 5 * time.Minute
)

// provisionEmployeeAccountIdentity 開通員工帳號身分的服務流程。
func (c HRService) provisionEmployeeAccountIdentity(ctx RequestContext, employee Employee, account Account, sendInvite bool) error {
	if c.Service == nil || c.identityProvisioner == nil || strings.TrimSpace(account.ID) == "" {
		return nil
	}
	email := strings.TrimSpace(utils.FirstNonEmpty(account.Email, employee.CompanyEmail))
	if email == "" {
		return domainValidation("employee account identity provisioning failed", FieldError{Tab: employeeTabBasicInfo, Field: "company_email", Code: "required", Message: "company_email is required to provision a login account"})
	}
	now := c.Now()
	return c.Service.store.AppendIdentityProvisioningOutboxEvent(goContext(ctx), domain.IdentityProvisioningOutboxEvent{
		ID:            utils.NewID("idp"),
		TenantID:      ctx.TenantID,
		AccountID:     account.ID,
		EmployeeID:    employee.ID,
		EmployeeNo:    employee.EmployeeNo,
		Email:         email,
		DisplayName:   utils.FirstNonEmpty(account.DisplayName, employee.Name),
		Enabled:       account.Status != string(AccountStatusDisabled),
		SendInvite:    sendInvite,
		Status:        domain.IdentityProvisioningStatusPending,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

// provisionEmployeeIdentityFromAccountID 開通員工身分 來源 帳號 ID 的服務流程。
func (c HRService) provisionEmployeeIdentityFromAccountID(ctx RequestContext, employee Employee, accountID string, sendInvite bool) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("account", accountID)
	}
	return c.provisionEmployeeAccountIdentity(ctx, employee, account, sendInvite)
}

// runIdentityProvisioningFastPath 執行身分開通 fast path 的服務流程。
func (c HRService) runIdentityProvisioningFastPath(ctx RequestContext) {
	if c.Service == nil || c.identityProvisioner == nil {
		return
	}
	if _, err := c.Service.ProcessIdentityProvisioningOutbox(goContext(ctx), ctx.TenantID, identityProvisioningFastPathBatch, identityProvisioningDefaultMaxRetries); err != nil {
		c.logWarn(ctx, "identity provisioning fast path failed", "error", err)
	}
}

// ProcessIdentityProvisioningOutbox 處理身分開通 outbox 的服務流程。
func (c *Service) ProcessIdentityProvisioningOutbox(ctx context.Context, tenantID string, batchSize, maxRetries int) (int, error) {
	if c == nil || c.identityProvisioner == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = identityProvisioningFastPathBatch
	}
	if maxRetries <= 0 {
		maxRetries = identityProvisioningDefaultMaxRetries
	}
	now := c.Now()
	events, err := c.store.ClaimIdentityProvisioningOutboxEvents(ctx, tenantID, batchSize, maxRetries, now, now.Add(identityProvisioningClaimLease))
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if err := c.processIdentityProvisioningEvent(ctx, event, maxRetries); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

// identityProvisioningEventDue 處理身分開通事件 到期狀態 的服務流程。
func (c *Service) identityProvisioningEventDue(event domain.IdentityProvisioningOutboxEvent) bool {
	if event.RetryCount <= 0 {
		return true
	}
	return !c.Now().Before(event.UpdatedAt.Add(identityProvisioningBackoff(event.RetryCount)))
}

// identityProvisioningBackoff 處理身分開通 backoff。
func identityProvisioningBackoff(retryCount int) time.Duration {
	backoff := identityProvisioningRetryBackoffBase
	for i := 1; i < retryCount; i++ {
		backoff *= 2
		if backoff >= identityProvisioningRetryBackoffMaxCap {
			return identityProvisioningRetryBackoffMaxCap
		}
	}
	return backoff
}

// processIdentityProvisioningEvent 處理身分開通事件的服務流程。
func (c *Service) processIdentityProvisioningEvent(ctx context.Context, event domain.IdentityProvisioningOutboxEvent, maxRetries int) error {
	external, err := c.identityProvisioner.EnsureUser(ctx, domain.IdentityProvisioningInput{
		TenantID:    event.TenantID,
		AccountID:   event.AccountID,
		EmployeeID:  event.EmployeeID,
		EmployeeNo:  event.EmployeeNo,
		Email:       event.Email,
		DisplayName: event.DisplayName,
		Enabled:     event.Enabled,
		SendInvite:  event.SendInvite,
	})
	if err != nil {
		if errors.Is(err, domain.ErrIdentityProvisioningOwnershipConflict) {
			return c.recordIdentityProvisioningFailure(ctx, event, err.Error())
		}
		return c.recordIdentityProvisioningRetry(ctx, event, maxRetries, err.Error())
	}
	provider := strings.TrimSpace(external.Provider)
	if provider == "" {
		provider = domain.IdentityProviderKeycloak
	}
	subject := strings.TrimSpace(external.Subject)
	if subject == "" {
		return c.recordIdentityProvisioningFailure(ctx, event, "identity provisioner returned empty subject")
	}
	existing, ok, err := c.store.GetUserIdentity(ctx, event.TenantID, provider, subject)
	if err != nil {
		return err
	}
	if ok && existing.AccountID != event.AccountID {
		// 衝突屬於永久錯誤；重試無法重新連結外部 subject。
		return c.recordIdentityProvisioningFailure(ctx, event, "external identity is already linked to another account")
	}
	if err := c.store.UpsertUserIdentity(ctx, domain.UserIdentity{
		ID:        utils.NewID("uid"),
		TenantID:  event.TenantID,
		AccountID: event.AccountID,
		Provider:  provider,
		Subject:   subject,
		Email:     strings.TrimSpace(utils.FirstNonEmpty(external.Email, event.Email)),
		CreatedAt: c.Now(),
	}); err != nil {
		return err
	}
	event.Status = domain.IdentityProvisioningStatusSucceeded
	event.LastError = ""
	event.ClaimExpiresAt = nil
	event.UpdatedAt = c.Now()
	return c.store.UpdateIdentityProvisioningOutboxEvent(ctx, event)
}

// recordIdentityProvisioningRetry 記錄身分開通 retry 的服務流程。
func (c *Service) recordIdentityProvisioningRetry(ctx context.Context, event domain.IdentityProvisioningOutboxEvent, maxRetries int, message string) error {
	event.RetryCount++
	event.LastError = truncateIdentityProvisioningError(message)
	event.Status = domain.IdentityProvisioningStatusPending
	if event.RetryCount >= maxRetries {
		event.Status = domain.IdentityProvisioningStatusFailed
	}
	event.UpdatedAt = c.Now()
	event.NextAttemptAt = event.UpdatedAt.Add(identityProvisioningBackoff(event.RetryCount))
	event.ClaimExpiresAt = nil
	c.logger.WarnContext(ctx, "identity provisioning attempt failed",
		"tenant_id", event.TenantID,
		"account_id", event.AccountID,
		"event_id", event.ID,
		"retry_count", event.RetryCount,
		"status", event.Status,
		"error", message,
	)
	return c.store.UpdateIdentityProvisioningOutboxEvent(ctx, event)
}

// recordIdentityProvisioningFailure 記錄身分開通 failure 的服務流程。
func (c *Service) recordIdentityProvisioningFailure(ctx context.Context, event domain.IdentityProvisioningOutboxEvent, message string) error {
	event.Status = domain.IdentityProvisioningStatusFailed
	event.LastError = truncateIdentityProvisioningError(message)
	event.ClaimExpiresAt = nil
	event.UpdatedAt = c.Now()
	c.logger.WarnContext(ctx, "identity provisioning failed permanently",
		"tenant_id", event.TenantID,
		"account_id", event.AccountID,
		"event_id", event.ID,
		"error", message,
	)
	return c.store.UpdateIdentityProvisioningOutboxEvent(ctx, event)
}

// truncateIdentityProvisioningError 截斷身分開通錯誤。
func truncateIdentityProvisioningError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= identityProvisioningMaxErrorLength {
		return value
	}
	return value[:identityProvisioningMaxErrorLength]
}

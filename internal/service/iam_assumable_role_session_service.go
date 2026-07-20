package service

import (
	"net/http"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

const (
	assumedRoleSessionReasonInvalid  = "assumed_role_session_invalid"
	assumedRoleSessionReasonExpired  = "assumed_role_session_expired"
	assumedRoleSessionReasonRevoked  = "assumed_role_session_revoked"
	assumedRoleSessionReasonForeign  = "assumed_role_session_foreign"
	assumedRoleSessionReasonRequired = "assumed_role_session_required"
)

// ownedActiveAssumableRoleSession validates the opaque bearer without ever
// reflecting it into errors, logs, or audit details.
func (c *Service) ownedActiveAssumableRoleSession(ctx RequestContext, accountID string) (AssumableRoleSession, error) {
	sessionID := strings.TrimSpace(ctx.AssumedRoleSessionID)
	if sessionID == "" {
		return AssumableRoleSession{}, domain.E(http.StatusBadRequest, "bad_request", "assumable role session is required").WithReasonCode(assumedRoleSessionReasonRequired)
	}
	session, ok, err := c.store.GetAssumableRoleSession(goContext(ctx), ctx.TenantID, sessionID)
	if err != nil {
		return AssumableRoleSession{}, err
	}
	if !ok {
		return AssumableRoleSession{}, assumedRoleSessionStateError(http.StatusNotFound, assumedRoleSessionReasonInvalid)
	}
	// Check ownership before lifecycle state so callers cannot probe whether
	// another account's opaque bearer is active, expired, or revoked.
	if session.AccountID != accountID {
		return AssumableRoleSession{}, assumedRoleSessionStateError(http.StatusForbidden, assumedRoleSessionReasonForeign)
	}
	if session.RevokedAt != nil {
		return AssumableRoleSession{}, assumedRoleSessionStateError(http.StatusNotFound, assumedRoleSessionReasonRevoked)
	}
	if !session.ExpiresAt.After(c.Now()) {
		return AssumableRoleSession{}, assumedRoleSessionStateError(http.StatusNotFound, assumedRoleSessionReasonExpired)
	}
	return session, nil
}

func assumedRoleSessionStateError(status int, reason string) *domain.AppError {
	message := "assumable role session is not active"
	if status == http.StatusForbidden {
		message = "assumable role session is not available to this account"
	}
	code := "not_found"
	if status == http.StatusForbidden {
		code = "forbidden"
	}
	return domain.E(status, code, message).WithReasonCode(reason)
}

// RevokeCurrentAssumableRoleSession lets an authenticated account return its
// own temporary role without requiring permissions from the narrowed role.
func (c IAMService) RevokeCurrentAssumableRoleSession(ctx RequestContext) error {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return err
	}
	session, err := c.ownedActiveAssumableRoleSession(ctx, account.ID)
	if err != nil {
		return err
	}
	revokedAt := c.Now()
	if err := c.withTransaction(ctx, func(tx IAMService) error {
		revoked, ok, err := tx.store.RevokeAssumableRoleSession(goContext(ctx), ctx.TenantID, account.ID, session.ID, revokedAt)
		if err != nil {
			return err
		}
		if !ok {
			return assumedRoleSessionStateError(http.StatusNotFound, assumedRoleSessionReasonRevoked)
		}
		if err := tx.touchAuthzConfig(ctx, "iam.assumable_role.session.revoke", map[string]any{
			"assumable_role_id": revoked.AssumableRoleID,
			"account_id":        account.ID,
			"revoked_at":        revokedAt.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		return tx.audit(ctx, "iam.assumable_role.session.revoke", "assumable_role", revoked.AssumableRoleID, "high", map[string]any{
			"account_id": account.ID,
			"revoked_at": revokedAt.Format(time.RFC3339),
		})
	}); err != nil {
		return err
	}
	c.logWarn(ctx, "assumable role session revoked",
		"assumable_role_id", session.AssumableRoleID,
		"account_id", account.ID,
		"revoked_at", revokedAt.Format(time.RFC3339),
	)
	return nil
}

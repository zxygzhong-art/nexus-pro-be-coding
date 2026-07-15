package repository

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
)

// IdentityStore 定義身分儲存層的行為契約。
type IdentityStore interface {
	UpsertUserIdentity(context.Context, domain.UserIdentity) error
	GetUserIdentity(ctx context.Context, tenantID, provider, subject string) (domain.UserIdentity, bool, error)
	ListUserIdentities(ctx context.Context, tenantID, accountID string) ([]domain.UserIdentity, error)
	AppendIdentityProvisioningOutboxEvent(context.Context, domain.IdentityProvisioningOutboxEvent) error
	ListPendingIdentityProvisioningOutboxEvents(ctx context.Context, tenantID string) ([]domain.IdentityProvisioningOutboxEvent, error)
	ClaimIdentityProvisioningOutboxEvents(ctx context.Context, tenantID string, batchSize, maxRetries int, claimedAt, leaseUntil time.Time) ([]domain.IdentityProvisioningOutboxEvent, error)
	UpdateIdentityProvisioningOutboxEvent(context.Context, domain.IdentityProvisioningOutboxEvent) error
}

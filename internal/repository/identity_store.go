package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// IdentityStore persists external identity provider bindings for local accounts.
type IdentityStore interface {
	UpsertUserIdentity(context.Context, domain.UserIdentity) error
	GetUserIdentity(ctx context.Context, tenantID, provider, subject string) (domain.UserIdentity, bool, error)
	ListUserIdentities(ctx context.Context, tenantID, accountID string) ([]domain.UserIdentity, error)
}

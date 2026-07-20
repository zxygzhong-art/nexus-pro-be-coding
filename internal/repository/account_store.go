package repository

import (
	"context"

	"nexus-pro-api/internal/domain"
)

// AccountStore 定義帳號儲存層的行為契約。
type AccountStore interface {
	UpsertAccount(context.Context, domain.Account) error
	GetAccount(ctx context.Context, tenantID, id string) (domain.Account, bool, error)
	UpdateAccountPreferredLocale(ctx context.Context, tenantID, id, preferredLocale string) (domain.Account, bool, error)
	ListAccounts(ctx context.Context, tenantID string) ([]domain.Account, error)
	AddAccountGroup(ctx context.Context, tenantID, accountID, groupID string) error
	RemoveAccountGroup(ctx context.Context, tenantID, accountID, groupID string) error
}

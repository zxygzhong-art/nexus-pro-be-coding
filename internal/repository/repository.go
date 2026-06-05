// Package repository provides GORM-backed data access over the tenant-scoped
// session in context. It implements authz.DataSource and exposes the management
// queries used by the IAM handlers. All reads/writes go through the
// reqctx.TenantDB(ctx) session so PostgreSQL RLS is always in force.
package repository

import (
	"context"
	"errors"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
	"gorm.io/gorm"
)

// ErrNoTenantTx indicates the request never established a tenant-scoped session.
var ErrNoTenantTx = errors.New("repository: no tenant-scoped db in context")

// Repository is a stateless GORM repository; every method resolves the
// tenant-scoped session from the context.
type Repository struct{}

// New builds a Repository.
func New() *Repository { return &Repository{} }

func tx(ctx context.Context) (*gorm.DB, error) {
	db := reqctx.TenantDB(ctx)
	if db == nil {
		return nil, ErrNoTenantTx
	}
	return db, nil
}

// --- Account / membership ---------------------------------------------------

// GetAccount loads a non-deleted account by id.
func (r *Repository) GetAccount(ctx context.Context, id string) (*models.Account, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var a models.Account
	if err := db.Where("id = ?", id).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// GroupIDsForAccount returns the ids of groups the account is an active member of.
func (r *Repository) GroupIDsForAccount(ctx context.Context, accountID string) ([]string, error) {
	db, err := tx(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	err = db.Model(&models.GroupMembership{}).
		Where("account_id = ?", accountID).
		Where("(valid_from IS NULL OR valid_from <= now())").
		Where("(valid_until IS NULL OR valid_until >= now())").
		Pluck("group_id", &ids).Error
	return ids, err
}

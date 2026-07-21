package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-api/internal/domain"
	sqlc "nexus-pro-api/internal/platform/postgres/db"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/utils"
	"nexus-pro-api/internal/utils/jsoncodec"
	"nexus-pro-api/internal/utils/tenantctx"
)

// Store 定義儲存層的資料結構。
type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	db   sqlc.DBTX
}

var _ repository.Store = (*Store)(nil)

// NewStore 建立儲存層。
func NewStore(pool *pgxpool.Pool) *Store {
	db := newTenantDBTX(pool)
	return &Store{pool: pool, q: sqlc.New(db), db: db}
}

// tenantContext 處理租戶 context。
func tenantContext(ctx context.Context, tenantID string) context.Context {
	return tenantctx.WithTenantID(ctx, tenantID)
}

// WithTenantTransaction 從儲存層附加租戶 transaction。
func (s *Store) WithTenantTransaction(execCtx context.Context, tenantID string, fn func(repository.Store) error) error {
	if s.pool == nil {
		return fn(s)
	}
	if execCtx == nil {
		execCtx = context.Background()
	}
	tx, err := s.pool.Begin(execCtx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(execCtx)
			panic(p)
		}
		// pgx 中 commit 後 rollback 無害，可保護每個提前返回路徑。
		_ = tx.Rollback(execCtx)
	}()
	if _, err := tx.Exec(execCtx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return err
	}
	if companyID := tenantctx.CompanyIDFromContext(execCtx); companyID != "" {
		if _, err := tx.Exec(execCtx, "SELECT set_config('app.company_id', $1, true)", companyID); err != nil {
			return err
		}
	}
	txStore := &Store{q: sqlc.New(tx), db: tx}
	if err := fn(txStore); err != nil {
		return err
	}
	return tx.Commit(execCtx)
}

// UpsertTenant 從儲存層處理 upsert 租戶。
func (s *Store) UpsertTenant(execCtx context.Context, v domain.Tenant) error {
	// tenants RLS policy 以自身 id 隔離資料列，因此寫入需套用相同 scope。
	_, err := s.q.UpsertTenant(tenantContext(execCtx, v.ID), sqlc.UpsertTenantParams{
		ID:        v.ID,
		Name:      v.Name,
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

// GetTenant 從儲存層取得租戶。
func (s *Store) GetTenant(execCtx context.Context, id string) (domain.Tenant, bool, error) {
	v, err := s.q.GetTenant(tenantContext(execCtx, id), id)
	if isNotFound(err) {
		return domain.Tenant{}, false, nil
	}
	if err != nil {
		return domain.Tenant{}, false, err
	}
	return fromTenant(v), true, nil
}

// ListTenants 從儲存層列出租戶。
func (s *Store) ListTenants(execCtx context.Context) ([]domain.Tenant, error) {
	// 列出所有 tenant 屬於跨 tenant 系統操作；system_task 設定會滿足唯讀 RLS policy。
	// 此設定會滿足 system_read_tenants 的唯讀 RLS policy。
	items, err := s.q.ListTenants(tenantctx.WithSystemTask(execCtx))
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromTenant), nil
}

// UpsertAccount 從儲存層處理 upsert 帳號。Version > 0 時執行樂觀鎖檢查。
func (s *Store) UpsertAccount(execCtx context.Context, v domain.Account) error {
	_, err := s.q.UpsertAccount(execCtx, sqlc.UpsertAccountParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		DisplayName:            v.DisplayName,
		Email:                  v.Email,
		EmployeeID:             v.EmployeeID,
		Status:                 v.Status,
		UserGroupIds:           textArray(v.UserGroupIDs),
		DirectPermissionSetIds: textArray(v.DirectPermissionSetIDs),
		ActiveAssumableRoleID:  v.ActiveAssumableRoleID,
		PreferredLocale:        domain.PreferredLocaleWithDefault(v.PreferredLocale),
		CreatedAt:              timestamptz(v.CreatedAt),
		ExpectedVersion:        v.Version,
	})
	if isNotFound(err) {
		return domain.Conflict("account was modified concurrently")
	}
	return err
}

// UpdateAccountPreferredLocale updates one account preference and returns the refreshed row.
func (s *Store) UpdateAccountPreferredLocale(execCtx context.Context, tenantID, id, preferredLocale string) (domain.Account, bool, error) {
	v, err := s.q.UpdateAccountPreferredLocale(tenantContext(execCtx, tenantID), sqlc.UpdateAccountPreferredLocaleParams{
		PreferredLocale: preferredLocale,
		TenantID:        tenantID,
		ID:              id,
	})
	if isNotFound(err) {
		return domain.Account{}, false, nil
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	return fromAccount(v), true, nil
}

// GetAccount 從儲存層取得帳號。
func (s *Store) GetAccount(execCtx context.Context, tenantID, id string) (domain.Account, bool, error) {
	v, err := s.q.GetAccount(execCtx, sqlc.GetAccountParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.Account{}, false, nil
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	return fromAccount(v), true, nil
}

// ListAccounts 從儲存層列出帳號。
func (s *Store) ListAccounts(execCtx context.Context, tenantID string) ([]domain.Account, error) {
	items, err := s.q.ListAccounts(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAccount), nil
}

// UpsertUserIdentity 從儲存層處理 upsert 使用者身分。
func (s *Store) UpsertUserIdentity(execCtx context.Context, v domain.UserIdentity) error {
	_, err := s.q.UpsertUserIdentity(tenantContext(execCtx, v.TenantID), sqlc.UpsertUserIdentityParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		Provider:  v.Provider,
		Subject:   v.Subject,
		Email:     v.Email,
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

// GetUserIdentity 從儲存層取得使用者身分。
func (s *Store) GetUserIdentity(execCtx context.Context, tenantID, provider, subject string) (domain.UserIdentity, bool, error) {
	v, err := s.q.GetUserIdentity(tenantContext(execCtx, tenantID), sqlc.GetUserIdentityParams{
		TenantID: tenantID,
		Provider: provider,
		Subject:  subject,
	})
	if isNotFound(err) {
		return domain.UserIdentity{}, false, nil
	}
	if err != nil {
		return domain.UserIdentity{}, false, err
	}
	return fromUserIdentity(v), true, nil
}

// ListUserIdentities 從儲存層列出使用者身分。
func (s *Store) ListUserIdentities(execCtx context.Context, tenantID, accountID string) ([]domain.UserIdentity, error) {
	items, err := s.q.ListUserIdentities(tenantContext(execCtx, tenantID), sqlc.ListUserIdentitiesParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromUserIdentity), nil
}

// AppendIdentityProvisioningOutboxEvent 從儲存層附加身分開通 outbox 事件。
func (s *Store) AppendIdentityProvisioningOutboxEvent(execCtx context.Context, v domain.IdentityProvisioningOutboxEvent) error {
	if v.NextAttemptAt.IsZero() {
		v.NextAttemptAt = v.CreatedAt
	}
	_, err := s.q.AppendIdentityProvisioningOutboxEvent(tenantContext(execCtx, v.TenantID), sqlc.AppendIdentityProvisioningOutboxEventParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		EmployeeID:     v.EmployeeID,
		EmployeeNo:     v.EmployeeNo,
		Email:          v.Email,
		DisplayName:    v.DisplayName,
		Enabled:        v.Enabled,
		SendInvite:     v.SendInvite,
		Status:         v.Status,
		RetryCount:     int32(v.RetryCount),
		LastError:      v.LastError,
		NextAttemptAt:  timestamptz(v.NextAttemptAt),
		ClaimExpiresAt: nullableTimestamptz(v.ClaimExpiresAt),
		CreatedAt:      timestamptz(v.CreatedAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
	})
	return err
}

// ClaimIdentityProvisioningOutboxEvents atomically leases due events to one worker.
func (s *Store) ClaimIdentityProvisioningOutboxEvents(execCtx context.Context, tenantID string, batchSize, maxRetries int, claimedAt, leaseUntil time.Time) ([]domain.IdentityProvisioningOutboxEvent, error) {
	items, err := s.q.ClaimIdentityProvisioningOutboxEvents(tenantContext(execCtx, tenantID), sqlc.ClaimIdentityProvisioningOutboxEventsParams{
		TenantID: tenantID, MaxRetries: int32(maxRetries), ClaimedAt: timestamptz(claimedAt), BatchSize: int32(batchSize), LeaseUntil: timestamptz(leaseUntil),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromIdentityProvisioningOutboxEvent), nil
}

// ListPendingIdentityProvisioningOutboxEvents 從儲存層列出 pending 身分開通 outbox 事件。
func (s *Store) ListPendingIdentityProvisioningOutboxEvents(execCtx context.Context, tenantID string) ([]domain.IdentityProvisioningOutboxEvent, error) {
	items, err := s.q.ListPendingIdentityProvisioningOutboxEvents(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromIdentityProvisioningOutboxEvent), nil
}

// UpdateIdentityProvisioningOutboxEvent 從儲存層更新身分開通 outbox 事件。
func (s *Store) UpdateIdentityProvisioningOutboxEvent(execCtx context.Context, v domain.IdentityProvisioningOutboxEvent) error {
	_, err := s.q.UpdateIdentityProvisioningOutboxEvent(tenantContext(execCtx, v.TenantID), sqlc.UpdateIdentityProvisioningOutboxEventParams{
		TenantID:       v.TenantID,
		ID:             v.ID,
		Status:         v.Status,
		RetryCount:     int32(v.RetryCount),
		LastError:      v.LastError,
		NextAttemptAt:  timestamptz(v.NextAttemptAt),
		ClaimExpiresAt: nullableTimestamptz(v.ClaimExpiresAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
	})
	return err
}

// AddAccountGroup 從儲存層處理 add 帳號羣組。
func (s *Store) AddAccountGroup(execCtx context.Context, tenantID, accountID, groupID string) error {
	account, ok, err := s.GetAccount(execCtx, tenantID, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if utils.ContainsString(account.UserGroupIDs, groupID) {
		return nil
	}
	account.UserGroupIDs = append(account.UserGroupIDs, groupID)
	return s.UpsertAccount(execCtx, account)
}

// RemoveAccountGroup 從儲存層處理 remove 帳號羣組。
func (s *Store) RemoveAccountGroup(execCtx context.Context, tenantID, accountID, groupID string) error {
	account, ok, err := s.GetAccount(execCtx, tenantID, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	next := make([]string, 0, len(account.UserGroupIDs))
	for _, id := range account.UserGroupIDs {
		if id != groupID {
			next = append(next, id)
		}
	}
	account.UserGroupIDs = next
	return s.UpsertAccount(execCtx, account)
}

// UpsertUserGroup 從儲存層處理 upsert 使用者羣組。Version > 0 時執行樂觀鎖檢查。
func (s *Store) UpsertUserGroup(execCtx context.Context, v domain.UserGroup) error {
	_, err := s.q.UpsertUserGroup(execCtx, sqlc.UpsertUserGroupParams{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Name:                 v.Name,
		Description:          v.Description,
		MemberAccountIds:     textArray(v.MemberAccountIDs),
		PermissionSetIds:     textArray(v.PermissionSetIDs),
		SourceTemplateKey:    v.SourceTemplateKey,
		SourcePackageVersion: v.SourcePackageVersion,
		CreatedAt:            timestamptz(v.CreatedAt),
		ExpectedVersion:      v.Version,
	})
	if isNotFound(err) {
		return domain.Conflict("user group was modified concurrently")
	}
	return err
}

// GetUserGroup 從儲存層取得使用者羣組。
func (s *Store) GetUserGroup(execCtx context.Context, tenantID, id string) (domain.UserGroup, bool, error) {
	v, err := s.q.GetUserGroup(execCtx, sqlc.GetUserGroupParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.UserGroup{}, false, nil
	}
	if err != nil {
		return domain.UserGroup{}, false, err
	}
	return fromUserGroup(v), true, nil
}

// ListUserGroups 從儲存層列出使用者羣組。
func (s *Store) ListUserGroups(execCtx context.Context, tenantID string) ([]domain.UserGroup, error) {
	items, err := s.q.ListUserGroups(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromUserGroup), nil
}

// DeleteUserGroup 從儲存層刪除使用者羣組。
func (s *Store) DeleteUserGroup(execCtx context.Context, tenantID, id string) (domain.UserGroup, bool, error) {
	v, err := s.q.DeleteUserGroup(tenantContext(execCtx, tenantID), sqlc.DeleteUserGroupParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.UserGroup{}, false, nil
	}
	if err != nil {
		return domain.UserGroup{}, false, err
	}
	return fromUserGroup(v), true, nil
}

// UpsertGroupMembership 從儲存層處理 upsert 使用者羣組成員關係。
func (s *Store) UpsertGroupMembership(execCtx context.Context, v domain.GroupMembership) error {
	_, err := s.q.UpsertGroupMembership(tenantContext(execCtx, v.TenantID), sqlc.UpsertGroupMembershipParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		UserGroupID:        v.UserGroupID,
		AccountID:          v.AccountID,
		ValidFrom:          timestamptz(v.ValidFrom),
		ValidUntil:         nullableTimestamptz(v.ValidUntil),
		Source:             v.Source,
		ApprovalInstanceID: v.ApprovalInstanceID,
		CreatedBy:          v.CreatedBy,
		CreatedAt:          timestamptz(v.CreatedAt),
	})
	return err
}

// DeleteGroupMembership 從儲存層刪除使用者羣組成員關係。
func (s *Store) DeleteGroupMembership(execCtx context.Context, tenantID, userGroupID, accountID string) (domain.GroupMembership, bool, error) {
	v, err := s.q.DeleteGroupMembership(tenantContext(execCtx, tenantID), sqlc.DeleteGroupMembershipParams{
		TenantID:    tenantID,
		UserGroupID: userGroupID,
		AccountID:   accountID,
	})
	if isNotFound(err) {
		return domain.GroupMembership{}, false, nil
	}
	if err != nil {
		return domain.GroupMembership{}, false, err
	}
	return fromGroupMembership(v), true, nil
}

// CloseGroupMembership ends the active membership interval without deleting history.
func (s *Store) CloseGroupMembership(execCtx context.Context, tenantID, userGroupID, accountID string, validUntil time.Time) (domain.GroupMembership, bool, error) {
	v, err := s.q.CloseGroupMembership(tenantContext(execCtx, tenantID), sqlc.CloseGroupMembershipParams{
		TenantID: tenantID, UserGroupID: userGroupID, AccountID: accountID, ValidUntil: timestamptz(validUntil),
	})
	if isNotFound(err) {
		return domain.GroupMembership{}, false, nil
	}
	if err != nil {
		return domain.GroupMembership{}, false, err
	}
	return fromGroupMembership(v), true, nil
}

// GetGroupMembership 從儲存層取得使用者羣組成員關係。
func (s *Store) GetGroupMembership(execCtx context.Context, tenantID, userGroupID, accountID string) (domain.GroupMembership, bool, error) {
	v, err := s.q.GetGroupMembership(tenantContext(execCtx, tenantID), sqlc.GetGroupMembershipParams{
		TenantID:    tenantID,
		UserGroupID: userGroupID,
		AccountID:   accountID,
	})
	if isNotFound(err) {
		return domain.GroupMembership{}, false, nil
	}
	if err != nil {
		return domain.GroupMembership{}, false, err
	}
	return fromGroupMembership(v), true, nil
}

// ListGroupMembershipsForGroup 從儲存層列出使用者羣組成員關係。
func (s *Store) ListGroupMembershipsForGroup(execCtx context.Context, tenantID, userGroupID string) ([]domain.GroupMembership, error) {
	items, err := s.q.ListGroupMembershipsForGroup(tenantContext(execCtx, tenantID), sqlc.ListGroupMembershipsForGroupParams{
		TenantID:    tenantID,
		UserGroupID: userGroupID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromGroupMembership), nil
}

// ListActiveGroupMembershipsForAccount 從儲存層列出帳號有效使用者羣組成員關係。
func (s *Store) ListActiveGroupMembershipsForAccount(execCtx context.Context, tenantID, accountID string, at time.Time) ([]domain.GroupMembership, error) {
	items, err := s.q.ListActiveGroupMembershipsForAccount(tenantContext(execCtx, tenantID), sqlc.ListActiveGroupMembershipsForAccountParams{
		TenantID:  tenantID,
		AccountID: accountID,
		ValidFrom: timestamptz(at),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromGroupMembership), nil
}

// UpsertPermissionSet 從儲存層處理 upsert 權限集合。
func (s *Store) UpsertPermissionSet(execCtx context.Context, v domain.PermissionSet) error {
	_, err := s.q.UpsertPermissionSet(execCtx, sqlc.UpsertPermissionSetParams{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Name:                 v.Name,
		Description:          v.Description,
		Column5:              mustJSON(v.Permissions),
		SourceTemplateKey:    v.SourceTemplateKey,
		SourcePackageVersion: v.SourcePackageVersion,
		CreatedAt:            timestamptz(v.CreatedAt),
	})
	return err
}

// GetPermissionSet 從儲存層取得權限集合。
func (s *Store) GetPermissionSet(execCtx context.Context, tenantID, id string) (domain.PermissionSet, bool, error) {
	v, err := s.q.GetPermissionSet(execCtx, sqlc.GetPermissionSetParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.PermissionSet{}, false, nil
	}
	if err != nil {
		return domain.PermissionSet{}, false, err
	}
	return fromPermissionSet(v), true, nil
}

// ListPermissionSets 從儲存層列出權限集合。
func (s *Store) ListPermissionSets(execCtx context.Context, tenantID string) ([]domain.PermissionSet, error) {
	items, err := s.q.ListPermissionSets(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSet), nil
}

// DeletePermissionSet 從儲存層刪除權限集合。
func (s *Store) DeletePermissionSet(execCtx context.Context, tenantID, id string) (domain.PermissionSet, bool, error) {
	if err := s.q.DeletePermissionSetItemsForSet(execCtx, sqlc.DeletePermissionSetItemsForSetParams{
		TenantID:        tenantID,
		PermissionSetID: id,
	}); err != nil {
		return domain.PermissionSet{}, false, err
	}
	v, err := s.q.DeletePermissionSet(tenantContext(execCtx, tenantID), sqlc.DeletePermissionSetParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.PermissionSet{}, false, nil
	}
	if err != nil {
		return domain.PermissionSet{}, false, err
	}
	return fromPermissionSet(v), true, nil
}

// ReplacePermissionSetItems 從儲存層替換權限集合項。
func (s *Store) ReplacePermissionSetItems(execCtx context.Context, tenantID, permissionSetID string, items []domain.PermissionSetItem) error {
	if err := s.q.DeletePermissionSetItemsForSet(execCtx, sqlc.DeletePermissionSetItemsForSetParams{
		TenantID:        tenantID,
		PermissionSetID: permissionSetID,
	}); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := s.q.UpsertPermissionSetItem(execCtx, sqlc.UpsertPermissionSetItemParams{
			ID:              item.ID,
			TenantID:        item.TenantID,
			PermissionSetID: item.PermissionSetID,
			PermissionID:    item.PermissionID,
			CreatedAt:       timestamptz(item.CreatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListPermissionSetItemsForSet 從儲存層列出權限集合項。
func (s *Store) ListPermissionSetItemsForSet(execCtx context.Context, tenantID, permissionSetID string) ([]domain.PermissionSetItem, error) {
	items, err := s.q.ListPermissionSetItemsForSet(tenantContext(execCtx, tenantID), sqlc.ListPermissionSetItemsForSetParams{
		TenantID:        tenantID,
		PermissionSetID: permissionSetID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSetItem), nil
}

// UpsertPermissionCatalogItem 從儲存層處理 upsert 權限 catalog 項。
func (s *Store) UpsertPermissionCatalogItem(execCtx context.Context, v domain.PermissionCatalogItem) error {
	_, err := s.q.UpsertPermissionCatalogItem(tenantContext(execCtx, v.TenantID), sqlc.UpsertPermissionCatalogItemParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		Application:    v.Application,
		Resource:       v.Resource,
		Action:         v.Action,
		PermissionType: string(v.PermissionType),
		MenuKey:        v.MenuKey,
		Name:           v.Name,
		Description:    v.Description,
		HighRisk:       v.HighRisk,
		Severity:       v.Severity,
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

// GetPermissionCatalogItemByKey 從儲存層取得權限 catalog 項。
func (s *Store) GetPermissionCatalogItemByKey(execCtx context.Context, tenantID, application, resource, action string, permissionType domain.PermissionType) (domain.PermissionCatalogItem, bool, error) {
	v, err := s.q.GetPermissionCatalogItemByKey(tenantContext(execCtx, tenantID), sqlc.GetPermissionCatalogItemByKeyParams{
		TenantID:       tenantID,
		Application:    application,
		Resource:       resource,
		Action:         action,
		PermissionType: string(permissionType),
	})
	if isNotFound(err) {
		return domain.PermissionCatalogItem{}, false, nil
	}
	if err != nil {
		return domain.PermissionCatalogItem{}, false, err
	}
	return fromPermissionCatalogItem(v), true, nil
}

// ListPermissionCatalogItems 從儲存層列出權限 catalog。
func (s *Store) ListPermissionCatalogItems(execCtx context.Context, tenantID string) ([]domain.PermissionCatalogItem, error) {
	items, err := s.q.ListPermissionCatalogItems(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionCatalogItem), nil
}

// UpsertMenuItem 從儲存層處理 upsert 選單項。
func (s *Store) UpsertMenuItem(execCtx context.Context, v domain.MenuItem) error {
	_, err := s.q.UpsertMenuItem(tenantContext(execCtx, v.TenantID), sqlc.UpsertMenuItemParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Key:       v.Key,
		Label:     v.Label,
		Path:      v.Path,
		Icon:      v.Icon,
		ParentKey: v.ParentKey,
		SortOrder: int32(v.SortOrder),
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

// ListMenuItems 從儲存層列出選單項。
func (s *Store) ListMenuItems(execCtx context.Context, tenantID string) ([]domain.MenuItem, error) {
	items, err := s.q.ListMenuItems(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromMenuItem), nil
}

// UpsertPermissionPackage 從儲存層處理 upsert 權限包。
func (s *Store) UpsertPermissionPackage(execCtx context.Context, v domain.PermissionPackage) error {
	_, err := s.q.UpsertPermissionPackage(execCtx, sqlc.UpsertPermissionPackageParams{
		ID:              v.ID,
		ApplicationCode: v.ApplicationCode,
		Version:         v.Version,
		Status:          string(v.Status),
		Content:         mustJSON(v.Content),
		Checksum:        v.Checksum,
		CreatedAt:       timestamptz(v.CreatedAt),
		PublishedAt:     nullableTimestamptz(v.PublishedAt),
	})
	return err
}

// UpdatePermissionPackageStatus 從儲存層更新權限包狀態。
func (s *Store) UpdatePermissionPackageStatus(execCtx context.Context, id string, status domain.PermissionPackageStatus, publishedAt *time.Time) (domain.PermissionPackage, bool, error) {
	v, err := s.q.UpdatePermissionPackageStatus(execCtx, sqlc.UpdatePermissionPackageStatusParams{
		ID:          id,
		Status:      string(status),
		PublishedAt: nullableTimestamptz(publishedAt),
	})
	if isNotFound(err) {
		return domain.PermissionPackage{}, false, nil
	}
	if err != nil {
		return domain.PermissionPackage{}, false, err
	}
	return fromPermissionPackage(v), true, nil
}

// GetPermissionPackage 從儲存層取得權限包。
func (s *Store) GetPermissionPackage(execCtx context.Context, id string) (domain.PermissionPackage, bool, error) {
	v, err := s.q.GetPermissionPackage(execCtx, id)
	if isNotFound(err) {
		return domain.PermissionPackage{}, false, nil
	}
	if err != nil {
		return domain.PermissionPackage{}, false, err
	}
	return fromPermissionPackage(v), true, nil
}

// GetPermissionPackageByApplicationVersion 從儲存層取得權限包 by application/version。
func (s *Store) GetPermissionPackageByApplicationVersion(execCtx context.Context, applicationCode, version string) (domain.PermissionPackage, bool, error) {
	v, err := s.q.GetPermissionPackageByApplicationVersion(execCtx, sqlc.GetPermissionPackageByApplicationVersionParams{ApplicationCode: applicationCode, Version: version})
	if isNotFound(err) {
		return domain.PermissionPackage{}, false, nil
	}
	if err != nil {
		return domain.PermissionPackage{}, false, err
	}
	return fromPermissionPackage(v), true, nil
}

// ListPermissionPackages 從儲存層列出權限包。
func (s *Store) ListPermissionPackages(execCtx context.Context) ([]domain.PermissionPackage, error) {
	items, err := s.q.ListPermissionPackages(execCtx)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionPackage), nil
}

// UpsertPermissionSetTemplate 從儲存層處理 upsert 權限集合模板。
func (s *Store) UpsertPermissionSetTemplate(execCtx context.Context, v domain.PermissionSetTemplate) error {
	_, err := s.q.UpsertPermissionSetTemplate(execCtx, sqlc.UpsertPermissionSetTemplateParams{
		ID:          v.ID,
		PackageID:   v.PackageID,
		TemplateKey: v.TemplateKey,
		Name:        v.Name,
		Content:     mustJSON(v.Content),
		Version:     v.Version,
	})
	return err
}

// ListPermissionSetTemplates 從儲存層列出權限集合模板。
func (s *Store) ListPermissionSetTemplates(execCtx context.Context, packageID string) ([]domain.PermissionSetTemplate, error) {
	items, err := s.q.ListPermissionSetTemplates(execCtx, packageID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSetTemplate), nil
}

// UpsertUserGroupTemplate 從儲存層處理 upsert 使用者羣組模板。
func (s *Store) UpsertUserGroupTemplate(execCtx context.Context, v domain.UserGroupTemplate) error {
	_, err := s.q.UpsertUserGroupTemplate(execCtx, sqlc.UpsertUserGroupTemplateParams{
		ID:          v.ID,
		PackageID:   v.PackageID,
		TemplateKey: v.TemplateKey,
		Name:        v.Name,
		Content:     mustJSON(v.Content),
		Version:     v.Version,
	})
	return err
}

// ListUserGroupTemplates 從儲存層列出使用者羣組模板。
func (s *Store) ListUserGroupTemplates(execCtx context.Context, packageID string) ([]domain.UserGroupTemplate, error) {
	items, err := s.q.ListUserGroupTemplates(execCtx, packageID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromUserGroupTemplate), nil
}

// UpsertAssumableRoleTemplate 從儲存層處理 upsert 可承擔角色模板。
func (s *Store) UpsertAssumableRoleTemplate(execCtx context.Context, v domain.AssumableRoleTemplate) error {
	_, err := s.q.UpsertAssumableRoleTemplate(execCtx, sqlc.UpsertAssumableRoleTemplateParams{
		ID:          v.ID,
		PackageID:   v.PackageID,
		TemplateKey: v.TemplateKey,
		Name:        v.Name,
		Content:     mustJSON(v.Content),
		Version:     v.Version,
	})
	return err
}

// ListAssumableRoleTemplates 從儲存層列出可承擔角色模板。
func (s *Store) ListAssumableRoleTemplates(execCtx context.Context, packageID string) ([]domain.AssumableRoleTemplate, error) {
	items, err := s.q.ListAssumableRoleTemplates(execCtx, packageID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAssumableRoleTemplate), nil
}

// UpsertPermissionPackageImport 從儲存層處理 upsert 權限包導入記錄。
func (s *Store) UpsertPermissionPackageImport(execCtx context.Context, v domain.PermissionPackageImport) error {
	_, err := s.q.UpsertPermissionPackageImport(tenantContext(execCtx, v.TenantID), sqlc.UpsertPermissionPackageImportParams{
		ID:            v.ID,
		TenantID:      v.TenantID,
		PackageID:     v.PackageID,
		Version:       v.Version,
		ImportedAt:    timestamptz(v.ImportedAt),
		ImportedBy:    v.ImportedBy,
		ArtifactIDMap: mustJSON(v.ArtifactIDMap),
	})
	return err
}

// GetPermissionPackageImport 從儲存層取得權限包導入記錄。
func (s *Store) GetPermissionPackageImport(execCtx context.Context, tenantID, packageID, version string) (domain.PermissionPackageImport, bool, error) {
	v, err := s.q.GetPermissionPackageImport(tenantContext(execCtx, tenantID), sqlc.GetPermissionPackageImportParams{
		TenantID:  tenantID,
		PackageID: packageID,
		Version:   version,
	})
	if isNotFound(err) {
		return domain.PermissionPackageImport{}, false, nil
	}
	if err != nil {
		return domain.PermissionPackageImport{}, false, err
	}
	return fromPermissionPackageImport(v), true, nil
}

// ListPermissionPackageImports 從儲存層列出租戶權限包導入記錄。
func (s *Store) ListPermissionPackageImports(execCtx context.Context, tenantID string) ([]domain.PermissionPackageImport, error) {
	items, err := s.q.ListPermissionPackageImports(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionPackageImport), nil
}

// UpsertPermissionSetAssignment 從儲存層處理 upsert 權限集合指派。
func (s *Store) UpsertPermissionSetAssignment(execCtx context.Context, v domain.PermissionSetAssignment) error {
	_, err := s.q.UpsertAuthzPermissionSetAssignment(execCtx, sqlc.UpsertAuthzPermissionSetAssignmentParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		PrincipalType:   v.PrincipalType,
		PrincipalID:     v.PrincipalID,
		PermissionSetID: v.PermissionSetID,
		Effect:          v.Effect,
		DataScopeID:     v.DataScopeID,
		ConditionID:     v.ConditionID,
		StartsAt:        nullableTimestamptz(v.StartsAt),
		ExpiresAt:       nullableTimestamptz(v.ExpiresAt),
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	return err
}

// DeletePermissionSetAssignment 從儲存層刪除權限集合指派。
func (s *Store) DeletePermissionSetAssignment(execCtx context.Context, tenantID, id string) (domain.PermissionSetAssignment, bool, error) {
	v, err := s.q.DeleteAuthzPermissionSetAssignment(tenantContext(execCtx, tenantID), sqlc.DeleteAuthzPermissionSetAssignmentParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.PermissionSetAssignment{}, false, nil
	}
	if err != nil {
		return domain.PermissionSetAssignment{}, false, err
	}
	return fromPermissionSetAssignment(v), true, nil
}

// ListPermissionSetAssignments 從儲存層列出權限集合指派。
func (s *Store) ListPermissionSetAssignments(execCtx context.Context, tenantID string) ([]domain.PermissionSetAssignment, error) {
	items, err := s.q.ListAuthzPermissionSetAssignments(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSetAssignment), nil
}

// ListPermissionSetAssignmentsForPrincipal 從儲存層列出權限集合指派 for principal。
func (s *Store) ListPermissionSetAssignmentsForPrincipal(execCtx context.Context, tenantID, principalType, principalID string) ([]domain.PermissionSetAssignment, error) {
	items, err := s.q.ListAuthzPermissionSetAssignmentsForPrincipal(execCtx, sqlc.ListAuthzPermissionSetAssignmentsForPrincipalParams{
		TenantID:      tenantID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSetAssignment), nil
}

// UpsertDataScope 從儲存層處理 upsert 資料範圍。
func (s *Store) UpsertDataScope(execCtx context.Context, v domain.DataScope) error {
	_, err := s.q.UpsertAuthzDataScope(execCtx, sqlc.UpsertAuthzDataScopeParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		ScopeType: v.ScopeType,
		Column6:   mustJSON(v.Params),
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

// GetDataScope 從儲存層取得資料範圍。
func (s *Store) GetDataScope(execCtx context.Context, tenantID, id string) (domain.DataScope, bool, error) {
	v, err := s.q.GetAuthzDataScope(execCtx, sqlc.GetAuthzDataScopeParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.DataScope{}, false, nil
	}
	if err != nil {
		return domain.DataScope{}, false, err
	}
	return fromDataScope(v), true, nil
}

// ListDataScopes 從儲存層列出資料範圍。
func (s *Store) ListDataScopes(execCtx context.Context, tenantID string) ([]domain.DataScope, error) {
	items, err := s.q.ListAuthzDataScopes(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromDataScope), nil
}

// UpdateDataScope 從儲存層更新資料範圍。
func (s *Store) UpdateDataScope(execCtx context.Context, v domain.DataScope) error {
	_, err := s.q.UpdateAuthzDataScope(tenantContext(execCtx, v.TenantID), sqlc.UpdateAuthzDataScopeParams{
		TenantID:  v.TenantID,
		ID:        v.ID,
		Code:      v.Code,
		Name:      v.Name,
		ScopeType: v.ScopeType,
		Column6:   mustJSON(v.Params),
	})
	return err
}

// DeleteDataScope 從儲存層刪除資料範圍。
func (s *Store) DeleteDataScope(execCtx context.Context, tenantID, id string) (domain.DataScope, bool, error) {
	v, err := s.q.DeleteAuthzDataScope(tenantContext(execCtx, tenantID), sqlc.DeleteAuthzDataScopeParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.DataScope{}, false, nil
	}
	if err != nil {
		return domain.DataScope{}, false, err
	}
	return fromDataScope(v), true, nil
}

// UpsertFieldPolicy 從儲存層處理 upsert 欄位政策。
func (s *Store) UpsertFieldPolicy(execCtx context.Context, v domain.FieldPolicy) error {
	_, err := s.q.UpsertAuthzFieldPolicy(execCtx, sqlc.UpsertAuthzFieldPolicyParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		ApplicationCode: v.ApplicationCode,
		ResourceType:    v.ResourceType,
		FieldName:       v.FieldName,
		Effect:          v.Effect,
		MaskStrategy:    v.MaskStrategy,
		PermissionID:    v.PermissionID,
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	return err
}

// GetFieldPolicy 從儲存層取得欄位政策。
func (s *Store) GetFieldPolicy(execCtx context.Context, tenantID, id string) (domain.FieldPolicy, bool, error) {
	v, err := s.q.GetAuthzFieldPolicy(tenantContext(execCtx, tenantID), sqlc.GetAuthzFieldPolicyParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FieldPolicy{}, false, nil
	}
	if err != nil {
		return domain.FieldPolicy{}, false, err
	}
	return fromFieldPolicy(v), true, nil
}

// ListFieldPolicies 從儲存層列出欄位政策。
func (s *Store) ListFieldPolicies(execCtx context.Context, tenantID, applicationCode, resourceType string) ([]domain.FieldPolicy, error) {
	items, err := s.q.ListAuthzFieldPolicies(tenantContext(execCtx, tenantID), sqlc.ListAuthzFieldPoliciesParams{
		TenantID: tenantID,
		Column2:  applicationCode,
		Column3:  resourceType,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFieldPolicy), nil
}

// DeleteFieldPolicy 從儲存層刪除欄位政策。
func (s *Store) DeleteFieldPolicy(execCtx context.Context, tenantID, id string) (domain.FieldPolicy, bool, error) {
	v, err := s.q.DeleteAuthzFieldPolicy(tenantContext(execCtx, tenantID), sqlc.DeleteAuthzFieldPolicyParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FieldPolicy{}, false, nil
	}
	if err != nil {
		return domain.FieldPolicy{}, false, err
	}
	return fromFieldPolicy(v), true, nil
}

// UpsertAssumableRole 從儲存層處理 upsert assumable 角色。
func (s *Store) UpsertAssumableRole(execCtx context.Context, v domain.AssumableRole) error {
	duration := v.SessionDurationSeconds
	if duration <= 0 {
		duration = 28800
	}
	_, err := s.q.UpsertAssumableRole(execCtx, sqlc.UpsertAssumableRoleParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		Description:            v.Description,
		PermissionSetIds:       textArray(v.PermissionSetIDs),
		Trusted:                v.Trusted,
		Column7:                mustJSON(v.TrustPolicy),
		Column8:                mustJSON(v.PermissionBoundary),
		SessionDurationSeconds: int32(duration),
		SourceTemplateKey:      v.SourceTemplateKey,
		SourcePackageVersion:   v.SourcePackageVersion,
		CreatedAt:              timestamptz(v.CreatedAt),
	})
	return err
}

// GetAssumableRole 從儲存層取得 assumable 角色。
func (s *Store) GetAssumableRole(execCtx context.Context, tenantID, id string) (domain.AssumableRole, bool, error) {
	v, err := s.q.GetAssumableRole(execCtx, sqlc.GetAssumableRoleParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AssumableRole{}, false, nil
	}
	if err != nil {
		return domain.AssumableRole{}, false, err
	}
	return fromAssumableRole(v), true, nil
}

// ListAssumableRoles 從儲存層列出 assumable 角色。
func (s *Store) ListAssumableRoles(execCtx context.Context, tenantID string) ([]domain.AssumableRole, error) {
	items, err := s.q.ListAssumableRoles(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAssumableRole), nil
}

// DeleteAssumableRole 從儲存層刪除 assumable 角色。
func (s *Store) DeleteAssumableRole(execCtx context.Context, tenantID, id string) (domain.AssumableRole, bool, error) {
	if err := s.q.DeleteAuthzAssumableRoleSessionsForRole(execCtx, sqlc.DeleteAuthzAssumableRoleSessionsForRoleParams{
		TenantID:        tenantID,
		AssumableRoleID: id,
	}); err != nil {
		return domain.AssumableRole{}, false, err
	}
	v, err := s.q.DeleteAssumableRole(tenantContext(execCtx, tenantID), sqlc.DeleteAssumableRoleParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AssumableRole{}, false, nil
	}
	if err != nil {
		return domain.AssumableRole{}, false, err
	}
	return fromAssumableRole(v), true, nil
}

// UpsertAssumableRoleSession 從儲存層處理 upsert assumable 角色 session。
func (s *Store) UpsertAssumableRoleSession(execCtx context.Context, v domain.AssumableRoleSession) error {
	_, err := s.q.CreateAuthzAssumableRoleSession(execCtx, sqlc.CreateAuthzAssumableRoleSessionParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		AccountID:       v.AccountID,
		AssumableRoleID: v.AssumableRoleID,
		Column5:         mustJSON(v.SessionPolicy),
		ExpiresAt:       timestamptz(v.ExpiresAt),
		RevokedAt:       nullableTimestamptz(v.RevokedAt),
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	return err
}

// GetAssumableRoleSession 取得 session 原始狀態，供服務層區分失效原因並執行 ownership 驗證。
func (s *Store) GetAssumableRoleSession(execCtx context.Context, tenantID, id string) (domain.AssumableRoleSession, bool, error) {
	v, err := s.q.GetAuthzAssumableRoleSession(execCtx, sqlc.GetAuthzAssumableRoleSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AssumableRoleSession{}, false, nil
	}
	if err != nil {
		return domain.AssumableRoleSession{}, false, err
	}
	return fromAssumableRoleSession(v), true, nil
}

// GetActiveAssumableRoleSession 從儲存層取得啟用中 assumable 角色 session。
func (s *Store) GetActiveAssumableRoleSession(execCtx context.Context, tenantID, id string) (domain.AssumableRoleSession, bool, error) {
	v, err := s.q.GetActiveAuthzAssumableRoleSession(execCtx, sqlc.GetActiveAuthzAssumableRoleSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AssumableRoleSession{}, false, nil
	}
	if err != nil {
		return domain.AssumableRoleSession{}, false, err
	}
	return fromAssumableRoleSession(v), true, nil
}

// RevokeAssumableRoleSession 僅撤銷同租戶、同帳號且尚未撤銷的 session。
func (s *Store) RevokeAssumableRoleSession(execCtx context.Context, tenantID, accountID, id string, revokedAt time.Time) (domain.AssumableRoleSession, bool, error) {
	v, err := s.q.RevokeAuthzAssumableRoleSession(execCtx, sqlc.RevokeAuthzAssumableRoleSessionParams{
		RevokedAt: timestamptz(revokedAt),
		TenantID:  tenantID,
		AccountID: accountID,
		ID:        id,
	})
	if isNotFound(err) {
		return domain.AssumableRoleSession{}, false, nil
	}
	if err != nil {
		return domain.AssumableRoleSession{}, false, err
	}
	return fromAssumableRoleSession(v), true, nil
}

// ListActiveAssumableRoleSessionsForRole 從儲存層列出角色啟用中 session。
func (s *Store) ListActiveAssumableRoleSessionsForRole(execCtx context.Context, tenantID, roleID string) ([]domain.AssumableRoleSession, error) {
	items, err := s.q.ListActiveAuthzAssumableRoleSessionsForRole(execCtx, sqlc.ListActiveAuthzAssumableRoleSessionsForRoleParams{
		TenantID:        tenantID,
		AssumableRoleID: roleID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAssumableRoleSession), nil
}

// UpsertOrgUnit 從儲存層處理 upsert 組織單位。
func (s *Store) UpsertOrgUnit(execCtx context.Context, v domain.OrgUnit) error {
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	_, err := s.q.UpsertOrgUnit(execCtx, sqlc.UpsertOrgUnitParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		NameEn:    v.NameEN,
		ParentID:  v.ParentID,
		Path:      utils.CopyStrings(v.Path),
		Source:    v.Source,
		Closed:    v.Closed,
		CreatedAt: timestamptz(v.CreatedAt),
		UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

// UpdateOrgUnitOrgChartVisibility 更新組織單位在組織圖預覽中的可見性。
func (s *Store) UpdateOrgUnitOrgChartVisibility(execCtx context.Context, tenantID, id string, showInOrgChart bool, updatedAt time.Time) error {
	return s.q.UpdateOrgUnitOrgChartVisibility(execCtx, sqlc.UpdateOrgUnitOrgChartVisibilityParams{
		TenantID:       tenantID,
		ID:             id,
		ShowInOrgChart: showInOrgChart,
		UpdatedAt:      timestamptz(updatedAt),
	})
}

// GetOrgUnit 從儲存層取得組織單位。
func (s *Store) GetOrgUnit(execCtx context.Context, tenantID, id string) (domain.OrgUnit, bool, error) {
	v, err := s.q.GetOrgUnit(execCtx, sqlc.GetOrgUnitParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.OrgUnit{}, false, nil
	}
	if err != nil {
		return domain.OrgUnit{}, false, err
	}
	return fromOrgUnit(v), true, nil
}

// ListOrgUnits 從儲存層列出組織單位。
func (s *Store) ListOrgUnits(execCtx context.Context, tenantID string) ([]domain.OrgUnit, error) {
	items, err := s.q.ListOrgUnits(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOrgUnit), nil
}

// UpsertPosition 從儲存層處理 upsert 崗位。
func (s *Store) UpsertPosition(execCtx context.Context, v domain.Position) error {
	_, err := s.q.UpsertPosition(execCtx, sqlc.UpsertPositionParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		Code:        v.Code,
		Name:        v.Name,
		NameEn:      v.NameEN,
		Level:       v.Level,
		Status:      v.Status,
		Description: v.Description,
		Source:      v.Source,
		CreatedAt:   timestamptz(v.CreatedAt),
		UpdatedAt:   timestamptz(v.UpdatedAt),
	})
	return err
}

// GetPosition 從儲存層取得崗位。
func (s *Store) GetPosition(execCtx context.Context, tenantID, id string) (domain.Position, bool, error) {
	v, err := s.q.GetPosition(execCtx, sqlc.GetPositionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.Position{}, false, nil
	}
	if err != nil {
		return domain.Position{}, false, err
	}
	return fromPosition(v), true, nil
}

// GetPositionByCode 從儲存層取得崗位 by code。
func (s *Store) GetPositionByCode(execCtx context.Context, tenantID, code string) (domain.Position, bool, error) {
	v, err := s.q.GetPositionByCode(execCtx, sqlc.GetPositionByCodeParams{TenantID: tenantID, Lower: code})
	if isNotFound(err) {
		return domain.Position{}, false, nil
	}
	if err != nil {
		return domain.Position{}, false, err
	}
	return fromPosition(v), true, nil
}

// GetPositionByName 從儲存層取得崗位 by name。
func (s *Store) GetPositionByName(execCtx context.Context, tenantID, name string) (domain.Position, bool, error) {
	v, err := s.q.GetPositionByName(execCtx, sqlc.GetPositionByNameParams{TenantID: tenantID, Lower: name})
	if isNotFound(err) {
		return domain.Position{}, false, nil
	}
	if err != nil {
		return domain.Position{}, false, err
	}
	return fromPosition(v), true, nil
}

// ListPositions 從儲存層列出崗位。
func (s *Store) ListPositions(execCtx context.Context, tenantID string) ([]domain.Position, error) {
	items, err := s.q.ListPositions(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPosition), nil
}

// UpsertEmployee 從儲存層處理 upsert 員工。
func (s *Store) UpsertEmployee(execCtx context.Context, v domain.Employee) error {
	_, err := s.q.UpsertEmployee(execCtx, sqlc.UpsertEmployeeParams{
		ID:                    v.ID,
		TenantID:              v.TenantID,
		EmployeeNo:            v.EmployeeNo,
		Name:                  v.Name,
		CompanyEmail:          v.CompanyEmail,
		PersonalEmail:         v.PersonalEmail,
		Phone:                 v.Phone,
		OrgUnitID:             v.OrgUnitID,
		AccountID:             v.AccountID,
		ManagerEmployeeID:     nullableText(v.ManagerEmployeeID),
		PositionID:            v.PositionID,
		Position:              v.Position,
		Category:              v.Category,
		Status:                v.Status,
		EmploymentStatus:      v.EmploymentStatus,
		HireDate:              nullableTimestamptz(v.HireDate),
		ResignDate:            nullableTimestamptz(v.ResignDate),
		BasicInfo:             mustJSON(v.BasicInfo),
		EmploymentInfo:        mustJSON(v.EmploymentInfo),
		EducationMilitaryInfo: mustJSON(v.EducationMilitaryInfo),
		ContactInfo:           mustJSON(v.ContactInfo),
		InsuranceInfo:         mustJSON(v.InsuranceInfo),
		InternalExperiences:   mustJSON(v.InternalExperiences),
		CreatedAt:             timestamptz(v.CreatedAt),
		UpdatedAt:             timestamptz(v.UpdatedAt),
	})
	return err
}

// UpdateEmployeeOrgChartVisibility 更新員工在組織圖預覽中的可見性。
func (s *Store) UpdateEmployeeOrgChartVisibility(execCtx context.Context, tenantID, id string, showInOrgChart bool, updatedAt time.Time) error {
	return s.q.UpdateEmployeeOrgChartVisibility(execCtx, sqlc.UpdateEmployeeOrgChartVisibilityParams{
		TenantID:       tenantID,
		ID:             id,
		ShowInOrgChart: showInOrgChart,
		UpdatedAt:      timestamptz(updatedAt),
	})
}

// GetEmployee 從儲存層取得員工。
func (s *Store) GetEmployee(execCtx context.Context, tenantID, id string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployee(execCtx, sqlc.GetEmployeeParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

// GetEmployeeByEmployeeNo 從儲存層取得員工 by 員工 no。
func (s *Store) GetEmployeeByEmployeeNo(execCtx context.Context, tenantID, employeeNo string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByEmployeeNo(execCtx, sqlc.GetEmployeeByEmployeeNoParams{TenantID: tenantID, EmployeeNo: employeeNo})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

// GetEmployeeByCompanyEmail 從儲存層取得員工 by 公司 email。
func (s *Store) GetEmployeeByCompanyEmail(execCtx context.Context, tenantID, companyEmail string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByCompanyEmail(execCtx, sqlc.GetEmployeeByCompanyEmailParams{TenantID: tenantID, CompanyEmail: companyEmail})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

// GetEmployeeByPersonalEmail 從儲存層取得員工 by personal email。
func (s *Store) GetEmployeeByPersonalEmail(execCtx context.Context, tenantID, personalEmail string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByPersonalEmail(execCtx, sqlc.GetEmployeeByPersonalEmailParams{TenantID: tenantID, PersonalEmail: personalEmail})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

// GetEmployeeByAccountID 從儲存層取得員工 by 帳號 ID。
func (s *Store) GetEmployeeByAccountID(execCtx context.Context, tenantID, accountID string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByAccountID(execCtx, sqlc.GetEmployeeByAccountIDParams{TenantID: tenantID, AccountID: accountID})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

// GetEmployeeByBasicInfoField 從儲存層取得員工 by 基本 info 欄位。
func (s *Store) GetEmployeeByBasicInfoField(execCtx context.Context, tenantID, fieldName, fieldValue string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByBasicInfoField(execCtx, sqlc.GetEmployeeByBasicInfoFieldParams{TenantID: tenantID, FieldName: fieldName, FieldValue: fieldValue})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

// ListEmployees 從儲存層列出員工。
func (s *Store) ListEmployees(execCtx context.Context, tenantID string) ([]domain.Employee, error) {
	items, err := s.q.ListEmployees(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromEmployee), nil
}

// ListEmployeesByQuery 從儲存層列出員工 by 查詢。
func (s *Store) ListEmployeesByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, error) {
	items, err := s.q.ListEmployeesFiltered(execCtx, sqlc.ListEmployeesFilteredParams{
		TenantID:         tenantID,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		Sort:             query.Sort,
	})
	if err != nil {
		return nil, err
	}
	return filterPostgresEmployeesByScope(mapSlice(items, fromEmployee), query.Scope), nil
}

// ListEmployeePageByQuery 從儲存層列出員工分頁 by 查詢。
func (s *Store) ListEmployeePageByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, int, error) {
	if employeeQueryHasScope(query) {
		items, err := s.ListEmployeesByQuery(execCtx, tenantID, query)
		if err != nil {
			return nil, 0, err
		}
		page, pageSize := normalizePostgresEmployeePage(query)
		return paginatePostgresEmployees(items, page, pageSize), len(items), nil
	}
	params := sqlc.CountEmployeesFilteredParams{
		TenantID:         tenantID,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
	}
	total, err := s.q.CountEmployeesFiltered(execCtx, params)
	if err != nil {
		return nil, 0, err
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	items, err := s.q.ListEmployeesFilteredPage(execCtx, sqlc.ListEmployeesFilteredPageParams{
		TenantID:         params.TenantID,
		Keyword:          params.Keyword,
		DepartmentID:     params.DepartmentID,
		EmploymentStatus: params.EmploymentStatus,
		Category:         params.Category,
		Sort:             query.Sort,
		OffsetCount:      int32((page - 1) * pageSize),
		LimitCount:       int32(pageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromEmployee), int(total), nil
}

// CountEmployeesByQuery 從儲存層處理 count 員工 by 查詢。
func (s *Store) CountEmployeesByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) (int, error) {
	if employeeQueryHasScope(query) {
		items, err := s.ListEmployeesByQuery(execCtx, tenantID, query)
		if err != nil {
			return 0, err
		}
		return len(items), nil
	}
	total, err := s.q.CountEmployeesFiltered(execCtx, sqlc.CountEmployeesFilteredParams{
		TenantID:         tenantID,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
	})
	if err != nil {
		return 0, err
	}
	return int(total), nil
}

// employeeQueryHasScope 處理員工查詢 has 範圍。
func employeeQueryHasScope(query domain.EmployeeQuery) bool {
	return query.Scope.DenyAll || len(query.Scope.EmployeeIDs) > 0 || len(query.Scope.OrgUnitIDs) > 0 || len(query.Scope.Statuses) > 0
}

// filterPostgresEmployeesByScope 處理篩選 Postgres 員工 by 範圍。
func filterPostgresEmployeesByScope(items []domain.Employee, scope domain.EmployeeScopeConstraint) []domain.Employee {
	if scope.DenyAll {
		return []domain.Employee{}
	}
	employeeAllowed := postgresStringSet(scope.EmployeeIDs)
	orgAllowed := postgresStringSet(scope.OrgUnitIDs)
	statusAllowed := postgresStringSet(scope.Statuses)
	if len(employeeAllowed) == 0 && len(orgAllowed) == 0 && len(statusAllowed) == 0 {
		return items
	}
	out := make([]domain.Employee, 0, len(items))
	for _, item := range items {
		status := utils.FirstNonEmpty(item.EmploymentStatus, item.Status)
		if len(employeeAllowed) > 0 {
			if _, ok := employeeAllowed[item.ID]; !ok {
				continue
			}
		}
		if len(orgAllowed) > 0 {
			if _, ok := orgAllowed[item.OrgUnitID]; !ok {
				continue
			}
		}
		if len(statusAllowed) > 0 {
			if _, ok := statusAllowed[status]; !ok {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

// postgresStringSet 處理 Postgres 字串集合。
func postgresStringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// normalizePostgresEmployeePage 正規化Postgres 員工分頁。
func normalizePostgresEmployeePage(query domain.EmployeeQuery) (int, int) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	return page, pageSize
}

// paginatePostgresEmployees 處理 paginate Postgres 員工。
func paginatePostgresEmployees(items []domain.Employee, page, pageSize int) []domain.Employee {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []domain.Employee{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	out := make([]domain.Employee, end-start)
	copy(out, items[start:end])
	return out
}

// NextEmployeeNo 從儲存層處理 next 員工 no。
func (s *Store) NextEmployeeNo(execCtx context.Context, tenantID, prefix string) (string, error) {
	nextSeq, err := s.q.NextEmployeeNoSequence(execCtx, sqlc.NextEmployeeNoSequenceParams{
		TenantID:    tenantID,
		Prefix:      prefix,
		InitialNext: 1,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%03d", prefix, nextSeq), nil
}

// UpsertEmployeeImportSession 從儲存層處理 upsert 員工 import session。
func (s *Store) UpsertEmployeeImportSession(execCtx context.Context, v domain.EmployeeImportSession) error {
	_, err := s.q.UpsertEmployeeImportSession(execCtx, sqlc.UpsertEmployeeImportSessionParams{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Filename:             v.Filename,
		ObjectProvider:       v.ObjectProvider,
		ObjectBucket:         v.ObjectBucket,
		ObjectKey:            v.ObjectKey,
		ContentType:          v.ContentType,
		SizeBytes:            v.SizeBytes,
		Sha256:               v.SHA256,
		Status:               v.Status,
		Rows:                 mustJSON(v.Rows),
		Summary:              mustJSON(v.Summary),
		CreatedByAccountID:   v.CreatedByAccountID,
		ConfirmedByAccountID: v.ConfirmedByAccountID,
		CreatedAt:            timestamptz(v.CreatedAt),
		ExpiresAt:            timestamptz(v.ExpiresAt),
		ConfirmedAt:          nullableTimestamptz(v.ConfirmedAt),
	})
	return err
}

// GetEmployeeImportSession 從儲存層取得員工 import session。
func (s *Store) GetEmployeeImportSession(execCtx context.Context, tenantID, id string) (domain.EmployeeImportSession, bool, error) {
	v, err := s.q.GetEmployeeImportSession(execCtx, sqlc.GetEmployeeImportSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.EmployeeImportSession{}, false, nil
	}
	if err != nil {
		return domain.EmployeeImportSession{}, false, err
	}
	return fromEmployeeImportSession(v), true, nil
}

// UpsertEmploymentContract 從儲存層處理 upsert 員工合約。
func (s *Store) UpsertEmploymentContract(execCtx context.Context, v domain.EmploymentContract) error {
	_, err := s.q.UpsertEmploymentContract(execCtx, sqlc.UpsertEmploymentContractParams{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		ContractType:        v.ContractType,
		ContractNo:          v.ContractNo,
		StartDate:           timestamptz(v.StartDate),
		EndDate:             nullableTimestamptz(v.EndDate),
		Status:              v.Status,
		AttachmentObjectKey: v.AttachmentObjectKey,
		Notes:               v.Notes,
		Version:             v.Version,
		CreatedAt:           timestamptz(v.CreatedAt),
		UpdatedAt:           timestamptz(v.UpdatedAt),
	})
	return err
}

// GetEmploymentContract 從儲存層取得員工合約。
func (s *Store) GetEmploymentContract(execCtx context.Context, tenantID, id string) (domain.EmploymentContract, bool, error) {
	v, err := s.q.GetEmploymentContract(execCtx, sqlc.GetEmploymentContractParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.EmploymentContract{}, false, nil
	}
	if err != nil {
		return domain.EmploymentContract{}, false, err
	}
	return fromEmploymentContract(v), true, nil
}

// ListEmploymentContracts 從儲存層列出員工合約。
func (s *Store) ListEmploymentContracts(execCtx context.Context, tenantID string) ([]domain.EmploymentContract, error) {
	items, err := s.q.ListEmploymentContracts(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromEmploymentContract), nil
}

// ListEmploymentContractsByEmployee 從儲存層列出員工合約 by 員工。
func (s *Store) ListEmploymentContractsByEmployee(execCtx context.Context, tenantID, employeeID string) ([]domain.EmploymentContract, error) {
	items, err := s.q.ListEmploymentContractsByEmployee(tenantContext(execCtx, tenantID), sqlc.ListEmploymentContractsByEmployeeParams{TenantID: tenantID, EmployeeID: employeeID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromEmploymentContract), nil
}

// UpsertAttendancePolicy 從儲存層處理 upsert 考勤政策。
func (s *Store) UpsertAttendancePolicy(execCtx context.Context, v domain.AttendancePolicy) error {
	version := v.Version
	if version <= 0 {
		version = 1
	}
	_, err := s.q.UpsertAttendancePolicy(execCtx, sqlc.UpsertAttendancePolicyParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		WorkTime:           mustJSON(v.WorkTime),
		LeaveTypes:         mustJSON(v.LeaveTypes),
		Version:            int32(version),
		EffectiveFrom:      nullableTimestamptz(v.EffectiveFrom),
		UpdatedByAccountID: v.UpdatedByAccountID,
		CreatedAt:          timestamptz(v.CreatedAt),
		UpdatedAt:          timestamptz(v.UpdatedAt),
	})
	if isNotFound(err) {
		return domain.Conflict("attendance policy was modified concurrently")
	}
	return err
}

// GetAttendancePolicy 從儲存層取得考勤政策。
func (s *Store) GetAttendancePolicy(execCtx context.Context, tenantID string) (domain.AttendancePolicy, bool, error) {
	v, err := s.q.GetAttendancePolicy(tenantContext(execCtx, tenantID), tenantID)
	if isNotFound(err) {
		return domain.AttendancePolicy{}, false, nil
	}
	if err != nil {
		return domain.AttendancePolicy{}, false, err
	}
	return fromAttendancePolicy(v), true, nil
}

// ReplaceEHRMSLeaveTypes replaces one tenant's upstream snapshot inside the caller's transaction.
func (s *Store) ReplaceEHRMSLeaveTypes(execCtx context.Context, tenantID string, items []domain.EHRMSLeaveType, syncedAt time.Time) error {
	execCtx = tenantContext(execCtx, tenantID)
	if _, err := s.db.Exec(execCtx, `DELETE FROM ehrms_leave_types WHERE tenant_id = $1`, tenantID); err != nil {
		return err
	}
	for position, item := range items {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("encode ehrms leave type %q: %w", item.Code, err)
		}
		if _, err := s.db.Exec(execCtx, `
INSERT INTO ehrms_leave_types (tenant_id, code, position, payload, synced_at)
VALUES ($1, $2, $3, $4::jsonb, $5)`, tenantID, item.Code, position, payload, syncedAt); err != nil {
			return err
		}
	}
	return nil
}

// ListEHRMSLeaveTypes returns the persisted snapshot in the exact upstream order.
func (s *Store) ListEHRMSLeaveTypes(execCtx context.Context, tenantID string) ([]domain.EHRMSLeaveType, time.Time, error) {
	rows, err := s.db.Query(tenantContext(execCtx, tenantID), `
SELECT payload, synced_at
FROM ehrms_leave_types
WHERE tenant_id = $1
ORDER BY position`, tenantID)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	items := make([]domain.EHRMSLeaveType, 0)
	var syncedAt time.Time
	for rows.Next() {
		var payload []byte
		var rowSyncedAt time.Time
		if err := rows.Scan(&payload, &rowSyncedAt); err != nil {
			return nil, time.Time{}, err
		}
		var item domain.EHRMSLeaveType
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, time.Time{}, fmt.Errorf("decode persisted ehrms leave type: %w", err)
		}
		items = append(items, item)
		if rowSyncedAt.After(syncedAt) {
			syncedAt = rowSyncedAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	return items, syncedAt, nil
}

// GetLeaveTypeExternalMapping resolves a tenant-specific upstream leave alias.
func (s *Store) GetLeaveTypeExternalMapping(execCtx context.Context, tenantID, source, externalCode string, asOf time.Time) (domain.LeaveTypeExternalMapping, bool, error) {
	v, err := s.q.GetLeaveTypeExternalMapping(tenantContext(execCtx, tenantID), sqlc.GetLeaveTypeExternalMappingParams{
		TenantID:     tenantID,
		Source:       source,
		ExternalCode: externalCode,
		AsOf:         pgtype.Date{Time: asOf, Valid: true},
	})
	if isNotFound(err) {
		return domain.LeaveTypeExternalMapping{}, false, nil
	}
	if err != nil {
		return domain.LeaveTypeExternalMapping{}, false, err
	}
	return domain.LeaveTypeExternalMapping{
		ID:            v.ID,
		TenantID:      v.TenantID,
		Source:        v.Source,
		ExternalCode:  v.ExternalCode,
		LeaveTypeID:   v.LeaveTypeID,
		LeaveTypeCode: v.LeaveTypeCode,
		EffectiveFrom: dateTextFrom(v.EffectiveFrom),
		EffectiveTo:   dateTextFrom(v.EffectiveTo),
		CreatedAt:     timeFrom(v.CreatedAt),
		UpdatedAt:     timeFrom(v.UpdatedAt),
	}, true, nil
}

// ListLeaveTypeExternalMappings returns the mapping history used by the HR admin view.
func (s *Store) ListLeaveTypeExternalMappings(execCtx context.Context, tenantID string) ([]domain.LeaveTypeExternalMapping, error) {
	rows, err := s.q.ListLeaveTypeExternalMappings(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.LeaveTypeExternalMapping, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.LeaveTypeExternalMapping{
			ID: row.ID, TenantID: row.TenantID, Source: row.Source, ExternalCode: row.ExternalCode,
			LeaveTypeID: row.LeaveTypeID, LeaveTypeCode: row.LeaveTypeCode,
			EffectiveFrom: dateTextFrom(row.EffectiveFrom), EffectiveTo: dateTextFrom(row.EffectiveTo),
			CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt),
		})
	}
	return out, nil
}

// LockLeaveTypeExternalMappingKey serializes overlap validation for one normalized upstream code.
func (s *Store) LockLeaveTypeExternalMappingKey(execCtx context.Context, tenantID, source, externalCode string) error {
	tenantID = strings.TrimSpace(tenantID)
	source = strings.ToLower(strings.TrimSpace(source))
	externalCode = strings.ToLower(strings.TrimSpace(externalCode))
	lockKey := fmt.Sprintf(
		"leave-type-external-mapping|%d:%s|%d:%s|%d:%s",
		len(tenantID), tenantID,
		len(source), source,
		len(externalCode), externalCode,
	)
	_, err := s.db.Exec(
		tenantContext(execCtx, tenantID),
		"SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))",
		lockKey,
	)
	return err
}

// UpsertLeaveTypeExternalMapping persists one effective upstream alias.
func (s *Store) UpsertLeaveTypeExternalMapping(execCtx context.Context, mapping domain.LeaveTypeExternalMapping) error {
	tenantCtx := tenantContext(execCtx, mapping.TenantID)
	if err := s.q.EnsureLeaveTypeCatalog(tenantCtx, sqlc.EnsureLeaveTypeCatalogParams{
		ID: mapping.LeaveTypeID, TenantID: mapping.TenantID, Code: mapping.LeaveTypeCode,
		CreatedAt: timestamptz(mapping.CreatedAt), UpdatedAt: timestamptz(mapping.UpdatedAt),
	}); err != nil {
		return err
	}
	effectiveFrom, err := nullableDate(mapping.EffectiveFrom)
	if err != nil {
		return err
	}
	effectiveTo, err := nullableDate(mapping.EffectiveTo)
	if err != nil {
		return err
	}
	return s.q.UpsertLeaveTypeExternalMapping(tenantCtx, sqlc.UpsertLeaveTypeExternalMappingParams{
		ID: mapping.ID, TenantID: mapping.TenantID, Source: mapping.Source, ExternalCode: mapping.ExternalCode,
		LeaveTypeID: mapping.LeaveTypeID, EffectiveFrom: effectiveFrom, EffectiveTo: effectiveTo,
		CreatedAt: timestamptz(mapping.CreatedAt), UpdatedAt: timestamptz(mapping.UpdatedAt),
	})
}

// ExpireLeaveTypeExternalMapping ends one mapping while keeping its audit history.
func (s *Store) ExpireLeaveTypeExternalMapping(execCtx context.Context, tenantID, id, effectiveTo string, updatedAt time.Time) (bool, error) {
	date, err := nullableDate(effectiveTo)
	if err != nil {
		return false, err
	}
	rows, err := s.q.ExpireLeaveTypeExternalMapping(tenantContext(execCtx, tenantID), sqlc.ExpireLeaveTypeExternalMappingParams{
		EffectiveTo: date, UpdatedAt: timestamptz(updatedAt), TenantID: tenantID, ID: id,
	})
	return rows > 0, err
}

// UpsertLeaveTypeSyncIssue groups repeated unknown upstream codes into one issue.
func (s *Store) UpsertLeaveTypeSyncIssue(execCtx context.Context, issue domain.LeaveTypeSyncIssue) error {
	return s.q.UpsertLeaveTypeSyncIssue(tenantContext(execCtx, issue.TenantID), sqlc.UpsertLeaveTypeSyncIssueParams{
		ID: issue.ID, TenantID: issue.TenantID, Source: issue.Source, ExternalCode: issue.ExternalCode,
		IssueCode: issue.IssueCode, Message: issue.Message,
		FirstSeenAt: timestamptz(issue.FirstSeenAt), LastSeenAt: timestamptz(issue.LastSeenAt),
	})
}

// ListOpenLeaveTypeSyncIssues returns unresolved upstream leave codes.
func (s *Store) ListOpenLeaveTypeSyncIssues(execCtx context.Context, tenantID string) ([]domain.LeaveTypeSyncIssue, error) {
	rows, err := s.q.ListOpenLeaveTypeSyncIssues(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.LeaveTypeSyncIssue, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.LeaveTypeSyncIssue{
			ID: row.ID, TenantID: row.TenantID, Source: row.Source, ExternalCode: row.ExternalCode,
			IssueCode: row.IssueCode, Message: row.Message, Occurrences: int(row.Occurrences), Status: row.Status,
			FirstSeenAt: timeFrom(row.FirstSeenAt), LastSeenAt: timeFrom(row.LastSeenAt), ResolvedAt: timePtrFrom(row.ResolvedAt),
		})
	}
	return out, nil
}

// ResolveLeaveTypeSyncIssues closes mapping work after HR provides an alias.
func (s *Store) ResolveLeaveTypeSyncIssues(execCtx context.Context, tenantID, source, externalCode string, resolvedAt time.Time) error {
	return s.q.ResolveLeaveTypeSyncIssues(tenantContext(execCtx, tenantID), sqlc.ResolveLeaveTypeSyncIssuesParams{
		ResolvedAt: timestamptz(resolvedAt), TenantID: tenantID, Source: source, ExternalCode: externalCode,
	})
}

// UpsertLeaveBalance 從儲存層處理 upsert 請假 balance。
func (s *Store) UpsertLeaveBalance(execCtx context.Context, v domain.LeaveBalance) error {
	v.LeaveType = strings.ToLower(strings.TrimSpace(v.LeaveType))
	if strings.TrimSpace(v.LeaveTypeID) == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	source := strings.TrimSpace(v.Source)
	if source == "" {
		source = "legacy"
	}
	remainingHours, err := numericFromFloat64(v.RemainingHours)
	if err != nil {
		return err
	}
	grantedHours, err := numericFromFloat64(v.GrantedHours)
	if err != nil {
		return err
	}
	usedHours, err := numericFromFloat64(v.UsedHours)
	if err != nil {
		return err
	}
	periodStart, err := nullableDate(v.PeriodStart)
	if err != nil {
		return err
	}
	periodEnd, err := nullableDate(v.PeriodEnd)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertLeaveBalance(execCtx, sqlc.UpsertLeaveBalanceParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EmployeeID:     v.EmployeeID,
		LeaveType:      v.LeaveType,
		LeaveTypeID:    v.LeaveTypeID,
		RemainingHours: remainingHours,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		GrantedHours:   grantedHours,
		UsedHours:      usedHours,
		Source:         source,
		PolicyVersion:  int32(v.PolicyVersion),
		ProrateRatio:   float8Ptr(v.ProrateRatio),
		UpdatedAt:      timestamptz(v.UpdatedAt),
	})
	return err
}

// GetLeaveBalance 從儲存層取得請假 balance。
func (s *Store) GetLeaveBalance(execCtx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error) {
	v, err := s.q.GetLeaveBalance(execCtx, sqlc.GetLeaveBalanceParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.LeaveBalance{}, false, nil
	}
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	return fromLeaveBalance(v), true, nil
}

// ListLeaveBalances 從儲存層列出請假 balances。
func (s *Store) ListLeaveBalances(execCtx context.Context, tenantID string) ([]domain.LeaveBalance, error) {
	items, err := s.q.ListLeaveBalances(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveBalance), nil
}

// ReserveLeaveBalance 從儲存層保留請假 balance。
func (s *Store) ReserveLeaveBalance(execCtx context.Context, tenantID, employeeID, leaveType string, hours float64, asOf, updatedAt time.Time) (domain.LeaveBalance, bool, bool, error) {
	leaveType = strings.TrimSpace(leaveType)
	numericHours, err := numericFromFloat64(hours)
	if err != nil {
		return domain.LeaveBalance{}, false, false, err
	}
	v, err := s.q.ReserveLeaveBalance(tenantContext(execCtx, tenantID), sqlc.ReserveLeaveBalanceParams{
		TenantID:   tenantID,
		EmployeeID: employeeID,
		LeaveType:  leaveType,
		Hours:      numericHours,
		AsOf:       pgtype.Date{Time: asOf, Valid: true},
		UpdatedAt:  timestamptz(updatedAt),
	})
	if err == nil {
		return fromLeaveBalance(v), true, true, nil
	}
	if !isNotFound(err) {
		return domain.LeaveBalance{}, false, false, err
	}
	items, listErr := s.q.ListLeaveBalances(tenantContext(execCtx, tenantID), tenantID)
	if listErr != nil {
		return domain.LeaveBalance{}, false, false, listErr
	}
	for _, item := range items {
		balance := fromLeaveBalance(item)
		if balance.EmployeeID == employeeID && strings.EqualFold(balance.LeaveType, strings.TrimSpace(leaveType)) && leaveBalanceIncludesDate(balance, asOf) {
			return balance, false, true, nil
		}
	}
	return domain.LeaveBalance{}, false, false, nil
}

// leaveBalanceIncludesDate checks whether a balance period covers the requested date.
func leaveBalanceIncludesDate(balance domain.LeaveBalance, at time.Time) bool {
	date := at.Format("2006-01-02")
	return (balance.PeriodStart == "" || balance.PeriodStart <= date) && (balance.PeriodEnd == "" || balance.PeriodEnd >= date)
}

// ReleaseLeaveBalanceByID restores the exact balance projection reserved by a request.
func (s *Store) ReleaseLeaveBalanceByID(execCtx context.Context, tenantID, balanceID string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, error) {
	numericHours, err := numericFromFloat64(hours)
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	v, err := s.q.ReleaseLeaveBalanceByID(tenantContext(execCtx, tenantID), sqlc.ReleaseLeaveBalanceByIDParams{
		TenantID: tenantID, BalanceID: balanceID, Hours: numericHours, UpdatedAt: timestamptz(updatedAt),
	})
	if isNotFound(err) {
		return domain.LeaveBalance{}, false, nil
	}
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	return fromLeaveBalance(v), true, nil
}

// ReleaseLeaveBalance 從儲存層釋放請假 balance。
func (s *Store) ReleaseLeaveBalance(execCtx context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, error) {
	leaveType = strings.TrimSpace(leaveType)
	numericHours, err := numericFromFloat64(hours)
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	v, err := s.q.ReleaseLeaveBalance(tenantContext(execCtx, tenantID), sqlc.ReleaseLeaveBalanceParams{
		TenantID:   tenantID,
		EmployeeID: employeeID,
		LeaveType:  leaveType,
		Hours:      numericHours,
		UpdatedAt:  timestamptz(updatedAt),
	})
	if isNotFound(err) {
		return domain.LeaveBalance{}, false, nil
	}
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	return fromLeaveBalance(v), true, nil
}

// UpsertLeaveRequest 從儲存層處理 upsert 請假請求。
func (s *Store) UpsertLeaveRequest(execCtx context.Context, v domain.LeaveRequest) error {
	if strings.TrimSpace(v.LeaveTypeID) == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	ruleSnapshot := v.RuleSnapshot
	if ruleSnapshot == nil {
		ruleSnapshot = map[string]any{}
	}
	evaluationSnapshot := v.EvaluationSnapshot
	if evaluationSnapshot == nil {
		evaluationSnapshot = map[string]any{}
	}
	_, err := s.q.UpsertLeaveRequest(tenantContext(execCtx, v.TenantID), sqlc.UpsertLeaveRequestParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		EmployeeID:         v.EmployeeID,
		LeaveType:          v.LeaveType,
		LeaveTypeID:        v.LeaveTypeID,
		PolicyVersion:      int32(v.PolicyVersion),
		RuleSnapshot:       mustJSON(ruleSnapshot),
		EvaluationSnapshot: mustJSON(evaluationSnapshot),
		StartAt:            timestamptz(v.StartAt),
		EndAt:              timestamptz(v.EndAt),
		Hours:              v.Hours,
		Reason:             v.Reason,
		Status:             v.Status,
		FormInstanceID:     v.FormInstanceID,
		LeaveBalanceID:     nullableText(v.LeaveBalanceID),
		CreatedAt:          timestamptz(v.CreatedAt),
	})
	return err
}

// UpsertLeaveRequestAllocation persists the exact reserved balance bucket.
func (s *Store) UpsertLeaveRequestAllocation(execCtx context.Context, v domain.LeaveRequestAllocation) error {
	reservedHours, err := numericFromFloat64(v.ReservedHours)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertLeaveRequestAllocation(tenantContext(execCtx, v.TenantID), sqlc.UpsertLeaveRequestAllocationParams{
		TenantID:       v.TenantID,
		LeaveRequestID: v.LeaveRequestID,
		LeaveBalanceID: v.LeaveBalanceID,
		ReservedHours:  reservedHours,
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

// GetLeaveRequest 從儲存層取得請假請求。
func (s *Store) GetLeaveRequest(execCtx context.Context, tenantID, id string) (domain.LeaveRequest, bool, error) {
	v, err := s.q.GetLeaveRequest(tenantContext(execCtx, tenantID), sqlc.GetLeaveRequestParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.LeaveRequest{}, false, nil
	}
	if err != nil {
		return domain.LeaveRequest{}, false, err
	}
	return fromLeaveRequest(v), true, nil
}

// GetLeaveRequestByFormInstanceID 從儲存層取得請假請求 by 表單實例 ID。
func (s *Store) GetLeaveRequestByFormInstanceID(execCtx context.Context, tenantID, formInstanceID string) (domain.LeaveRequest, bool, error) {
	v, err := s.q.GetLeaveRequestByFormInstanceID(tenantContext(execCtx, tenantID), sqlc.GetLeaveRequestByFormInstanceIDParams{TenantID: tenantID, FormInstanceID: formInstanceID})
	if isNotFound(err) {
		return domain.LeaveRequest{}, false, nil
	}
	if err != nil {
		return domain.LeaveRequest{}, false, err
	}
	return fromLeaveRequest(v), true, nil
}

// ListLeaveRequests 從儲存層列出請假請求。
func (s *Store) ListLeaveRequests(execCtx context.Context, tenantID string) ([]domain.LeaveRequest, error) {
	items, err := s.q.ListLeaveRequests(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveRequest), nil
}

// ListLeaveRequestsByQuery 從儲存層列出請假請求 by 查詢。
func (s *Store) ListLeaveRequestsByQuery(execCtx context.Context, tenantID string, query domain.LeaveRequestQuery) ([]domain.LeaveRequest, error) {
	params := leaveRequestQueryParams(tenantID, query)
	items, err := s.q.ListLeaveRequestsByQuery(tenantContext(execCtx, tenantID), sqlc.ListLeaveRequestsByQueryParams{
		TenantID:    params.TenantID,
		EmployeeIds: params.EmployeeIds,
		Status:      params.Status,
		FromDate:    params.FromDate,
		ToDate:      params.ToDate,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveRequest), nil
}

// ListLeaveRequestPageByQuery 從儲存層列出請假請求分頁 by 查詢。
func (s *Store) ListLeaveRequestPageByQuery(execCtx context.Context, tenantID string, query domain.LeaveRequestQuery, page domain.PageRequest) ([]domain.LeaveRequest, int, error) {
	page = utils.NormalizePageRequest(page)
	countParams := leaveRequestQueryParams(tenantID, query)
	total, err := s.q.CountLeaveRequestsByQuery(tenantContext(execCtx, tenantID), countParams)
	if err != nil {
		return nil, 0, err
	}
	listParams := sqlc.ListLeaveRequestPageByQueryParams{
		TenantID:    countParams.TenantID,
		EmployeeIds: countParams.EmployeeIds,
		Status:      countParams.Status,
		FromDate:    countParams.FromDate,
		ToDate:      countParams.ToDate,
		Sort:        page.Sort,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	}
	items, err := s.q.ListLeaveRequestPageByQuery(tenantContext(execCtx, tenantID), listParams)
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromLeaveRequest), int(total), nil
}

// UpsertAttendanceWorksite 從儲存層處理 upsert 考勤工作地點。
func (s *Store) UpsertAttendanceWorksite(execCtx context.Context, v domain.AttendanceWorksite) error {
	_, err := s.q.UpsertAttendanceWorksite(execCtx, sqlc.UpsertAttendanceWorksiteParams{
		ID:           v.ID,
		TenantID:     v.TenantID,
		Name:         v.Name,
		Address:      v.Address,
		Latitude:     v.Latitude,
		Longitude:    v.Longitude,
		RadiusMeters: int32(v.RadiusMeters),
		Status:       v.Status,
		CreatedAt:    timestamptz(v.CreatedAt),
		UpdatedAt:    timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAttendanceWorksite 從儲存層取得考勤工作地點。
func (s *Store) GetAttendanceWorksite(execCtx context.Context, tenantID, id string) (domain.AttendanceWorksite, bool, error) {
	v, err := s.q.GetAttendanceWorksite(execCtx, sqlc.GetAttendanceWorksiteParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceWorksite{}, false, nil
	}
	if err != nil {
		return domain.AttendanceWorksite{}, false, err
	}
	return fromAttendanceWorksite(v), true, nil
}

// ListAttendanceWorksites 從儲存層列出考勤 worksites。
func (s *Store) ListAttendanceWorksites(execCtx context.Context, tenantID string) ([]domain.AttendanceWorksite, error) {
	items, err := s.q.ListAttendanceWorksites(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceWorksite), nil
}

// UpsertAttendanceShift 從儲存層處理 upsert 考勤班別。
func (s *Store) UpsertAttendanceShift(execCtx context.Context, v domain.AttendanceShift) error {
	_, err := s.q.UpsertAttendanceShift(execCtx, sqlc.UpsertAttendanceShiftParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		ClockInStart:           v.ClockInStart,
		ClockInEnd:             v.ClockInEnd,
		ClockOutStart:          v.ClockOutStart,
		ClockOutEnd:            v.ClockOutEnd,
		LateGraceMinutes:       int32(v.LateGraceMinutes),
		EarlyLeaveGraceMinutes: int32(v.EarlyLeaveGraceMinutes),
		Status:                 v.Status,
		CreatedAt:              timestamptz(v.CreatedAt),
		UpdatedAt:              timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAttendanceShift 從儲存層取得考勤班別。
func (s *Store) GetAttendanceShift(execCtx context.Context, tenantID, id string) (domain.AttendanceShift, bool, error) {
	v, err := s.q.GetAttendanceShift(execCtx, sqlc.GetAttendanceShiftParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceShift{}, false, nil
	}
	if err != nil {
		return domain.AttendanceShift{}, false, err
	}
	return fromAttendanceShift(v), true, nil
}

// ListAttendanceShifts 從儲存層列出考勤 shifts。
func (s *Store) ListAttendanceShifts(execCtx context.Context, tenantID string) ([]domain.AttendanceShift, error) {
	items, err := s.q.ListAttendanceShifts(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceShift), nil
}

// UpsertAttendanceShiftAssignment 儲存員工班別指派。
func (s *Store) UpsertAttendanceShiftAssignment(execCtx context.Context, v domain.AttendanceShiftAssignment) error {
	_, err := s.q.UpsertAttendanceShiftAssignment(execCtx, sqlc.UpsertAttendanceShiftAssignmentParams{ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, ShiftID: v.ShiftID, WorksiteID: v.WorksiteID, EffectiveFrom: timestamptz(v.EffectiveFrom), EffectiveTo: nullableTimestamptz(v.EffectiveTo), Status: v.Status, CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt)})
	if IsExclusionConstraint(err, "attendance_shift_assignments_active_no_overlap") {
		return domain.Conflict("active shift assignment overlaps existing assignment")
	}
	return err
}

// ListAttendanceShiftAssignments 列出租戶的員工班別指派。
func (s *Store) ListAttendanceShiftAssignments(execCtx context.Context, tenantID string) ([]domain.AttendanceShiftAssignment, error) {
	items, err := s.q.ListAttendanceShiftAssignments(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceShiftAssignment), nil
}

// FindEffectiveAttendanceShiftAssignment 取得指定時點生效的員工排班。
func (s *Store) FindEffectiveAttendanceShiftAssignment(execCtx context.Context, tenantID, employeeID string, at time.Time) (domain.AttendanceShiftAssignment, bool, error) {
	v, err := s.q.FindEffectiveAttendanceShiftAssignment(execCtx, sqlc.FindEffectiveAttendanceShiftAssignmentParams{TenantID: tenantID, EmployeeID: employeeID, EffectiveFrom: timestamptz(at)})
	if isNotFound(err) {
		return domain.AttendanceShiftAssignment{}, false, nil
	}
	if err != nil {
		return domain.AttendanceShiftAssignment{}, false, err
	}
	return fromAttendanceShiftAssignment(v), true, nil
}

// UpsertAttendanceClockRecord 從儲存層處理 upsert 考勤打卡 record。
func (s *Store) UpsertAttendanceClockRecord(execCtx context.Context, v domain.AttendanceClockRecord) error {
	_, err := s.q.UpsertAttendanceClockRecord(execCtx, sqlc.UpsertAttendanceClockRecordParams{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		ShiftAssignmentID:   nullableText(v.ShiftAssignmentID),
		ShiftID:             nullableText(v.ShiftID),
		WorksiteID:          nullableText(v.WorksiteID),
		WorkDate:            v.WorkDate,
		Direction:           v.Direction,
		ClientEventID:       v.ClientEventID,
		ClockedAt:           timestamptz(v.ClockedAt),
		Latitude:            v.Latitude,
		Longitude:           v.Longitude,
		AccuracyMeters:      v.AccuracyMeters,
		DistanceMeters:      v.DistanceMeters,
		RecordStatus:        v.RecordStatus,
		RejectionReason:     v.RejectionReason,
		Source:              v.Source,
		DeviceID:            v.DeviceID,
		Column19:            mustJSON(v.DeviceInfo),
		CorrectionRequestID: v.CorrectionRequestID,
		Voided:              v.Voided,
		VoidedAt:            nullableTimestamptz(v.VoidedAt),
		VoidedByAccountID:   v.VoidedByAccountID,
		VoidReason:          v.VoidReason,
		CreatedAt:           timestamptz(v.CreatedAt),
	})
	if isUniqueConstraint(err, "attendance_clock_records_client_event_idx") {
		return domain.Conflict("attendance clock client event already exists")
	}
	return err
}

// GetAttendanceClockRecordByClientEventID 依客戶端事件識別碼取得考勤打卡 record。
func (s *Store) GetAttendanceClockRecordByClientEventID(execCtx context.Context, tenantID, clientEventID string) (domain.AttendanceClockRecord, bool, error) {
	if strings.TrimSpace(clientEventID) == "" {
		return domain.AttendanceClockRecord{}, false, nil
	}
	v, err := s.q.GetAttendanceClockRecordByClientEventID(execCtx, sqlc.GetAttendanceClockRecordByClientEventIDParams{TenantID: tenantID, ClientEventID: clientEventID})
	return attendanceClockRecordResult(v, err)
}

// GetEarliestAcceptedAttendanceClockIn 取得未作廢的最早 accepted 上班卡。
func (s *Store) GetEarliestAcceptedAttendanceClockIn(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error) {
	v, err := s.q.GetEarliestAcceptedAttendanceClockIn(execCtx, sqlc.GetEarliestAcceptedAttendanceClockInParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: workDate})
	return attendanceClockRecordResult(v, err)
}

// GetLatestAcceptedAttendanceClockOut 取得未作廢的最晚 accepted 下班卡。
func (s *Store) GetLatestAcceptedAttendanceClockOut(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error) {
	v, err := s.q.GetLatestAcceptedAttendanceClockOut(execCtx, sqlc.GetLatestAcceptedAttendanceClockOutParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: workDate})
	return attendanceClockRecordResult(v, err)
}

// GetLatestAcceptedAttendanceClockRecord 取得未作廢的當日最新 accepted 打卡。
func (s *Store) GetLatestAcceptedAttendanceClockRecord(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error) {
	v, err := s.q.GetLatestAcceptedAttendanceClockRecord(execCtx, sqlc.GetLatestAcceptedAttendanceClockRecordParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: workDate})
	return attendanceClockRecordResult(v, err)
}

// attendanceClockRecordResult 將 SQLC 單筆打卡查詢轉為 repository optional result。
func attendanceClockRecordResult(v sqlc.AttendanceClockRecord, err error) (domain.AttendanceClockRecord, bool, error) {
	if isNotFound(err) {
		return domain.AttendanceClockRecord{}, false, nil
	}
	if err != nil {
		return domain.AttendanceClockRecord{}, false, err
	}
	return fromAttendanceClockRecord(v), true, nil
}

// ListAttendanceClockRecords 從儲存層列出考勤打卡 records。
func (s *Store) ListAttendanceClockRecords(execCtx context.Context, tenantID string, query domain.AttendanceClockRecordQuery) ([]domain.AttendanceClockRecord, error) {
	items, err := s.q.ListAttendanceClockRecords(tenantContext(execCtx, tenantID), sqlc.ListAttendanceClockRecordsParams{
		TenantID:     tenantID,
		EmployeeID:   query.EmployeeID,
		FromDate:     query.FromDate,
		ToDate:       query.ToDate,
		Direction:    query.Direction,
		RecordStatus: query.RecordStatus,
		Source:       query.Source,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceClockRecord), nil
}

// UpsertAttendanceDailySummary 從儲存層處理 upsert 考勤日彙總。
func (s *Store) UpsertAttendanceDailySummary(execCtx context.Context, v domain.AttendanceDailySummary) error {
	_, err := s.q.UpsertAttendanceDailySummary(execCtx, sqlc.UpsertAttendanceDailySummaryParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		EmployeeID:      v.EmployeeID,
		WorkDate:        v.WorkDate,
		ShiftStart:      v.ShiftStart,
		ShiftEnd:        v.ShiftEnd,
		ShiftHours:      v.ShiftHours,
		DailyHours:      v.DailyHours,
		ClockHours:      v.ClockHours,
		ClockStart:      v.ClockStart,
		ClockEnd:        v.ClockEnd,
		AttendStart:     v.AttendStart,
		AttendEnd:       v.AttendEnd,
		AttendHours:     v.AttendHours,
		AttendCounted:   v.AttendCounted,
		LeaveType:       v.LeaveType,
		LeaveStart:      v.LeaveStart,
		LeaveEnd:        v.LeaveEnd,
		LeaveHours:      v.LeaveHours,
		LeaveCounted:    v.LeaveCounted,
		Leave2Type:      v.Leave2Type,
		Leave2Start:     v.Leave2Start,
		Leave2End:       v.Leave2End,
		Leave2Hours:     v.Leave2Hours,
		Leave2Counted:   v.Leave2Counted,
		OvertimeStart:   v.OvertimeStart,
		OvertimeEnd:     v.OvertimeEnd,
		OvertimeHours:   v.OvertimeHours,
		OvertimeCounted: v.OvertimeCounted,
		Column30:        mustJSON(v.Payload),
		Source:          v.Source,
		ExternalRef:     v.ExternalRef,
		CreatedAt:       timestamptz(v.CreatedAt),
		UpdatedAt:       timestamptz(v.UpdatedAt),
	})
	if isUniqueConstraint(err, "attendance_daily_summaries_employee_date_idx") {
		return domain.Conflict("attendance daily summary already exists")
	}
	if isUniqueConstraint(err, "attendance_daily_summaries_external_ref_idx") {
		return domain.Conflict("attendance daily summary external_ref already exists")
	}
	return err
}

// GetAttendanceDailySummaryByExternalRef 從儲存層取得考勤日彙總 by external ref。
func (s *Store) GetAttendanceDailySummaryByExternalRef(execCtx context.Context, tenantID, externalRef string) (domain.AttendanceDailySummary, bool, error) {
	v, err := s.q.GetAttendanceDailySummaryByExternalRef(execCtx, sqlc.GetAttendanceDailySummaryByExternalRefParams{TenantID: tenantID, ExternalRef: externalRef})
	if isNotFound(err) {
		return domain.AttendanceDailySummary{}, false, nil
	}
	if err != nil {
		return domain.AttendanceDailySummary{}, false, err
	}
	return fromAttendanceDailySummary(v), true, nil
}

// GetAttendanceDailySummaryByEmployeeDate 從儲存層取得考勤日彙總 by 員工日期。
func (s *Store) GetAttendanceDailySummaryByEmployeeDate(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceDailySummary, bool, error) {
	v, err := s.q.GetAttendanceDailySummaryByEmployeeDate(execCtx, sqlc.GetAttendanceDailySummaryByEmployeeDateParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: workDate})
	if isNotFound(err) {
		return domain.AttendanceDailySummary{}, false, nil
	}
	if err != nil {
		return domain.AttendanceDailySummary{}, false, err
	}
	return fromAttendanceDailySummary(v), true, nil
}

// ListAttendanceDailySummaries 從儲存層列出考勤日彙總。
func (s *Store) ListAttendanceDailySummaries(execCtx context.Context, tenantID string, query domain.AttendanceDailySummaryQuery) ([]domain.AttendanceDailySummary, error) {
	items, err := s.q.ListAttendanceDailySummaries(tenantContext(execCtx, tenantID), sqlc.ListAttendanceDailySummariesParams{
		TenantID:   tenantID,
		EmployeeID: query.EmployeeID,
		FromDate:   query.FromDate,
		ToDate:     query.ToDate,
		Source:     query.Source,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceDailySummary), nil
}

// UpsertAttendanceCorrectionRequest 從儲存層處理 upsert 考勤 correction 請求。
func (s *Store) UpsertAttendanceCorrectionRequest(execCtx context.Context, v domain.AttendanceCorrectionRequest) error {
	correctionType := strings.TrimSpace(v.CorrectionType)
	if correctionType == "" {
		correctionType = "add_record"
	}
	_, err := s.q.UpsertAttendanceCorrectionRequest(execCtx, sqlc.UpsertAttendanceCorrectionRequestParams{
		ID:                       v.ID,
		TenantID:                 v.TenantID,
		EmployeeID:               v.EmployeeID,
		Direction:                v.Direction,
		RequestedClockedAt:       timestamptz(v.RequestedClockedAt),
		WorkDate:                 v.WorkDate,
		CorrectionType:           correctionType,
		TargetClockRecordID:      v.TargetClockRecordID,
		ReplacementClockRecordID: v.ReplacementClockRecordID,
		Reason:                   v.Reason,
		Status:                   v.Status,
		FormInstanceID:           v.FormInstanceID,
		ClockRecordID:            v.ClockRecordID,
		ReviewedByAccountID:      v.ReviewedByAccountID,
		ReviewReason:             v.ReviewReason,
		ReviewedAt:               nullableTimestamptz(v.ReviewedAt),
		CreatedAt:                timestamptz(v.CreatedAt),
		UpdatedAt:                timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAttendanceCorrectionRequest 從儲存層取得考勤 correction 請求。
func (s *Store) GetAttendanceCorrectionRequest(execCtx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error) {
	v, err := s.q.GetAttendanceCorrectionRequest(execCtx, sqlc.GetAttendanceCorrectionRequestParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceCorrectionRequest{}, false, nil
	}
	if err != nil {
		return domain.AttendanceCorrectionRequest{}, false, err
	}
	return fromAttendanceCorrectionRequest(v), true, nil
}

// ListAttendanceCorrectionRequests 從儲存層列出考勤 correction 請求。
func (s *Store) ListAttendanceCorrectionRequests(execCtx context.Context, tenantID string, query domain.AttendanceCorrectionQuery) ([]domain.AttendanceCorrectionRequest, error) {
	items, err := s.q.ListAttendanceCorrectionRequests(tenantContext(execCtx, tenantID), sqlc.ListAttendanceCorrectionRequestsParams{
		TenantID:   tenantID,
		EmployeeID: query.EmployeeID,
		FromDate:   query.FromDate,
		ToDate:     query.ToDate,
		Status:     query.Status,
		Direction:  query.Direction,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceCorrectionRequest), nil
}

// GetAttendanceCorrectionRequestByFormInstanceID 從儲存層取得考勤 correction 請求 by 表單實例 ID。
func (s *Store) GetAttendanceCorrectionRequestByFormInstanceID(execCtx context.Context, tenantID, formInstanceID string) (domain.AttendanceCorrectionRequest, bool, error) {
	v, err := s.q.GetAttendanceCorrectionRequestByFormInstanceID(tenantContext(execCtx, tenantID), sqlc.GetAttendanceCorrectionRequestByFormInstanceIDParams{TenantID: tenantID, FormInstanceID: formInstanceID})
	if isNotFound(err) {
		return domain.AttendanceCorrectionRequest{}, false, nil
	}
	if err != nil {
		return domain.AttendanceCorrectionRequest{}, false, err
	}
	return fromAttendanceCorrectionRequest(v), true, nil
}

// UpsertOvertimeRequest 從儲存層處理 upsert 加班申請。
func (s *Store) UpsertOvertimeRequest(execCtx context.Context, v domain.OvertimeRequest) error {
	_, err := s.q.UpsertOvertimeRequest(tenantContext(execCtx, v.TenantID), sqlc.UpsertOvertimeRequestParams{
		ID:               v.ID,
		TenantID:         v.TenantID,
		EmployeeID:       v.EmployeeID,
		WorkDate:         v.WorkDate,
		StartAt:          timestamptz(v.StartAt),
		EndAt:            timestamptz(v.EndAt),
		Hours:            v.Hours,
		OvertimeType:     v.OvertimeType,
		CompensationType: v.CompensationType,
		Reason:           v.Reason,
		Status:           v.Status,
		FormInstanceID:   v.FormInstanceID,
		CreatedAt:        timestamptz(v.CreatedAt),
		UpdatedAt:        timestamptz(v.UpdatedAt),
	})
	return err
}

// GetOvertimeRequest 從儲存層取得加班申請。
func (s *Store) GetOvertimeRequest(execCtx context.Context, tenantID, id string) (domain.OvertimeRequest, bool, error) {
	v, err := s.q.GetOvertimeRequest(tenantContext(execCtx, tenantID), sqlc.GetOvertimeRequestParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.OvertimeRequest{}, false, nil
	}
	if err != nil {
		return domain.OvertimeRequest{}, false, err
	}
	return fromOvertimeRequest(v), true, nil
}

// GetOvertimeRequestByFormInstanceID 從儲存層取得加班申請 by 表單實例 ID。
func (s *Store) GetOvertimeRequestByFormInstanceID(execCtx context.Context, tenantID, formInstanceID string) (domain.OvertimeRequest, bool, error) {
	v, err := s.q.GetOvertimeRequestByFormInstanceID(tenantContext(execCtx, tenantID), sqlc.GetOvertimeRequestByFormInstanceIDParams{TenantID: tenantID, FormInstanceID: formInstanceID})
	if isNotFound(err) {
		return domain.OvertimeRequest{}, false, nil
	}
	if err != nil {
		return domain.OvertimeRequest{}, false, err
	}
	return fromOvertimeRequest(v), true, nil
}

// ListOvertimeRequestsByQuery 從儲存層列出加班申請 by 查詢。
func (s *Store) ListOvertimeRequestsByQuery(execCtx context.Context, tenantID string, query domain.OvertimeRequestQuery) ([]domain.OvertimeRequest, error) {
	items, err := s.q.ListOvertimeRequestsByQuery(tenantContext(execCtx, tenantID), sqlc.ListOvertimeRequestsByQueryParams{
		TenantID:    tenantID,
		EmployeeIds: query.EmployeeIDs,
		Status:      query.Status,
		FromDate:    query.FromDate,
		ToDate:      query.ToDate,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOvertimeRequest), nil
}

// UpsertFormDefinitionDraft 保存草稿，並由 SQL 條件保護 revision 樂觀鎖。
func (s *Store) UpsertFormDefinitionDraft(execCtx context.Context, v domain.FormDefinitionDraft) error {
	if v.Revision <= 0 {
		v.Revision = 1
	}
	_, err := s.q.UpsertFormDefinitionDraft(execCtx, sqlc.UpsertFormDefinitionDraftParams{
		ID: v.ID, TenantID: v.TenantID, OwnerAccountID: v.OwnerAccountID, BaseTemplateID: v.BaseTemplateID,
		SchemaVersion: int32(v.SchemaVersion), AuthoringSchema: mustJSON(v.AuthoringSchema), CompiledSchema: mustJSON(v.CompiledSchema),
		Status: string(v.Status), Revision: v.Revision, Source: v.Source, AgentID: v.AgentID, AgentRunID: v.AgentRunID,
		AgentSessionID: v.AgentSessionID, ToolCallID: v.ToolCallID, ValidationResult: mustJSON(v.ValidationResult),
		SubmittedAt: nullableTimestamptz(v.SubmittedAt), PublishedTemplateID: v.PublishedTemplateID,
		CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt),
	})
	if isNotFound(err) {
		return domain.Conflict("form definition draft was modified concurrently")
	}
	return err
}

// GetFormDefinitionDraft 取得租戶內的表單定義草稿。
func (s *Store) GetFormDefinitionDraft(execCtx context.Context, tenantID, id string) (domain.FormDefinitionDraft, bool, error) {
	v, err := s.q.GetFormDefinitionDraft(tenantContext(execCtx, tenantID), sqlc.GetFormDefinitionDraftParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FormDefinitionDraft{}, false, nil
	}
	if err != nil {
		return domain.FormDefinitionDraft{}, false, err
	}
	return fromFormDefinitionDraft(v), true, nil
}

// GetFormDefinitionDraftByAgentCall 以 Agent run 與 tool call 保證重試冪等。
func (s *Store) GetFormDefinitionDraftByAgentCall(execCtx context.Context, tenantID, agentRunID, toolCallID string) (domain.FormDefinitionDraft, bool, error) {
	v, err := s.q.GetFormDefinitionDraftByAgentCall(tenantContext(execCtx, tenantID), sqlc.GetFormDefinitionDraftByAgentCallParams{TenantID: tenantID, AgentRunID: agentRunID, ToolCallID: toolCallID})
	if isNotFound(err) {
		return domain.FormDefinitionDraft{}, false, nil
	}
	if err != nil {
		return domain.FormDefinitionDraft{}, false, err
	}
	return fromFormDefinitionDraft(v), true, nil
}

// ListFormDefinitionDrafts 列出指定擁有者與狀態的草稿。
func (s *Store) ListFormDefinitionDrafts(execCtx context.Context, tenantID, ownerAccountID, status string) ([]domain.FormDefinitionDraft, error) {
	items, err := s.q.ListFormDefinitionDrafts(tenantContext(execCtx, tenantID), sqlc.ListFormDefinitionDraftsParams{TenantID: tenantID, OwnerAccountID: ownerAccountID, Status: status})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormDefinitionDraft), nil
}

// UpsertFormTemplate 從儲存層處理 upsert 表單範本。
func (s *Store) UpsertFormTemplate(execCtx context.Context, v domain.FormTemplate) error {
	v = normalizeFormTemplate(v)
	_, err := s.q.UpsertFormTemplate(execCtx, sqlc.UpsertFormTemplateParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		Key:            v.Key,
		Name:           v.Name,
		Description:    v.Description,
		Schema:         mustJSON(v.Schema),
		Status:         v.Status,
		CurrentVersion: int32(v.CurrentVersion),
		CreatedAt:      timestamptz(v.CreatedAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
		DeletedAt:      nullableTimestamptz(v.DeletedAt),
	})
	if err != nil {
		return err
	}
	version := domain.FormTemplateVersion{
		ID: utils.NewID("ftv"), TenantID: v.TenantID, TemplateID: v.ID,
		Version: v.CurrentVersion, Schema: v.Schema, Status: v.Status, CreatedAt: v.UpdatedAt,
	}
	if v.Status == "published" {
		publishedAt := v.UpdatedAt
		version.PublishedAt = &publishedAt
	}
	return s.InsertFormTemplateVersion(execCtx, version)
}

// GetFormTemplate 從儲存層取得表單範本。
func (s *Store) GetFormTemplate(execCtx context.Context, tenantID, id string) (domain.FormTemplate, bool, error) {
	v, err := s.q.GetFormTemplate(execCtx, sqlc.GetFormTemplateParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FormTemplate{}, false, nil
	}
	if err != nil {
		return domain.FormTemplate{}, false, err
	}
	return fromFormTemplate(v), true, nil
}

// GetFormTemplateByKey 從儲存層取得表單範本 by key。
func (s *Store) GetFormTemplateByKey(execCtx context.Context, tenantID, key string) (domain.FormTemplate, bool, error) {
	v, err := s.q.GetFormTemplateByKey(execCtx, sqlc.GetFormTemplateByKeyParams{TenantID: tenantID, Key: key})
	if isNotFound(err) {
		return domain.FormTemplate{}, false, nil
	}
	if err != nil {
		return domain.FormTemplate{}, false, err
	}
	return fromFormTemplate(v), true, nil
}

// ListFormTemplates 從儲存層列出表單範本。
func (s *Store) ListFormTemplates(execCtx context.Context, tenantID string) ([]domain.FormTemplate, error) {
	items, err := s.q.ListFormTemplates(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormTemplate), nil
}

// InsertFormTemplateVersion 寫入不可變的表單版本；相同版本存在時保留原快照。
func (s *Store) InsertFormTemplateVersion(execCtx context.Context, v domain.FormTemplateVersion) error {
	return s.q.InsertFormTemplateVersion(execCtx, sqlc.InsertFormTemplateVersionParams{
		ID: v.ID, TenantID: v.TenantID, TemplateID: v.TemplateID, Version: int32(v.Version),
		Schema: mustJSON(v.Schema), Status: v.Status, CreatedAt: timestamptz(v.CreatedAt),
		PublishedAt: nullableTimestamptz(v.PublishedAt),
	})
}

// GetFormTemplateVersion 依版本 ID 取得不可變快照。
func (s *Store) GetFormTemplateVersion(execCtx context.Context, tenantID, id string) (domain.FormTemplateVersion, bool, error) {
	v, err := s.q.GetFormTemplateVersion(tenantContext(execCtx, tenantID), sqlc.GetFormTemplateVersionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FormTemplateVersion{}, false, nil
	}
	if err != nil {
		return domain.FormTemplateVersion{}, false, err
	}
	return fromFormTemplateVersion(v), true, nil
}

// GetFormTemplateVersionByNumber 依模板與版本號取得不可變快照。
func (s *Store) GetFormTemplateVersionByNumber(execCtx context.Context, tenantID, templateID string, version int) (domain.FormTemplateVersion, bool, error) {
	v, err := s.q.GetFormTemplateVersionByNumber(tenantContext(execCtx, tenantID), sqlc.GetFormTemplateVersionByNumberParams{
		TenantID: tenantID, TemplateID: templateID, Version: int32(version),
	})
	if isNotFound(err) {
		return domain.FormTemplateVersion{}, false, nil
	}
	if err != nil {
		return domain.FormTemplateVersion{}, false, err
	}
	return fromFormTemplateVersion(v), true, nil
}

// UpsertFormInstance 從儲存層處理 upsert 表單實例。Version > 0 時執行樂觀鎖檢查。
func (s *Store) UpsertFormInstance(execCtx context.Context, v domain.FormInstance) error {
	if strings.TrimSpace(v.TemplateVersionID) == "" {
		template, err := s.q.GetFormTemplate(tenantContext(execCtx, v.TenantID), sqlc.GetFormTemplateParams{TenantID: v.TenantID, ID: v.TemplateID})
		if err != nil {
			return err
		}
		version, err := s.q.GetFormTemplateVersionByNumber(tenantContext(execCtx, v.TenantID), sqlc.GetFormTemplateVersionByNumberParams{
			TenantID: v.TenantID, TemplateID: v.TemplateID, Version: template.CurrentVersion,
		})
		if err != nil {
			return err
		}
		v.TemplateVersionID = version.ID
	}
	_, err := s.q.UpsertFormInstance(tenantContext(execCtx, v.TenantID), sqlc.UpsertFormInstanceParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		TemplateID:         v.TemplateID,
		TemplateVersionID:  v.TemplateVersionID,
		ApplicantAccountID: v.ApplicantAccountID,
		Status:             v.Status,
		Payload:            mustJSON(v.Payload),
		SubmittedAt:        timestamptz(v.SubmittedAt),
		ApprovedBy:         v.ApprovedBy,
		CurrentRunID:       v.CurrentRunID,
		UpdatedAt:          timestamptz(v.UpdatedAt),
		ExpectedVersion:    v.Version,
	})
	if isNotFound(err) {
		return domain.Conflict("form instance was modified concurrently")
	}
	return err
}

// GetFormInstance 從儲存層取得表單實例。
func (s *Store) GetFormInstance(execCtx context.Context, tenantID, id string) (domain.FormInstance, bool, error) {
	v, err := s.q.GetFormInstance(tenantContext(execCtx, tenantID), sqlc.GetFormInstanceParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FormInstance{}, false, nil
	}
	if err != nil {
		return domain.FormInstance{}, false, err
	}
	return fromFormInstance(v), true, nil
}

// ListFormInstances 從儲存層列出表單實例。
func (s *Store) ListFormInstances(execCtx context.Context, tenantID string) ([]domain.FormInstance, error) {
	items, err := s.q.ListFormInstances(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormInstance), nil
}

// ListFormInstancesByQuery 從儲存層列出表單實例 by 查詢。
func (s *Store) ListFormInstancesByQuery(execCtx context.Context, tenantID string, query domain.FormInstanceQuery) ([]domain.FormInstance, error) {
	params := formInstanceQueryParams(tenantID, query)
	items, err := s.q.ListFormInstancesByQuery(tenantContext(execCtx, tenantID), sqlc.ListFormInstancesByQueryParams{
		TenantID:           params.TenantID,
		Status:             params.Status,
		TemplateID:         params.TemplateID,
		TemplateKey:        params.TemplateKey,
		ApplicantAccountID: params.ApplicantAccountID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormInstance), nil
}

// ListFormInstancePageByQuery 從儲存層列出表單實例分頁 by 查詢。
func (s *Store) ListFormInstancePageByQuery(execCtx context.Context, tenantID string, query domain.FormInstanceQuery, page domain.PageRequest) ([]domain.FormInstance, int, error) {
	page = utils.NormalizePageRequest(page)
	countParams := formInstanceQueryParams(tenantID, query)
	total, err := s.q.CountFormInstancesByQuery(tenantContext(execCtx, tenantID), countParams)
	if err != nil {
		return nil, 0, err
	}
	listParams := sqlc.ListFormInstancePageByQueryParams{
		TenantID:           countParams.TenantID,
		Status:             countParams.Status,
		TemplateID:         countParams.TemplateID,
		TemplateKey:        countParams.TemplateKey,
		ApplicantAccountID: countParams.ApplicantAccountID,
		Sort:               page.Sort,
		LimitCount:         int32(page.PageSize),
		OffsetCount:        int32((page.Page - 1) * page.PageSize),
	}
	items, err := s.q.ListFormInstancePageByQuery(tenantContext(execCtx, tenantID), listParams)
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromFormInstance), int(total), nil
}

// ReplaceFormInstanceFieldValues 替換單一表單實例的可統計欄位投影。
func (s *Store) ReplaceFormInstanceFieldValues(execCtx context.Context, tenantID, formInstanceID string, values []domain.FormInstanceFieldValue) error {
	execCtx = tenantContext(execCtx, tenantID)
	if err := s.q.DeleteFormInstanceFieldValues(execCtx, sqlc.DeleteFormInstanceFieldValuesParams{TenantID: tenantID, FormInstanceID: formInstanceID}); err != nil {
		return err
	}
	for _, value := range values {
		if err := s.q.InsertFormInstanceFieldValue(execCtx, sqlc.InsertFormInstanceFieldValueParams{
			TenantID: value.TenantID, FormInstanceID: value.FormInstanceID, TemplateID: value.TemplateID,
			TemplateVersionID: value.TemplateVersionID, FieldID: value.FieldID, ValueType: value.ValueType,
			ValueText: value.ValueText, ValueNumber: value.ValueNumber, ValueBoolean: nullableBool(value.ValueBoolean),
			ValueDate: value.ValueDate, ValueTimestamp: value.ValueTimestamp, ValueJson: string(value.ValueJSON),
			CreatedAt: timestamptz(value.CreatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListFormInstanceFieldValues 列出單一表單實例的欄位投影。
func (s *Store) ListFormInstanceFieldValues(execCtx context.Context, tenantID, formInstanceID string) ([]domain.FormInstanceFieldValue, error) {
	items, err := s.q.ListFormInstanceFieldValues(tenantContext(execCtx, tenantID), sqlc.ListFormInstanceFieldValuesParams{TenantID: tenantID, FormInstanceID: formInstanceID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormInstanceFieldValue), nil
}

// DeleteFormInstance 從儲存層刪除表單實例。
func (s *Store) DeleteFormInstance(execCtx context.Context, tenantID, id string) error {
	return s.q.DeleteFormInstance(tenantContext(execCtx, tenantID), sqlc.DeleteFormInstanceParams{TenantID: tenantID, ID: id})
}

// UpsertPlatformTaskItem 從儲存層處理 upsert 平臺任務項目。
func (s *Store) UpsertPlatformTaskItem(execCtx context.Context, v domain.PlatformTaskRecordItem) error {
	_, err := s.q.UpsertPlatformTaskItem(tenantContext(execCtx, v.TenantID), sqlc.UpsertPlatformTaskItemParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		WorkDate:  v.WorkDate,
		Title:     v.Title,
		Category:  v.Category,
		Product:   v.Product,
		Hours:     v.Hours,
		Note:      v.Note,
		CreatedAt: timestamptz(v.CreatedAt),
		UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

// GetPlatformTaskItem 從儲存層取得平臺任務項目。
func (s *Store) GetPlatformTaskItem(execCtx context.Context, tenantID, accountID, id string) (domain.PlatformTaskRecordItem, bool, error) {
	v, err := s.q.GetPlatformTaskItem(tenantContext(execCtx, tenantID), sqlc.GetPlatformTaskItemParams{TenantID: tenantID, AccountID: accountID, ID: id})
	if isNotFound(err) {
		return domain.PlatformTaskRecordItem{}, false, nil
	}
	if err != nil {
		return domain.PlatformTaskRecordItem{}, false, err
	}
	return fromPlatformTaskItem(v), true, nil
}

// ListPlatformTaskItems 從儲存層列出平臺任務項目。
func (s *Store) ListPlatformTaskItems(execCtx context.Context, tenantID, accountID string) ([]domain.PlatformTaskRecordItem, error) {
	items, err := s.q.ListPlatformTaskItems(tenantContext(execCtx, tenantID), sqlc.ListPlatformTaskItemsParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPlatformTaskItem), nil
}

// DeletePlatformTaskItem 從儲存層刪除平臺任務項目。
func (s *Store) DeletePlatformTaskItem(execCtx context.Context, tenantID, accountID, id string) error {
	return s.q.DeletePlatformTaskItem(tenantContext(execCtx, tenantID), sqlc.DeletePlatformTaskItemParams{TenantID: tenantID, AccountID: accountID, ID: id})
}

// UpsertPlatformTaskTodo 從儲存層處理 upsert 平臺任務待辦。
func (s *Store) UpsertPlatformTaskTodo(execCtx context.Context, v domain.PlatformTaskTodoRecord) error {
	_, err := s.q.UpsertPlatformTaskTodo(tenantContext(execCtx, v.TenantID), sqlc.UpsertPlatformTaskTodoParams{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		AccountID:           v.AccountID,
		Text:                v.Text,
		DueDate:             v.DueDate,
		Status:              v.Status,
		ConvertedTaskItemID: v.ConvertedTaskItemID,
		CreatedAt:           timestamptz(v.CreatedAt),
		UpdatedAt:           timestamptz(v.UpdatedAt),
	})
	return err
}

// GetPlatformTaskTodo 從儲存層取得平臺任務待辦。
func (s *Store) GetPlatformTaskTodo(execCtx context.Context, tenantID, accountID, id string) (domain.PlatformTaskTodoRecord, bool, error) {
	v, err := s.q.GetPlatformTaskTodo(tenantContext(execCtx, tenantID), sqlc.GetPlatformTaskTodoParams{TenantID: tenantID, AccountID: accountID, ID: id})
	if isNotFound(err) {
		return domain.PlatformTaskTodoRecord{}, false, nil
	}
	if err != nil {
		return domain.PlatformTaskTodoRecord{}, false, err
	}
	return fromPlatformTaskTodo(v), true, nil
}

// ListPlatformTaskTodos 從儲存層列出平臺任務待辦。
func (s *Store) ListPlatformTaskTodos(execCtx context.Context, tenantID, accountID string) ([]domain.PlatformTaskTodoRecord, error) {
	items, err := s.q.ListPlatformTaskTodos(tenantContext(execCtx, tenantID), sqlc.ListPlatformTaskTodosParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPlatformTaskTodo), nil
}

// DeletePlatformTaskTodo 從儲存層刪除平臺任務待辦。
func (s *Store) DeletePlatformTaskTodo(execCtx context.Context, tenantID, accountID, id string) error {
	return s.q.DeletePlatformTaskTodo(tenantContext(execCtx, tenantID), sqlc.DeletePlatformTaskTodoParams{TenantID: tenantID, AccountID: accountID, ID: id})
}

// UpsertAgentRun 從儲存層處理 upsert agent 執行。
func (s *Store) UpsertAgentRun(execCtx context.Context, v domain.AgentRun) error {
	_, err := s.q.UpsertAgentRun(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentRunParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		AgentID:        nullableText(v.AgentID),
		SessionID:      nullableText(v.SessionID),
		Mode:           v.Mode,
		Prompt:         v.Prompt,
		Answer:         v.Answer,
		Status:         v.Status,
		ReferenceItems: mustJSON(v.References),
		LlmCallCount:   v.LLMCallCount,
		InputTokens:    v.InputTokens,
		CachedTokens:   v.CachedTokens,
		OutputTokens:   v.OutputTokens,
		TotalTokens:    v.TotalTokens,
		UsageComplete:  v.UsageComplete,
		CreatedAt:      timestamptz(v.CreatedAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
	})
	return err
}

// ListAgentRuns 從儲存層列出 agent 執行紀錄。
func (s *Store) ListAgentRuns(execCtx context.Context, tenantID string) ([]domain.AgentRun, error) {
	items, err := s.q.ListAgentRuns(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentRun), nil
}

// ListAgentRunsByAccount 從儲存層列出 agent 執行紀錄 by 帳號。
func (s *Store) ListAgentRunsByAccount(execCtx context.Context, tenantID, accountID string) ([]domain.AgentRun, error) {
	items, err := s.q.ListAgentRunsByAccount(tenantContext(execCtx, tenantID), sqlc.ListAgentRunsByAccountParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentRun), nil
}

// ListAgentRunPage 從儲存層列出 agent 執行分頁。
func (s *Store) ListAgentRunPage(execCtx context.Context, tenantID string, page domain.PageRequest) ([]domain.AgentRun, int, error) {
	page = utils.NormalizePageRequest(page)
	total, err := s.q.CountAgentRuns(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAgentRunsPage(tenantContext(execCtx, tenantID), sqlc.ListAgentRunsPageParams{
		TenantID:    tenantID,
		Sort:        page.Sort,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAgentRun), int(total), nil
}

// ListAgentRunPageByAccount 從儲存層列出 agent 執行分頁 by 帳號。
func (s *Store) ListAgentRunPageByAccount(execCtx context.Context, tenantID, accountID string, page domain.PageRequest) ([]domain.AgentRun, int, error) {
	page = utils.NormalizePageRequest(page)
	total, err := s.q.CountAgentRunsByAccount(tenantContext(execCtx, tenantID), sqlc.CountAgentRunsByAccountParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAgentRunsPageByAccount(tenantContext(execCtx, tenantID), sqlc.ListAgentRunsPageByAccountParams{
		TenantID:    tenantID,
		AccountID:   accountID,
		Sort:        page.Sort,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAgentRun), int(total), nil
}

// UpsertAgentModel 從儲存層處理 upsert agent 模型。
func (s *Store) UpsertAgentModel(execCtx context.Context, v domain.AgentModel) error {
	_, err := s.q.UpsertAgentModel(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentModelParams{
		ID:               v.ID,
		TenantID:         v.TenantID,
		Name:             v.Name,
		Provider:         v.Provider,
		ModelName:        v.ModelName,
		LitellmModel:     v.LiteLLMModel,
		ApiBaseUrl:       v.APIBaseURL,
		ApiKeyCiphertext: v.APIKeyCiphertext,
		ApiKeyPreview:    v.APIKeyPreview,
		RateLimitRpm:     int32(v.RateLimitRPM),
		Status:           string(v.Status),
		TimeoutSeconds:   int32(v.TimeoutSeconds),
		MonthlyQuota:     v.MonthlyQuota,
		UsedQuota:        v.UsedQuota,
		LastTestedAt:     nullableTimestamptz(v.LastTestedAt),
		LastTestStatus:   v.LastTestStatus,
		LastTestMessage:  v.LastTestMessage,
		SyncStatus:       string(v.SyncStatus),
		LastSyncedAt:     nullableTimestamptz(v.LastSyncedAt),
		LastSyncError:    v.LastSyncError,
		SyncedConfigHash: v.SyncedConfigHash,
		CreatedAt:        timestamptz(v.CreatedAt),
		UpdatedAt:        timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAgentModel 從儲存層取得 agent 模型。
func (s *Store) GetAgentModel(execCtx context.Context, tenantID, id string) (domain.AgentModel, bool, error) {
	v, err := s.q.GetAgentModel(tenantContext(execCtx, tenantID), sqlc.GetAgentModelParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentModel{}, false, nil
	}
	if err != nil {
		return domain.AgentModel{}, false, err
	}
	return fromAgentModel(v), true, nil
}

// ListAgentModels 從儲存層列出 agent 模型。
func (s *Store) ListAgentModels(execCtx context.Context, tenantID string) ([]domain.AgentModel, error) {
	items, err := s.q.ListAgentModels(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentModel), nil
}

// DeleteAgentModel 從儲存層刪除 agent 模型。
func (s *Store) DeleteAgentModel(execCtx context.Context, tenantID, id string) (domain.AgentModel, bool, error) {
	v, err := s.q.DeleteAgentModel(tenantContext(execCtx, tenantID), sqlc.DeleteAgentModelParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentModel{}, false, nil
	}
	if err != nil {
		return domain.AgentModel{}, false, err
	}
	return fromAgentModel(v), true, nil
}

// UpdateAgentModelTestResult 從儲存層更新模型測試結果。
func (s *Store) UpdateAgentModelTestResult(execCtx context.Context, tenantID, id, status, message string, testedAt time.Time) (domain.AgentModel, bool, error) {
	v, err := s.q.UpdateAgentModelTestResult(tenantContext(execCtx, tenantID), sqlc.UpdateAgentModelTestResultParams{
		TenantID:        tenantID,
		ID:              id,
		LastTestStatus:  status,
		LastTestMessage: message,
		LastTestedAt:    timestamptz(testedAt),
		UpdatedAt:       timestamptz(testedAt),
	})
	if isNotFound(err) {
		return domain.AgentModel{}, false, nil
	}
	if err != nil {
		return domain.AgentModel{}, false, err
	}
	return fromAgentModel(v), true, nil
}

// UpdateAgentModelSyncResult 從儲存層更新模型同步結果。
func (s *Store) UpdateAgentModelSyncResult(execCtx context.Context, tenantID, id string, status domain.AgentModelSyncStatus, lastError, configHash string, syncedAt *time.Time, updatedAt time.Time) (domain.AgentModel, bool, error) {
	v, err := s.q.UpdateAgentModelSyncResult(tenantContext(execCtx, tenantID), sqlc.UpdateAgentModelSyncResultParams{
		TenantID:         tenantID,
		ID:               id,
		SyncStatus:       string(status),
		LastSyncedAt:     nullableTimestamptz(syncedAt),
		LastSyncError:    lastError,
		SyncedConfigHash: configHash,
		UpdatedAt:        timestamptz(updatedAt),
	})
	if isNotFound(err) {
		return domain.AgentModel{}, false, nil
	}
	if err != nil {
		return domain.AgentModel{}, false, err
	}
	return fromAgentModel(v), true, nil
}

// ListAgentDefinitionRefsByModel 列出目前引用模型的 agent（僅當前定義，不含歷史版本）。
func (s *Store) ListAgentDefinitionRefsByModel(execCtx context.Context, tenantID, modelID string) ([]domain.AgentDefinitionRef, error) {
	rows, err := s.q.ListAgentDefinitionRefsByModel(tenantContext(execCtx, tenantID), sqlc.ListAgentDefinitionRefsByModelParams{TenantID: tenantID, ModelID: modelID})
	if err != nil {
		return nil, err
	}
	refs := make([]domain.AgentDefinitionRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, domain.AgentDefinitionRef{ID: row.ID, Name: row.Name})
	}
	return refs, nil
}

// InsertAgentExternalTool persists one tenant-scoped external tool registration.
func (s *Store) InsertAgentExternalTool(execCtx context.Context, item domain.AgentExternalTool) error {
	_, err := s.q.InsertAgentExternalTool(tenantContext(execCtx, item.TenantID), sqlc.InsertAgentExternalToolParams{
		ID:                   item.ID,
		TenantID:             item.TenantID,
		Name:                 item.Name,
		Description:          item.Description,
		Kind:                 item.Kind,
		Transport:            item.Transport,
		EndpointUrl:          item.EndpointURL,
		AuthType:             item.AuthType,
		AuthHeaderName:       item.AuthHeaderName,
		AuthUsername:         item.AuthUsername,
		AuthSecretCiphertext: item.AuthSecretCiphertext,
		CreatedByAccountID:   nullableText(item.CreatedByAccountID),
		CreatedAt:            timestamptz(item.CreatedAt),
	})
	return err
}

// ListAgentExternalTools returns external tools for one tenant.
func (s *Store) ListAgentExternalTools(execCtx context.Context, tenantID string) ([]domain.AgentExternalTool, error) {
	items, err := s.q.ListAgentExternalTools(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentExternalTool), nil
}

// DeleteAgentExternalTool deletes one tenant-scoped external tool registration.
func (s *Store) DeleteAgentExternalTool(execCtx context.Context, tenantID, id string) (domain.AgentExternalTool, bool, error) {
	item, err := s.q.DeleteAgentExternalTool(tenantContext(execCtx, tenantID), sqlc.DeleteAgentExternalToolParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentExternalTool{}, false, nil
	}
	if err != nil {
		return domain.AgentExternalTool{}, false, err
	}
	return fromAgentExternalTool(item), true, nil
}

// UpsertAgentDefinition 從儲存層處理 upsert agent 定義。
func (s *Store) UpsertAgentDefinition(execCtx context.Context, v domain.AgentDefinition) error {
	_, err := s.q.UpsertAgentDefinition(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentDefinitionParams{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		Name:                          v.Name,
		Description:                   v.Description,
		Emoji:                         v.Emoji,
		Category:                      string(v.Category),
		ModelID:                       v.ModelID,
		MainAgentRole:                 v.MainAgentRole,
		SubAgents:                     mustJSON(v.SubAgents),
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            mustJSON(v.SuggestedQuestions),
		SuggestedQuestionTranslations: mustJSON(v.SuggestedQuestionTranslations),
		Tools:                         mustJSON(v.Tools),
		KnowledgeBaseIds:              mustJSON(v.KnowledgeBaseIDs),
		Status:                        string(v.Status),
		Visibility:                    string(v.Visibility),
		VisibilityTargets:             mustJSON(v.VisibilityTargets),
		TimeoutSeconds:                int32(v.TimeoutSeconds),
		Version:                       int32(v.Version),
		PublishedVersion:              int32(v.PublishedVersion),
		UsageTotalRuns:                v.Usage.TotalRuns,
		UsageSuccessRuns:              v.Usage.SuccessRuns,
		UsageFailedRuns:               v.Usage.FailedRuns,
		UsageAvgLatencyMs:             int32(v.Usage.AvgLatencyMs),
		UsageLastRunAt:                nullableTimestamptz(v.Usage.LastRunAt),
		UsageTopPrompts:               mustJSON(v.Usage.TopPrompts),
		CreatedByAccountID:            nullableText(v.CreatedByAccountID),
		UpdatedByAccountID:            nullableText(v.UpdatedByAccountID),
		CreatedAt:                     timestamptz(v.CreatedAt),
		UpdatedAt:                     timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAgentDefinition 從儲存層取得 agent 定義。
func (s *Store) GetAgentDefinition(execCtx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error) {
	v, err := s.q.GetAgentDefinition(tenantContext(execCtx, tenantID), sqlc.GetAgentDefinitionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentDefinition{}, false, nil
	}
	if err != nil {
		return domain.AgentDefinition{}, false, err
	}
	return fromAgentDefinition(v), true, nil
}

// ListAgentDefinitions 從儲存層列出 agent 定義。
func (s *Store) ListAgentDefinitions(execCtx context.Context, tenantID string) ([]domain.AgentDefinition, error) {
	items, err := s.q.ListAgentDefinitions(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentDefinition), nil
}

// ListPublishedAgentDefinitions 從儲存層列出已發布 agent 定義。
func (s *Store) ListPublishedAgentDefinitions(execCtx context.Context, tenantID string) ([]domain.AgentDefinition, error) {
	items, err := s.q.ListPublishedAgentDefinitions(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentDefinition), nil
}

// DeleteAgentDefinition 從儲存層刪除 agent 定義。
func (s *Store) DeleteAgentDefinition(execCtx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error) {
	v, err := s.q.DeleteAgentDefinition(tenantContext(execCtx, tenantID), sqlc.DeleteAgentDefinitionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentDefinition{}, false, nil
	}
	if err != nil {
		return domain.AgentDefinition{}, false, err
	}
	return fromAgentDefinition(v), true, nil
}

// UpdateAgentDefinitionUsage 從儲存層更新 agent usage。
func (s *Store) UpdateAgentDefinitionUsage(execCtx context.Context, tenantID, id string, success bool, latencyMs int, prompt string, runAt time.Time) (domain.AgentDefinition, bool, error) {
	v, err := s.q.UpdateAgentDefinitionUsage(tenantContext(execCtx, tenantID), sqlc.UpdateAgentDefinitionUsageParams{
		TenantID:  tenantID,
		ID:        id,
		Success:   success,
		LatencyMs: int32(latencyMs),
		Prompt:    prompt,
		RunAt:     timestamptz(runAt),
	})
	if isNotFound(err) {
		return domain.AgentDefinition{}, false, nil
	}
	if err != nil {
		return domain.AgentDefinition{}, false, err
	}
	return fromAgentDefinition(v), true, nil
}

// InsertAgentDefinitionVersion 從儲存層新增 agent 版本。
func (s *Store) InsertAgentDefinitionVersion(execCtx context.Context, v domain.AgentDefinitionVersion) error {
	_, err := s.q.InsertAgentDefinitionVersion(tenantContext(execCtx, v.TenantID), sqlc.InsertAgentDefinitionVersionParams{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		AgentID:                       v.AgentID,
		Version:                       int32(v.Version),
		MainAgentRole:                 v.MainAgentRole,
		SubAgents:                     mustJSON(v.SubAgents),
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            mustJSON(v.SuggestedQuestions),
		SuggestedQuestionTranslations: mustJSON(v.SuggestedQuestionTranslations),
		Tools:                         mustJSON(v.Tools),
		KnowledgeBaseIds:              mustJSON(v.KnowledgeBaseIDs),
		ModelID:                       v.ModelID,
		Note:                          v.Note,
		CreatedByAccountID:            nullableText(v.CreatedByAccountID),
		CreatedAt:                     timestamptz(v.CreatedAt),
	})
	return err
}

// ListAgentDefinitionVersions 從儲存層列出 agent 版本。
func (s *Store) ListAgentDefinitionVersions(execCtx context.Context, tenantID, agentID string) ([]domain.AgentDefinitionVersion, error) {
	items, err := s.q.ListAgentDefinitionVersions(tenantContext(execCtx, tenantID), sqlc.ListAgentDefinitionVersionsParams{TenantID: tenantID, AgentID: agentID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentDefinitionVersion), nil
}

// GetAgentDefinitionVersion 從儲存層取得 agent 版本。
func (s *Store) GetAgentDefinitionVersion(execCtx context.Context, tenantID, agentID string, version int) (domain.AgentDefinitionVersion, bool, error) {
	v, err := s.q.GetAgentDefinitionVersion(tenantContext(execCtx, tenantID), sqlc.GetAgentDefinitionVersionParams{TenantID: tenantID, AgentID: agentID, Version: int32(version)})
	if isNotFound(err) {
		return domain.AgentDefinitionVersion{}, false, nil
	}
	if err != nil {
		return domain.AgentDefinitionVersion{}, false, err
	}
	return fromAgentDefinitionVersion(v), true, nil
}

// UpsertNotification 從儲存層處理 upsert 系統通知。
func (s *Store) UpsertNotification(execCtx context.Context, v domain.Notification) error {
	_, err := s.q.UpsertNotification(tenantContext(execCtx, v.TenantID), sqlc.UpsertNotificationParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		Tone:               v.Tone,
		Category:           v.Category,
		Title:              v.Title,
		Body:               v.Body,
		StatusText:         v.StatusText,
		LinkUrl:            v.LinkURL,
		SourceType:         v.SourceType,
		SourceID:           v.SourceID,
		CreatedByAccountID: nullableText(v.CreatedByAccountID),
		CreatedAt:          timestamptz(v.CreatedAt),
		ExpiresAt:          nullableTimestamptz(v.ExpiresAt),
	})
	return err
}

// UpsertNotificationRecipient 從儲存層處理 upsert 系統通知投遞狀態。
func (s *Store) UpsertNotificationRecipient(execCtx context.Context, v domain.NotificationRecipient) error {
	_, err := s.q.UpsertNotificationRecipient(tenantContext(execCtx, v.TenantID), sqlc.UpsertNotificationRecipientParams{
		NotificationID: v.NotificationID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		ReadAt:         nullableTimestamptz(v.ReadAt),
		DeletedAt:      nullableTimestamptz(v.DeletedAt),
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

// ListNotificationItems 從儲存層列出目前帳號可見的系統通知。
func (s *Store) ListNotificationItems(execCtx context.Context, tenantID, accountID string, query domain.NotificationListQuery) ([]domain.NotificationItem, error) {
	items, err := s.q.ListNotificationItems(tenantContext(execCtx, tenantID), sqlc.ListNotificationItemsParams{
		TenantID:        tenantID,
		AccountID:       accountID,
		Tone:            strings.TrimSpace(query.Tone),
		UnreadOnly:      query.UnreadOnly,
		HasCursor:       query.HasCursor,
		CursorCreatedAt: timestamptz(query.CursorCreatedAt),
		CursorID:        strings.TrimSpace(query.CursorID),
		LimitCount:      int32(query.Limit),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromNotificationListRow), nil
}

// CountUnreadNotifications 從儲存層統計目前帳號未讀通知數。
func (s *Store) CountUnreadNotifications(execCtx context.Context, tenantID, accountID string) (int, error) {
	count, err := s.q.CountUnreadNotifications(tenantContext(execCtx, tenantID), sqlc.CountUnreadNotificationsParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// CountNotificationTones 從儲存層統計目前帳號可見通知的 tone 分佈。
func (s *Store) CountNotificationTones(execCtx context.Context, tenantID, accountID string) (domain.NotificationToneCounts, error) {
	counts, err := s.q.CountNotificationTones(tenantContext(execCtx, tenantID), sqlc.CountNotificationTonesParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return domain.NotificationToneCounts{}, err
	}
	return domain.NotificationToneCounts{
		All:     int(counts.AllCount),
		Success: int(counts.SuccessCount),
		Info:    int(counts.InfoCount),
		Warning: int(counts.WarningCount),
	}, nil
}

// MarkNotificationRead 從儲存層將單筆通知標為已讀。
func (s *Store) MarkNotificationRead(execCtx context.Context, tenantID, accountID, notificationID string, readAt time.Time) (domain.NotificationItem, bool, error) {
	item, err := s.q.MarkNotificationRead(tenantContext(execCtx, tenantID), sqlc.MarkNotificationReadParams{
		TenantID:       tenantID,
		AccountID:      accountID,
		NotificationID: notificationID,
		ReadAt:         timestamptz(readAt),
	})
	if isNotFound(err) {
		return domain.NotificationItem{}, false, nil
	}
	if err != nil {
		return domain.NotificationItem{}, false, err
	}
	return fromNotificationReadRow(item), true, nil
}

// MarkAllNotificationsRead 從儲存層將目前帳號全部未讀通知標為已讀。
func (s *Store) MarkAllNotificationsRead(execCtx context.Context, tenantID, accountID string, readAt time.Time) (int, error) {
	count, err := s.q.MarkAllNotificationsRead(tenantContext(execCtx, tenantID), sqlc.MarkAllNotificationsReadParams{
		TenantID:  tenantID,
		AccountID: accountID,
		ReadAt:    timestamptz(readAt),
	})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// AppendAuditLog 從儲存層附加稽覈 log。
func (s *Store) AppendAuditLog(execCtx context.Context, v domain.AuditLog) error {
	_, err := s.q.AppendAuditLog(tenantContext(execCtx, v.TenantID), sqlc.AppendAuditLogParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		ActorAccountID: v.ActorAccountID,
		Action:         v.Action,
		Resource:       v.Resource,
		Target:         v.Target,
		Result:         v.Result,
		TraceID:        v.TraceID,
		Severity:       v.Severity,
		Column10:       mustJSON(v.Details),
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

// ListAuditLogs 從儲存層列出稽覈 logs。
func (s *Store) ListAuditLogs(execCtx context.Context, tenantID string) ([]domain.AuditLog, error) {
	items, err := s.q.ListAuditLogs(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAuditLog), nil
}

// ListAuditLogFacetSources returns tenant-scoped fields required for audit filters and omits sensitive details.
func (s *Store) ListAuditLogFacetSources(execCtx context.Context, tenantID string) ([]domain.WorkspaceAuditLogFacetSource, error) {
	items, err := s.q.ListAuditLogFacetSources(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkspaceAuditLogFacetSource, 0, len(items))
	for _, item := range items {
		out = append(out, domain.WorkspaceAuditLogFacetSource{
			ActorAccountID: item.ActorAccountID,
			Action:         item.Action,
			Resource:       item.Resource,
		})
	}
	return out, nil
}

// ListAuditLogPage 從儲存層列出稽覈 log 分頁。
func (s *Store) ListAuditLogPage(execCtx context.Context, tenantID string, page domain.PageRequest) ([]domain.AuditLog, int, error) {
	page = utils.NormalizePageRequest(page)
	total, err := s.q.CountAuditLogs(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAuditLogsPage(tenantContext(execCtx, tenantID), sqlc.ListAuditLogsPageParams{
		TenantID:    tenantID,
		Sort:        page.Sort,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAuditLog), int(total), nil
}

// ListAuditLogPageFiltered 從儲存層篩選並列出稽覈 log 分頁。
func (s *Store) ListAuditLogPageFiltered(execCtx context.Context, tenantID string, query domain.WorkspaceAuditLogQuery, page domain.PageRequest) ([]domain.AuditLog, int, error) {
	page = utils.NormalizePageRequest(page)
	params := auditLogFilterParams(tenantID, query, page)
	total, err := s.q.CountAuditLogsFiltered(tenantContext(execCtx, tenantID), sqlc.CountAuditLogsFilteredParams{
		TenantID:   params.TenantID,
		OperatorID: params.OperatorID,
		HasFrom:    params.HasFrom,
		FromTime:   params.FromTime,
		HasTo:      params.HasTo,
		ToTime:     params.ToTime,
		Type:       params.Type,
		Keyword:    params.Keyword,
	})
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAuditLogsFilteredPage(tenantContext(execCtx, tenantID), params)
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAuditLog), int(total), nil
}

func auditLogFilterParams(tenantID string, query domain.WorkspaceAuditLogQuery, page domain.PageRequest) sqlc.ListAuditLogsFilteredPageParams {
	from, hasFrom := auditLogFilterTime(query.From, false)
	to, hasTo := auditLogFilterTime(query.To, true)
	return sqlc.ListAuditLogsFilteredPageParams{
		TenantID:    tenantID,
		OperatorID:  strings.TrimSpace(query.OperatorID),
		HasFrom:     hasFrom,
		FromTime:    pgtype.Timestamptz{Time: from, Valid: hasFrom},
		HasTo:       hasTo,
		ToTime:      pgtype.Timestamptz{Time: to, Valid: hasTo},
		Type:        strings.TrimSpace(query.Type),
		Keyword:     strings.TrimSpace(query.Keyword),
		Sort:        page.Sort,
		OffsetCount: int32((page.Page - 1) * page.PageSize),
		LimitCount:  int32(page.PageSize),
	}
}

func auditLogFilterTime(value string, endExclusive bool) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.DateOnly, trimmed); err == nil {
		if endExclusive {
			return parsed.AddDate(0, 0, 1), true
		}
		return parsed, true
	}
	return time.Time{}, false
}

// GetPermissionVersion 從儲存層取得權限 version。
func (s *Store) GetPermissionVersion(execCtx context.Context, tenantID string) (int64, error) {
	v, err := s.q.GetAuthzPermissionVersion(tenantContext(execCtx, tenantID), tenantID)
	if isNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return v.Version, nil
}

// IncrementPermissionVersion 從儲存層處理 increment 權限 version。
func (s *Store) IncrementPermissionVersion(execCtx context.Context, tenantID string) (int64, error) {
	v, err := s.q.IncrementAuthzPermissionVersion(execCtx, sqlc.IncrementAuthzPermissionVersionParams{
		TenantID:  tenantID,
		UpdatedAt: timestamptz(time.Now()),
	})
	if err != nil {
		return 0, err
	}
	return v.Version, nil
}

// UpsertAuthzRelationshipTuple 從儲存層處理 upsert 授權關係 tuple。
func (s *Store) UpsertAuthzRelationshipTuple(execCtx context.Context, v domain.AuthzRelationshipTuple) error {
	_, err := s.q.UpsertAuthzRelationshipTuple(execCtx, sqlc.UpsertAuthzRelationshipTupleParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		ObjectType:  v.ObjectType,
		ObjectID:    v.ObjectID,
		Relation:    v.Relation,
		SubjectType: v.SubjectType,
		SubjectID:   v.SubjectID,
		CreatedAt:   timestamptz(v.CreatedAt),
	})
	return err
}

// DeleteAuthzRelationshipTuple 從儲存層刪除授權關係 tuple。
func (s *Store) DeleteAuthzRelationshipTuple(execCtx context.Context, v domain.AuthzRelationshipTuple) error {
	return s.q.DeleteAuthzRelationshipTuple(execCtx, sqlc.DeleteAuthzRelationshipTupleParams{
		TenantID:    v.TenantID,
		ObjectType:  v.ObjectType,
		ObjectID:    v.ObjectID,
		Relation:    v.Relation,
		SubjectType: v.SubjectType,
		SubjectID:   v.SubjectID,
	})
}

// ListAuthzRelationshipTuplesForObject 從儲存層列出授權關係 tuple for 物件。
func (s *Store) ListAuthzRelationshipTuplesForObject(execCtx context.Context, tenantID, objectType, objectID string) ([]domain.AuthzRelationshipTuple, error) {
	items, err := s.q.ListAuthzRelationshipTuplesForObject(tenantContext(execCtx, tenantID), sqlc.ListAuthzRelationshipTuplesForObjectParams{
		TenantID:   tenantID,
		ObjectType: objectType,
		ObjectID:   objectID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAuthzRelationshipTuple), nil
}

// AppendOutboxEvent 從儲存層附加 outbox 事件。
func (s *Store) AppendOutboxEvent(execCtx context.Context, v domain.OutboxEvent) error {
	_, err := s.q.AppendOutboxEvent(execCtx, sqlc.AppendOutboxEventParams{
		ID:            v.ID,
		TenantID:      v.TenantID,
		EventType:     v.EventType,
		AggregateType: v.AggregateType,
		AggregateID:   v.AggregateID,
		Column6:       mustJSON(v.Payload),
		Status:        v.Status,
		RetryCount:    int32(v.RetryCount),
		LastError:     v.LastError,
		CreatedAt:     timestamptz(v.CreatedAt),
		ProcessedAt:   nullableTimestamptz(v.ProcessedAt),
	})
	return err
}

// ListOutboxEvents 從儲存層列出 outbox 事件。
func (s *Store) ListOutboxEvents(execCtx context.Context, tenantID string) ([]domain.OutboxEvent, error) {
	items, err := s.q.ListOutboxEvents(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOutboxEvent), nil
}

// GetOutboxEventByID 從儲存層依主鍵取得 outbox 事件。
func (s *Store) GetOutboxEventByID(execCtx context.Context, tenantID, id string) (domain.OutboxEvent, bool, error) {
	v, err := s.q.GetOutboxEventByID(tenantContext(execCtx, tenantID), sqlc.GetOutboxEventByIDParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.OutboxEvent{}, false, nil
	}
	if err != nil {
		return domain.OutboxEvent{}, false, err
	}
	return fromOutboxEvent(v), true, nil
}

// ListOutboxEventPage 從儲存層篩選並列出 outbox 事件分頁。
func (s *Store) ListOutboxEventPage(execCtx context.Context, tenantID string, query domain.OutboxEventQuery, page domain.PageRequest) ([]domain.OutboxEvent, int, error) {
	page = utils.NormalizePageRequest(page)
	params := outboxEventFilterParams(tenantID, query, page)
	total, err := s.q.CountOutboxEventsFiltered(tenantContext(execCtx, tenantID), sqlc.CountOutboxEventsFilteredParams{
		TenantID:       params.TenantID,
		Status:         params.Status,
		EventType:      params.EventType,
		LastError:      params.LastError,
		HasRetryCount:  params.HasRetryCount,
		RetryCount:     params.RetryCount,
		FilterHasError: params.FilterHasError,
		HasError:       params.HasError,
	})
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListOutboxEventPage(tenantContext(execCtx, tenantID), params)
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromOutboxEvent), int(total), nil
}

func outboxEventFilterParams(tenantID string, query domain.OutboxEventQuery, page domain.PageRequest) sqlc.ListOutboxEventPageParams {
	params := sqlc.ListOutboxEventPageParams{
		TenantID:    tenantID,
		Status:      strings.TrimSpace(query.Status),
		EventType:   strings.TrimSpace(query.EventType),
		LastError:   strings.TrimSpace(query.LastError),
		Sort:        page.Sort,
		OffsetCount: int32((page.Page - 1) * page.PageSize),
		LimitCount:  int32(page.PageSize),
	}
	if query.RetryCount != nil {
		params.HasRetryCount = true
		params.RetryCount = int32(*query.RetryCount)
	}
	if query.HasError != nil {
		params.FilterHasError = true
		params.HasError = *query.HasError
	}
	return params
}

// ClaimOutboxEvents atomically claims dispatchable outbox events for a worker.
func (s *Store) ClaimOutboxEvents(execCtx context.Context, tenantID string, limit, maxRetries int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		return nil, nil
	}
	if maxRetries <= 0 {
		maxRetries = 1
	}
	items, err := s.q.ClaimOutboxEvents(tenantContext(execCtx, tenantID), sqlc.ClaimOutboxEventsParams{
		TenantID:   tenantID,
		MaxRetries: int32(maxRetries),
		BatchLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOutboxEvent), nil
}

// UpdateOutboxEvent 從儲存層更新 outbox 事件處理狀態。
func (s *Store) UpdateOutboxEvent(execCtx context.Context, v domain.OutboxEvent) error {
	_, err := s.q.UpdateOutboxEvent(tenantContext(execCtx, v.TenantID), sqlc.UpdateOutboxEventParams{
		TenantID:    v.TenantID,
		ID:          v.ID,
		Status:      v.Status,
		RetryCount:  int32(v.RetryCount),
		LastError:   v.LastError,
		ProcessedAt: nullableTimestamptz(v.ProcessedAt),
	})
	return err
}

// DeleteSucceededOutboxEventsBefore 從儲存層刪除已成功且早於 cutoff 的 outbox 事件。
func (s *Store) DeleteSucceededOutboxEventsBefore(execCtx context.Context, tenantID string, before time.Time) (int64, error) {
	return s.q.DeleteSucceededOutboxEventsBefore(tenantContext(execCtx, tenantID), sqlc.DeleteSucceededOutboxEventsBeforeParams{
		TenantID: tenantID,
		Before:   timestamptz(before),
	})
}

// isNotFound 判斷是否為not found。
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// isUniqueConstraint 判斷是否為unique constraint。
func isUniqueConstraint(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// IsExclusionConstraint reports whether an error is the named PostgreSQL exclusion-constraint violation.
func IsExclusionConstraint(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23P01" && pgErr.ConstraintName == constraint
}

// timestamptz 處理 timestamptz。
func timestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

// nullableTimestamptz 處理 nullable timestamptz。
func nullableTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*t)
}

// nullableBool 轉換可選布林值。
func nullableBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

// normalizeFormTemplate 補齊舊種子資料尚未提供的版本欄位。
func normalizeFormTemplate(v domain.FormTemplate) domain.FormTemplate {
	if strings.TrimSpace(v.Status) == "" {
		v.Status = "published"
	}
	if v.CurrentVersion <= 0 {
		v.CurrentVersion = 1
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	return v
}

// float8Ptr 轉換 *float64 為 pgtype.Float8。
func float8Ptr(v *float64) pgtype.Float8 {
	if v == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *v, Valid: true}
}

// float64PtrFrom 轉換 pgtype.Float8 為 *float64。
func float64PtrFrom(v pgtype.Float8) *float64 {
	if !v.Valid {
		return nil
	}
	out := v.Float64
	return &out
}

// nullableText 處理 nullable text。
func nullableText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// textArray 處理 text array。
func textArray(values []string) []string {
	out := utils.CopyStrings(values)
	if out == nil {
		return []string{}
	}
	return out
}

// leaveRequestQueryParams 處理請假請求查詢 params。
func leaveRequestQueryParams(tenantID string, query domain.LeaveRequestQuery) sqlc.CountLeaveRequestsByQueryParams {
	return sqlc.CountLeaveRequestsByQueryParams{
		TenantID:    tenantID,
		EmployeeIds: textArray(trimmedStrings(query.EmployeeIDs)),
		Status:      strings.TrimSpace(query.Status),
		FromDate:    strings.TrimSpace(query.FromDate),
		ToDate:      strings.TrimSpace(query.ToDate),
	}
}

// formInstanceQueryParams 處理表單實例查詢 params。
func formInstanceQueryParams(tenantID string, query domain.FormInstanceQuery) sqlc.CountFormInstancesByQueryParams {
	return sqlc.CountFormInstancesByQueryParams{
		TenantID:           tenantID,
		Status:             strings.TrimSpace(query.Status),
		TemplateID:         strings.TrimSpace(query.TemplateID),
		TemplateKey:        strings.TrimSpace(query.TemplateKey),
		ApplicantAccountID: strings.TrimSpace(query.ApplicantAccountID),
	}
}

// trimmedStrings 處理 trimmed 字串。
func trimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// textFrom 處理 text from。
func textFrom(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// timeFrom 處理時間 from。
func timeFrom(v pgtype.Timestamptz) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time.UTC()
}

// timePtrFrom 處理時間 ptr from。
func timePtrFrom(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

// boolPtrFrom 轉換 nullable boolean。
func boolPtrFrom(v pgtype.Bool) *bool {
	if !v.Valid {
		return nil
	}
	out := v.Bool
	return &out
}

// numericTextFrom 保留 PostgreSQL numeric 的精確十進位表示。
func numericTextFrom(v pgtype.Numeric) string {
	if !v.Valid {
		return ""
	}
	raw, err := v.MarshalJSON()
	if err != nil {
		return ""
	}
	return string(raw)
}

// numericFromFloat64 converts API float input into the fixed two-decimal database representation.
func numericFromFloat64(value float64) (pgtype.Numeric, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return pgtype.Numeric{}, fmt.Errorf("numeric value must be finite")
	}
	var out pgtype.Numeric
	if err := out.Scan(strconv.FormatFloat(value, 'f', 2, 64)); err != nil {
		return pgtype.Numeric{}, err
	}
	return out, nil
}

// float64FromNumeric maps fixed-precision storage back to the existing API contract.
func float64FromNumeric(value pgtype.Numeric) float64 {
	parsed, _ := strconv.ParseFloat(numericTextFrom(value), 64)
	return parsed
}

// nullableDate parses the canonical YYYY-MM-DD policy period representation.
func nullableDate(value string) (pgtype.Date, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Date{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("date must use YYYY-MM-DD: %w", err)
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

// dateTextFrom 轉換 nullable date。
func dateTextFrom(v pgtype.Date) string {
	if !v.Valid {
		return ""
	}
	return v.Time.Format("2006-01-02")
}

// timestampTextFrom 轉換 nullable timestamp。
func timestampTextFrom(v pgtype.Timestamptz) string {
	if !v.Valid {
		return ""
	}
	return v.Time.UTC().Format(time.RFC3339Nano)
}

// mustJSON 處理 must JSON。
func mustJSON(v any) []byte {
	return jsoncodec.Must(v)
}

// jsonMap 處理 JSON map。
func jsonMap(b []byte) map[string]any {
	return jsoncodec.Map(b)
}

// logJSONDecodeFailure 在持久化 JSONB 解碼失敗時輸出 warn 日誌。
// 損壞資料維持 fail-closed 還原為空值,但必須帶表/記錄上下文可觀測。
func logJSONDecodeFailure(target string, err error, attrs ...any) {
	args := make([]any, 0, len(attrs)+4)
	args = append(args, "target", target, "error", err)
	args = append(args, attrs...)
	slog.Warn("postgres store: failed to decode persisted JSON payload", args...)
}

// jsonStrings 處理 JSON 字串陣列。
func jsonStrings(target string, b []byte, attrs ...any) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		logJSONDecodeFailure(target, err, attrs...)
		return nil
	}
	return out
}

// jsonAgentTeamMembers 將數據庫中的 Team 成員快照還原為領域結構。
func jsonAgentTeamMembers(target string, b []byte, attrs ...any) []domain.AgentTeamMember {
	if len(b) == 0 {
		return nil
	}
	var out []domain.AgentTeamMember
	if err := json.Unmarshal(b, &out); err != nil {
		logJSONDecodeFailure(target, err, attrs...)
		return nil
	}
	return out
}

// jsonLocalizedAgentSuggestedQuestions restores ordered locale maps from JSONB.
func jsonLocalizedAgentSuggestedQuestions(b []byte) []domain.LocalizedAgentSuggestedQuestion {
	if len(b) == 0 {
		return nil
	}
	var out []domain.LocalizedAgentSuggestedQuestion
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// jsonEmployeeExperiences 處理 JSON 員工 experiences。
func jsonEmployeeExperiences(b []byte) []domain.EmployeeExperience {
	if len(b) == 0 {
		return nil
	}
	var out []domain.EmployeeExperience
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// jsonEmployeeImportRows 處理 JSON 員工 import 列。
func jsonEmployeeImportRows(b []byte) []domain.EmployeeImportRow {
	if len(b) == 0 {
		return nil
	}
	var out []domain.EmployeeImportRow
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// jsonAttendancePolicyWorkTime 處理 JSON 考勤政策 work 時間。
func jsonAttendancePolicyWorkTime(b []byte) domain.AttendancePolicyWorkTime {
	if len(b) == 0 {
		return domain.AttendancePolicyWorkTime{}
	}
	var out domain.AttendancePolicyWorkTime
	if err := json.Unmarshal(b, &out); err != nil {
		return domain.AttendancePolicyWorkTime{}
	}
	return out
}

// jsonAttendanceLeaveTypes 處理 JSON 考勤請假 types。
func jsonAttendanceLeaveTypes(b []byte) []domain.AttendanceLeaveType {
	if len(b) == 0 {
		return nil
	}
	var out []domain.AttendanceLeaveType
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// jsonPermissions 處理 JSON 權限。
func jsonPermissions(b []byte) []domain.Permission {
	return jsoncodec.Permissions(b)
}

// jsonPermissionPackageContent 處理 JSON 權限包內容。
func jsonPermissionPackageContent(b []byte) domain.PermissionPackageContent {
	if len(b) == 0 {
		return domain.PermissionPackageContent{}
	}
	var out domain.PermissionPackageContent
	if err := json.Unmarshal(b, &out); err != nil {
		return domain.PermissionPackageContent{}
	}
	return out
}

// jsonPermissionSetTemplateContent 處理 JSON 權限集合模板內容。
func jsonPermissionSetTemplateContent(b []byte) domain.PermissionSetTemplateContent {
	if len(b) == 0 {
		return domain.PermissionSetTemplateContent{}
	}
	var out domain.PermissionSetTemplateContent
	if err := json.Unmarshal(b, &out); err != nil {
		return domain.PermissionSetTemplateContent{}
	}
	return out
}

// jsonUserGroupTemplateContent 處理 JSON 使用者羣組模板內容。
func jsonUserGroupTemplateContent(b []byte) domain.UserGroupTemplateContent {
	if len(b) == 0 {
		return domain.UserGroupTemplateContent{}
	}
	var out domain.UserGroupTemplateContent
	if err := json.Unmarshal(b, &out); err != nil {
		return domain.UserGroupTemplateContent{}
	}
	return out
}

// jsonAssumableRoleTemplateContent 處理 JSON 可承擔角色模板內容。
func jsonAssumableRoleTemplateContent(b []byte) domain.AssumableRoleTemplateContent {
	if len(b) == 0 {
		return domain.AssumableRoleTemplateContent{}
	}
	var out domain.AssumableRoleTemplateContent
	if err := json.Unmarshal(b, &out); err != nil {
		return domain.AssumableRoleTemplateContent{}
	}
	return out
}

// jsonRefs 處理 JSON refs。
func jsonRefs(b []byte) []domain.Reference {
	if len(b) == 0 {
		return nil
	}
	var out []domain.Reference
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// mapSlice 映射slice。
func mapSlice[S any, D any](items []S, convert func(S) D) []D {
	if len(items) == 0 {
		return []D{}
	}
	out := make([]D, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

// fromTenant 轉換租戶。
func fromTenant(v sqlc.Tenant) domain.Tenant {
	return domain.Tenant{ID: v.ID, Name: v.Name, CreatedAt: timeFrom(v.CreatedAt)}
}

// fromAccount 轉換帳號。
func fromAccount(v sqlc.Account) domain.Account {
	return domain.Account{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		DisplayName:            v.DisplayName,
		Email:                  v.Email,
		EmployeeID:             v.EmployeeID,
		Status:                 v.Status,
		UserGroupIDs:           utils.CopyStrings(v.UserGroupIds),
		DirectPermissionSetIDs: utils.CopyStrings(v.DirectPermissionSetIds),
		ActiveAssumableRoleID:  v.ActiveAssumableRoleID,
		PreferredLocale:        v.PreferredLocale,
		Version:                v.Version,
		CreatedAt:              timeFrom(v.CreatedAt),
	}
}

// fromUserIdentity 轉換使用者身分。
func fromUserIdentity(v sqlc.UserIdentity) domain.UserIdentity {
	return domain.UserIdentity{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		Provider:  v.Provider,
		Subject:   v.Subject,
		Email:     v.Email,
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

// fromUserGroup 轉換使用者羣組。
func fromUserGroup(v sqlc.UserGroup) domain.UserGroup {
	return domain.UserGroup{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Name:                 v.Name,
		Description:          v.Description,
		MemberAccountIDs:     utils.CopyStrings(v.MemberAccountIds),
		PermissionSetIDs:     utils.CopyStrings(v.PermissionSetIds),
		SourceTemplateKey:    v.SourceTemplateKey,
		SourcePackageVersion: v.SourcePackageVersion,
		Version:              v.Version,
		CreatedAt:            timeFrom(v.CreatedAt),
	}
}

// fromGroupMembership 轉換使用者羣組成員關係。
func fromGroupMembership(v sqlc.AuthzGroupMembership) domain.GroupMembership {
	return domain.GroupMembership{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		UserGroupID:        v.UserGroupID,
		AccountID:          v.AccountID,
		ValidFrom:          timeFrom(v.ValidFrom),
		ValidUntil:         timePtrFrom(v.ValidUntil),
		Source:             v.Source,
		ApprovalInstanceID: v.ApprovalInstanceID,
		CreatedBy:          v.CreatedBy,
		CreatedAt:          timeFrom(v.CreatedAt),
	}
}

// fromPermissionSet 轉換權限集合。
func fromPermissionSet(v sqlc.PermissionSet) domain.PermissionSet {
	return domain.PermissionSet{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Name:                 v.Name,
		Description:          v.Description,
		Permissions:          jsonPermissions(v.Permissions),
		SourceTemplateKey:    v.SourceTemplateKey,
		SourcePackageVersion: v.SourcePackageVersion,
		CreatedAt:            timeFrom(v.CreatedAt),
	}
}

// fromPermissionCatalogItem 轉換權限 catalog 項。
func fromPermissionCatalogItem(v sqlc.Permission) domain.PermissionCatalogItem {
	return domain.PermissionCatalogItem{
		ID:             v.ID,
		TenantID:       v.TenantID,
		Application:    v.Application,
		Resource:       v.Resource,
		Action:         v.Action,
		PermissionType: domain.PermissionType(v.PermissionType),
		MenuKey:        v.MenuKey,
		Name:           v.Name,
		Description:    v.Description,
		HighRisk:       v.HighRisk,
		Severity:       v.Severity,
		CreatedAt:      timeFrom(v.CreatedAt),
	}
}

// fromMenuItem 轉換選單項。
func fromMenuItem(v sqlc.MenuItem) domain.MenuItem {
	return domain.MenuItem{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Key:       v.Key,
		Label:     v.Label,
		Path:      v.Path,
		Icon:      v.Icon,
		ParentKey: v.ParentKey,
		SortOrder: int(v.SortOrder),
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

// fromPermissionPackage 轉換權限包。
func fromPermissionPackage(v sqlc.PermissionPackage) domain.PermissionPackage {
	return domain.PermissionPackage{
		ID:              v.ID,
		ApplicationCode: v.ApplicationCode,
		Version:         v.Version,
		Status:          domain.PermissionPackageStatus(v.Status),
		Content:         jsonPermissionPackageContent(v.Content),
		Checksum:        v.Checksum,
		CreatedAt:       timeFrom(v.CreatedAt),
		PublishedAt:     timePtrFrom(v.PublishedAt),
	}
}

// fromPermissionSetTemplate 轉換權限集合模板。
func fromPermissionSetTemplate(v sqlc.PermissionSetTemplate) domain.PermissionSetTemplate {
	return domain.PermissionSetTemplate{
		ID:          v.ID,
		PackageID:   v.PackageID,
		TemplateKey: v.TemplateKey,
		Name:        v.Name,
		Content:     jsonPermissionSetTemplateContent(v.Content),
		Version:     v.Version,
	}
}

// fromUserGroupTemplate 轉換使用者羣組模板。
func fromUserGroupTemplate(v sqlc.UserGroupTemplate) domain.UserGroupTemplate {
	return domain.UserGroupTemplate{
		ID:          v.ID,
		PackageID:   v.PackageID,
		TemplateKey: v.TemplateKey,
		Name:        v.Name,
		Content:     jsonUserGroupTemplateContent(v.Content),
		Version:     v.Version,
	}
}

// fromAssumableRoleTemplate 轉換可承擔角色模板。
func fromAssumableRoleTemplate(v sqlc.AssumableRoleTemplate) domain.AssumableRoleTemplate {
	return domain.AssumableRoleTemplate{
		ID:          v.ID,
		PackageID:   v.PackageID,
		TemplateKey: v.TemplateKey,
		Name:        v.Name,
		Content:     jsonAssumableRoleTemplateContent(v.Content),
		Version:     v.Version,
	}
}

// fromPermissionPackageImport 轉換權限包導入記錄。
func fromPermissionPackageImport(v sqlc.PermissionPackageImport) domain.PermissionPackageImport {
	return domain.PermissionPackageImport{
		ID:            v.ID,
		TenantID:      v.TenantID,
		PackageID:     v.PackageID,
		Version:       v.Version,
		ImportedAt:    timeFrom(v.ImportedAt),
		ImportedBy:    v.ImportedBy,
		ArtifactIDMap: jsonMap(v.ArtifactIDMap),
	}
}

// fromPermissionSetItem 轉換權限集合項。
func fromPermissionSetItem(v sqlc.PermissionSetItem) domain.PermissionSetItem {
	return domain.PermissionSetItem{
		ID:              v.ID,
		TenantID:        v.TenantID,
		PermissionSetID: v.PermissionSetID,
		PermissionID:    v.PermissionID,
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

// fromPermissionSetAssignment 轉換權限集合指派。
func fromPermissionSetAssignment(v sqlc.AuthzPermissionSetAssignment) domain.PermissionSetAssignment {
	return domain.PermissionSetAssignment{
		ID:              v.ID,
		TenantID:        v.TenantID,
		PrincipalType:   v.PrincipalType,
		PrincipalID:     v.PrincipalID,
		PermissionSetID: v.PermissionSetID,
		Effect:          v.Effect,
		DataScopeID:     v.DataScopeID,
		ConditionID:     v.ConditionID,
		StartsAt:        timePtrFrom(v.StartsAt),
		ExpiresAt:       timePtrFrom(v.ExpiresAt),
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

// fromDataScope 轉換資料範圍。
func fromDataScope(v sqlc.AuthzDataScope) domain.DataScope {
	return domain.DataScope{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		ScopeType: v.ScopeType,
		Params:    jsonMap(v.Params),
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

// fromFieldPolicy 轉換欄位政策。
func fromFieldPolicy(v sqlc.AuthzFieldPolicy) domain.FieldPolicy {
	return domain.FieldPolicy{
		ID:              v.ID,
		TenantID:        v.TenantID,
		ApplicationCode: v.ApplicationCode,
		ResourceType:    v.ResourceType,
		FieldName:       v.FieldName,
		Effect:          v.Effect,
		MaskStrategy:    v.MaskStrategy,
		PermissionID:    v.PermissionID,
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

// fromAssumableRole 轉換 assumable 角色。
func fromAssumableRole(v sqlc.AssumableRole) domain.AssumableRole {
	return domain.AssumableRole{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		Description:            v.Description,
		PermissionSetIDs:       utils.CopyStrings(v.PermissionSetIds),
		Trusted:                v.Trusted,
		TrustPolicy:            jsonMap(v.TrustPolicy),
		PermissionBoundary:     jsonMap(v.PermissionBoundary),
		SessionDurationSeconds: int(v.SessionDurationSeconds),
		SourceTemplateKey:      v.SourceTemplateKey,
		SourcePackageVersion:   v.SourcePackageVersion,
		CreatedAt:              timeFrom(v.CreatedAt),
	}
}

// fromAssumableRoleSession 轉換 assumable 角色 session。
func fromAssumableRoleSession(v sqlc.AuthzAssumableRoleSession) domain.AssumableRoleSession {
	return domain.AssumableRoleSession{
		ID:              v.ID,
		TenantID:        v.TenantID,
		AccountID:       v.AccountID,
		AssumableRoleID: v.AssumableRoleID,
		SessionPolicy:   jsonMap(v.SessionPolicy),
		ExpiresAt:       timeFrom(v.ExpiresAt),
		RevokedAt:       timePtrFrom(v.RevokedAt),
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

// fromOrgUnit 轉換組織單位。
func fromOrgUnit(v sqlc.OrgUnit) domain.OrgUnit {
	return domain.OrgUnit{
		ID:             v.ID,
		TenantID:       v.TenantID,
		Code:           v.Code,
		Name:           v.Name,
		NameEN:         v.NameEn,
		ParentID:       v.ParentID,
		Path:           utils.CopyStrings(v.Path),
		Source:         v.Source,
		Closed:         v.Closed,
		ShowInOrgChart: v.ShowInOrgChart,
		CreatedAt:      timeFrom(v.CreatedAt),
		UpdatedAt:      timeFrom(v.UpdatedAt),
	}
}

// fromPosition 轉換崗位。
func fromPosition(v sqlc.Position) domain.Position {
	return domain.Position{
		ID:          v.ID,
		TenantID:    v.TenantID,
		Code:        v.Code,
		Name:        v.Name,
		NameEN:      v.NameEn,
		Level:       v.Level,
		Status:      v.Status,
		Description: v.Description,
		Source:      v.Source,
		CreatedAt:   timeFrom(v.CreatedAt),
		UpdatedAt:   timeFrom(v.UpdatedAt),
	}
}

// fromEmployee 轉換員工。
func fromEmployee(v sqlc.Employee) domain.Employee {
	return domain.Employee{
		ID:                    v.ID,
		TenantID:              v.TenantID,
		EmployeeNo:            v.EmployeeNo,
		Name:                  v.Name,
		CompanyEmail:          v.CompanyEmail,
		PersonalEmail:         v.PersonalEmail,
		Phone:                 v.Phone,
		OrgUnitID:             v.OrgUnitID,
		AccountID:             v.AccountID,
		ManagerEmployeeID:     textFrom(v.ManagerEmployeeID),
		PositionID:            v.PositionID,
		Position:              v.Position,
		Category:              v.Category,
		Status:                v.Status,
		EmploymentStatus:      v.EmploymentStatus,
		ShowInOrgChart:        v.ShowInOrgChart,
		HireDate:              timePtrFrom(v.HireDate),
		ResignDate:            timePtrFrom(v.ResignDate),
		BasicInfo:             jsonMap(v.BasicInfo),
		EmploymentInfo:        jsonMap(v.EmploymentInfo),
		EducationMilitaryInfo: jsonMap(v.EducationMilitaryInfo),
		ContactInfo:           jsonMap(v.ContactInfo),
		InsuranceInfo:         jsonMap(v.InsuranceInfo),
		InternalExperiences:   jsonEmployeeExperiences(v.InternalExperiences),
		CreatedAt:             timeFrom(v.CreatedAt),
		UpdatedAt:             timeFrom(v.UpdatedAt),
	}
}

// fromEmployeeImportSession 轉換員工 import session。
func fromEmployeeImportSession(v sqlc.EmployeeImportSession) domain.EmployeeImportSession {
	return domain.EmployeeImportSession{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Filename:             v.Filename,
		ObjectProvider:       v.ObjectProvider,
		ObjectBucket:         v.ObjectBucket,
		ObjectKey:            v.ObjectKey,
		ContentType:          v.ContentType,
		SizeBytes:            v.SizeBytes,
		SHA256:               v.Sha256,
		Status:               v.Status,
		Rows:                 jsonEmployeeImportRows(v.Rows),
		Summary:              jsonMap(v.Summary),
		CreatedByAccountID:   v.CreatedByAccountID,
		ConfirmedByAccountID: v.ConfirmedByAccountID,
		CreatedAt:            timeFrom(v.CreatedAt),
		ExpiresAt:            timeFrom(v.ExpiresAt),
		ConfirmedAt:          timePtrFrom(v.ConfirmedAt),
	}
}

// fromEmploymentContract 轉換員工合約。
func fromEmploymentContract(v sqlc.EmploymentContract) domain.EmploymentContract {
	return domain.EmploymentContract{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		ContractType:        v.ContractType,
		ContractNo:          v.ContractNo,
		StartDate:           timeFrom(v.StartDate),
		EndDate:             timePtrFrom(v.EndDate),
		Status:              v.Status,
		AttachmentObjectKey: v.AttachmentObjectKey,
		Notes:               v.Notes,
		Version:             v.Version,
		CreatedAt:           timeFrom(v.CreatedAt),
		UpdatedAt:           timeFrom(v.UpdatedAt),
	}
}

// fromOutboxEvent 轉換 outbox 事件。
func fromOutboxEvent(v sqlc.OutboxEvent) domain.OutboxEvent {
	return domain.OutboxEvent{
		ID:            v.ID,
		TenantID:      v.TenantID,
		EventType:     v.EventType,
		AggregateType: v.AggregateType,
		AggregateID:   v.AggregateID,
		Payload:       jsonMap(v.Payload),
		Status:        v.Status,
		RetryCount:    int(v.RetryCount),
		LastError:     v.LastError,
		CreatedAt:     timeFrom(v.CreatedAt),
		ProcessedAt:   timePtrFrom(v.ProcessedAt),
	}
}

// fromAttendancePolicy 轉換考勤政策。
func fromAttendancePolicy(v sqlc.AttendancePolicy) domain.AttendancePolicy {
	return domain.AttendancePolicy{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		WorkTime:           jsonAttendancePolicyWorkTime(v.WorkTime),
		LeaveTypes:         jsonAttendanceLeaveTypes(v.LeaveTypes),
		Version:            int(v.Version),
		EffectiveFrom:      timePtrFrom(v.EffectiveFrom),
		UpdatedByAccountID: v.UpdatedByAccountID,
		CreatedAt:          timeFrom(v.CreatedAt),
		UpdatedAt:          timeFrom(v.UpdatedAt),
	}
}

// fromLeaveBalance 轉換請假 balance。
func fromLeaveBalance(v sqlc.LeaveBalance) domain.LeaveBalance {
	return domain.LeaveBalance{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EmployeeID:     v.EmployeeID,
		LeaveType:      v.LeaveType,
		LeaveTypeID:    v.LeaveTypeID,
		RemainingHours: float64FromNumeric(v.RemainingHours),
		PeriodStart:    dateTextFrom(v.PeriodStart),
		PeriodEnd:      dateTextFrom(v.PeriodEnd),
		GrantedHours:   float64FromNumeric(v.GrantedHours),
		UsedHours:      float64FromNumeric(v.UsedHours),
		Source:         v.Source,
		PolicyVersion:  int(v.PolicyVersion),
		ProrateRatio:   float64PtrFrom(v.ProrateRatio),
		UpdatedAt:      timeFrom(v.UpdatedAt),
	}
}

// fromLeaveRequest 轉換請假請求。
func fromLeaveRequest(v sqlc.LeaveRequest) domain.LeaveRequest {
	return domain.LeaveRequest{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		EmployeeID:         v.EmployeeID,
		LeaveType:          v.LeaveType,
		LeaveTypeID:        v.LeaveTypeID,
		PolicyVersion:      int(v.PolicyVersion),
		RuleSnapshot:       jsonMap(v.RuleSnapshot),
		EvaluationSnapshot: jsonMap(v.EvaluationSnapshot),
		StartAt:            timeFrom(v.StartAt),
		EndAt:              timeFrom(v.EndAt),
		Hours:              v.Hours,
		Reason:             v.Reason,
		Status:             v.Status,
		FormInstanceID:     v.FormInstanceID,
		LeaveBalanceID:     textFrom(v.LeaveBalanceID),
		CreatedAt:          timeFrom(v.CreatedAt),
	}
}

// fromAttendanceWorksite 轉換考勤工作地點。
func fromAttendanceWorksite(v sqlc.AttendanceWorksite) domain.AttendanceWorksite {
	return domain.AttendanceWorksite{
		ID:           v.ID,
		TenantID:     v.TenantID,
		Name:         v.Name,
		Address:      v.Address,
		Latitude:     v.Latitude,
		Longitude:    v.Longitude,
		RadiusMeters: int(v.RadiusMeters),
		Status:       v.Status,
		CreatedAt:    timeFrom(v.CreatedAt),
		UpdatedAt:    timeFrom(v.UpdatedAt),
	}
}

// fromAttendanceShift 轉換考勤班別。
func fromAttendanceShift(v sqlc.AttendanceShift) domain.AttendanceShift {
	return domain.AttendanceShift{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		ClockInStart:           v.ClockInStart,
		ClockInEnd:             v.ClockInEnd,
		ClockOutStart:          v.ClockOutStart,
		ClockOutEnd:            v.ClockOutEnd,
		LateGraceMinutes:       int(v.LateGraceMinutes),
		EarlyLeaveGraceMinutes: int(v.EarlyLeaveGraceMinutes),
		Status:                 v.Status,
		CreatedAt:              timeFrom(v.CreatedAt),
		UpdatedAt:              timeFrom(v.UpdatedAt),
	}
}

// fromAttendanceShiftAssignment 轉換員工班別指派。
func fromAttendanceShiftAssignment(v sqlc.AttendanceShiftAssignment) domain.AttendanceShiftAssignment {
	return domain.AttendanceShiftAssignment{ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, ShiftID: v.ShiftID, WorksiteID: v.WorksiteID, EffectiveFrom: timeFrom(v.EffectiveFrom), EffectiveTo: timePtrFrom(v.EffectiveTo), Status: v.Status, CreatedAt: timeFrom(v.CreatedAt), UpdatedAt: timeFrom(v.UpdatedAt)}
}

// fromAttendanceClockRecord 轉換考勤打卡 record。
func fromAttendanceClockRecord(v sqlc.AttendanceClockRecord) domain.AttendanceClockRecord {
	return domain.AttendanceClockRecord{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		ShiftAssignmentID:   textFrom(v.ShiftAssignmentID),
		ShiftID:             textFrom(v.ShiftID),
		WorksiteID:          textFrom(v.WorksiteID),
		WorkDate:            v.WorkDate,
		Direction:           v.Direction,
		ClientEventID:       v.ClientEventID,
		ClockedAt:           timeFrom(v.ClockedAt),
		Latitude:            v.Latitude,
		Longitude:           v.Longitude,
		AccuracyMeters:      v.AccuracyMeters,
		DistanceMeters:      v.DistanceMeters,
		RecordStatus:        v.RecordStatus,
		RejectionReason:     v.RejectionReason,
		Source:              v.Source,
		DeviceID:            v.DeviceID,
		DeviceInfo:          jsonMap(v.DeviceInfo),
		CorrectionRequestID: v.CorrectionRequestID,
		Voided:              v.Voided,
		VoidedAt:            timePtrFrom(v.VoidedAt),
		VoidedByAccountID:   v.VoidedByAccountID,
		VoidReason:          v.VoidReason,
		CreatedAt:           timeFrom(v.CreatedAt),
	}
}

// fromAttendanceDailySummary 轉換考勤日彙總。
func fromAttendanceDailySummary(v sqlc.AttendanceDailySummary) domain.AttendanceDailySummary {
	return domain.AttendanceDailySummary{
		ID:              v.ID,
		TenantID:        v.TenantID,
		EmployeeID:      v.EmployeeID,
		WorkDate:        v.WorkDate,
		ShiftStart:      v.ShiftStart,
		ShiftEnd:        v.ShiftEnd,
		ShiftHours:      v.ShiftHours,
		DailyHours:      v.DailyHours,
		ClockHours:      v.ClockHours,
		ClockStart:      v.ClockStart,
		ClockEnd:        v.ClockEnd,
		AttendStart:     v.AttendStart,
		AttendEnd:       v.AttendEnd,
		AttendHours:     v.AttendHours,
		AttendCounted:   v.AttendCounted,
		LeaveType:       v.LeaveType,
		LeaveStart:      v.LeaveStart,
		LeaveEnd:        v.LeaveEnd,
		LeaveHours:      v.LeaveHours,
		LeaveCounted:    v.LeaveCounted,
		Leave2Type:      v.Leave2Type,
		Leave2Start:     v.Leave2Start,
		Leave2End:       v.Leave2End,
		Leave2Hours:     v.Leave2Hours,
		Leave2Counted:   v.Leave2Counted,
		OvertimeStart:   v.OvertimeStart,
		OvertimeEnd:     v.OvertimeEnd,
		OvertimeHours:   v.OvertimeHours,
		OvertimeCounted: v.OvertimeCounted,
		Payload:         jsonMap(v.Payload),
		Source:          v.Source,
		ExternalRef:     v.ExternalRef,
		CreatedAt:       timeFrom(v.CreatedAt),
		UpdatedAt:       timeFrom(v.UpdatedAt),
	}
}

// fromAttendanceCorrectionRequest 轉換考勤 correction 請求。
func fromAttendanceCorrectionRequest(v sqlc.AttendanceCorrectionRequest) domain.AttendanceCorrectionRequest {
	return domain.AttendanceCorrectionRequest{
		ID:                       v.ID,
		TenantID:                 v.TenantID,
		EmployeeID:               v.EmployeeID,
		Direction:                v.Direction,
		RequestedClockedAt:       timeFrom(v.RequestedClockedAt),
		WorkDate:                 v.WorkDate,
		CorrectionType:           v.CorrectionType,
		TargetClockRecordID:      v.TargetClockRecordID,
		ReplacementClockRecordID: v.ReplacementClockRecordID,
		Reason:                   v.Reason,
		Status:                   v.Status,
		FormInstanceID:           v.FormInstanceID,
		ClockRecordID:            v.ClockRecordID,
		ReviewedByAccountID:      v.ReviewedByAccountID,
		ReviewReason:             v.ReviewReason,
		ReviewedAt:               timePtrFrom(v.ReviewedAt),
		CreatedAt:                timeFrom(v.CreatedAt),
		UpdatedAt:                timeFrom(v.UpdatedAt),
	}
}

// fromOvertimeRequest 轉換加班申請。
func fromOvertimeRequest(v sqlc.OvertimeRequest) domain.OvertimeRequest {
	return domain.OvertimeRequest{
		ID:               v.ID,
		TenantID:         v.TenantID,
		EmployeeID:       v.EmployeeID,
		WorkDate:         v.WorkDate,
		StartAt:          timeFrom(v.StartAt),
		EndAt:            timeFrom(v.EndAt),
		Hours:            v.Hours,
		OvertimeType:     v.OvertimeType,
		CompensationType: v.CompensationType,
		Reason:           v.Reason,
		Status:           v.Status,
		FormInstanceID:   v.FormInstanceID,
		CreatedAt:        timeFrom(v.CreatedAt),
		UpdatedAt:        timeFrom(v.UpdatedAt),
	}
}

// fromFormDefinitionDraft 轉換表單定義草稿。
func fromFormDefinitionDraft(v sqlc.FormDefinitionDraft) domain.FormDefinitionDraft {
	decodeCtx := []any{"table", "form_definition_drafts", "draft_id", v.ID, "tenant_id", v.TenantID}
	var authoring domain.FormDefinitionSchemaV2
	if err := json.Unmarshal(v.AuthoringSchema, &authoring); err != nil {
		logJSONDecodeFailure("authoring_schema", err, decodeCtx...)
		authoring = domain.FormDefinitionSchemaV2{}
	}
	var compiled map[string]any
	if err := json.Unmarshal(v.CompiledSchema, &compiled); err != nil {
		logJSONDecodeFailure("compiled_schema", err, decodeCtx...)
		compiled = map[string]any{}
	}
	var validation domain.FormDefinitionValidation
	if err := json.Unmarshal(v.ValidationResult, &validation); err != nil {
		logJSONDecodeFailure("validation_result", err, decodeCtx...)
		validation = domain.FormDefinitionValidation{}
	}
	return domain.FormDefinitionDraft{
		ID: v.ID, TenantID: v.TenantID, OwnerAccountID: v.OwnerAccountID, BaseTemplateID: v.BaseTemplateID,
		SchemaVersion: int(v.SchemaVersion), AuthoringSchema: authoring, CompiledSchema: compiled,
		Status: domain.FormDefinitionDraftStatus(v.Status), Revision: v.Revision, Source: v.Source,
		AgentID: v.AgentID, AgentRunID: v.AgentRunID, AgentSessionID: v.AgentSessionID, ToolCallID: v.ToolCallID,
		ValidationResult: validation, SubmittedAt: timePtrFrom(v.SubmittedAt), PublishedTemplateID: v.PublishedTemplateID,
		CreatedAt: timeFrom(v.CreatedAt), UpdatedAt: timeFrom(v.UpdatedAt),
	}
}

// fromFormTemplate 轉換表單範本。
func fromFormTemplate(v sqlc.FormTemplate) domain.FormTemplate {
	return domain.FormTemplate{
		ID:             v.ID,
		TenantID:       v.TenantID,
		Key:            v.Key,
		Name:           v.Name,
		Description:    v.Description,
		Schema:         jsonMap(v.Schema),
		Status:         v.Status,
		CurrentVersion: int(v.CurrentVersion),
		CreatedAt:      timeFrom(v.CreatedAt),
		UpdatedAt:      timeFrom(v.UpdatedAt),
		DeletedAt:      timePtrFrom(v.DeletedAt),
	}
}

// fromFormTemplateVersion 轉換不可變表單版本。
func fromFormTemplateVersion(v sqlc.FormTemplateVersion) domain.FormTemplateVersion {
	return domain.FormTemplateVersion{
		ID: v.ID, TenantID: v.TenantID, TemplateID: v.TemplateID, Version: int(v.Version),
		Schema: jsonMap(v.Schema), Status: v.Status, CreatedAt: timeFrom(v.CreatedAt), PublishedAt: timePtrFrom(v.PublishedAt),
	}
}

// fromFormInstance 轉換表單實例。
func fromFormInstance(v sqlc.FormInstance) domain.FormInstance {
	return domain.FormInstance{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		TemplateID:         v.TemplateID,
		TemplateVersionID:  v.TemplateVersionID,
		ApplicantAccountID: v.ApplicantAccountID,
		Status:             v.Status,
		Payload:            jsonMap(v.Payload),
		SubmittedAt:        timeFrom(v.SubmittedAt),
		ApprovedBy:         v.ApprovedBy,
		CurrentRunID:       v.CurrentRunID,
		Version:            v.Version,
		UpdatedAt:          timeFrom(v.UpdatedAt),
	}
}

// fromFormInstanceFieldValue 轉換類型化欄位投影。
func fromFormInstanceFieldValue(v sqlc.FormInstanceFieldValue) domain.FormInstanceFieldValue {
	return domain.FormInstanceFieldValue{
		TenantID: v.TenantID, FormInstanceID: v.FormInstanceID, TemplateID: v.TemplateID,
		TemplateVersionID: v.TemplateVersionID, FieldID: v.FieldID, ValueType: v.ValueType,
		ValueText: textFrom(v.ValueText), ValueNumber: numericTextFrom(v.ValueNumber), ValueBoolean: boolPtrFrom(v.ValueBoolean),
		ValueDate: dateTextFrom(v.ValueDate), ValueTimestamp: timestampTextFrom(v.ValueTimestamp),
		ValueJSON: append([]byte(nil), v.ValueJson...), CreatedAt: timeFrom(v.CreatedAt),
	}
}

// fromPlatformTaskItem 轉換平臺任務項目。
func fromPlatformTaskItem(v sqlc.PlatformTaskItem) domain.PlatformTaskRecordItem {
	return domain.PlatformTaskRecordItem{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		WorkDate:  v.WorkDate,
		Title:     v.Title,
		Category:  v.Category,
		Product:   v.Product,
		Hours:     v.Hours,
		Note:      v.Note,
		CreatedAt: timeFrom(v.CreatedAt),
		UpdatedAt: timeFrom(v.UpdatedAt),
	}
}

// fromPlatformTaskTodo 轉換平臺任務待辦。
func fromPlatformTaskTodo(v sqlc.PlatformTaskTodo) domain.PlatformTaskTodoRecord {
	return domain.PlatformTaskTodoRecord{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		AccountID:           v.AccountID,
		Text:                v.Text,
		DueDate:             v.DueDate,
		Status:              v.Status,
		ConvertedTaskItemID: v.ConvertedTaskItemID,
		CreatedAt:           timeFrom(v.CreatedAt),
		UpdatedAt:           timeFrom(v.UpdatedAt),
	}
}

// fromAgentRun 轉換 agent 執行。
func fromAgentRun(v sqlc.AgentRun) domain.AgentRun {
	return domain.AgentRun{
		ID:            v.ID,
		TenantID:      v.TenantID,
		AccountID:     v.AccountID,
		AgentID:       textFrom(v.AgentID),
		SessionID:     textFrom(v.SessionID),
		Mode:          v.Mode,
		Prompt:        v.Prompt,
		Answer:        v.Answer,
		Status:        v.Status,
		References:    jsonRefs(v.ReferenceItems),
		LLMCallCount:  v.LlmCallCount,
		InputTokens:   v.InputTokens,
		CachedTokens:  v.CachedTokens,
		OutputTokens:  v.OutputTokens,
		TotalTokens:   v.TotalTokens,
		UsageComplete: v.UsageComplete,
		CreatedAt:     timeFrom(v.CreatedAt),
		UpdatedAt:     timeFrom(v.UpdatedAt),
	}
}

// fromAgentModel 轉換 agent 模型。
func fromAgentModel(v sqlc.AgentModel) domain.AgentModel {
	return domain.AgentModel{
		ID:               v.ID,
		TenantID:         v.TenantID,
		Name:             v.Name,
		Provider:         v.Provider,
		ModelName:        v.ModelName,
		LiteLLMModel:     v.LitellmModel,
		APIBaseURL:       v.ApiBaseUrl,
		APIKeyCiphertext: v.ApiKeyCiphertext,
		APIKeySet:        strings.TrimSpace(v.ApiKeyCiphertext) != "",
		APIKeyPreview:    v.ApiKeyPreview,
		RateLimitRPM:     int(v.RateLimitRpm),
		Status:           domain.AgentModelStatus(v.Status),
		TimeoutSeconds:   int(v.TimeoutSeconds),
		MonthlyQuota:     v.MonthlyQuota,
		UsedQuota:        v.UsedQuota,
		LastTestedAt:     timePtrFrom(v.LastTestedAt),
		LastTestStatus:   v.LastTestStatus,
		LastTestMessage:  v.LastTestMessage,
		SyncStatus:       domain.AgentModelSyncStatus(v.SyncStatus),
		LastSyncedAt:     timePtrFrom(v.LastSyncedAt),
		LastSyncError:    v.LastSyncError,
		SyncedConfigHash: v.SyncedConfigHash,
		CreatedAt:        timeFrom(v.CreatedAt),
		UpdatedAt:        timeFrom(v.UpdatedAt),
	}
}

// fromAgentExternalTool maps generated database fields into the domain contract.
func fromAgentExternalTool(v sqlc.AgentExternalTool) domain.AgentExternalTool {
	return domain.AgentExternalTool{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Name:                 v.Name,
		Description:          v.Description,
		Kind:                 v.Kind,
		Transport:            v.Transport,
		EndpointURL:          v.EndpointUrl,
		AuthType:             v.AuthType,
		AuthHeaderName:       v.AuthHeaderName,
		AuthUsername:         v.AuthUsername,
		AuthSecretCiphertext: v.AuthSecretCiphertext,
		CredentialSet:        v.AuthSecretCiphertext != "",
		CreatedByAccountID:   textFrom(v.CreatedByAccountID),
		CreatedAt:            timeFrom(v.CreatedAt),
	}
}

func maskStoredSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

// fromAgentDefinition 轉換 agent 定義。
func fromAgentDefinition(v sqlc.AgentDefinition) domain.AgentDefinition {
	decodeCtx := []any{"table", "agent_definitions", "agent_id", v.ID, "tenant_id", v.TenantID}
	return domain.AgentDefinition{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		Name:                          v.Name,
		Description:                   v.Description,
		Emoji:                         v.Emoji,
		Category:                      domain.AgentCategory(v.Category),
		ModelID:                       v.ModelID,
		MainAgentRole:                 v.MainAgentRole,
		SubAgents:                     jsonAgentTeamMembers("sub_agents", v.SubAgents, decodeCtx...),
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            jsonStrings("suggested_questions", v.SuggestedQuestions, decodeCtx...),
		SuggestedQuestionTranslations: jsonLocalizedAgentSuggestedQuestions(v.SuggestedQuestionTranslations),
		Tools:                         jsonStrings("tools", v.Tools, decodeCtx...),
		KnowledgeBaseIDs:              jsonStrings("knowledge_base_ids", v.KnowledgeBaseIds, decodeCtx...),
		Status:                        domain.AgentDefinitionStatus(v.Status),
		Visibility:                    domain.AgentVisibility(v.Visibility),
		VisibilityTargets:             jsonStrings("visibility_targets", v.VisibilityTargets, decodeCtx...),
		TimeoutSeconds:                int(v.TimeoutSeconds),
		Version:                       int(v.Version),
		PublishedVersion:              int(v.PublishedVersion),
		Usage: domain.AgentUsageStats{
			TotalRuns:    v.UsageTotalRuns,
			SuccessRuns:  v.UsageSuccessRuns,
			FailedRuns:   v.UsageFailedRuns,
			AvgLatencyMs: int(v.UsageAvgLatencyMs),
			LastRunAt:    timePtrFrom(v.UsageLastRunAt),
			TopPrompts:   jsonStrings("usage_top_prompts", v.UsageTopPrompts, decodeCtx...),
		},
		CreatedByAccountID: textFrom(v.CreatedByAccountID),
		UpdatedByAccountID: textFrom(v.UpdatedByAccountID),
		CreatedAt:          timeFrom(v.CreatedAt),
		UpdatedAt:          timeFrom(v.UpdatedAt),
	}
}

// fromAgentDefinitionVersion 轉換 agent 定義版本。
func fromAgentDefinitionVersion(v sqlc.AgentDefinitionVersion) domain.AgentDefinitionVersion {
	decodeCtx := []any{"table", "agent_definition_versions", "agent_definition_version_id", v.ID, "agent_id", v.AgentID, "tenant_id", v.TenantID}
	return domain.AgentDefinitionVersion{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		AgentID:                       v.AgentID,
		Version:                       int(v.Version),
		MainAgentRole:                 v.MainAgentRole,
		SubAgents:                     jsonAgentTeamMembers("sub_agents", v.SubAgents, decodeCtx...),
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            jsonStrings("suggested_questions", v.SuggestedQuestions, decodeCtx...),
		SuggestedQuestionTranslations: jsonLocalizedAgentSuggestedQuestions(v.SuggestedQuestionTranslations),
		Tools:                         jsonStrings("tools", v.Tools, decodeCtx...),
		KnowledgeBaseIDs:              jsonStrings("knowledge_base_ids", v.KnowledgeBaseIds, decodeCtx...),
		ModelID:                       v.ModelID,
		Note:                          v.Note,
		CreatedByAccountID:            textFrom(v.CreatedByAccountID),
		CreatedAt:                     timeFrom(v.CreatedAt),
	}
}

// fromNotificationListRow 轉換通知列表 row。
func fromNotificationListRow(v sqlc.ListNotificationItemsRow) domain.NotificationItem {
	return notificationItemFromFields(v.ID, v.Tone, v.Category, v.Title, v.Body, v.StatusText, v.LinkUrl, v.ReadAt, v.CreatedAt)
}

// fromNotificationReadRow 轉換通知已讀 row。
func fromNotificationReadRow(v sqlc.MarkNotificationReadRow) domain.NotificationItem {
	return notificationItemFromFields(v.ID, v.Tone, v.Category, v.Title, v.Body, v.StatusText, v.LinkUrl, v.ReadAt, v.CreatedAt)
}

// notificationItemFromFields 組合目前帳號的通知投影。
func notificationItemFromFields(id, tone, category, title, body, statusText, linkURL string, readAt, createdAt pgtype.Timestamptz) domain.NotificationItem {
	return domain.NotificationItem{
		ID:         id,
		Tone:       tone,
		Category:   category,
		Title:      title,
		Body:       body,
		StatusText: statusText,
		LinkURL:    linkURL,
		ReadAt:     timePtrFrom(readAt),
		CreatedAt:  timeFrom(createdAt),
	}
}

// fromAuditLog 轉換稽覈 log。
func fromAuditLog(v sqlc.AuditLog) domain.AuditLog {
	return domain.AuditLog{
		ID:             v.ID,
		TenantID:       v.TenantID,
		ActorAccountID: v.ActorAccountID,
		Action:         v.Action,
		Resource:       v.Resource,
		Target:         v.Target,
		Result:         v.Result,
		TraceID:        v.TraceID,
		Severity:       v.Severity,
		Details:        jsonMap(v.Details),
		CreatedAt:      timeFrom(v.CreatedAt),
	}
}

// fromIdentityProvisioningOutboxEvent 轉換身分開通 outbox 事件。
func fromIdentityProvisioningOutboxEvent(v sqlc.IdentityProvisioningOutbox) domain.IdentityProvisioningOutboxEvent {
	return domain.IdentityProvisioningOutboxEvent{
		ID:             v.ID,
		TenantID:       v.TenantID,
		AccountID:      v.AccountID,
		EmployeeID:     v.EmployeeID,
		EmployeeNo:     v.EmployeeNo,
		Email:          v.Email,
		DisplayName:    v.DisplayName,
		Enabled:        v.Enabled,
		SendInvite:     v.SendInvite,
		Status:         v.Status,
		RetryCount:     int(v.RetryCount),
		LastError:      v.LastError,
		NextAttemptAt:  timeFrom(v.NextAttemptAt),
		ClaimExpiresAt: timePtrFrom(v.ClaimExpiresAt),
		CreatedAt:      timeFrom(v.CreatedAt),
		UpdatedAt:      timeFrom(v.UpdatedAt),
	}
}

// fromAuthzRelationshipTuple 轉換授權關係 tuple。
func fromAuthzRelationshipTuple(v sqlc.AuthzRelationshipTuple) domain.AuthzRelationshipTuple {
	return domain.AuthzRelationshipTuple{
		ID:          v.ID,
		TenantID:    v.TenantID,
		ObjectType:  v.ObjectType,
		ObjectID:    v.ObjectID,
		Relation:    v.Relation,
		SubjectType: v.SubjectType,
		SubjectID:   v.SubjectID,
		CreatedAt:   timeFrom(v.CreatedAt),
	}
}

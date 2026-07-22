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
		ID:                v.ID,
		TenantID:          v.TenantID,
		Code:              v.Code,
		Name:              v.Name,
		NameEn:            v.NameEN,
		ParentID:          v.ParentID,
		Path:              utils.CopyStrings(v.Path),
		Source:            v.Source,
		Closed:            v.Closed,
		ManagerPositionID: v.ManagerPositionID,
		CreatedAt:         timestamptz(v.CreatedAt),
		UpdatedAt:         timestamptz(v.UpdatedAt),
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
		ScopeDenyAll:     query.Scope.DenyAll,
		ScopeMatchAny:    query.Scope.MatchAnyEntity,
		ScopeEmployeeIds: query.Scope.EmployeeIDs,
		ScopeOrgUnitIds:  query.Scope.OrgUnitIDs,
		ScopeStatuses:    query.Scope.Statuses,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		PresentFrom:      query.PresentFrom,
		PresentTo:        query.PresentTo,
		Sort:             query.Sort,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromEmployee), nil
}

// ListEmployeePageByQuery 從儲存層列出員工分頁 by 查詢。
func (s *Store) ListEmployeePageByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, int, error) {
	params := sqlc.CountEmployeesFilteredParams{
		TenantID:         tenantID,
		ScopeDenyAll:     query.Scope.DenyAll,
		ScopeMatchAny:    query.Scope.MatchAnyEntity,
		ScopeEmployeeIds: query.Scope.EmployeeIDs,
		ScopeOrgUnitIds:  query.Scope.OrgUnitIDs,
		ScopeStatuses:    query.Scope.Statuses,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		PresentFrom:      query.PresentFrom,
		PresentTo:        query.PresentTo,
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
		ScopeDenyAll:     params.ScopeDenyAll,
		ScopeMatchAny:    params.ScopeMatchAny,
		ScopeEmployeeIds: params.ScopeEmployeeIds,
		ScopeOrgUnitIds:  params.ScopeOrgUnitIds,
		ScopeStatuses:    params.ScopeStatuses,
		Keyword:          params.Keyword,
		DepartmentID:     params.DepartmentID,
		EmploymentStatus: params.EmploymentStatus,
		Category:         params.Category,
		PresentFrom:      params.PresentFrom,
		PresentTo:        params.PresentTo,
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
	total, err := s.q.CountEmployeesFiltered(execCtx, sqlc.CountEmployeesFilteredParams{
		TenantID:         tenantID,
		ScopeDenyAll:     query.Scope.DenyAll,
		ScopeMatchAny:    query.Scope.MatchAnyEntity,
		ScopeEmployeeIds: query.Scope.EmployeeIDs,
		ScopeOrgUnitIds:  query.Scope.OrgUnitIDs,
		ScopeStatuses:    query.Scope.Statuses,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		PresentFrom:      query.PresentFrom,
		PresentTo:        query.PresentTo,
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

// InsertAttendancePolicyVersion appends one immutable attendance policy version.
func (s *Store) InsertAttendancePolicyVersion(execCtx context.Context, v domain.AttendancePolicy) error {
	version := v.Version
	if version <= 0 {
		version = 1
	}
	effectiveFrom := v.PublishedAt
	if v.EffectiveFrom != nil {
		effectiveFrom = *v.EffectiveFrom
	}
	_, err := s.q.InsertAttendancePolicyVersion(tenantContext(execCtx, v.TenantID), sqlc.InsertAttendancePolicyVersionParams{
		TenantID:             v.TenantID,
		Version:              int32(version),
		WorkTime:             mustJSON(v.WorkTime),
		EffectiveFrom:        timestamptz(effectiveFrom),
		PublishedByAccountID: v.PublishedByAccountID,
		PublishedAt:          timestamptz(v.PublishedAt),
	})
	if isUniqueConstraint(err, "attendance_policy_versions_pkey") {
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

// GetAttendancePolicyAsOf returns the policy effective at the requested instant.
func (s *Store) GetAttendancePolicyAsOf(execCtx context.Context, tenantID string, asOf time.Time) (domain.AttendancePolicy, bool, error) {
	v, err := s.q.GetAttendancePolicyAsOf(tenantContext(execCtx, tenantID), sqlc.GetAttendancePolicyAsOfParams{
		TenantID: tenantID,
		AsOf:     timestamptz(asOf),
	})
	if isNotFound(err) {
		return domain.AttendancePolicy{}, false, nil
	}
	if err != nil {
		return domain.AttendancePolicy{}, false, err
	}
	return fromAttendancePolicy(v), true, nil
}

// ListLeaveTypes returns manually maintained leave_types for one tenant.
func (s *Store) ListLeaveTypes(execCtx context.Context, tenantID string) ([]domain.LeaveType, error) {
	rows, err := s.db.Query(tenantContext(execCtx, tenantID), `
SELECT
    id,
    code,
    name_zh,
    name_en,
    requires_balance,
    status = 'active',
    display_order
FROM leave_types
WHERE tenant_id = $1
ORDER BY display_order ASC, code ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.LeaveType, 0)
	for rows.Next() {
		var item domain.LeaveType
		if err := rows.Scan(
			&item.ID, &item.Code, &item.NameZH, &item.NameEN,
			&item.RequiresBalance, &item.Enabled, &item.DisplayOrder,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpsertLeaveTypeEnabled updates leave_types.status for an existing tenant leave type.
func (s *Store) UpsertLeaveTypeEnabled(execCtx context.Context, tenantID, code string, enabled bool, _ string, updatedAt time.Time) error {
	status := "inactive"
	if enabled {
		status = "active"
	}
	result, err := s.db.Exec(tenantContext(execCtx, tenantID), `
UPDATE leave_types
SET status = $3, updated_at = $4
WHERE tenant_id = $1 AND lower(code) = lower($2)`, tenantID, code, status, updatedAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.NotFound("leave type", code)
	}
	return nil
}

// UpsertLeaveBalance 從儲存層處理 upsert 請假 balance。
func (s *Store) UpsertLeaveBalance(execCtx context.Context, v domain.LeaveBalance) error {
	v.LeaveType = strings.ToLower(strings.TrimSpace(v.LeaveType))
	v.LeaveTypeID = strings.TrimSpace(v.LeaveTypeID)
	if v.LeaveTypeID == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	tenantCtx := tenantContext(execCtx, v.TenantID)
	source := strings.TrimSpace(v.Source)
	if source == "" {
		source = "manual_snapshot"
	}
	if source == "local_anchor" {
		_, err := s.EnsureLocalLeaveBalanceAnchor(execCtx, v)
		return err
	}
	carryExpire, err := nullableDate(v.CarryExpire)
	if err != nil {
		return err
	}
	entitlementYear := pgtype.Int4{}
	if v.EntitlementYear > 0 {
		entitlementYear = pgtype.Int4{Int32: int32(v.EntitlementYear), Valid: true}
	}
	periodStart, err := nullableDate(v.PeriodStart)
	if err != nil {
		return err
	}
	periodEnd, err := nullableDate(v.PeriodEnd)
	if err != nil {
		return err
	}
	err = s.q.UpsertLeaveBalance(tenantCtx, sqlc.UpsertLeaveBalanceParams{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		EmployeeID:           v.EmployeeID,
		LeaveTypeID:          v.LeaveTypeID,
		RemainingMinutes:     int32(v.RemainingMinutes),
		PeriodStart:          periodStart,
		PeriodEnd:            periodEnd,
		GrantedMinutes:       int32(v.GrantedMinutes),
		UsedMinutes:          int32(v.UsedMinutes),
		Source:               source,
		ExternalLeaveCode:    v.ExternalLeaveCode,
		ExternalCategoryCode: v.ExternalCategoryCode,
		EntitlementYear:      entitlementYear,
		CarryInMinutes:       int32(v.CarryInMinutes),
		CarryExpire:          carryExpire,
		RawPayload:           mustJSON(v.RawPayload),
		LastSyncedAt:         nullableTimestamptz(v.LastSyncedAt),
		UpdatedAt:            timestamptz(v.UpdatedAt),
	})
	return err
}

// EnsureLocalLeaveBalanceAnchor creates the one timeless zero snapshot bucket
// used as the append-only target for locally earned credits.
func (s *Store) EnsureLocalLeaveBalanceAnchor(execCtx context.Context, v domain.LeaveBalance) (domain.LeaveBalance, error) {
	v.LeaveType = strings.ToLower(strings.TrimSpace(v.LeaveType))
	v.LeaveTypeID = strings.TrimSpace(v.LeaveTypeID)
	if v.LeaveTypeID == "" {
		v.LeaveTypeID = domain.StableLeaveTypeID(v.LeaveType)
	}
	row, err := s.q.EnsureLocalLeaveBalanceAnchor(tenantContext(execCtx, v.TenantID), sqlc.EnsureLocalLeaveBalanceAnchorParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		EmployeeID:  v.EmployeeID,
		LeaveTypeID: v.LeaveTypeID,
		UpdatedAt:   timestamptz(v.UpdatedAt),
	})
	if err != nil {
		return domain.LeaveBalance{}, err
	}
	return fromLeaveBalance(row, v.LeaveType), nil
}

func (s *Store) UpsertLeaveTypeExternalRef(execCtx context.Context, v domain.LeaveTypeExternalRef) error {
	effectiveFrom, err := nullableDate(v.EffectiveFrom)
	if err != nil {
		return err
	}
	effectiveTo, err := nullableDate(v.EffectiveTo)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertLeaveTypeExternalRef(tenantContext(execCtx, v.TenantID), sqlc.UpsertLeaveTypeExternalRefParams{
		ID: v.ID, TenantID: v.TenantID, SourceSystem: v.SourceSystem, ExternalCode: v.ExternalCode,
		ExternalCategoryCode: v.ExternalCategoryCode, LeaveTypeID: v.LeaveTypeID,
		EffectiveFrom: effectiveFrom, EffectiveTo: effectiveTo,
		CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetLeaveTypeExternalRef(execCtx context.Context, tenantID, sourceSystem, externalCode, externalCategoryCode string, asOf time.Time) (domain.LeaveTypeExternalRef, bool, error) {
	v, err := s.q.GetLeaveTypeExternalRef(tenantContext(execCtx, tenantID), sqlc.GetLeaveTypeExternalRefParams{
		TenantID: tenantID, SourceSystem: sourceSystem, ExternalCategoryCode: externalCategoryCode,
		ExternalCode: externalCode, AsOf: pgtype.Date{Time: asOf, Valid: true},
	})
	if isNotFound(err) {
		return domain.LeaveTypeExternalRef{}, false, nil
	}
	if err != nil {
		return domain.LeaveTypeExternalRef{}, false, err
	}
	return fromLeaveTypeExternalRef(v), true, nil
}

// GetLeaveBalance 從儲存層取得請假 balance。
func (s *Store) GetLeaveBalance(execCtx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error) {
	v, err := s.q.GetLeaveBalance(tenantContext(execCtx, tenantID), sqlc.GetLeaveBalanceParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.LeaveBalance{}, false, nil
	}
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	return fromLeaveBalance(v.LeaveBalance, v.LeaveType), true, nil
}

func (s *Store) ListLeaveBalancesForOverlay(execCtx context.Context, tenantID, employeeID, leaveTypeID string, asOf time.Time) ([]domain.LeaveBalance, error) {
	items, err := s.q.ListLeaveBalancesForOverlay(tenantContext(execCtx, tenantID), sqlc.ListLeaveBalancesForOverlayParams{
		TenantID: tenantID, EmployeeID: employeeID, LeaveTypeID: leaveTypeID,
		AsOf: pgtype.Date{Time: asOf, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.LeaveBalance, 0, len(items))
	for _, item := range items {
		out = append(out, fromLeaveBalance(item.LeaveBalance, item.LeaveType))
	}
	return out, nil
}

func (s *Store) GetLeaveBalanceForOverlay(execCtx context.Context, tenantID, employeeID, leaveTypeID string, asOf time.Time) (domain.LeaveBalance, bool, error) {
	v, err := s.q.GetLeaveBalanceForOverlay(tenantContext(execCtx, tenantID), sqlc.GetLeaveBalanceForOverlayParams{
		TenantID: tenantID, EmployeeID: employeeID, LeaveTypeID: leaveTypeID, AsOf: pgtype.Date{Time: asOf, Valid: true},
	})
	if isNotFound(err) {
		return domain.LeaveBalance{}, false, nil
	}
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	return fromLeaveBalance(v.LeaveBalance, v.LeaveType), true, nil
}

// ListLeaveBalances 從儲存層列出請假 balances。
func (s *Store) ListLeaveBalances(execCtx context.Context, tenantID string) ([]domain.LeaveBalance, error) {
	items, err := s.q.ListLeaveBalances(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.LeaveBalance, 0, len(items))
	for _, item := range items {
		out = append(out, fromLeaveBalance(item.LeaveBalance, item.LeaveType))
	}
	return out, nil
}

func (s *Store) AppendLeaveBalanceEntry(execCtx context.Context, v domain.LeaveBalanceEntry) (bool, error) {
	_, err := s.q.AppendLeaveBalanceEntry(tenantContext(execCtx, v.TenantID), sqlc.AppendLeaveBalanceEntryParams{
		ID: v.ID, TenantID: v.TenantID, BalanceID: v.BalanceID,
		AllocationID: nullableInt8(v.AllocationID), LeaveRequestID: nullableText(v.LeaveRequestID),
		LeaveCaseID: nullableText(v.LeaveCaseID), OvertimeRequestID: nullableText(v.OvertimeRequestID),
		EntryType: v.EntryType, AmountMinutes: int32(v.AmountMinutes), IdempotencyKey: v.IdempotencyKey,
		Metadata: mustJSON(v.Metadata), OccurredAt: timestamptz(v.OccurredAt), CreatedAt: timestamptz(v.CreatedAt),
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) AppendStandaloneLeaveBalanceEntry(execCtx context.Context, v domain.LeaveBalanceEntry) (bool, error) {
	_, err := s.q.AppendStandaloneLeaveBalanceEntry(tenantContext(execCtx, v.TenantID), sqlc.AppendStandaloneLeaveBalanceEntryParams{
		ID: v.ID, TenantID: v.TenantID, BalanceID: v.BalanceID,
		LeaveCaseID: nullableText(v.LeaveCaseID), OvertimeRequestID: nullableText(v.OvertimeRequestID),
		EntryType: v.EntryType, AmountMinutes: int32(v.AmountMinutes), IdempotencyKey: v.IdempotencyKey,
		Metadata: mustJSON(v.Metadata), OccurredAt: timestamptz(v.OccurredAt), CreatedAt: timestamptz(v.CreatedAt),
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListLeaveBalanceEntries(execCtx context.Context, tenantID string) ([]domain.LeaveBalanceEntry, error) {
	items, err := s.q.ListLeaveBalanceEntries(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveBalanceEntry), nil
}

func (s *Store) ListLeaveBalanceEntriesByBalance(execCtx context.Context, tenantID, balanceID string) ([]domain.LeaveBalanceEntry, error) {
	items, err := s.q.ListLeaveBalanceEntriesByBalance(tenantContext(execCtx, tenantID), sqlc.ListLeaveBalanceEntriesByBalanceParams{
		TenantID: tenantID, BalanceID: balanceID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveBalanceEntry), nil
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
	if strings.TrimSpace(v.ReconciliationStatus) == "" {
		v.ReconciliationStatus = "not_required"
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	_, err := s.q.UpsertLeaveRequest(tenantContext(execCtx, v.TenantID), sqlc.UpsertLeaveRequestParams{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		EmployeeID:           v.EmployeeID,
		LeaveType:            v.LeaveType,
		LeaveTypeID:          v.LeaveTypeID,
		PolicyVersion:        int32(v.PolicyVersion),
		RuleSnapshot:         mustJSON(ruleSnapshot),
		EvaluationSnapshot:   mustJSON(evaluationSnapshot),
		StartAt:              timestamptz(v.StartAt),
		EndAt:                timestamptz(v.EndAt),
		RequestedMinutes:     int32(v.RequestedMinutes),
		Reason:               v.Reason,
		Status:               v.Status,
		FormInstanceID:       v.FormInstanceID,
		ReconciliationStatus: v.ReconciliationStatus,
		CreatedAt:            timestamptz(v.CreatedAt),
		UpdatedAt:            timestamptz(v.UpdatedAt),
	})
	return err
}

// UpsertLeaveRequestAllocation persists the exact reserved balance bucket.
func (s *Store) UpsertLeaveRequestAllocation(execCtx context.Context, v domain.LeaveRequestAllocation) error {
	tenantCtx := tenantContext(execCtx, v.TenantID)
	_, err := s.q.UpsertLeaveRequestAllocation(tenantCtx, sqlc.UpsertLeaveRequestAllocationParams{
		TenantID:        v.TenantID,
		LeaveRequestID:  v.LeaveRequestID,
		LeaveBalanceID:  v.LeaveBalanceID,
		Cycle:           int32(v.Cycle),
		ReservedMinutes: int32(v.ReservedMinutes),
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	if !isNotFound(err) {
		return err
	}
	existing, getErr := s.q.GetLeaveRequestAllocationByCycleBalance(tenantCtx, sqlc.GetLeaveRequestAllocationByCycleBalanceParams{
		TenantID: v.TenantID, LeaveRequestID: v.LeaveRequestID,
		LeaveBalanceID: v.LeaveBalanceID, Cycle: int32(v.Cycle),
	})
	if getErr != nil {
		return getErr
	}
	if existing.ReservedMinutes != int32(v.ReservedMinutes) {
		return domain.Conflict("leave allocation is immutable within a request cycle")
	}
	return nil
}

func (s *Store) ListLeaveRequestAllocationsByRequest(execCtx context.Context, tenantID, leaveRequestID string) ([]domain.LeaveRequestAllocation, error) {
	items, err := s.q.ListLeaveRequestAllocationsByRequest(tenantContext(execCtx, tenantID), sqlc.ListLeaveRequestAllocationsByRequestParams{
		TenantID: tenantID, LeaveRequestID: leaveRequestID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveRequestAllocation), nil
}

func (s *Store) ListLeaveRequestAllocationsByRequestCycle(execCtx context.Context, tenantID, leaveRequestID string, cycle int) ([]domain.LeaveRequestAllocation, error) {
	items, err := s.q.ListLeaveRequestAllocationsByRequestCycle(tenantContext(execCtx, tenantID), sqlc.ListLeaveRequestAllocationsByRequestCycleParams{
		TenantID: tenantID, LeaveRequestID: leaveRequestID, Cycle: int32(cycle),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveRequestAllocation), nil
}

func (s *Store) UpsertLeaveCase(execCtx context.Context, v domain.LeaveCase) error {
	_, err := s.q.UpsertLeaveCase(tenantContext(execCtx, v.TenantID), sqlc.UpsertLeaveCaseParams{
		ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, LeaveTypeID: v.LeaveTypeID,
		StartAt: timestamptz(v.StartAt), EndAt: timestamptz(v.EndAt), NetMinutes: int32(v.NetMinutes),
		Status: v.Status, Origin: v.Origin, CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetLeaveCaseByLeaveRequest(execCtx context.Context, tenantID, leaveRequestID string) (domain.LeaveCase, bool, error) {
	v, err := s.q.GetLeaveCaseByLeaveRequestID(tenantContext(execCtx, tenantID), sqlc.GetLeaveCaseByLeaveRequestIDParams{
		TenantID: tenantID, LeaveRequestID: nullableText(leaveRequestID),
	})
	if isNotFound(err) {
		return domain.LeaveCase{}, false, nil
	}
	if err != nil {
		return domain.LeaveCase{}, false, err
	}
	return fromLeaveCase(v.LeaveCase), true, nil
}

func (s *Store) GetLeaveCaseByExternalRecord(execCtx context.Context, tenantID, externalLeaveRecordID string) (domain.LeaveCase, bool, error) {
	v, err := s.q.GetLeaveCaseByExternalLeaveRecordID(tenantContext(execCtx, tenantID), sqlc.GetLeaveCaseByExternalLeaveRecordIDParams{
		TenantID: tenantID, ExternalLeaveRecordID: nullableText(externalLeaveRecordID),
	})
	if isNotFound(err) {
		return domain.LeaveCase{}, false, nil
	}
	if err != nil {
		return domain.LeaveCase{}, false, err
	}
	return fromLeaveCase(v.LeaveCase), true, nil
}

func (s *Store) ListConfirmedActiveLeaveCasesByQuery(execCtx context.Context, tenantID string, employeeIDs []string, fromAt, toAt time.Time) ([]domain.LeaveCase, error) {
	items, err := s.q.ListConfirmedActiveLeaveCasesByQuery(tenantContext(execCtx, tenantID), sqlc.ListConfirmedActiveLeaveCasesByQueryParams{
		TenantID: tenantID, EmployeeIds: textArray(employeeIDs), FromAt: timestamptz(fromAt), ToAt: timestamptz(toAt),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveCase), nil
}

func (s *Store) UpsertLeaveCaseSource(execCtx context.Context, v domain.LeaveCaseSource) error {
	switch {
	case strings.TrimSpace(v.LeaveRequestID) != "" && strings.TrimSpace(v.ExternalLeaveRecordID) == "":
		_, err := s.q.UpsertLeaveRequestCaseSource(tenantContext(execCtx, v.TenantID), sqlc.UpsertLeaveRequestCaseSourceParams{
			TenantID: v.TenantID, LeaveCaseID: v.LeaveCaseID, LeaveRequestID: nullableText(v.LeaveRequestID),
			MatchMethod: v.MatchMethod, MatchStatus: v.MatchStatus, CreatedAt: timestamptz(v.CreatedAt),
		})
		return err
	case strings.TrimSpace(v.ExternalLeaveRecordID) != "" && strings.TrimSpace(v.LeaveRequestID) == "":
		_, err := s.q.UpsertExternalLeaveCaseSource(tenantContext(execCtx, v.TenantID), sqlc.UpsertExternalLeaveCaseSourceParams{
			TenantID: v.TenantID, LeaveCaseID: v.LeaveCaseID, ExternalLeaveRecordID: nullableText(v.ExternalLeaveRecordID),
			MatchMethod: v.MatchMethod, MatchStatus: v.MatchStatus, CreatedAt: timestamptz(v.CreatedAt),
		})
		return err
	default:
		return fmt.Errorf("leave case source must reference exactly one typed source")
	}
}

func (s *Store) DeleteLeaveCaseIfUnreferenced(execCtx context.Context, tenantID, id string) error {
	_, err := s.q.DeleteLeaveCaseIfUnreferenced(tenantContext(execCtx, tenantID), sqlc.DeleteLeaveCaseIfUnreferencedParams{
		TenantID: tenantID, ID: id,
	})
	return err
}

func (s *Store) UpsertExternalLeaveRecord(execCtx context.Context, v domain.ExternalLeaveRecord) error {
	_, err := s.q.UpsertExternalLeaveRecord(tenantContext(execCtx, v.TenantID), sqlc.UpsertExternalLeaveRecordParams{
		ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, SourceSystem: v.SourceSystem, ExternalRef: v.ExternalRef,
		ExternalLeaveCode: v.ExternalLeaveCode, ExternalCategoryCode: v.ExternalCategoryCode, LeaveTypeID: v.LeaveTypeID,
		LeaveName: v.LeaveName, StartAt: timestamptz(v.StartAt), EndAt: timestamptz(v.EndAt),
		GrossMinutes: int32(v.GrossMinutes), DeductMinutes: int32(v.DeductMinutes), NetMinutes: int32(v.NetMinutes),
		Remark: v.Remark, SourceLabel: v.SourceLabel, Status: v.Status, RawPayload: mustJSON(v.RawPayload), PayloadHash: v.PayloadHash,
		FirstSeenAt: timestamptz(v.FirstSeenAt), LastSeenAt: timestamptz(v.LastSeenAt), DeletedAt: nullableTimestamptz(v.DeletedAt),
	})
	return err
}

func (s *Store) GetExternalLeaveRecordByRef(execCtx context.Context, tenantID, sourceSystem, externalRef string) (domain.ExternalLeaveRecord, bool, error) {
	v, err := s.q.GetExternalLeaveRecordByRef(tenantContext(execCtx, tenantID), sqlc.GetExternalLeaveRecordByRefParams{TenantID: tenantID, SourceSystem: sourceSystem, ExternalRef: externalRef})
	if isNotFound(err) {
		return domain.ExternalLeaveRecord{}, false, nil
	}
	if err != nil {
		return domain.ExternalLeaveRecord{}, false, err
	}
	return fromExternalLeaveRecord(v), true, nil
}

func (s *Store) ListExternalLeaveRecords(execCtx context.Context, tenantID string) ([]domain.ExternalLeaveRecord, error) {
	items, err := s.q.ListExternalLeaveRecords(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromExternalLeaveRecord), nil
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

// UpsertAttendanceClockRecord 從儲存層處理 upsert 考勤打卡 record。
func (s *Store) UpsertAttendanceClockRecord(execCtx context.Context, v domain.AttendanceClockRecord) error {
	workDate, err := requiredDate(v.WorkDate)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertAttendanceClockRecord(tenantContext(execCtx, v.TenantID), sqlc.UpsertAttendanceClockRecordParams{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		WorksiteID:          nullableText(v.WorksiteID),
		WorkDate:            workDate,
		Direction:           v.Direction,
		ClientEventID:       v.ClientEventID,
		ClockedAt:           timestamptz(v.ClockedAt),
		Latitude:            pgtype.Float8{Float64: v.Latitude, Valid: true},
		Longitude:           pgtype.Float8{Float64: v.Longitude, Valid: true},
		AccuracyMeters:      pgtype.Float8{Float64: v.AccuracyMeters, Valid: true},
		DistanceMeters:      pgtype.Float8{Float64: v.DistanceMeters, Valid: true},
		RecordStatus:        v.RecordStatus,
		RejectionReason:     v.RejectionReason,
		Source:              v.Source,
		DeviceID:            v.DeviceID,
		DeviceInfo:          mustJSON(v.DeviceInfo),
		CorrectionRequestID: nullableText(v.CorrectionRequestID),
		Voided:              v.Voided,
		VoidedAt:            nullableTimestamptz(v.VoidedAt),
		VoidedByAccountID:   nullableText(v.VoidedByAccountID),
		VoidReason:          nullableText(v.VoidReason),
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
	v, err := s.q.GetAttendanceClockRecordByClientEventID(tenantContext(execCtx, tenantID), sqlc.GetAttendanceClockRecordByClientEventIDParams{TenantID: tenantID, ClientEventID: clientEventID})
	return attendanceClockRecordResult(v, err)
}

// GetEarliestAcceptedAttendanceClockIn 取得未作廢的最早 accepted 上班卡。
func (s *Store) GetEarliestAcceptedAttendanceClockIn(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error) {
	date, err := requiredDate(workDate)
	if err != nil {
		return domain.AttendanceClockRecord{}, false, err
	}
	v, err := s.q.GetEarliestAcceptedAttendanceClockIn(tenantContext(execCtx, tenantID), sqlc.GetEarliestAcceptedAttendanceClockInParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: date})
	return attendanceClockRecordResult(v, err)
}

// GetLatestAcceptedAttendanceClockOut 取得未作廢的最晚 accepted 下班卡。
func (s *Store) GetLatestAcceptedAttendanceClockOut(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error) {
	date, err := requiredDate(workDate)
	if err != nil {
		return domain.AttendanceClockRecord{}, false, err
	}
	v, err := s.q.GetLatestAcceptedAttendanceClockOut(tenantContext(execCtx, tenantID), sqlc.GetLatestAcceptedAttendanceClockOutParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: date})
	return attendanceClockRecordResult(v, err)
}

// GetLatestAcceptedAttendanceClockRecord 取得未作廢的當日最新 accepted 打卡。
func (s *Store) GetLatestAcceptedAttendanceClockRecord(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error) {
	date, err := requiredDate(workDate)
	if err != nil {
		return domain.AttendanceClockRecord{}, false, err
	}
	v, err := s.q.GetLatestAcceptedAttendanceClockRecord(tenantContext(execCtx, tenantID), sqlc.GetLatestAcceptedAttendanceClockRecordParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: date})
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
		EmployeeIds:  query.EmployeeIDs,
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
	workDate, err := requiredDate(v.WorkDate)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertAttendanceDailySummary(tenantContext(execCtx, v.TenantID), sqlc.UpsertAttendanceDailySummaryParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		EmployeeID:      v.EmployeeID,
		Column4:         workDate,
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
	v, err := s.q.GetAttendanceDailySummaryByExternalRef(tenantContext(execCtx, tenantID), sqlc.GetAttendanceDailySummaryByExternalRefParams{TenantID: tenantID, ExternalRef: externalRef})
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
	date, err := requiredDate(workDate)
	if err != nil {
		return domain.AttendanceDailySummary{}, false, err
	}
	v, err := s.q.GetAttendanceDailySummaryByEmployeeDate(tenantContext(execCtx, tenantID), sqlc.GetAttendanceDailySummaryByEmployeeDateParams{TenantID: tenantID, EmployeeID: employeeID, WorkDate: date})
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
		TenantID:    tenantID,
		EmployeeID:  query.EmployeeID,
		EmployeeIds: query.EmployeeIDs,
		FromDate:    query.FromDate,
		ToDate:      query.ToDate,
		Source:      query.Source,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceDailySummary), nil
}

func (s *Store) UpsertAttendanceDayProjection(execCtx context.Context, v domain.AttendanceDayProjection) error {
	workDate, err := requiredDate(v.WorkDate)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertAttendanceDayProjection(tenantContext(execCtx, v.TenantID), sqlc.UpsertAttendanceDayProjectionParams{
		TenantID: v.TenantID, EmployeeID: v.EmployeeID, WorkDate: workDate,
		PolicyVersion:    int32(v.PolicyVersion),
		ScheduledStartAt: nullableTimestamptz(v.ScheduledStartAt), ScheduledEndAt: nullableTimestamptz(v.ScheduledEndAt),
		ClockInRecordID: nullableText(v.ClockInRecordID), ClockOutRecordID: nullableText(v.ClockOutRecordID),
		LastPunchRecordID: nullableText(v.LastPunchRecordID), PunchCount: int32(v.PunchCount),
		WorkedMinutes: int32(v.WorkedMinutes), ApprovedLeaveMinutes: int32(v.ApprovedLeaveMinutes),
		PendingLeaveMinutes: int32(v.PendingLeaveMinutes), RequiredMinutes: int32(v.RequiredMinutes),
		OvertimeMinutes: int32(v.OvertimeMinutes), DayStatus: v.DayStatus,
		AnomalyReasons: textArray(v.AnomalyReasons), InputFingerprint: v.InputFingerprint,
		Payload: mustJSON(v.Payload), ComputedAt: timestamptz(v.ComputedAt), UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetAttendanceDayProjection(execCtx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceDayProjection, bool, error) {
	date, err := requiredDate(workDate)
	if err != nil {
		return domain.AttendanceDayProjection{}, false, err
	}
	v, err := s.q.GetAttendanceDayProjection(tenantContext(execCtx, tenantID), sqlc.GetAttendanceDayProjectionParams{
		TenantID: tenantID, EmployeeID: employeeID, WorkDate: date,
	})
	if isNotFound(err) {
		return domain.AttendanceDayProjection{}, false, nil
	}
	if err != nil {
		return domain.AttendanceDayProjection{}, false, err
	}
	return fromAttendanceDayProjection(v), true, nil
}

func (s *Store) ListAttendanceDayProjections(execCtx context.Context, tenantID string, employeeIDs []string, fromDate, toDate string) ([]domain.AttendanceDayProjection, error) {
	items, err := s.q.ListAttendanceDayProjections(tenantContext(execCtx, tenantID), sqlc.ListAttendanceDayProjectionsParams{
		TenantID: tenantID, EmployeeIds: textArray(employeeIDs),
		FromDate: strings.TrimSpace(fromDate), ToDate: strings.TrimSpace(toDate),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceDayProjection), nil
}

// UpsertAttendanceCorrectionRequest 從儲存層處理 upsert 考勤 correction 請求。
func (s *Store) UpsertAttendanceCorrectionRequest(execCtx context.Context, v domain.AttendanceCorrectionRequest) error {
	correctionType := strings.TrimSpace(v.CorrectionType)
	if correctionType == "" {
		correctionType = "add_record"
	}
	workDate, err := requiredDate(v.WorkDate)
	if err != nil {
		return err
	}
	_, err = s.q.UpsertAttendanceCorrectionRequest(tenantContext(execCtx, v.TenantID), sqlc.UpsertAttendanceCorrectionRequestParams{
		ID:                       v.ID,
		TenantID:                 v.TenantID,
		EmployeeID:               v.EmployeeID,
		Direction:                v.Direction,
		RequestedClockedAt:       timestamptz(v.RequestedClockedAt),
		WorkDate:                 workDate,
		CorrectionType:           correctionType,
		TargetClockRecordID:      nullableText(v.TargetClockRecordID),
		ReplacementClockRecordID: nullableText(v.ReplacementClockRecordID),
		Reason:                   v.Reason,
		Status:                   v.Status,
		FormInstanceID:           v.FormInstanceID,
		ClockRecordID:            nullableText(v.ClockRecordID),
		ReviewedByAccountID:      nullableText(v.ReviewedByAccountID),
		ReviewReason:             v.ReviewReason,
		ReviewedAt:               nullableTimestamptz(v.ReviewedAt),
		CreatedAt:                timestamptz(v.CreatedAt),
		UpdatedAt:                timestamptz(v.UpdatedAt),
	})
	return err
}

// GetAttendanceCorrectionRequest 從儲存層取得考勤 correction 請求。
func (s *Store) GetAttendanceCorrectionRequest(execCtx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error) {
	v, err := s.q.GetAttendanceCorrectionRequest(tenantContext(execCtx, tenantID), sqlc.GetAttendanceCorrectionRequestParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceCorrectionRequest{}, false, nil
	}
	if err != nil {
		return domain.AttendanceCorrectionRequest{}, false, err
	}
	return fromAttendanceCorrectionRequest(v), true, nil
}

func (s *Store) GetAttendanceCorrectionRequestForUpdate(execCtx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error) {
	v, err := s.q.GetAttendanceCorrectionRequestForUpdate(tenantContext(execCtx, tenantID), sqlc.GetAttendanceCorrectionRequestForUpdateParams{
		TenantID: tenantID, ID: id,
	})
	if isNotFound(err) {
		return domain.AttendanceCorrectionRequest{}, false, nil
	}
	if err != nil {
		return domain.AttendanceCorrectionRequest{}, false, err
	}
	return fromAttendanceCorrectionRequest(v), true, nil
}

func (s *Store) ClaimAttendanceCorrectionReview(execCtx context.Context, tenantID, formInstanceID, reviewerID string, claimedAt time.Time) (domain.AttendanceCorrectionRequest, bool, error) {
	v, err := s.q.ClaimAttendanceCorrectionReview(tenantContext(execCtx, tenantID), sqlc.ClaimAttendanceCorrectionReviewParams{
		TenantID: tenantID, FormInstanceID: formInstanceID,
		ReviewerID: nullableText(reviewerID), ClaimedAt: timestamptz(claimedAt),
	})
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

// GetWorkflowStageInstanceForUpdate locks one stage row for a transactional transition.
func (s *Store) GetWorkflowStageInstanceForUpdate(execCtx context.Context, tenantID, id string) (domain.WorkflowStageInstance, bool, error) {
	v, err := s.q.GetWorkflowStageInstanceForUpdate(tenantContext(execCtx, tenantID), sqlc.GetWorkflowStageInstanceForUpdateParams{
		TenantID: tenantID, ID: id,
	})
	if isNotFound(err) {
		return domain.WorkflowStageInstance{}, false, nil
	}
	if err != nil {
		return domain.WorkflowStageInstance{}, false, err
	}
	return fromWorkflowStageInstance(v), true, nil
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
		Search:             params.Search,
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
		Search:             countParams.Search,
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
func (s *Store) ListPlatformTaskItems(execCtx context.Context, tenantID, accountID string, query domain.PlatformTasksQuery) ([]domain.PlatformTaskRecordItem, error) {
	items, err := s.q.ListPlatformTaskItems(tenantContext(execCtx, tenantID), sqlc.ListPlatformTaskItemsParams{
		TenantID:        tenantID,
		AccountID:       accountID,
		FromCreatedAt:   timestamptz(query.From),
		ToCreatedAt:     timestamptz(query.To),
		HasCursor:       query.HasCursor,
		CursorCreatedAt: timestamptz(query.CursorCreatedAt),
		CursorID:        strings.TrimSpace(query.CursorID),
		LimitCount:      int32(query.PageSize),
	})
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
func (s *Store) ListPlatformTaskTodos(execCtx context.Context, tenantID, accountID string, query domain.PlatformTasksQuery) ([]domain.PlatformTaskTodoRecord, error) {
	items, err := s.q.ListPlatformTaskTodos(tenantContext(execCtx, tenantID), sqlc.ListPlatformTaskTodosParams{
		TenantID:        tenantID,
		AccountID:       accountID,
		FromCreatedAt:   timestamptz(query.From),
		ToCreatedAt:     timestamptz(query.To),
		HasCursor:       query.HasCursor,
		CursorCreatedAt: timestamptz(query.CursorCreatedAt),
		CursorID:        strings.TrimSpace(query.CursorID),
		LimitCount:      int32(query.PageSize),
	})
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
		AgentID:        v.AgentID,
		SessionID:      v.SessionID,
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
	return mapSlice(items, fromListAgentRunsRow), nil
}

// ListAgentRunsByAccount 從儲存層列出 agent 執行紀錄 by 帳號。
func (s *Store) ListAgentRunsByAccount(execCtx context.Context, tenantID, accountID string) ([]domain.AgentRun, error) {
	items, err := s.q.ListAgentRunsByAccount(tenantContext(execCtx, tenantID), sqlc.ListAgentRunsByAccountParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromListAgentRunsByAccountRow), nil
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
	return mapSlice(items, fromListAgentRunsPageRow), int(total), nil
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
	return mapSlice(items, fromListAgentRunsPageByAccountRow), int(total), nil
}

// UpsertAgentModel 從儲存層處理 upsert agent 模型。
func (s *Store) UpsertAgentModel(execCtx context.Context, v domain.AgentModel) error {
	_, err := s.q.UpsertAgentModel(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentModelParams{
		CredentialSecretID: v.CredentialSecretID,
		ID:                 v.ID,
		TenantID:           v.TenantID,
		Name:               v.Name,
		Provider:           v.Provider,
		ModelName:          v.ModelName,
		LitellmModel:       v.LiteLLMModel,
		ApiBaseUrl:         v.APIBaseURL,
		ApiKeyCiphertext:   v.APIKeyCiphertext,
		ApiKeyPreview:      v.APIKeyPreview,
		RateLimitRpm:       int32(v.RateLimitRPM),
		Status:             string(v.Status),
		TimeoutSeconds:     int32(v.TimeoutSeconds),
		LastTestedAt:       nullableTimestamptz(v.LastTestedAt),
		LastTestStatus:     v.LastTestStatus,
		LastTestMessage:    v.LastTestMessage,
		SyncStatus:         string(v.SyncStatus),
		LastSyncedAt:       nullableTimestamptz(v.LastSyncedAt),
		LastSyncError:      v.LastSyncError,
		SyncedConfigHash:   v.SyncedConfigHash,
		CreatedAt:          timestamptz(v.CreatedAt),
		UpdatedAt:          timestamptz(v.UpdatedAt),
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
	return agentModelFromGetRow(v), true, nil
}

// ListAgentModels 從儲存層列出 agent 模型。
func (s *Store) ListAgentModels(execCtx context.Context, tenantID string) ([]domain.AgentModel, error) {
	items, err := s.q.ListAgentModels(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, agentModelFromListRow), nil
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
	return agentModelFromDeleteRow(v), true, nil
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
	return agentModelFromTestRow(v), true, nil
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
	return agentModelFromSyncRow(v), true, nil
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
		CredentialSecretID:   item.CredentialSecretID,
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
		TimeoutSeconds:       int32(item.TimeoutSeconds),
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
	out := make([]domain.AgentExternalTool, 0, len(items))
	for _, item := range items {
		connection := agentExternalToolFromListRow(item)
		connection.Capabilities, err = s.listAgentExternalToolCapabilitiesAll(execCtx, tenantID, connection.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, connection)
	}
	return out, nil
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
	return agentExternalToolFromDeleteRow(item), true, nil
}

// UpsertAgentDefinition 從儲存層處理 upsert agent 定義。
func (s *Store) UpsertAgentDefinition(execCtx context.Context, v domain.AgentDefinition) error {
	_, err := s.q.UpsertAgentDefinition(tenantContext(execCtx, v.TenantID), sqlc.UpsertAgentDefinitionParams{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		DraftRevisionID:               v.DraftRevisionID,
		PublishedRevisionID:           v.PublishedRevisionID,
		Name:                          v.Name,
		Description:                   v.Description,
		Emoji:                         v.Emoji,
		Category:                      string(v.Category),
		ModelID:                       v.ModelID,
		MainAgentRole:                 v.MainAgentRole,
		SubAgents:                     string(mustJSON(v.SubAgents)),
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            mustJSON(v.SuggestedQuestions),
		SuggestedQuestionTranslations: mustJSON(v.SuggestedQuestionTranslations),
		Tools:                         string(mustJSON(v.Tools)),
		ExternalToolIds:               string(mustJSON(v.ExternalToolIDs)),
		KnowledgeBaseIds:              string(mustJSON(v.KnowledgeBaseIDs)),
		Visibility:                    string(v.Visibility),
		VisibilityTargets:             mustJSON(v.VisibilityTargets),
		TimeoutSeconds:                int32(v.TimeoutSeconds),
		Version:                       int32(v.Version),
		CreatedByAccountID:            nullableText(v.CreatedByAccountID),
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
	return agentDefinitionFromGetRow(v), true, nil
}

// ListAgentDefinitions 從儲存層列出 agent 定義。
func (s *Store) ListAgentDefinitions(execCtx context.Context, tenantID string) ([]domain.AgentDefinition, error) {
	items, err := s.q.ListAgentDefinitions(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentDefinition, 0, len(items))
	for _, item := range items {
		definition, ok, getErr := s.GetAgentDefinition(execCtx, tenantID, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		if ok {
			out = append(out, definition)
		}
	}
	return out, nil
}

// ListPublishedAgentDefinitions 從儲存層列出已發布 agent 定義。
func (s *Store) ListPublishedAgentDefinitions(execCtx context.Context, tenantID string) ([]domain.AgentDefinition, error) {
	items, err := s.q.ListPublishedAgentDefinitions(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentDefinition, 0, len(items))
	for _, item := range items {
		definition, ok, getErr := s.GetAgentDefinition(execCtx, tenantID, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		if ok && definition.Status == domain.AgentDefinitionStatusPublished {
			out = append(out, definition)
		}
	}
	return out, nil
}

// DeleteAgentDefinition 從儲存層刪除 agent 定義。
func (s *Store) DeleteAgentDefinition(execCtx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error) {
	current, ok, err := s.GetAgentDefinition(execCtx, tenantID, id)
	if err != nil || !ok {
		return domain.AgentDefinition{}, ok, err
	}
	_, err = s.q.DeleteAgentDefinition(tenantContext(execCtx, tenantID), sqlc.DeleteAgentDefinitionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentDefinition{}, false, nil
	}
	if err != nil {
		return domain.AgentDefinition{}, false, err
	}
	return current, true, nil
}

// UpdateAgentDefinitionUsage 從儲存層更新 agent usage。
func (s *Store) UpdateAgentDefinitionUsage(execCtx context.Context, tenantID, id string, success bool, latencyMs int, prompt string, runAt time.Time) (domain.AgentDefinition, bool, error) {
	return s.GetAgentDefinition(execCtx, tenantID, id)
}

// InsertAgentDefinitionVersion 從儲存層新增 agent 版本。
func (s *Store) InsertAgentDefinitionVersion(execCtx context.Context, v domain.AgentDefinitionVersion) error {
	_, err := s.q.InsertAgentDefinitionVersion(tenantContext(execCtx, v.TenantID), sqlc.InsertAgentDefinitionVersionParams{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		AgentID:                       v.AgentID,
		Version:                       int32(v.Version),
		Name:                          v.Name,
		Description:                   v.Description,
		Emoji:                         v.Emoji,
		Category:                      string(v.Category),
		Visibility:                    string(v.Visibility),
		VisibilityTargets:             mustJSON(v.VisibilityTargets),
		MainAgentRole:                 v.MainAgentRole,
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            mustJSON(v.SuggestedQuestions),
		SuggestedQuestionTranslations: mustJSON(v.SuggestedQuestionTranslations),
		ModelID:                       v.ModelID,
		ModelConfigChecksum:           v.ModelConfigChecksum,
		TimeoutSeconds:                int32(v.TimeoutSeconds),
		ConfigSchemaVersion:           int32(v.ConfigSchemaVersion),
		Checksum:                      v.Checksum,
		Note:                          v.Note,
		CreatedByAccountID:            nullableText(v.CreatedByAccountID),
		CreatedAt:                     timestamptz(v.CreatedAt),
		SubAgents:                     mustJSON(v.SubAgents),
		Tools:                         mustJSON(v.Tools),
		ExternalToolIds:               mustJSON(v.ExternalToolIDs),
		KnowledgeBaseIds:              mustJSON(v.KnowledgeBaseIDs),
	})
	return err
}

// ListAgentDefinitionVersions 從儲存層列出 agent 版本。
func (s *Store) ListAgentDefinitionVersions(execCtx context.Context, tenantID, agentID string) ([]domain.AgentDefinitionVersion, error) {
	items, err := s.q.ListAgentDefinitionVersions(tenantContext(execCtx, tenantID), sqlc.ListAgentDefinitionVersionsParams{TenantID: tenantID, AgentID: agentID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, agentDefinitionVersionFromListRow), nil
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
	return agentDefinitionVersionFromGetRow(v), true, nil
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
	if v.Status == "" {
		v.Status = domain.OutboxStatusPending
	}
	if v.AttemptCount == 0 && v.RetryCount > 0 {
		v.AttemptCount = v.RetryCount
	}
	_, err := s.q.AppendOutboxEvent(execCtx, sqlc.AppendOutboxEventParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EventType:      v.EventType,
		AggregateType:  v.AggregateType,
		AggregateID:    v.AggregateID,
		Payload:        mustJSON(v.Payload),
		PayloadVersion: int32(v.PayloadVersion),
		IdempotencyKey: v.IdempotencyKey,
		Status:         v.Status,
		RetryCount:     int32(v.RetryCount),
		AttemptCount:   int32(v.AttemptCount),
		MaxAttempts:    nullableInt4(v.MaxAttempts),
		LastError:      v.LastError,
		NextAttemptAt:  timestamptz(v.NextAttemptAt),
		ClaimOwner:     v.ClaimOwner,
		ClaimToken:     v.ClaimToken,
		ClaimExpiresAt: nullableTimestamptz(v.ClaimExpiresAt),
		LastAttemptAt:  nullableTimestamptz(v.LastAttemptAt),
		CreatedAt:      timestamptz(v.CreatedAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
		ProcessedAt:    nullableTimestamptz(v.ProcessedAt),
		DeadLetteredAt: nullableTimestamptz(v.DeadLetteredAt),
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

// ClaimOutboxEvents atomically leases due or expired outbox events for a worker.
func (s *Store) ClaimOutboxEvents(execCtx context.Context, tenantID string, limit int, claimedAt, leaseUntil time.Time, claimOwner, claimToken string) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		return nil, nil
	}
	items, err := s.q.ClaimOutboxEvents(tenantContext(execCtx, tenantID), sqlc.ClaimOutboxEventsParams{
		TenantID:   tenantID,
		BatchLimit: int32(limit),
		ClaimedAt:  timestamptz(claimedAt),
		LeaseUntil: timestamptz(leaseUntil),
		ClaimOwner: claimOwner,
		ClaimToken: claimToken,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOutboxEvent), nil
}

// FinalizeOutboxEvent persists a result only if the processing token is still current.
func (s *Store) FinalizeOutboxEvent(execCtx context.Context, v domain.OutboxEvent) (bool, error) {
	_, err := s.q.FinalizeOutboxEvent(tenantContext(execCtx, v.TenantID), sqlc.FinalizeOutboxEventParams{
		TenantID:       v.TenantID,
		ID:             v.ID,
		ClaimToken:     v.ClaimToken,
		Status:         v.Status,
		RetryCount:     int32(v.RetryCount),
		AttemptCount:   int32(v.AttemptCount),
		LastError:      v.LastError,
		NextAttemptAt:  timestamptz(v.NextAttemptAt),
		UpdatedAt:      timestamptz(v.UpdatedAt),
		ProcessedAt:    nullableTimestamptz(v.ProcessedAt),
		DeadLetteredAt: nullableTimestamptz(v.DeadLetteredAt),
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// RetryOutboxEvent resets a failed terminal state for immediate operator retry.
func (s *Store) RetryOutboxEvent(execCtx context.Context, tenantID, id string, retriedAt time.Time) (domain.OutboxEvent, bool, error) {
	v, err := s.q.RetryOutboxEvent(tenantContext(execCtx, tenantID), sqlc.RetryOutboxEventParams{
		TenantID:  tenantID,
		ID:        id,
		RetriedAt: timestamptz(retriedAt),
	})
	if isNotFound(err) {
		return domain.OutboxEvent{}, false, nil
	}
	if err != nil {
		return domain.OutboxEvent{}, false, err
	}
	return fromOutboxEvent(v), true, nil
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

// nullableInt4 converts an optional integer while preserving an explicit zero.
func nullableInt4(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func nullableInt8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
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
		Search:             strings.TrimSpace(query.Search),
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

func requiredDate(value string) (pgtype.Date, error) {
	date, err := nullableDate(strings.TrimSpace(value))
	if err != nil {
		return pgtype.Date{}, err
	}
	if !date.Valid {
		return pgtype.Date{}, fmt.Errorf("date is required")
	}
	return date, nil
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

func jsonBytes(v any) []byte {
	switch value := v.(type) {
	case nil:
		return nil
	case []byte:
		return value
	case string:
		return []byte(value)
	case json.RawMessage:
		return []byte(value)
	default:
		return mustJSON(value)
	}
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
		ID:                v.ID,
		TenantID:          v.TenantID,
		Code:              v.Code,
		Name:              v.Name,
		NameEN:            v.NameEn,
		ParentID:          v.ParentID,
		Path:              utils.CopyStrings(v.Path),
		Source:            v.Source,
		Closed:            v.Closed,
		ShowInOrgChart:    v.ShowInOrgChart,
		ManagerPositionID: v.ManagerPositionID,
		CreatedAt:         timeFrom(v.CreatedAt),
		UpdatedAt:         timeFrom(v.UpdatedAt),
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

// fromOutboxEvent 轉換 outbox 事件。
func fromOutboxEvent(v sqlc.OutboxEvent) domain.OutboxEvent {
	maxAttempts := int(v.MaxAttempts)
	return domain.OutboxEvent{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EventType:      v.EventType,
		AggregateType:  v.AggregateType,
		AggregateID:    v.AggregateID,
		Payload:        jsonMap(v.Payload),
		PayloadVersion: int(v.PayloadVersion),
		IdempotencyKey: v.IdempotencyKey,
		Status:         v.Status,
		RetryCount:     int(v.RetryCount),
		AttemptCount:   int(v.AttemptCount),
		MaxAttempts:    &maxAttempts,
		LastError:      v.LastError,
		NextAttemptAt:  timeFrom(v.NextAttemptAt),
		ClaimOwner:     v.ClaimOwner,
		ClaimToken:     v.ClaimToken,
		ClaimExpiresAt: timePtrFrom(v.ClaimExpiresAt),
		LastAttemptAt:  timePtrFrom(v.LastAttemptAt),
		CreatedAt:      timeFrom(v.CreatedAt),
		UpdatedAt:      timeFrom(v.UpdatedAt),
		ProcessedAt:    timePtrFrom(v.ProcessedAt),
		DeadLetteredAt: timePtrFrom(v.DeadLetteredAt),
	}
}

// fromAttendancePolicy 轉換考勤政策。
func fromAttendancePolicy(v sqlc.AttendancePolicyVersion) domain.AttendancePolicy {
	return domain.AttendancePolicy{
		TenantID:             v.TenantID,
		WorkTime:             jsonAttendancePolicyWorkTime(v.WorkTime),
		Version:              int(v.Version),
		EffectiveFrom:        timePtrFrom(v.EffectiveFrom),
		PublishedByAccountID: v.PublishedByAccountID,
		PublishedAt:          timeFrom(v.PublishedAt),
	}
}

// fromLeaveBalance 轉換請假 balance。
func fromLeaveBalance(v sqlc.LeaveBalance, leaveTypes ...string) domain.LeaveBalance {
	leaveType := ""
	if len(leaveTypes) > 0 {
		leaveType = leaveTypes[0]
	}
	return domain.LeaveBalance{
		ID:                       v.ID,
		TenantID:                 v.TenantID,
		EmployeeID:               v.EmployeeID,
		LeaveType:                leaveType,
		LeaveTypeID:              v.LeaveTypeID,
		RemainingMinutes:         int(v.RemainingMinutes),
		PeriodStart:              dateTextFrom(v.PeriodStart),
		PeriodEnd:                dateTextFrom(v.PeriodEnd),
		GrantedMinutes:           int(v.GrantedMinutes),
		UsedMinutes:              int(v.UsedMinutes),
		Source:                   v.Source,
		ExternalLeaveCode:        v.ExternalLeaveCode,
		ExternalCategoryCode:     v.ExternalCategoryCode,
		EntitlementYear:          int(v.EntitlementYear.Int32),
		CarryInMinutes:           int(v.CarryInMinutes),
		CarryExpire:              dateTextFrom(v.CarryExpire),
		RawPayload:               jsonMap(v.RawPayload),
		LastSyncedAt:             timePtrFrom(v.LastSyncedAt),
		SnapshotRemainingMinutes: int(v.RemainingMinutes),
		UpdatedAt:                timeFrom(v.UpdatedAt),
	}
}

func fromLeaveTypeExternalRef(v sqlc.LeaveTypeExternalRef) domain.LeaveTypeExternalRef {
	return domain.LeaveTypeExternalRef{
		ID: v.ID, TenantID: v.TenantID, SourceSystem: v.SourceSystem, ExternalCode: v.ExternalCode,
		ExternalCategoryCode: v.ExternalCategoryCode, LeaveTypeID: v.LeaveTypeID,
		EffectiveFrom: dateTextFrom(v.EffectiveFrom), EffectiveTo: dateTextFrom(v.EffectiveTo),
		CreatedAt: timeFrom(v.CreatedAt), UpdatedAt: timeFrom(v.UpdatedAt),
	}
}

func fromLeaveBalanceEntry(v sqlc.LeaveBalanceEntry) domain.LeaveBalanceEntry {
	return domain.LeaveBalanceEntry{
		ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, LeaveTypeID: v.LeaveTypeID,
		BalanceID: v.BalanceID, LeaveRequestID: textFrom(v.LeaveRequestID), LeaveCaseID: textFrom(v.LeaveCaseID),
		AllocationID: v.AllocationID.Int64, OvertimeRequestID: textFrom(v.OvertimeRequestID),
		EntryType: v.EntryType, AmountMinutes: int(v.AmountMinutes), IdempotencyKey: v.IdempotencyKey,
		Metadata: jsonMap(v.Metadata), OccurredAt: timeFrom(v.OccurredAt), CreatedAt: timeFrom(v.CreatedAt),
	}
}

// fromLeaveRequest 轉換請假請求。
func fromLeaveRequest(v sqlc.LeaveRequest) domain.LeaveRequest {
	return domain.LeaveRequest{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		EmployeeID:           v.EmployeeID,
		LeaveType:            v.LeaveType,
		LeaveTypeID:          v.LeaveTypeID,
		PolicyVersion:        int(v.PolicyVersion),
		RuleSnapshot:         jsonMap(v.RuleSnapshot),
		EvaluationSnapshot:   jsonMap(v.EvaluationSnapshot),
		StartAt:              timeFrom(v.StartAt),
		EndAt:                timeFrom(v.EndAt),
		RequestedMinutes:     int(v.RequestedMinutes),
		Reason:               v.Reason,
		Status:               v.Status,
		FormInstanceID:       v.FormInstanceID,
		ReconciliationStatus: v.ReconciliationStatus,
		CreatedAt:            timeFrom(v.CreatedAt),
		UpdatedAt:            timeFrom(v.UpdatedAt),
	}
}

func fromLeaveRequestAllocation(v sqlc.LeaveRequestAllocation) domain.LeaveRequestAllocation {
	return domain.LeaveRequestAllocation{
		ID: v.ID, TenantID: v.TenantID, LeaveRequestID: v.LeaveRequestID,
		LeaveBalanceID: v.LeaveBalanceID, EmployeeID: v.EmployeeID, LeaveTypeID: v.LeaveTypeID,
		Cycle: int(v.Cycle), ReservedMinutes: int(v.ReservedMinutes), CreatedAt: timeFrom(v.CreatedAt),
	}
}

func fromLeaveCase(v sqlc.LeaveCase) domain.LeaveCase {
	return domain.LeaveCase{
		ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, LeaveTypeID: v.LeaveTypeID,
		StartAt: timeFrom(v.StartAt), EndAt: timeFrom(v.EndAt), NetMinutes: int(v.NetMinutes),
		Status: v.Status, Origin: v.Origin, CreatedAt: timeFrom(v.CreatedAt), UpdatedAt: timeFrom(v.UpdatedAt),
	}
}

func fromExternalLeaveRecord(v sqlc.ExternalLeaveRecord) domain.ExternalLeaveRecord {
	return domain.ExternalLeaveRecord{
		ID: v.ID, TenantID: v.TenantID, EmployeeID: v.EmployeeID, SourceSystem: v.SourceSystem, ExternalRef: v.ExternalRef,
		ExternalLeaveCode: v.ExternalLeaveCode, ExternalCategoryCode: v.ExternalCategoryCode, LeaveTypeID: v.LeaveTypeID,
		LeaveName: v.LeaveName, StartAt: timeFrom(v.StartAt), EndAt: timeFrom(v.EndAt),
		GrossMinutes: int(v.GrossMinutes), DeductMinutes: int(v.DeductMinutes), NetMinutes: int(v.NetMinutes),
		Remark: v.Remark, SourceLabel: v.SourceLabel, Status: v.Status, RawPayload: jsonMap(v.RawPayload), PayloadHash: v.PayloadHash,
		FirstSeenAt: timeFrom(v.FirstSeenAt), LastSeenAt: timeFrom(v.LastSeenAt), DeletedAt: timePtrFrom(v.DeletedAt),
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

// fromAttendanceClockRecord 轉換考勤打卡 record。
func fromAttendanceClockRecord(v sqlc.AttendanceClockRecord) domain.AttendanceClockRecord {
	return domain.AttendanceClockRecord{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		WorksiteID:          textFrom(v.WorksiteID),
		WorkDate:            dateTextFrom(v.WorkDate),
		Direction:           v.Direction,
		ClientEventID:       v.ClientEventID,
		ClockedAt:           timeFrom(v.ClockedAt),
		Latitude:            v.Latitude.Float64,
		Longitude:           v.Longitude.Float64,
		AccuracyMeters:      v.AccuracyMeters.Float64,
		DistanceMeters:      v.DistanceMeters.Float64,
		RecordStatus:        v.RecordStatus,
		RejectionReason:     v.RejectionReason,
		Source:              v.Source,
		DeviceID:            v.DeviceID,
		DeviceInfo:          jsonMap(v.DeviceInfo),
		CorrectionRequestID: textFrom(v.CorrectionRequestID),
		Voided:              v.Voided,
		VoidedAt:            timePtrFrom(v.VoidedAt),
		VoidedByAccountID:   textFrom(v.VoidedByAccountID),
		VoidReason:          textFrom(v.VoidReason),
		CreatedAt:           timeFrom(v.CreatedAt),
	}
}

// fromAttendanceDailySummary 轉換考勤日彙總。
func fromAttendanceDailySummary(v sqlc.AttendanceDailySummary) domain.AttendanceDailySummary {
	return domain.AttendanceDailySummary{
		ID:              v.ID,
		TenantID:        v.TenantID,
		EmployeeID:      v.EmployeeID,
		WorkDate:        dateTextFrom(v.WorkDate),
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

func fromAttendanceDayProjection(v sqlc.AttendanceDayProjection) domain.AttendanceDayProjection {
	return domain.AttendanceDayProjection{
		TenantID: v.TenantID, EmployeeID: v.EmployeeID, WorkDate: dateTextFrom(v.WorkDate),
		PolicyVersion:    int(v.PolicyVersion),
		ScheduledStartAt: timePtrFrom(v.ScheduledStartAt), ScheduledEndAt: timePtrFrom(v.ScheduledEndAt),
		ClockInRecordID: textFrom(v.ClockInRecordID), ClockOutRecordID: textFrom(v.ClockOutRecordID),
		LastPunchRecordID: textFrom(v.LastPunchRecordID), PunchCount: int(v.PunchCount),
		WorkedMinutes: int(v.WorkedMinutes), ApprovedLeaveMinutes: int(v.ApprovedLeaveMinutes),
		PendingLeaveMinutes: int(v.PendingLeaveMinutes), RequiredMinutes: int(v.RequiredMinutes),
		OvertimeMinutes: int(v.OvertimeMinutes), DayStatus: v.DayStatus,
		AnomalyReasons: utils.CopyStrings(v.AnomalyReasons), InputFingerprint: v.InputFingerprint,
		Payload: jsonMap(v.Payload), ComputedAt: timeFrom(v.ComputedAt), UpdatedAt: timeFrom(v.UpdatedAt),
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
		WorkDate:                 dateTextFrom(v.WorkDate),
		CorrectionType:           v.CorrectionType,
		TargetClockRecordID:      textFrom(v.TargetClockRecordID),
		ReplacementClockRecordID: textFrom(v.ReplacementClockRecordID),
		Reason:                   v.Reason,
		Status:                   v.Status,
		FormInstanceID:           v.FormInstanceID,
		ClockRecordID:            textFrom(v.ClockRecordID),
		ReviewedByAccountID:      textFrom(v.ReviewedByAccountID),
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

func agentRunFromFields(id, tenantID, accountID, agentID, sessionID, segmentID, inputMessageID, agentRevisionID, modelConnectionID, mode, prompt, answer, status string, referenceItems any, llmCallCount, inputTokens, cachedTokens, outputTokens, totalTokens int64, usageComplete bool, startedAt, completedAt pgtype.Timestamptz, errorCode, errorCategory string, createdAt, updatedAt pgtype.Timestamptz) domain.AgentRun {
	return domain.AgentRun{
		ID: id, TenantID: tenantID, AccountID: accountID, AgentID: agentID,
		SessionID: sessionID, SegmentID: segmentID, InputMessageID: inputMessageID,
		AgentRevisionID: agentRevisionID, ModelConnectionID: modelConnectionID,
		Mode: mode, Prompt: prompt, Answer: answer, Status: status,
		References: jsonRefs(jsonBytes(referenceItems)), LLMCallCount: llmCallCount,
		InputTokens: inputTokens, CachedTokens: cachedTokens, OutputTokens: outputTokens,
		TotalTokens: totalTokens, UsageComplete: usageComplete,
		StartedAt: timePtrFrom(startedAt), CompletedAt: timePtrFrom(completedAt),
		ErrorCode: errorCode, ErrorCategory: errorCategory,
		CreatedAt: timeFrom(createdAt), UpdatedAt: timeFrom(updatedAt),
	}
}

func fromListAgentRunsRow(v sqlc.ListAgentRunsRow) domain.AgentRun {
	return agentRunFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SessionID, v.SegmentID, v.InputMessageID, textFrom(v.AgentRevisionID), textFrom(v.ModelConnectionID), v.Mode, v.Prompt, v.Answer, v.Status, v.ReferenceItems, v.LlmCallCount, v.InputTokens, v.CachedTokens, v.OutputTokens, v.TotalTokens, v.UsageComplete, v.StartedAt, v.CompletedAt, v.ErrorCode, v.ErrorCategory, v.CreatedAt, v.UpdatedAt)
}

func fromListAgentRunsByAccountRow(v sqlc.ListAgentRunsByAccountRow) domain.AgentRun {
	return agentRunFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SessionID, v.SegmentID, v.InputMessageID, textFrom(v.AgentRevisionID), textFrom(v.ModelConnectionID), v.Mode, v.Prompt, v.Answer, v.Status, v.ReferenceItems, v.LlmCallCount, v.InputTokens, v.CachedTokens, v.OutputTokens, v.TotalTokens, v.UsageComplete, v.StartedAt, v.CompletedAt, v.ErrorCode, v.ErrorCategory, v.CreatedAt, v.UpdatedAt)
}

func fromListAgentRunsPageRow(v sqlc.ListAgentRunsPageRow) domain.AgentRun {
	return agentRunFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SessionID, v.SegmentID, v.InputMessageID, textFrom(v.AgentRevisionID), textFrom(v.ModelConnectionID), v.Mode, v.Prompt, v.Answer, v.Status, v.ReferenceItems, v.LlmCallCount, v.InputTokens, v.CachedTokens, v.OutputTokens, v.TotalTokens, v.UsageComplete, v.StartedAt, v.CompletedAt, v.ErrorCode, v.ErrorCategory, v.CreatedAt, v.UpdatedAt)
}

func fromListAgentRunsPageByAccountRow(v sqlc.ListAgentRunsPageByAccountRow) domain.AgentRun {
	return agentRunFromFields(v.ID, v.TenantID, v.AccountID, textFrom(v.AgentID), v.SessionID, v.SegmentID, v.InputMessageID, textFrom(v.AgentRevisionID), textFrom(v.ModelConnectionID), v.Mode, v.Prompt, v.Answer, v.Status, v.ReferenceItems, v.LlmCallCount, v.InputTokens, v.CachedTokens, v.OutputTokens, v.TotalTokens, v.UsageComplete, v.StartedAt, v.CompletedAt, v.ErrorCode, v.ErrorCategory, v.CreatedAt, v.UpdatedAt)
}

func agentModelFromFields(id, tenantID, name, provider, modelName, liteLLMModel, apiBaseURL, apiKeyCiphertext, apiKeyPreview, credentialSecretID string, rateLimitRPM int32, status string, timeoutSeconds int32, monthlyQuota, usedQuota int64, lastTestedAt pgtype.Timestamptz, lastTestStatus, lastTestMessage, syncStatus string, lastSyncedAt pgtype.Timestamptz, lastSyncError, syncedConfigHash string, createdAt, updatedAt pgtype.Timestamptz) domain.AgentModel {
	return domain.AgentModel{
		ID: id, TenantID: tenantID, Name: name, Provider: provider, ModelName: modelName,
		LiteLLMModel: liteLLMModel, APIBaseURL: apiBaseURL, APIKeyCiphertext: apiKeyCiphertext,
		CredentialSecretID: credentialSecretID, APIKeySet: strings.TrimSpace(apiKeyCiphertext) != "",
		APIKeyPreview: apiKeyPreview, RateLimitRPM: int(rateLimitRPM), Status: domain.AgentModelStatus(status),
		TimeoutSeconds: int(timeoutSeconds), MonthlyQuota: monthlyQuota, UsedQuota: usedQuota,
		LastTestedAt: timePtrFrom(lastTestedAt), LastTestStatus: lastTestStatus, LastTestMessage: lastTestMessage,
		SyncStatus: domain.AgentModelSyncStatus(syncStatus), LastSyncedAt: timePtrFrom(lastSyncedAt),
		LastSyncError: lastSyncError, SyncedConfigHash: syncedConfigHash,
		CreatedAt: timeFrom(createdAt), UpdatedAt: timeFrom(updatedAt),
	}
}

func agentModelFromGetRow(v sqlc.GetAgentModelRow) domain.AgentModel {
	return agentModelFromFields(v.ID, v.TenantID, v.Name, v.Provider, v.ModelName, v.LitellmModel, v.ApiBaseUrl, v.ApiKeyCiphertext, v.ApiKeyPreview, textFrom(v.CredentialSecretID), v.RateLimitRpm, v.Status, v.TimeoutSeconds, v.MonthlyQuota, v.UsedQuota, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, v.SyncStatus, v.LastSyncedAt, v.LastSyncError, v.SyncedConfigHash, v.CreatedAt, v.UpdatedAt)
}

func agentModelFromListRow(v sqlc.ListAgentModelsRow) domain.AgentModel {
	return agentModelFromFields(v.ID, v.TenantID, v.Name, v.Provider, v.ModelName, v.LitellmModel, v.ApiBaseUrl, v.ApiKeyCiphertext, v.ApiKeyPreview, textFrom(v.CredentialSecretID), v.RateLimitRpm, v.Status, v.TimeoutSeconds, v.MonthlyQuota, v.UsedQuota, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, v.SyncStatus, v.LastSyncedAt, v.LastSyncError, v.SyncedConfigHash, v.CreatedAt, v.UpdatedAt)
}

func agentModelFromDeleteRow(v sqlc.DeleteAgentModelRow) domain.AgentModel {
	return agentModelFromFields(v.ID, v.TenantID, v.Name, v.Provider, v.ModelName, v.LitellmModel, v.ApiBaseUrl, v.ApiKeyCiphertext, v.ApiKeyPreview, textFrom(v.CredentialSecretID), v.RateLimitRpm, v.Status, v.TimeoutSeconds, v.MonthlyQuota, v.UsedQuota, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, v.SyncStatus, v.LastSyncedAt, v.LastSyncError, v.SyncedConfigHash, v.CreatedAt, v.UpdatedAt)
}

func agentModelFromTestRow(v sqlc.UpdateAgentModelTestResultRow) domain.AgentModel {
	return agentModelFromFields(v.ID, v.TenantID, v.Name, v.Provider, v.ModelName, v.LitellmModel, v.ApiBaseUrl, v.ApiKeyCiphertext, v.ApiKeyPreview, textFrom(v.CredentialSecretID), v.RateLimitRpm, v.Status, v.TimeoutSeconds, v.MonthlyQuota, v.UsedQuota, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, v.SyncStatus, v.LastSyncedAt, v.LastSyncError, v.SyncedConfigHash, v.CreatedAt, v.UpdatedAt)
}

func agentModelFromSyncRow(v sqlc.UpdateAgentModelSyncResultRow) domain.AgentModel {
	return agentModelFromFields(v.ID, v.TenantID, v.Name, v.Provider, v.ModelName, v.LitellmModel, v.ApiBaseUrl, v.ApiKeyCiphertext, v.ApiKeyPreview, textFrom(v.CredentialSecretID), v.RateLimitRpm, v.Status, v.TimeoutSeconds, v.MonthlyQuota, v.UsedQuota, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, v.SyncStatus, v.LastSyncedAt, v.LastSyncError, v.SyncedConfigHash, v.CreatedAt, v.UpdatedAt)
}

func agentExternalToolFromFields(id, tenantID, name, description, kind, transport, endpointURL, authType, authHeaderName, authUsername, authSecretCiphertext, credentialSecretID string, timeoutSeconds int32, status string, lastTestedAt pgtype.Timestamptz, lastTestStatus, lastTestMessage, createdByAccountID string, createdAt, updatedAt, archivedAt pgtype.Timestamptz) domain.AgentExternalTool {
	return domain.AgentExternalTool{
		ID: id, TenantID: tenantID, Name: name, Description: description, Kind: kind, Transport: transport,
		EndpointURL: endpointURL, AuthType: authType, AuthHeaderName: authHeaderName, AuthUsername: authUsername,
		TimeoutSeconds: int(timeoutSeconds), AuthSecretCiphertext: authSecretCiphertext,
		CredentialSecretID: credentialSecretID, CredentialSet: credentialSecretID != "", Status: status,
		LastTestedAt: timePtrFrom(lastTestedAt), LastTestStatus: lastTestStatus, LastTestMessage: lastTestMessage,
		CreatedByAccountID: createdByAccountID, CreatedAt: timeFrom(createdAt), UpdatedAt: timeFrom(updatedAt),
		ArchivedAt: timePtrFrom(archivedAt),
	}
}

func agentExternalToolFromListRow(v sqlc.ListAgentExternalToolsRow) domain.AgentExternalTool {
	return agentExternalToolFromFields(v.ID, v.TenantID, v.Name, v.Description, v.Kind, v.Transport, v.EndpointUrl, v.AuthType, v.AuthHeaderName, v.AuthUsername, v.AuthSecretCiphertext, textFrom(v.CredentialSecretID), v.TimeoutSeconds, v.Status, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, textFrom(v.CreatedByAccountID), v.CreatedAt, v.UpdatedAt, v.ArchivedAt)
}

func agentExternalToolFromDeleteRow(v sqlc.DeleteAgentExternalToolRow) domain.AgentExternalTool {
	return agentExternalToolFromFields(v.ID, v.TenantID, v.Name, v.Description, v.Kind, v.Transport, v.EndpointUrl, v.AuthType, v.AuthHeaderName, v.AuthUsername, v.AuthSecretCiphertext, textFrom(v.CredentialSecretID), v.TimeoutSeconds, v.Status, v.LastTestedAt, v.LastTestStatus, v.LastTestMessage, textFrom(v.CreatedByAccountID), v.CreatedAt, v.UpdatedAt, v.ArchivedAt)
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

func agentDefinitionFromGetRow(v sqlc.GetAgentDefinitionRow) domain.AgentDefinition {
	decodeCtx := []any{"table", "agent_revisions", "agent_id", v.ID, "tenant_id", v.TenantID}
	return domain.AgentDefinition{
		ID:                            v.ID,
		TenantID:                      v.TenantID,
		Name:                          v.Name,
		Description:                   v.Description,
		Emoji:                         v.Emoji,
		Category:                      domain.AgentCategory(v.Category),
		ModelID:                       v.ModelID,
		MainAgentRole:                 v.MainAgentRole,
		SubAgents:                     jsonAgentTeamMembers("sub_agents", jsonBytes(v.SubAgents), decodeCtx...),
		SystemPrompt:                  v.SystemPrompt,
		WelcomeMessage:                v.WelcomeMessage,
		SuggestedQuestions:            jsonStrings("suggested_questions", v.SuggestedQuestions, decodeCtx...),
		SuggestedQuestionTranslations: jsonLocalizedAgentSuggestedQuestions(v.SuggestedQuestionTranslations),
		Tools:                         jsonStrings("tools", jsonBytes(v.Tools), decodeCtx...),
		ExternalToolIDs:               jsonStrings("external_tool_ids", jsonBytes(v.ExternalToolIds), decodeCtx...),
		KnowledgeBaseIDs:              jsonStrings("knowledge_base_ids", jsonBytes(v.KnowledgeBaseIds), decodeCtx...),
		Status:                        domain.AgentDefinitionStatus(v.Status),
		Visibility:                    domain.AgentVisibility(v.Visibility),
		VisibilityTargets:             jsonStrings("visibility_targets", v.VisibilityTargets, decodeCtx...),
		TimeoutSeconds:                int(v.TimeoutSeconds),
		Version:                       int(v.Version),
		PublishedVersion:              int(v.PublishedVersion),
		DraftRevisionID:               textFrom(v.DraftRevisionID),
		PublishedRevisionID:           textFrom(v.PublishedRevisionID),
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

func agentDefinitionVersionFromFields(
	id, tenantID, agentID string,
	version int32,
	name, description, emoji, category, visibility string,
	visibilityTargets []byte,
	mainAgentRole string,
	subAgents any,
	systemPrompt, welcomeMessage string,
	suggestedQuestions, suggestedQuestionTranslations []byte,
	tools, externalToolIDs, knowledgeBaseIDs any,
	modelID, modelConfigChecksum string,
	timeoutSeconds, configSchemaVersion int32,
	checksum, note, createdByAccountID string,
	createdAt pgtype.Timestamptz,
) domain.AgentDefinitionVersion {
	decodeCtx := []any{"table", "agent_revisions", "agent_revision_id", id, "agent_id", agentID, "tenant_id", tenantID}
	return domain.AgentDefinitionVersion{
		ID: id, TenantID: tenantID, AgentID: agentID, Version: int(version),
		Name: name, Description: description, Emoji: emoji, Category: domain.AgentCategory(category),
		Visibility: domain.AgentVisibility(visibility), VisibilityTargets: jsonStrings("visibility_targets", visibilityTargets, decodeCtx...),
		MainAgentRole: mainAgentRole,
		SubAgents:     jsonAgentTeamMembers("sub_agents", jsonBytes(subAgents), decodeCtx...),
		SystemPrompt:  systemPrompt, WelcomeMessage: welcomeMessage,
		SuggestedQuestions:            jsonStrings("suggested_questions", suggestedQuestions, decodeCtx...),
		SuggestedQuestionTranslations: jsonLocalizedAgentSuggestedQuestions(suggestedQuestionTranslations),
		Tools:                         jsonStrings("tools", jsonBytes(tools), decodeCtx...),
		ExternalToolIDs:               jsonStrings("external_tool_ids", jsonBytes(externalToolIDs), decodeCtx...),
		KnowledgeBaseIDs:              jsonStrings("knowledge_base_ids", jsonBytes(knowledgeBaseIDs), decodeCtx...),
		ModelID:                       modelID, ModelConfigChecksum: modelConfigChecksum,
		TimeoutSeconds: int(timeoutSeconds), ConfigSchemaVersion: int(configSchemaVersion),
		Checksum: checksum, Note: note, CreatedByAccountID: createdByAccountID, CreatedAt: timeFrom(createdAt),
	}
}

func agentDefinitionVersionFromListRow(v sqlc.ListAgentDefinitionVersionsRow) domain.AgentDefinitionVersion {
	return agentDefinitionVersionFromFields(
		v.ID, v.TenantID, v.AgentID, v.Version,
		v.Name, v.Description, v.Emoji, v.Category, v.Visibility, v.VisibilityTargets,
		v.MainAgentRole, v.SubAgents, v.SystemPrompt, v.WelcomeMessage,
		v.SuggestedQuestions, v.SuggestedQuestionTranslations,
		v.Tools, v.ExternalToolIds, v.KnowledgeBaseIds,
		v.ModelID, v.ModelConfigChecksum, v.TimeoutSeconds, v.ConfigSchemaVersion,
		v.Checksum, v.Note, textFrom(v.CreatedByAccountID), v.CreatedAt,
	)
}

func agentDefinitionVersionFromGetRow(v sqlc.GetAgentDefinitionVersionRow) domain.AgentDefinitionVersion {
	return agentDefinitionVersionFromFields(
		v.ID, v.TenantID, v.AgentID, v.Version,
		v.Name, v.Description, v.Emoji, v.Category, v.Visibility, v.VisibilityTargets,
		v.MainAgentRole, v.SubAgents, v.SystemPrompt, v.WelcomeMessage,
		v.SuggestedQuestions, v.SuggestedQuestionTranslations,
		v.Tools, v.ExternalToolIds, v.KnowledgeBaseIds,
		v.ModelID, v.ModelConfigChecksum, v.TimeoutSeconds, v.ConfigSchemaVersion,
		v.Checksum, v.Note, textFrom(v.CreatedByAccountID), v.CreatedAt,
	)
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

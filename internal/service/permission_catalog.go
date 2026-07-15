package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
)

// SyncPermissionCatalogForAllTenants 將程式宣告的權限與選單 catalog 同步到所有租戶。
func (c *Service) SyncPermissionCatalogForAllTenants(ctx context.Context) error {
	if err := c.ensureBuiltinPermissionPackage(ctx); err != nil {
		return err
	}
	tenants, err := c.store.ListTenants(ctx)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		if err := c.SyncPermissionCatalog(ctx, tenant.ID); err != nil {
			return err
		}
	}
	return nil
}

// SyncPermissionCatalog 將程式宣告的權限與選單 catalog 同步到指定租戶。
func (c *Service) SyncPermissionCatalog(ctx context.Context, tenantID string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return BadRequest("tenant id is required")
	}
	if err := c.ensureBuiltinPermissionPackage(ctx); err != nil {
		return err
	}
	now := c.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := repository.WithinTenantTransaction(ctx, c.store, tenantID, func(tx repository.Store) error {
		return syncPermissionCatalogForTenant(ctx, tx, tenantID, now)
	}); err != nil {
		return err
	}
	c.invalidateAuthzSnapshots(ctx, tenantID)
	return nil
}

// upsertPermissionSetWithItems writes the JSON authoring source and rebuilds its normalized projection in one transaction.
func (c *Service) upsertPermissionSetWithItems(ctx RequestContext, set PermissionSet) (int, error) {
	if err := c.store.UpsertPermissionSet(goContext(ctx), set); err != nil {
		return 0, err
	}
	return syncPermissionSetItems(goContext(ctx), c.store, set, c.Now())
}

// syncPermissionCatalogForTenant 同步租戶權限 catalog、選單 catalog，並回填權限集合項。
func syncPermissionCatalogForTenant(ctx context.Context, store repository.Store, tenantID string, now time.Time) error {
	return syncPermissionCatalogFromPackageForTenant(ctx, store, tenantID, DefaultHRPermissionPackageContent(), now)
}

// syncPermissionCatalogFromPackageForTenant 從權限包內容同步租戶權限 catalog、選單 catalog，並回填權限集合項。
func syncPermissionCatalogFromPackageForTenant(ctx context.Context, store repository.IAMStore, tenantID string, content PermissionPackageContent, now time.Time) error {
	for _, item := range permissionCatalogItemsFromPackage(tenantID, content, now) {
		if err := store.UpsertPermissionCatalogItem(ctx, item); err != nil {
			return err
		}
	}
	for _, item := range menuItemsFromPermissionPackage(tenantID, content.Menus, now) {
		if err := store.UpsertMenuItem(ctx, item); err != nil {
			return err
		}
	}
	sets, err := store.ListPermissionSets(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, set := range sets {
		if _, err := syncPermissionSetItems(ctx, store, set, now); err != nil {
			return err
		}
	}
	return nil
}

// permissionCatalogItemsFromPackage 由權限包內容產生租戶 catalog 項。
func permissionCatalogItemsFromPackage(tenantID string, content PermissionPackageContent, now time.Time) []PermissionCatalogItem {
	permissions := normalizePermissions(content.Permissions)
	out := make([]PermissionCatalogItem, 0, len(permissions)+countPermissionPackageMenus(content.Menus))
	for _, permission := range permissions {
		out = append(out, permissionCatalogItemFromPermission(tenantID, permission, now))
	}
	for _, menu := range menuItemsFromPermissionPackage(tenantID, content.Menus, now) {
		out = append(out, PermissionCatalogItem{
			ID:             stableCatalogID("perm", tenantID, string(PermissionTypeMenu), menu.Key),
			TenantID:       tenantID,
			Application:    applicationForMenuKey(menu.Key),
			Resource:       menu.Key,
			Action:         string(ActionRead),
			PermissionType: PermissionTypeMenu,
			MenuKey:        menu.Key,
			Name:           menu.Label,
			Description:    "Menu visibility: " + menu.Key,
			Severity:       string(SeverityLow),
			CreatedAt:      now,
		})
	}
	return out
}

// menuItemsFromPermissionPackage 扁平化權限包選單。
func menuItemsFromPermissionPackage(tenantID string, nodes []PermissionPackageMenu, now time.Time) []MenuItem {
	out := make([]MenuItem, 0, countPermissionPackageMenus(nodes))
	appendPermissionPackageMenuItems(&out, tenantID, "", nodes, now)
	return out
}

// appendPermissionPackageMenuItems 附加扁平化權限包選單。
func appendPermissionPackageMenuItems(out *[]MenuItem, tenantID, parentKey string, nodes []PermissionPackageMenu, now time.Time) {
	for index, node := range nodes {
		sortOrder := node.SortOrder
		if sortOrder == 0 {
			sortOrder = index
		}
		item := MenuItem{
			ID:        stableCatalogID("menu", tenantID, node.Key),
			TenantID:  tenantID,
			Key:       node.Key,
			Label:     node.Label,
			Path:      node.Path,
			Icon:      node.Icon,
			ParentKey: parentKey,
			SortOrder: sortOrder,
			CreatedAt: now,
		}
		*out = append(*out, item)
		appendPermissionPackageMenuItems(out, tenantID, node.Key, node.Children, now)
	}
}

// countPermissionPackageMenus 計算權限包選單節點數。
func countPermissionPackageMenus(nodes []PermissionPackageMenu) int {
	total := 0
	for _, node := range nodes {
		total++
		total += countPermissionPackageMenus(node.Children)
	}
	return total
}

// syncPermissionSetItems 將權限集合內的權限映射到 catalog 項並替換關聯表。
func syncPermissionSetItems(ctx context.Context, store repository.IAMStore, set PermissionSet, now time.Time) (int, error) {
	permissions := normalizePermissions(set.Permissions)
	items := make([]PermissionSetItem, 0, len(permissions))
	seen := map[string]struct{}{}
	for _, permission := range permissions {
		catalog := permissionCatalogItemFromPermission(set.TenantID, permission, now)
		if err := store.UpsertPermissionCatalogItem(ctx, catalog); err != nil {
			return 0, err
		}
		stored, ok, err := store.GetPermissionCatalogItemByKey(ctx, catalog.TenantID, catalog.Application, catalog.Resource, catalog.Action, catalog.PermissionType)
		if err != nil {
			return 0, err
		}
		if ok {
			catalog.ID = stored.ID
		}
		if _, exists := seen[catalog.ID]; exists {
			continue
		}
		seen[catalog.ID] = struct{}{}
		items = append(items, PermissionSetItem{
			ID:              stableCatalogID("psi", set.TenantID, set.ID, catalog.ID),
			TenantID:        set.TenantID,
			PermissionSetID: set.ID,
			PermissionID:    catalog.ID,
			CreatedAt:       now,
		})
	}
	if err := store.ReplacePermissionSetItems(ctx, set.TenantID, set.ID, items); err != nil {
		return 0, err
	}
	return len(items), nil
}

// defaultPermissionCatalogItems 由程式宣告的路由政策與選單 catalog 產生預設權限 catalog。
func defaultPermissionCatalogItems(tenantID string, now time.Time) []PermissionCatalogItem {
	permissions := defaultPermissions()
	out := make([]PermissionCatalogItem, 0, len(permissions)+countMenuNodes(defaultMenuCatalog))
	for _, permission := range permissions {
		out = append(out, permissionCatalogItemFromPermission(tenantID, permission, now))
	}
	for _, menu := range defaultMenuItems(tenantID, now) {
		out = append(out, PermissionCatalogItem{
			ID:             stableCatalogID("perm", tenantID, string(PermissionTypeMenu), menu.Key),
			TenantID:       tenantID,
			Application:    applicationForMenuKey(menu.Key),
			Resource:       menu.Key,
			Action:         string(ActionRead),
			PermissionType: PermissionTypeMenu,
			MenuKey:        menu.Key,
			Name:           menu.Label,
			Description:    "Menu visibility: " + menu.Key,
			Severity:       string(SeverityLow),
			CreatedAt:      now,
		})
	}
	return out
}

// permissionCatalogItemFromPermission 將 domain.Permission 正規化為 catalog 項。
func permissionCatalogItemFromPermission(tenantID string, permission Permission, now time.Time) PermissionCatalogItem {
	permission = normalizePermission(permission)
	permissionType := permission.PermissionType
	if permissionType == "" {
		permissionType = PermissionTypeAPI
	}
	menuKey := canonicalPageMenuKey(permission.MenuKey)
	if menuKey == "" {
		menuKey = canonicalMenuKeyForPermission(permission)
	}
	application := string(permission.ApplicationCode)
	resource := strings.TrimSpace(permission.Resource)
	action := strings.TrimSpace(string(permission.Action))
	severity := strings.TrimSpace(permission.Severity)
	if severity == "" {
		switch permission.RiskLevel {
		case string(domain.RiskHigh):
			severity = string(SeverityHigh)
		case string(domain.RiskCritical):
			severity = string(domain.SeverityCritical)
		default:
			severity = string(SeverityLow)
		}
	}
	name := strings.TrimSpace(permission.Name)
	if name == "" {
		name = resource + "." + action
	}
	return PermissionCatalogItem{
		ID:             stableCatalogID("perm", tenantID, application, resource, action, string(permissionType)),
		TenantID:       tenantID,
		Application:    application,
		Resource:       resource,
		Action:         action,
		PermissionType: permissionType,
		MenuKey:        menuKey,
		Name:           name,
		Description:    strings.TrimSpace(permission.Description),
		HighRisk:       permission.HighRisk || isHighRiskPermission(permission),
		Severity:       severity,
		CreatedAt:      now,
	}
}

// defaultMenuItems 扁平化預設選單 catalog。
func defaultMenuItems(tenantID string, now time.Time) []MenuItem {
	out := make([]MenuItem, 0, countMenuNodes(defaultMenuCatalog))
	appendMenuItems(&out, tenantID, "", defaultMenuCatalog, now)
	return out
}

// appendMenuItems 附加扁平化選單。
func appendMenuItems(out *[]MenuItem, tenantID, parentKey string, nodes []MenuNode, now time.Time) {
	for index, node := range nodes {
		item := MenuItem{
			ID:        stableCatalogID("menu", tenantID, node.Key),
			TenantID:  tenantID,
			Key:       node.Key,
			Label:     node.Label,
			Path:      node.Path,
			Icon:      node.Icon,
			ParentKey: parentKey,
			SortOrder: index,
			CreatedAt: now,
		}
		*out = append(*out, item)
		appendMenuItems(out, tenantID, node.Key, node.Children, now)
	}
}

// countMenuNodes 計算選單節點數。
func countMenuNodes(nodes []MenuNode) int {
	total := 0
	for _, node := range nodes {
		total++
		total += countMenuNodes(node.Children)
	}
	return total
}

// applicationForMenuKey 推導選單所屬應用。
func applicationForMenuKey(key string) string {
	head, _, _ := strings.Cut(strings.TrimSpace(key), ".")
	switch head {
	case string(AppHR), string(AppIAM), string(AppWorkflow), string(AppAttendance), string(AppAudit):
		return head
	case "agents":
		return string(AppAgent)
	default:
		return string(AppPlatform)
	}
}

// stableCatalogID 建立跨啟動穩定且全域唯一的 catalog ID。
func stableCatalogID(prefix, tenantID string, parts ...string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(strings.TrimSpace(tenantID)))
	for _, part := range parts {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
	}
	return prefix + "-" + hex.EncodeToString(h.Sum(nil))[:20]
}

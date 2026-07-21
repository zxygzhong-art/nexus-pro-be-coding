package service

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// optionCursorSeparator 分隔遊標中的排序鍵與主鍵。
const optionCursorSeparator = "\x00"

// ListIamAccountOptions 列出 IAM 帳號的輕量選項。
func (c IAMService) ListIamAccountOptions(ctx RequestContext, query OptionQuery) (OptionPage, error) {
	if _, _, err := c.requireIAMAuthz(ctx, ResourceIAMAccount, ActionRead, ""); err != nil {
		return OptionPage{}, err
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return OptionPage{}, err
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	items := make([]OptionItem, 0, len(accounts))
	for _, account := range accounts {
		if keyword != "" && !accountMatchesKeyword(account, keyword) {
			continue
		}
		label := strings.TrimSpace(account.DisplayName)
		if label == "" {
			label = strings.TrimSpace(account.Email)
		}
		if label == "" {
			label = account.ID
		}
		items = append(items, OptionItem{
			ID:    account.ID,
			Label: label,
			Meta: map[string]any{
				"email":       account.Email,
				"employee_id": account.EmployeeID,
				"status":      account.Status,
			},
		})
	}
	return optionPageFromItems(items, query)
}

// ListPermissionSetOptions 列出權限集合的輕量選項。
func (c IAMService) ListPermissionSetOptions(ctx RequestContext, query OptionQuery) (OptionPage, error) {
	sets, err := c.ListPermissionSets(ctx)
	if err != nil {
		return OptionPage{}, err
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	items := make([]OptionItem, 0, len(sets))
	for _, set := range sets {
		if keyword != "" && !optionTextMatches(keyword, set.ID, set.Name, set.Description) {
			continue
		}
		items = append(items, OptionItem{
			ID:    set.ID,
			Label: optionLabel(set.Name, set.ID),
			Meta: map[string]any{
				"description":      set.Description,
				"permission_count": len(set.Permissions),
			},
		})
	}
	return optionPageFromItems(items, query)
}

// ListUserGroupOptions 列出使用者羣組的輕量選項。
func (c IAMService) ListUserGroupOptions(ctx RequestContext, query OptionQuery) (OptionPage, error) {
	groups, err := c.ListUserGroups(ctx)
	if err != nil {
		return OptionPage{}, err
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	items := make([]OptionItem, 0, len(groups))
	for _, group := range groups {
		if keyword != "" && !optionTextMatches(keyword, group.ID, group.Name, group.Description) {
			continue
		}
		items = append(items, OptionItem{
			ID:    group.ID,
			Label: optionLabel(group.Name, group.ID),
			Meta: map[string]any{
				"description":  group.Description,
				"member_count": len(group.MemberAccountIDs),
			},
		})
	}
	return optionPageFromItems(items, query)
}

// ListAssumableRoleOptions 列出 assumable 角色的輕量選項。
func (c IAMService) ListAssumableRoleOptions(ctx RequestContext, query OptionQuery) (OptionPage, error) {
	roles, err := c.ListAssumableRoles(ctx)
	if err != nil {
		return OptionPage{}, err
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	items := make([]OptionItem, 0, len(roles))
	for _, role := range roles {
		if keyword != "" && !optionTextMatches(keyword, role.ID, role.Name, role.Description) {
			continue
		}
		items = append(items, OptionItem{
			ID:    role.ID,
			Label: optionLabel(role.Name, role.ID),
			Meta: map[string]any{
				"description":              role.Description,
				"trusted":                  role.Trusted,
				"session_duration_seconds": role.SessionDurationSeconds,
			},
		})
	}
	return optionPageFromItems(items, query)
}

// optionLabel 回退到主鍵以確保選項永遠有可顯示文字。
func optionLabel(name, id string) string {
	if label := strings.TrimSpace(name); label != "" {
		return label
	}
	return id
}

// optionTextMatches 判斷任一候選欄位是否包含關鍵字。
func optionTextMatches(keyword string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), keyword) {
			return true
		}
	}
	return false
}

// optionPageFromItems 以 (label, id) 穩定排序做 keyset 遊標分頁。
func optionPageFromItems(items []OptionItem, query OptionQuery) (OptionPage, error) {
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = DefaultOptionPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := optionSortKey(items[i]), optionSortKey(items[j])
		if left == right {
			return items[i].ID < items[j].ID
		}
		return left < right
	})
	cursor := strings.TrimSpace(query.Cursor)
	if cursor != "" {
		cursorLabel, cursorID, err := decodeOptionCursor(cursor)
		if err != nil {
			return OptionPage{}, BadRequest("cursor is invalid")
		}
		remaining := make([]OptionItem, 0, len(items))
		for _, item := range items {
			key := optionSortKey(item)
			if key > cursorLabel || (key == cursorLabel && item.ID > cursorID) {
				remaining = append(remaining, item)
			}
		}
		items = remaining
	}
	nextCursor := ""
	if len(items) > pageSize {
		last := items[pageSize-1]
		nextCursor = encodeOptionCursor(optionSortKey(last), last.ID)
		items = items[:pageSize]
	}
	if items == nil {
		items = []OptionItem{}
	}
	return OptionPage{Items: items, NextCursor: nextCursor}, nil
}

// optionSortKey 以大小寫不敏感的 label 作為主要排序鍵。
func optionSortKey(item OptionItem) string {
	return strings.ToLower(item.Label)
}

// encodeOptionCursor 將最後一筆的 (label, id) 排序鍵序列化為遊標。
func encodeOptionCursor(label, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(label + optionCursorSeparator + id))
}

// decodeOptionCursor 解析 encodeOptionCursor 產生的遊標。
func decodeOptionCursor(cursor string) (string, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), optionCursorSeparator, 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid cursor")
	}
	return parts[0], parts[1], nil
}

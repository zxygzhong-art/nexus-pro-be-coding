package service

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

// NotificationService 實作目前帳號的系統通知流程。
type NotificationService struct {
	*Service
}

// Notifications 回傳通知服務 facade。
func (c *Service) Notifications() NotificationService {
	return NotificationService{Service: c}
}

// ListNotifications 回傳目前帳號可見通知的一頁遊標結果。
func (c NotificationService) ListNotifications(ctx RequestContext, query NotificationListQuery) (NotificationListResponse, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return NotificationListResponse{}, err
	}
	query, err := normalizeNotificationListQuery(query)
	if err != nil {
		return NotificationListResponse{}, err
	}
	storeQuery := query
	storeQuery.Limit = query.Limit + 1
	items, err := c.store.ListNotificationItems(goContext(ctx), ctx.TenantID, ctx.AccountID, storeQuery)
	if err != nil {
		return NotificationListResponse{}, err
	}
	nextCursor := ""
	if len(items) > query.Limit {
		items = items[:query.Limit]
		last := items[len(items)-1]
		nextCursor = encodeNotificationCursor(last)
	}
	unread, err := c.store.CountUnreadNotifications(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return NotificationListResponse{}, err
	}
	toneCounts, err := c.store.CountNotificationTones(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return NotificationListResponse{}, err
	}
	if items == nil {
		items = []NotificationItem{}
	}
	return NotificationListResponse{Items: items, NextCursor: nextCursor, UnreadCount: unread, ToneCounts: toneCounts}, nil
}

// UnreadNotificationCount 回傳目前未讀 badge 數量。
func (c NotificationService) UnreadNotificationCount(ctx RequestContext) (NotificationUnreadCountResponse, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return NotificationUnreadCountResponse{}, err
	}
	count, err := c.store.CountUnreadNotifications(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return NotificationUnreadCountResponse{}, err
	}
	return NotificationUnreadCountResponse{UnreadCount: count}, nil
}

// MarkNotificationRead 將目前帳號的一筆已送達通知標記為已讀。
func (c NotificationService) MarkNotificationRead(ctx RequestContext, id string) (NotificationReadResponse, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return NotificationReadResponse{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return NotificationReadResponse{}, BadRequest("notification id is required")
	}
	readAt := c.Now()
	item, ok, err := c.store.MarkNotificationRead(goContext(ctx), ctx.TenantID, ctx.AccountID, id, readAt)
	if err != nil {
		return NotificationReadResponse{}, err
	}
	if !ok {
		return NotificationReadResponse{}, NotFound("notification", id)
	}
	if item.ReadAt != nil {
		readAt = *item.ReadAt
	}
	return NotificationReadResponse{ID: item.ID, ReadAt: readAt}, nil
}

// MarkAllNotificationsRead 將所有已送達的未讀通知標記為已讀。
func (c NotificationService) MarkAllNotificationsRead(ctx RequestContext) (NotificationReadAllResponse, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return NotificationReadAllResponse{}, err
	}
	updated, err := c.store.MarkAllNotificationsRead(goContext(ctx), ctx.TenantID, ctx.AccountID, c.Now())
	if err != nil {
		return NotificationReadAllResponse{}, err
	}
	unread, err := c.store.CountUnreadNotifications(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return NotificationReadAllResponse{}, err
	}
	return NotificationReadAllResponse{UpdatedCount: updated, UnreadCount: unread}, nil
}

// normalizeNotificationListQuery 驗證篩選條件並解析遊標。
func normalizeNotificationListQuery(query NotificationListQuery) (NotificationListQuery, error) {
	query.Tone = strings.TrimSpace(query.Tone)
	if query.Tone != "" && domain.NormalizeNotificationTone(query.Tone) == "" {
		return NotificationListQuery{}, BadRequest("tone must be success, info or warning")
	}
	if query.Limit <= 0 {
		query.Limit = domain.DefaultNotificationLimit
	}
	if query.Limit > domain.MaxNotificationLimit {
		query.Limit = domain.MaxNotificationLimit
	}
	query.Cursor = strings.TrimSpace(query.Cursor)
	if query.Cursor == "" {
		return query, nil
	}
	createdAt, id, err := decodeNotificationCursor(query.Cursor)
	if err != nil {
		return NotificationListQuery{}, BadRequest("cursor is invalid")
	}
	query.HasCursor = true
	query.CursorCreatedAt = createdAt
	query.CursorID = id
	return query, nil
}

// encodeNotificationCursor 將最後一列的穩定倒序排序鍵序列化。
func encodeNotificationCursor(item NotificationItem) string {
	raw := fmt.Sprintf("%d|%s", item.CreatedAt.UTC().UnixNano(), item.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeNotificationCursor 解析 encodeNotificationCursor 產生的遊標。
func decodeNotificationCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return time.Time{}, "", fmt.Errorf("invalid cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", err
	}
	return time.Unix(0, nanos).UTC(), parts[1], nil
}

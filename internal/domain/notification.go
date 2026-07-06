package domain

import "time"

// NotificationTone 表示 Header 鈴鐺使用的視覺狀態。
type NotificationTone string

const (
	NotificationToneSuccess NotificationTone = "success"
	NotificationToneInfo    NotificationTone = "info"
	NotificationToneWarning NotificationTone = "warning"

	DefaultNotificationLimit = 20
	MaxNotificationLimit     = 50
)

// Notification 保存同一租戶內多個收件者共用的通知內容。
type Notification struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	Tone               string     `json:"tone"`
	Category           string     `json:"category"`
	Title              string     `json:"title"`
	Body               string     `json:"body"`
	StatusText         string     `json:"status_text"`
	LinkURL            string     `json:"link_url,omitempty"`
	SourceType         string     `json:"source_type,omitempty"`
	SourceID           string     `json:"source_id,omitempty"`
	CreatedByAccountID string     `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
}

// NotificationRecipient 保存單一帳號的通知送達與已讀狀態。
type NotificationRecipient struct {
	NotificationID string     `json:"notification_id"`
	TenantID       string     `json:"tenant_id"`
	AccountID      string     `json:"account_id"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// NotificationItem 表示回傳給目前帳號的通知列表列資料。
type NotificationItem struct {
	ID         string     `json:"id"`
	Tone       string     `json:"tone"`
	Category   string     `json:"category"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	StatusText string     `json:"status_text"`
	LinkURL    string     `json:"link_url,omitempty"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// NotificationToneCounts 彙總 modal 篩選器可見的通知數量。
type NotificationToneCounts struct {
	All     int `json:"all"`
	Success int `json:"success"`
	Info    int `json:"info"`
	Warning int `json:"warning"`
}

// NotificationListQuery 保存目前帳號的通知查詢條件。
type NotificationListQuery struct {
	Tone            string
	UnreadOnly      bool
	Cursor          string
	Limit           int
	HasCursor       bool
	CursorCreatedAt time.Time
	CursorID        string
}

// NotificationListResponse 包裝一頁游標結果與 header badge metadata。
type NotificationListResponse struct {
	Items       []NotificationItem     `json:"items"`
	NextCursor  string                 `json:"next_cursor,omitempty"`
	UnreadCount int                    `json:"unread_count"`
	ToneCounts  NotificationToneCounts `json:"tone_counts"`
}

// NotificationUnreadCountResponse 是未讀數輪詢用的輕量回應。
type NotificationUnreadCountResponse struct {
	UnreadCount int `json:"unread_count"`
}

// NotificationReadResponse 回報單筆已讀更新結果。
type NotificationReadResponse struct {
	ID     string    `json:"id"`
	ReadAt time.Time `json:"read_at"`
}

// NotificationReadAllResponse 回報批次已讀更新影響的未讀列數。
type NotificationReadAllResponse struct {
	UpdatedCount int `json:"updated_count"`
	UnreadCount  int `json:"unread_count"`
}

// NormalizeNotificationTone 在 tone 可識別時回傳標準值。
func NormalizeNotificationTone(raw string) string {
	switch NotificationTone(raw) {
	case NotificationToneSuccess, NotificationToneInfo, NotificationToneWarning:
		return raw
	default:
		return ""
	}
}

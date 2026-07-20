package repository

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"
)

// NotificationStore 持久化系統通知與各帳號的已讀狀態。
type NotificationStore interface {
	UpsertNotification(context.Context, domain.Notification) error
	UpsertNotificationRecipient(context.Context, domain.NotificationRecipient) error
	ListNotificationItems(context.Context, string, string, domain.NotificationListQuery) ([]domain.NotificationItem, error)
	CountUnreadNotifications(context.Context, string, string) (int, error)
	CountNotificationTones(context.Context, string, string) (domain.NotificationToneCounts, error)
	MarkNotificationRead(context.Context, string, string, string, time.Time) (domain.NotificationItem, bool, error)
	MarkAllNotificationsRead(context.Context, string, string, time.Time) (int, error)
}

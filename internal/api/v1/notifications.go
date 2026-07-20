package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// NotificationCtrl 連接目前帳號的系統通知 endpoints。
type NotificationCtrl struct {
	routes routeBinder
	svc    service.NotificationFacade
}

// RegisterRoutes 將系統通知路由掛到 v1 group。
func (c NotificationCtrl) RegisterRoutes(router *gin.RouterGroup) {
	notifications := router.Group("/notifications")
	notifications.GET("", c.routes.Handle("me", "read", c.listNotifications))
	notifications.GET("/unread-count", c.routes.Handle("me", "read", c.unreadCount))
	notifications.POST("/:id/read", c.routes.Handle("me", "update", c.markRead, PathParam(PathParamID)))
	notifications.POST("/read-all", c.routes.Handle("me", "update", c.markAllRead))
}

// listNotifications 回傳目前帳號可見的通知列表。
func (c NotificationCtrl) listNotifications(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := notificationListQueryFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListNotifications(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// unreadCount 回傳目前帳號的未讀通知數。
func (c NotificationCtrl) unreadCount(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	result, err := c.svc.UnreadNotificationCount(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// markRead 將目前帳號的一筆通知標記為已讀。
func (c NotificationCtrl) markRead(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	result, err := c.svc.MarkNotificationRead(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// markAllRead 將目前帳號的所有未讀通知標記為已讀。
func (c NotificationCtrl) markAllRead(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	result, err := c.svc.MarkAllNotificationsRead(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// notificationListQueryFromRequest 從 query params 解析通知篩選條件。
func notificationListQueryFromRequest(r *http.Request) (domain.NotificationListQuery, error) {
	values := r.URL.Query()
	unreadOnly, err := optionalBoolQuery(values.Get("unread_only"), "unread_only")
	if err != nil {
		return domain.NotificationListQuery{}, err
	}
	limit, err := positiveIntQuery(values.Get("limit"), "limit", domain.MaxNotificationLimit)
	if err != nil {
		return domain.NotificationListQuery{}, err
	}
	return domain.NotificationListQuery{
		Tone:       strings.TrimSpace(values.Get("tone")),
		UnreadOnly: unreadOnly,
		Cursor:     strings.TrimSpace(values.Get("cursor")),
		Limit:      limit,
	}, nil
}

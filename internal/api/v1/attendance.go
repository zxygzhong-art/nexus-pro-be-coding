package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// AttendanceCtrl wires leave endpoints to the attendance service facade.
type AttendanceCtrl struct {
	routes routeBinder
	svc    service.AttendanceFacade
}

// RegisterRoutes attaches attendance and leave routes to the v1 route group.
func (c AttendanceCtrl) RegisterRoutes(router *gin.RouterGroup) {
	attendance := router.Group("/attendance")
	attendance.GET("/leave-balances", c.routes.Handle("attendance.leave", "read", c.listLeaveBalances))
	attendance.GET("/leave-requests", c.routes.Handle("attendance.leave", "read", c.listLeaveRequests))
	attendance.POST("/leave-requests", c.routes.Handle("attendance.leave", "create", c.createLeaveRequest))
}

func (c AttendanceCtrl) listLeaveBalances(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListLeaveBalancePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

func (c AttendanceCtrl) listLeaveRequests(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListLeaveRequestPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

func (c AttendanceCtrl) createLeaveRequest(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateLeaveRequestInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateLeaveRequest(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

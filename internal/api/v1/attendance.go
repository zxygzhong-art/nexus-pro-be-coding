package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// AttendanceCtrl 定義考勤 ctrl 的資料結構。
type AttendanceCtrl struct {
	routes routeBinder
	svc    service.AttendanceFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c AttendanceCtrl) RegisterRoutes(router *gin.RouterGroup) {
	attendance := router.Group("/attendance")
	attendance.GET("/leave-balances", c.routes.Handle("attendance.leave", "read", c.listLeaveBalances))
	attendance.GET("/leave-requests", c.routes.Handle("attendance.leave", "read", c.listLeaveRequests))
	attendance.POST("/leave-requests", c.routes.Handle("attendance.leave", "create", c.createLeaveRequest))
	attendance.GET("/policies/current", c.routes.Handle("attendance.leave", "read", c.currentPolicy))
	attendance.PATCH("/policies/current", c.routes.Handle("attendance.leave", "update", c.updatePolicy))
	attendance.GET("/worksites", c.routes.Handle("attendance.worksite", "read", c.listWorksites))
	attendance.POST("/worksites", c.routes.Handle("attendance.worksite", "create", c.createWorksite))
	attendance.PATCH("/worksites", c.routes.Handle("attendance.worksite", "update", c.updateWorksite))
	attendance.GET("/shifts", c.routes.Handle("attendance.shift", "read", c.listShifts))
	attendance.POST("/shifts", c.routes.Handle("attendance.shift", "create", c.createShift))
	attendance.PATCH("/shifts", c.routes.Handle("attendance.shift", "update", c.updateShift))
	attendance.GET("/shift-assignments", c.routes.Handle("attendance.shift_assignment", "read", c.listShiftAssignments))
	attendance.POST("/shift-assignments", c.routes.Handle("attendance.shift_assignment", "create", c.createShiftAssignment))
	attendance.GET("/clock-status", c.routes.Handle("attendance.clock", "read", c.clockStatus))
	attendance.GET("/clock-records", c.routes.Handle("attendance.clock", "read", c.listClockRecords))
	attendance.POST("/clock-records", c.routes.Handle("attendance.clock", "create", c.createClockRecord))
	attendance.GET("/corrections", c.routes.Handle("attendance.correction", "read", c.listCorrections))
	attendance.POST("/corrections", c.routes.Handle("attendance.correction", "create", c.createCorrection))
	attendance.POST("/corrections/:id/approve", c.routes.Handle("attendance.correction", "approve", c.approveCorrection, ResourceID(PathParamID)))
	attendance.POST("/corrections/:id/reject", c.routes.Handle("attendance.correction", "update", c.rejectCorrection, ResourceID(PathParamID)))
}

// listLeaveBalances 處理請假 balances 的 HTTP 請求。
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

// listLeaveRequests 處理請假請求的 HTTP 請求。
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

// createLeaveRequest 處理請假請求的 HTTP 請求。
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

// currentPolicy 處理目前政策的 HTTP 請求。
func (c AttendanceCtrl) currentPolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.CurrentAttendancePolicy(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updatePolicy 處理政策的 HTTP 請求。
func (c AttendanceCtrl) updatePolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAttendancePolicyInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateAttendancePolicy(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listWorksites 處理 worksites 的 HTTP 請求。
func (c AttendanceCtrl) listWorksites(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListAttendanceWorksitePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createWorksite 處理工作地點的 HTTP 請求。
func (c AttendanceCtrl) createWorksite(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAttendanceWorksiteInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateAttendanceWorksite(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateWorksite 處理工作地點的 HTTP 請求。
func (c AttendanceCtrl) updateWorksite(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAttendanceWorksiteInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateAttendanceWorksite(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listShifts 處理 shifts 的 HTTP 請求。
func (c AttendanceCtrl) listShifts(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListAttendanceShiftPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createShift 處理班別的 HTTP 請求。
func (c AttendanceCtrl) createShift(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAttendanceShiftInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateAttendanceShift(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateShift 處理班別的 HTTP 請求。
func (c AttendanceCtrl) updateShift(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAttendanceShiftInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateAttendanceShift(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listShiftAssignments 處理班別指派的 HTTP 請求。
func (c AttendanceCtrl) listShiftAssignments(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListAttendanceShiftAssignmentPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createShiftAssignment 處理班別指派的 HTTP 請求。
func (c AttendanceCtrl) createShiftAssignment(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAttendanceShiftAssignmentInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateAttendanceShiftAssignment(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// clockStatus 處理打卡狀態的 HTTP 請求。
func (c AttendanceCtrl) clockStatus(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.AttendanceClockStatus(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listClockRecords 處理打卡 records 的 HTTP 請求。
func (c AttendanceCtrl) listClockRecords(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListAttendanceClockRecordPage(ctx, attendanceClockRecordQueryFromRequest(r), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createClockRecord 處理打卡 record 的 HTTP 請求。
func (c AttendanceCtrl) createClockRecord(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAttendanceClockRecordInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateAttendanceClockRecord(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// listCorrections 處理 corrections 的 HTTP 請求。
func (c AttendanceCtrl) listCorrections(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListAttendanceCorrectionPage(ctx, attendanceCorrectionQueryFromRequest(r), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createCorrection 處理 correction 的 HTTP 請求。
func (c AttendanceCtrl) createCorrection(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateAttendanceCorrectionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateAttendanceCorrection(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// approveCorrection 處理 correction 的 HTTP 請求。
func (c AttendanceCtrl) approveCorrection(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.ReviewAttendanceCorrectionInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.ApproveAttendanceCorrection(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// rejectCorrection 處理 correction 的 HTTP 請求。
func (c AttendanceCtrl) rejectCorrection(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.ReviewAttendanceCorrectionInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.RejectAttendanceCorrection(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// attendanceClockRecordQueryFromRequest 處理考勤打卡 record 查詢 來源 請求。
func attendanceClockRecordQueryFromRequest(r *http.Request) domain.AttendanceClockRecordQuery {
	values := r.URL.Query()
	return domain.AttendanceClockRecordQuery{
		EmployeeID:   strings.TrimSpace(values.Get("employee_id")),
		FromDate:     strings.TrimSpace(values.Get("from_date")),
		ToDate:       strings.TrimSpace(values.Get("to_date")),
		Direction:    strings.TrimSpace(values.Get("direction")),
		RecordStatus: strings.TrimSpace(values.Get("record_status")),
		Source:       strings.TrimSpace(values.Get("source")),
	}
}

// attendanceCorrectionQueryFromRequest 處理考勤 correction 查詢 來源 請求。
func attendanceCorrectionQueryFromRequest(r *http.Request) domain.AttendanceCorrectionQuery {
	values := r.URL.Query()
	return domain.AttendanceCorrectionQuery{
		EmployeeID: strings.TrimSpace(values.Get("employee_id")),
		FromDate:   strings.TrimSpace(values.Get("from_date")),
		ToDate:     strings.TrimSpace(values.Get("to_date")),
		Status:     strings.TrimSpace(values.Get("status")),
		Direction:  strings.TrimSpace(values.Get("direction")),
	}
}

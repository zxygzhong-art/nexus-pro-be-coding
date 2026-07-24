package v1

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
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
	attendance.GET("/policies/current", c.routes.Handle("attendance.leave", "read", c.currentPolicy))
	attendance.POST("/policies/validate", c.routes.Handle("attendance.leave", "update", c.validatePolicy))
	attendance.POST("/policies/publish", c.routes.Handle("attendance.leave", "update", c.publishPolicy))
	attendance.GET("/leave-types", c.routes.Handle("attendance.leave", "read", c.listLeaveTypes))
	attendance.PATCH("/leave-types/:id", c.routes.Handle("attendance.leave", "update", c.setLeaveTypeEnabled, ResourceID(PathParamID)))
	attendance.GET("/worksites", c.routes.Handle("attendance.worksite", "read", c.listWorksites))
	attendance.POST("/worksites", c.routes.Handle("attendance.worksite", "create", c.createWorksite))
	attendance.PATCH("/worksites", c.routes.Handle("attendance.worksite", "update", c.updateWorksite))
	attendance.GET("/clock-status", c.routes.Handle("attendance.clock", "read", c.clockStatus))
	attendance.GET("/monthly-summary", c.routes.Handle("attendance.clock", "read", c.monthlySummary))
	attendance.GET("/clock-records", c.routes.Handle("attendance.clock", "read", c.listClockRecords))
	attendance.POST("/clock-records", c.routes.Handle("attendance.clock", "create", c.createClockRecord))
	attendance.POST("/ehrms/leave-types/sync", c.routes.Handle("attendance.clock", "import", c.syncEHRMSLeaveTypes))
	attendance.POST("/ehrms/sync", c.routes.Handle("attendance.clock", "import", c.syncEHRMSAttendance))
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
	items, err := c.svc.ListLeaveBalancePageByQuery(ctx, leaveBalanceQueryFromRequest(r), page)
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
	items, err := c.svc.ListLeaveRequestPageByQuery(ctx, leaveRequestQueryFromRequest(r), page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
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

// validatePolicy checks a local draft without changing the published version.
func (c AttendanceCtrl) validatePolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAttendancePolicyInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.ValidateAttendancePolicy(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// publishPolicy advances the immutable policy version after validation.
func (c AttendanceCtrl) publishPolicy(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateAttendancePolicyInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.PublishAttendancePolicy(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listLeaveTypes returns the tenant leave_types catalog.
func (c AttendanceCtrl) listLeaveTypes(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	catalog, err := c.svc.ListLeaveTypes(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, catalog)
	return nil
}

// setLeaveTypeEnabled updates leave_types.status for one leave type.
func (c AttendanceCtrl) setLeaveTypeEnabled(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.SetLeaveTypeEnabledInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.SetLeaveTypeEnabled(ctx, r.PathValue(PathParamID), input)
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

// clockStatus 處理打卡狀態的 HTTP 請求。
func (c AttendanceCtrl) clockStatus(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.AttendanceClockStatus(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// monthlySummary returns the current employee's authoritative projection for a selected month.
func (c AttendanceCtrl) monthlySummary(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.AttendanceMonthlySummary(ctx, strings.TrimSpace(r.URL.Query().Get("month")))
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

// syncEHRMSLeaveTypes synchronizes only the EHRMS leave type catalog.
func (c AttendanceCtrl) syncEHRMSLeaveTypes(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.SyncEHRMSLeaveTypes(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// syncEHRMSAttendance 處理 eHRMS 考勤同步的 HTTP 請求。
func (c AttendanceCtrl) syncEHRMSAttendance(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.EHRMSAttendanceSyncInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	defaultManualEHRMSAttendanceSyncRange(&input, time.Now())
	item, err := c.svc.SyncEHRMSAttendance(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func defaultManualEHRMSAttendanceSyncRange(input *domain.EHRMSAttendanceSyncInput, now time.Time) {
	if strings.TrimSpace(input.Start) != "" || strings.TrimSpace(input.End) != "" {
		return
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := now.In(location)
	input.Start = start.Format(time.DateOnly)
	input.End = start.AddDate(0, 0, 1).Format(time.DateOnly)
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

// leaveBalanceQueryFromRequest parses employee-scoped balance filters.
func leaveBalanceQueryFromRequest(r *http.Request) domain.LeaveBalanceQuery {
	employeeID := strings.TrimSpace(r.URL.Query().Get("employee_id"))
	if employeeID == "" {
		return domain.LeaveBalanceQuery{}
	}
	return domain.LeaveBalanceQuery{EmployeeIDs: []string{employeeID}}
}

// leaveRequestQueryFromRequest parses leave-history filters before service-level authorization.
func leaveRequestQueryFromRequest(r *http.Request) domain.LeaveRequestQuery {
	values := r.URL.Query()
	employeeID := strings.TrimSpace(values.Get("employee_id"))
	query := domain.LeaveRequestQuery{
		Status:   strings.TrimSpace(values.Get("status")),
		FromDate: strings.TrimSpace(values.Get("from_date")),
		ToDate:   strings.TrimSpace(values.Get("to_date")),
	}
	if employeeID != "" {
		query.EmployeeIDs = []string{employeeID}
	}
	return query
}

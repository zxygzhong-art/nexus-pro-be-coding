package v1

import (
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// HRCtrl 定義 HR ctrl 的資料結構。
type HRCtrl struct {
	routes routeBinder
	svc    service.HRFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c HRCtrl) RegisterRoutes(router *gin.RouterGroup) {
	hr := router.Group("/hr")

	positions := hr.Group("/positions")
	positions.GET("", c.routes.Handle("hr.position", "read", c.listPositions))

	employees := hr.Group("/employees")
	employees.GET("", c.routes.Handle("hr.employee", "read", c.listEmployees))
	employees.GET("/stats", c.routes.Handle("hr.employee", "read", c.employeeStats))
	employees.POST("/ehrms/sync", c.routes.Handle("hr.employee", "import", c.syncEHRMSEmployees))
	employees.GET("/export", c.routes.Handle("hr.employee", "export", c.exportEmployeesCSV))
	employees.GET("/:id", c.routes.Handle("hr.employee", "read", c.getEmployee, TargetEmployeeID(PathParamID)))
	employees.PATCH("/:id", c.routes.Handle("hr.employee", "update", c.updateEmployee, TargetEmployeeID(PathParamID)))

	org := router.Group("/org")
	org.GET("/units", c.routes.Handle("hr.org_unit", "read", c.listOrgUnits))
	org.POST("/units", c.routes.Handle("hr.org_unit", "create", c.createOrgUnit))
	org.POST("/units/ehrms/sync", c.routes.Handle("hr.org_unit", "create", c.syncEHRMSOrgUnits))
	org.PATCH("/units/:id", c.routes.Handle("hr.org_unit", "update", c.updateOrgUnit, ResourceID(PathParamID)))
}

// listEmployees 處理員工的 HTTP 請求。
func (c HRCtrl) listEmployees(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := employeeQueryFromRequest(r)
	if err != nil {
		return err
	}
	response, err := c.svc.QueryEmployees(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

// getEmployee 處理員工的 HTTP 請求。
func (c HRCtrl) getEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetEmployee(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.EmployeeDetailFromEmployee(item))
	return nil
}

// updateEmployee 處理員工的 HTTP 請求。
func (c HRCtrl) updateEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateEmployeeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	if err := input.ValidateBasicInfoOnly(); err != nil {
		return err
	}
	item, err := c.svc.UpdateEmployee(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.EmployeeDetailFromEmployee(item))
	return nil
}

// employeeStats 處理員工 stats 的 HTTP 請求。
func (c HRCtrl) employeeStats(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := employeeQueryFromRequest(r)
	if err != nil {
		return err
	}
	stats, err := c.svc.EmployeeStats(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, stats)
	return nil
}

// listPositions 處理崗位列表的 HTTP 請求。
func (c HRCtrl) listPositions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListPositionPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// syncEHRMSEmployees 處理 eHRMS 員工的 HTTP 請求。
func (c HRCtrl) syncEHRMSEmployees(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.EHRMSEmployeeSyncInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.SyncEHRMSEmployees(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// syncEHRMSOrgUnits 處理 eHRMS 組織單位同步的 HTTP 請求。
func (c HRCtrl) syncEHRMSOrgUnits(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	result, err := c.svc.SyncEHRMSOrgUnits(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// exportEmployeesCSV 處理員工 CSV 的 HTTP 請求。
func (c HRCtrl) exportEmployeesCSV(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := employeeQueryFromRequest(r)
	if err != nil {
		return err
	}
	raw, filename, err := c.svc.ExportEmployeesCSV(ctx, query)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
	return nil
}

// listOrgUnits 處理組織單位的 HTTP 請求。
func (c HRCtrl) listOrgUnits(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := orgUnitQueryFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListOrgUnitPage(ctx, query)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// orgUnitQueryFromRequest 解析組織單位列表查詢參數。
func orgUnitQueryFromRequest(r *http.Request) (domain.OrgUnitQuery, error) {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return domain.OrgUnitQuery{}, err
	}
	return domain.OrgUnitQuery{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Page:     page.Page,
		PageSize: page.PageSize,
		Sort:     page.Sort,
	}, nil
}

// createOrgUnit 處理組織單位的 HTTP 請求。
func (c HRCtrl) createOrgUnit(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateOrgUnitInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateOrgUnit(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateOrgUnit 處理更新組織單位的 HTTP 請求。
func (c HRCtrl) updateOrgUnit(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateOrgUnitInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateOrgUnit(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// employeeQueryFromRequest 處理員工查詢 來源 請求。
func employeeQueryFromRequest(r *http.Request) (domain.EmployeeQuery, error) {
	values := r.URL.Query()
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return domain.EmployeeQuery{}, err
	}
	return domain.EmployeeQuery{
		Keyword:          strings.TrimSpace(values.Get("keyword")),
		DepartmentID:     strings.TrimSpace(values.Get("department_id")),
		EmploymentStatus: strings.TrimSpace(values.Get("employment_status")),
		Category:         strings.TrimSpace(values.Get("category")),
		Page:             page.Page,
		PageSize:         page.PageSize,
		Sort:             page.Sort,
	}, nil
}

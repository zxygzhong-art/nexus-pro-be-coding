package v1

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

const (
	// 員工匯入與頭像 endpoint 保持嚴格的 request size 限制。
	employeeImportMultipartMaxBytes = 16 << 20
	employeeImportFileMaxBytes      = 10 << 20
	employeeAvatarMultipartMaxBytes = 4 << 20
	employeeAvatarFileMaxBytes      = 3 << 20
)

// HRCtrl 定義 HR ctrl 的資料結構。
type HRCtrl struct {
	routes routeBinder
	svc    service.HRFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c HRCtrl) RegisterRoutes(router *gin.RouterGroup) {
	hr := router.Group("/hr")
	hr.GET("/employee-options", c.routes.Handle("hr.employee", "read", c.employeeOptions))

	positions := hr.Group("/positions")
	positions.GET("", c.routes.Handle("hr.position", "read", c.listPositions))
	positions.POST("", c.routes.Handle("hr.position", "create", c.createPosition))
	positions.POST("/ehrms/sync", c.routes.Handle("hr.position", "create", c.syncEHRMSPositions))
	positions.GET("/:id", c.routes.Handle("hr.position", "read", c.getPosition, ResourceID(PathParamID)))
	positions.PATCH("/:id", c.routes.Handle("hr.position", "update", c.updatePosition, ResourceID(PathParamID)))
	positions.DELETE("/:id", c.routes.Handle("hr.position", "delete", c.deletePosition, ResourceID(PathParamID)))

	employees := hr.Group("/employees")
	employees.GET("", c.routes.Handle("hr.employee", "read", c.listEmployees))
	employees.POST("", c.routes.Handle("hr.employee", "create", c.createEmployee))
	employees.POST("/preview", c.routes.Handle("hr.employee", "create", c.previewCreateEmployee))
	employees.GET("/stats", c.routes.Handle("hr.employee", "read", c.employeeStats))
	employees.GET("/import/template", c.routes.Handle("hr.employee", "read", c.employeeImportTemplate))
	employees.POST("/import/preview", c.routes.Handle("hr.employee", "import", c.previewEmployeeImport))
	employees.POST("/import/:id/confirm", c.routes.Handle("hr.employee", "import", c.confirmEmployeeImport, ResourceID(PathParamID)))
	employees.POST("/ehrms/sync", c.routes.Handle("hr.employee", "import", c.syncEHRMSEmployees))
	employees.GET("/export", c.routes.Handle("hr.employee", "export", c.exportEmployeesCSV))
	employees.POST("/export", c.routes.Handle("hr.employee", "export", c.exportEmployees))
	employees.POST("/batch-delete", c.routes.Handle("hr.employee", "delete", c.batchDeleteEmployees))
	employees.GET("/:id/contracts", c.routes.Handle("hr.employment_contract", "read", c.listEmploymentContractsByEmployee, ResourceID(PathParamID), TargetEmployeeID(PathParamID)))
	employees.POST("/:id/contracts", c.routes.Handle("hr.employment_contract", "create", c.createEmploymentContract, ResourceID(PathParamID), TargetEmployeeID(PathParamID)))
	employees.GET("/:id", c.routes.Handle("hr.employee", "read", c.getEmployee, TargetEmployeeID(PathParamID)))
	employees.PATCH("/:id", c.routes.Handle("hr.employee", "update", c.updateEmployee, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/preview", c.routes.Handle("hr.employee", "update", c.previewUpdateEmployee, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/avatar", c.routes.Handle("hr.employee", "update", c.updateEmployeeAvatar, TargetEmployeeID(PathParamID)))
	employees.DELETE("/:id/avatar", c.routes.Handle("hr.employee", "update", c.deleteEmployeeAvatar, TargetEmployeeID(PathParamID)))
	employees.DELETE("/:id", c.routes.Handle("hr.employee", "delete", c.deleteEmployee, TargetEmployeeID(PathParamID)))
	employees.PATCH("/:id/status", c.routes.Handle("hr.employee", "update_status", c.updateEmployeeStatus, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/invite", c.routes.Handle("hr.employee", "invite", c.inviteEmployee, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/status-transition", c.routes.Handle("hr.employee", "status_transition", c.transitionEmployeeStatus, TargetEmployeeID(PathParamID)))

	contracts := hr.Group("/contracts")
	contracts.GET("/:id", c.routes.Handle("hr.employment_contract", "read", c.getEmploymentContract, ResourceID(PathParamID)))
	contracts.PATCH("/:id", c.routes.Handle("hr.employment_contract", "update", c.updateEmploymentContract, ResourceID(PathParamID)))
	contracts.DELETE("/:id", c.routes.Handle("hr.employment_contract", "delete", c.deleteEmploymentContract, ResourceID(PathParamID)))

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

// createEmployee 處理員工的 HTTP 請求。
func (c HRCtrl) createEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateEmployeeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateEmployee(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, domain.EmployeeDetailFromEmployee(item))
	return nil
}

// previewCreateEmployee 處理 create 員工的 HTTP 請求。
func (c HRCtrl) previewCreateEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateEmployeeInput
	if err := readJSONNoValidate(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.PreviewCreateEmployee(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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

// previewUpdateEmployee 處理 update 員工的 HTTP 請求。
func (c HRCtrl) previewUpdateEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateEmployeeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	if err := input.ValidateBasicInfoOnly(); err != nil {
		return err
	}
	item, err := c.svc.PreviewUpdateEmployee(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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

// employeeOptions 處理員工選項的 HTTP 請求。
func (c HRCtrl) employeeOptions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	options, err := c.svc.EmployeeOptions(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, options)
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

// createPosition 處理建立崗位的 HTTP 請求。
func (c HRCtrl) createPosition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreatePositionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreatePosition(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// getPosition 處理取得崗位的 HTTP 請求。
func (c HRCtrl) getPosition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetPosition(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updatePosition 處理更新崗位的 HTTP 請求。
func (c HRCtrl) updatePosition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdatePositionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdatePosition(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deletePosition 處理刪除崗位的 HTTP 請求。
func (c HRCtrl) deletePosition(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeletePosition(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// employeeImportTemplate 處理員工 import 範本的 HTTP 請求。
func (c HRCtrl) employeeImportTemplate(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	raw, filename, contentType, err := c.svc.EmployeeImportTemplate(ctx, r.URL.Query().Get("format"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
	return nil
}

// previewEmployeeImport 處理員工 import 的 HTTP 請求。
func (c HRCtrl) previewEmployeeImport(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	input, err := employeeImportPreviewInput(w, r)
	if err != nil {
		return err
	}
	session, err := c.svc.PreviewEmployeeImport(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, session)
	return nil
}

// confirmEmployeeImport 處理員工 import 的 HTTP 請求。
func (c HRCtrl) confirmEmployeeImport(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.EmployeeImportConfirmInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	session, err := c.svc.ConfirmEmployeeImport(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, session)
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

// syncEHRMSPositions 處理 eHRMS 崗位同步的 HTTP 請求。
func (c HRCtrl) syncEHRMSPositions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	result, err := c.svc.SyncEHRMSPositions(ctx)
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

// exportEmployees 處理員工的 HTTP 請求。
func (c HRCtrl) exportEmployees(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	query, err := employeeQueryFromRequest(r)
	if err != nil {
		return err
	}
	var bodyQuery domain.EmployeeQuery
	if ok, err := readOptionalJSON(w, r, &bodyQuery); err != nil {
		return err
	} else if ok {
		query = bodyQuery
	}
	items, err := c.svc.ExportEmployees(ctx, query)
	if err != nil {
		return err
	}
	page := pageResponseRequest(query.Page, query.PageSize, query.Sort)
	writeJSON(w, http.StatusOK, domain.PageResponse[domain.Employee]{Items: items, Total: len(items), Page: page.Page, PageSize: page.PageSize, Sort: page.Sort})
	return nil
}

// batchDeleteEmployees 處理批次 delete 員工的 HTTP 請求。
func (c HRCtrl) batchDeleteEmployees(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.BatchDeleteEmployeesInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.BatchDeleteEmployees(ctx, input)
	if err != nil {
		return err
	}
	status := http.StatusOK
	for _, item := range result.Results {
		if !item.Success {
			status = http.StatusMultiStatus
			break
		}
	}
	writeJSON(w, status, result)
	return nil
}

// deleteEmployee 處理員工的 HTTP 請求。
func (c HRCtrl) deleteEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteEmployee(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateEmployeeAvatar 處理員工 avatar 的 HTTP 請求。
func (c HRCtrl) updateEmployeeAvatar(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	input, err := employeeAvatarInput(w, r)
	if err != nil {
		return err
	}
	item, err := c.svc.UpdateEmployeeAvatar(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteEmployeeAvatar 處理員工 avatar 的 HTTP 請求。
func (c HRCtrl) deleteEmployeeAvatar(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteEmployeeAvatar(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// inviteEmployee 處理員工的 HTTP 請求。
func (c HRCtrl) inviteEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.InviteEmployeeInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.InviteEmployee(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// transitionEmployeeStatus 處理員工狀態的 HTTP 請求。
func (c HRCtrl) transitionEmployeeStatus(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.StatusTransitionInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.TransitionEmployeeStatus(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateEmployeeStatus 處理員工狀態的 HTTP 請求。
func (c HRCtrl) updateEmployeeStatus(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateEmployeeStatusInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateEmployeeStatus(ctx, r.PathValue(PathParamID), input.Status)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listEmploymentContractsByEmployee 處理員工合約列表的 HTTP 請求。
func (c HRCtrl) listEmploymentContractsByEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListEmploymentContractsByEmployee(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createEmploymentContract 處理建立員工合約的 HTTP 請求。
func (c HRCtrl) createEmploymentContract(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateEmploymentContractInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateEmploymentContract(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// getEmploymentContract 處理取得員工合約的 HTTP 請求。
func (c HRCtrl) getEmploymentContract(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetEmploymentContract(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateEmploymentContract 處理更新員工合約的 HTTP 請求。
func (c HRCtrl) updateEmploymentContract(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateEmploymentContractInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateEmploymentContract(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteEmploymentContract 處理刪除員工合約的 HTTP 請求。
func (c HRCtrl) deleteEmploymentContract(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteEmploymentContract(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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

// employeeImportPreviewInput 處理員工 import preview 輸入。
func employeeImportPreviewInput(w http.ResponseWriter, r *http.Request) (domain.EmployeeImportPreviewInput, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, employeeImportMultipartMaxBytes)
		if err := r.ParseMultipartForm(employeeImportMultipartMaxBytes); err != nil {
			return domain.EmployeeImportPreviewInput{}, domain.BadRequestCode(domain.ErrorCodeInvalidMultipartForm, "invalid multipart form: "+err.Error())
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return domain.EmployeeImportPreviewInput{}, domain.BadRequestCode(domain.ErrorCodeRequiredMultipartFile, "file is required")
		}
		defer file.Close()
		raw, err := io.ReadAll(http.MaxBytesReader(w, file, employeeImportFileMaxBytes))
		if err != nil {
			return domain.EmployeeImportPreviewInput{}, domain.BadRequestCode(domain.ErrorCodeMultipartFileReadFailed, "read import file: "+err.Error())
		}
		filename := strings.TrimSpace(r.FormValue("filename"))
		if filename == "" && header != nil {
			filename = header.Filename
		}
		return domain.EmployeeImportPreviewInput{Filename: filename, Content: string(raw)}, nil
	}
	var input domain.EmployeeImportPreviewInput
	if err := readJSON(w, r, &input); err != nil {
		return domain.EmployeeImportPreviewInput{}, err
	}
	return input, nil
}

// employeeAvatarInput 處理員工 avatar 輸入。
func employeeAvatarInput(w http.ResponseWriter, r *http.Request) (domain.EmployeeAvatarInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, employeeAvatarMultipartMaxBytes)
	if err := r.ParseMultipartForm(employeeAvatarMultipartMaxBytes); err != nil {
		return domain.EmployeeAvatarInput{}, domain.BadRequestCode(domain.ErrorCodeInvalidMultipartForm, "invalid multipart form: "+err.Error())
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return domain.EmployeeAvatarInput{}, domain.BadRequestCode(domain.ErrorCodeRequiredMultipartFile, "file is required")
	}
	defer file.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, file, employeeAvatarFileMaxBytes))
	if err != nil {
		return domain.EmployeeAvatarInput{}, domain.BadRequestCode(domain.ErrorCodeMultipartFileReadFailed, "read avatar file: "+err.Error())
	}
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" && len(raw) > 0 {
		contentType = http.DetectContentType(raw)
	}
	return domain.EmployeeAvatarInput{Filename: header.Filename, ContentType: contentType, Content: raw}, nil
}

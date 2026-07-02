package v1

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

const (
	// Employee import and avatar endpoints keep strict request size limits.
	employeeImportMultipartMaxBytes = 16 << 20
	employeeImportFileMaxBytes      = 10 << 20
	employeeAvatarMultipartMaxBytes = 4 << 20
	employeeAvatarFileMaxBytes      = 3 << 20
)

// HRCtrl wires people-domain and organization endpoints to the HR service facade.
type HRCtrl struct {
	routes routeBinder
	svc    service.HRFacade
}

// RegisterRoutes attaches people-domain and organization routes to the v1 route group.
func (c HRCtrl) RegisterRoutes(router *gin.RouterGroup) {
	hr := router.Group("/hr")
	hr.GET("/employee-options", c.routes.Handle("hr.employee", "read", c.employeeOptions))

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
	employees.GET("/:id", c.routes.Handle("hr.employee", "read", c.getEmployee, TargetEmployeeID(PathParamID)))
	employees.PATCH("/:id", c.routes.Handle("hr.employee", "update", c.updateEmployee, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/preview", c.routes.Handle("hr.employee", "update", c.previewUpdateEmployee, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/avatar", c.routes.Handle("hr.employee", "update", c.updateEmployeeAvatar, TargetEmployeeID(PathParamID)))
	employees.DELETE("/:id/avatar", c.routes.Handle("hr.employee", "update", c.deleteEmployeeAvatar, TargetEmployeeID(PathParamID)))
	employees.DELETE("/:id", c.routes.Handle("hr.employee", "delete", c.deleteEmployee, TargetEmployeeID(PathParamID)))
	employees.PATCH("/:id/status", c.routes.Handle("hr.employee", "update_status", c.updateEmployeeStatus, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/invite", c.routes.Handle("hr.employee", "invite", c.inviteEmployee, TargetEmployeeID(PathParamID)))
	employees.POST("/:id/status-transition", c.routes.Handle("hr.employee", "status_transition", c.transitionEmployeeStatus, TargetEmployeeID(PathParamID)))

	org := router.Group("/org")
	org.GET("/units", c.routes.Handle("hr.org_unit", "read", c.listOrgUnits))
	org.POST("/units", c.routes.Handle("hr.org_unit", "create", c.createOrgUnit))
}

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

func (c HRCtrl) getEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetEmployee(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.EmployeeDetailFromEmployee(item))
	return nil
}

func (c HRCtrl) updateEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateEmployeeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateEmployee(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.EmployeeDetailFromEmployee(item))
	return nil
}

func (c HRCtrl) previewUpdateEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateEmployeeInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.PreviewUpdateEmployee(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

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

func (c HRCtrl) employeeOptions(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	options, err := c.svc.EmployeeOptions(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, options)
	return nil
}

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

func (c HRCtrl) deleteEmployee(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteEmployee(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

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

func (c HRCtrl) deleteEmployeeAvatar(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteEmployeeAvatar(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

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

func (c HRCtrl) listOrgUnits(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListOrgUnitPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

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

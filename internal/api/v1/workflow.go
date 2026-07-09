package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// WorkflowCtrl 定義流程 ctrl 的資料結構。
type WorkflowCtrl struct {
	routes routeBinder
	svc    service.WorkflowFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c WorkflowCtrl) RegisterRoutes(router *gin.RouterGroup) {
	forms := router.Group("/forms")
	forms.GET("/templates", c.routes.Handle("workflow.form_template", "read", c.listFormTemplates))
	forms.POST("/templates", c.routes.Handle("workflow.form_template", "create", c.createFormTemplate))

	workflows := router.Group("/workflows")
	workflows.GET("/forms", c.routes.Handle("workflow.form_instance", "read", c.listFormInstances))
	workflows.GET("/reviews", c.routes.Handle("workflow.form_instance", "read", c.reviewQueue))
	workflows.POST("/reviews/bulk-action", c.routes.Handle("workflow.form_instance", "update", c.bulkReviewForms))
	workflows.POST("/forms/drafts", c.routes.Handle("workflow.form_instance", "submit", c.saveFormDraft))
	workflows.GET("/forms/:id/workflow", c.routes.Handle("workflow.form_instance", "read", c.getWorkflowFormState, PathParam(PathParamID)))
	workflows.GET("/forms/:id/export", c.routes.Handle("workflow.form_instance", "read", c.exportForm, PathParam(PathParamID)))
	workflows.PATCH("/forms/:id", c.routes.Handle("workflow.form_instance", "update", c.updateFormDraft, PathParam(PathParamID)))
	workflows.DELETE("/forms/:id", c.routes.Handle("workflow.form_instance", "delete", c.deleteFormDraft, PathParam(PathParamID)))
	workflows.POST("/forms/:id/submit", c.routes.Handle("workflow.form_instance", "submit", c.submitForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/approve", c.routes.Handle("workflow.form_instance", "approve", c.approveForm, ResourceID(PathParamID)))
	workflows.POST("/forms/:id/reject", c.routes.Handle("workflow.form_instance", "update", c.rejectForm, ResourceID(PathParamID)))
	workflows.POST("/forms/:id/return", c.routes.Handle("workflow.form_instance", "update", c.returnForm, ResourceID(PathParamID)))
	workflows.POST("/forms/:id/cancel", c.routes.Handle("workflow.form_instance", "update", c.cancelForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/duplicate", c.routes.Handle("workflow.form_instance", "submit", c.duplicateForm, PathParam(PathParamID)))
}

// listFormTemplates 處理表單範本的 HTTP 請求。
func (c WorkflowCtrl) listFormTemplates(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListFormTemplatePage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createFormTemplate 處理表單範本的 HTTP 請求。
func (c WorkflowCtrl) createFormTemplate(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateFormTemplateInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateFormTemplate(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// listFormInstances 處理表單實例的 HTTP 請求。
func (c WorkflowCtrl) listFormInstances(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	query, err := formInstanceQueryFromRequest(r)
	if err != nil {
		return err
	}
	items, err := c.svc.ListFormInstancePage(ctx, query, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// reviewQueue 處理審核佇列的 HTTP 請求。
func (c WorkflowCtrl) reviewQueue(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ReviewQueue(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// bulkReviewForms 處理批次審核表單的 HTTP 請求。
func (c WorkflowCtrl) bulkReviewForms(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.BulkReviewFormsInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.BulkReviewForms(ctx, input)
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

// saveFormDraft 處理表單草稿的 HTTP 請求。
func (c WorkflowCtrl) saveFormDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.SaveFormDraftInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.SaveFormDraft(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateFormDraft 處理表單草稿的 HTTP 請求。
func (c WorkflowCtrl) updateFormDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateFormDraftInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateFormDraft(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteFormDraft 處理表單草稿的 HTTP 請求。
func (c WorkflowCtrl) deleteFormDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteFormDraft(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// submitForm 處理表單的 HTTP 請求。
func (c WorkflowCtrl) submitForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.SubmitFormInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	if pathID := strings.TrimSpace(r.PathValue(PathParamID)); pathID != "" {
		input.TemplateKey = pathID
	}
	item, err := c.svc.SubmitForm(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// approveForm 處理表單的 HTTP 請求。
func (c WorkflowCtrl) approveForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.ApproveFormInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.ApproveForm(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// rejectForm 處理表單的 HTTP 請求。
func (c WorkflowCtrl) rejectForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.RejectFormInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.RejectForm(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// returnForm 處理表單的 HTTP 請求。
func (c WorkflowCtrl) returnForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.ReturnFormInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.ReturnForm(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// cancelForm 處理表單的 HTTP 請求。
func (c WorkflowCtrl) cancelForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CancelFormInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CancelForm(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// duplicateForm 處理 duplicate 表單的 HTTP 請求。
func (c WorkflowCtrl) duplicateForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DuplicateForm(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// getWorkflowFormState 回傳單據流程運行狀態。
func (c WorkflowCtrl) getWorkflowFormState(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	state, err := c.svc.GetWorkflowFormState(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, state)
	return nil
}

// exportForm 處理表單的 HTTP 請求。
func (c WorkflowCtrl) exportForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	file, err := c.svc.ExportForm(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+file.FileName+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Body)
	return nil
}

// formInstanceQueryFromRequest 處理表單實例查詢 來源 請求。
func formInstanceQueryFromRequest(r *http.Request) (domain.FormInstanceQuery, error) {
	values := r.URL.Query()
	mine, err := optionalBoolQuery(values.Get("mine"), "mine")
	if err != nil {
		return domain.FormInstanceQuery{}, err
	}
	return domain.FormInstanceQuery{
		Status:             values.Get("status"),
		TemplateID:         values.Get("template_id"),
		TemplateKey:        values.Get("template_key"),
		ApplicantAccountID: values.Get("applicant_account_id"),
		Mine:               mine,
	}, nil
}

// optionalBoolQuery 處理可選布林值查詢。
func optionalBoolQuery(raw, name string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	if strings.EqualFold(raw, "true") || raw == "1" {
		return true, nil
	}
	if strings.EqualFold(raw, "false") || raw == "0" {
		return false, nil
	}
	return false, domain.BadRequest(name + " must be true or false")
}

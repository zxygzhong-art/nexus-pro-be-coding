package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// WorkflowCtrl wires form template and form instance endpoints to the workflow service facade.
type WorkflowCtrl struct {
	routes routeBinder
	svc    service.WorkflowFacade
}

// RegisterRoutes attaches workflow form routes to the v1 route group.
func (c WorkflowCtrl) RegisterRoutes(router *gin.RouterGroup) {
	forms := router.Group("/forms")
	forms.GET("/templates", c.routes.Handle("workflow.form_template", "read", c.listFormTemplates))
	forms.POST("/templates", c.routes.Handle("workflow.form_template", "create", c.createFormTemplate))

	workflows := router.Group("/workflows")
	workflows.GET("/forms", c.routes.Handle("workflow.form_instance", "read", c.listFormInstances))
	workflows.GET("/reviews", c.routes.Handle("workflow.form_instance", "read", c.reviewQueue))
	workflows.POST("/reviews/bulk-action", c.routes.Handle("workflow.form_instance", "update", c.bulkReviewForms))
	workflows.POST("/forms/drafts", c.routes.Handle("workflow.form_instance", "submit", c.saveFormDraft))
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

func (c WorkflowCtrl) reviewQueue(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ReviewQueue(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

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

// saveFormDraft creates a resumable workflow form draft.
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

// updateFormDraft persists the latest payload for an existing draft.
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

// deleteFormDraft removes a draft form instance.
func (c WorkflowCtrl) deleteFormDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteFormDraft(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (c WorkflowCtrl) submitForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.SubmitFormInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	if input.TemplateKey == "" {
		input.TemplateKey = r.PathValue(PathParamID)
	}
	item, err := c.svc.SubmitForm(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

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

// returnForm sends a workflow form instance back to the applicant for revision.
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

// cancelForm marks a visible submitted form instance as cancelled.
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

// duplicateForm creates a draft copy from an existing form instance.
func (c WorkflowCtrl) duplicateForm(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DuplicateForm(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// exportForm streams a JSON representation of one visible form instance.
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

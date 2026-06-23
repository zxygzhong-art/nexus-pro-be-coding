package v1

import (
	"net/http"

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
	workflows.POST("/forms/:id/submit", c.routes.Handle("workflow.form_instance", "submit", c.submitForm, ResourceID(PathParamID)))
	workflows.POST("/forms/:id/approve", c.routes.Handle("workflow.form_instance", "approve", c.approveForm, ResourceID(PathParamID)))
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

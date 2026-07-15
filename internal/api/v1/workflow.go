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
	builder := router.Group("/form-builder")
	builder.GET("/capabilities", c.routes.Handle("workflow.form_definition_draft", "read", c.formBuilderCapabilities))
	builder.GET("/data-sources", c.routes.Handle("workflow.form_definition_draft", "read", c.formBuilderCapabilities))
	builder.GET("/workflow-targets", c.routes.Handle("workflow.form_definition_draft", "read", c.formBuilderCapabilities))
	builder.GET("/drafts", c.routes.Handle("workflow.form_definition_draft", "read", c.listFormDefinitionDrafts))
	builder.POST("/drafts", c.routes.Handle("workflow.form_definition_draft", "create", c.createFormDefinitionDraft))
	builder.GET("/drafts/:id", c.routes.Handle("workflow.form_definition_draft", "read", c.getFormDefinitionDraft, ResourceID(PathParamID)))
	builder.PATCH("/drafts/:id", c.routes.Handle("workflow.form_definition_draft", "update", c.updateFormDefinitionDraft, ResourceID(PathParamID)))
	builder.POST("/drafts/:id/validate", c.routes.Handle("workflow.form_definition_draft", "read", c.validateFormDefinitionDraft, ResourceID(PathParamID)))
	builder.POST("/drafts/:id/preview", c.routes.Handle("workflow.form_definition_draft", "read", c.previewFormDefinitionDraft, ResourceID(PathParamID)))
	builder.POST("/drafts/:id/simulate", c.routes.Handle("workflow.form_definition_draft", "read", c.simulateFormDefinitionWorkflow, ResourceID(PathParamID)))
	builder.POST("/drafts/:id/submit-review", c.routes.Handle("workflow.form_definition_draft", "submit", c.submitFormDefinitionDraftForReview, ResourceID(PathParamID)))
	builder.POST("/drafts/:id/publish", c.routes.Handle("workflow.form_definition_draft", "approve", c.publishFormDefinitionDraft, ResourceID(PathParamID)))

	forms := router.Group("/forms")
	forms.GET("/templates", c.routes.Handle("workflow.form_template", "read", c.listFormTemplates))
	forms.POST("/templates", c.routes.Handle("workflow.form_template", "create", c.createFormTemplate))

	workflows := router.Group("/workflows")
	workflows.GET("/form-data-sources", c.routes.Handle("workflow.form_instance", "read", c.formDataSources))
	workflows.GET("/form-templates/:key", c.routes.Handle("workflow.form_instance", "read", c.getRuntimeFormTemplate, PathParam("key")))
	workflows.GET("/forms", c.routes.Handle("workflow.form_instance", "read", c.listFormInstances))
	workflows.GET("/forms/:id", c.routes.Handle("workflow.form_instance", "read", c.getFormInstance, PathParam(PathParamID)))
	workflows.GET("/reviews", c.routes.Handle("workflow.form_instance", "read", c.reviewQueue))
	workflows.POST("/reviews/bulk-action", c.routes.Handle("workflow.form_instance", "read", c.bulkReviewForms))
	workflows.POST("/forms/drafts", c.routes.Handle("workflow.form_instance", "submit", c.saveFormDraft))
	workflows.GET("/forms/:id/workflow", c.routes.Handle("workflow.form_instance", "read", c.getWorkflowFormState, PathParam(PathParamID)))
	workflows.GET("/forms/:id/export", c.routes.Handle("workflow.form_instance", "read", c.exportForm, PathParam(PathParamID)))
	workflows.PATCH("/forms/:id", c.routes.Handle("workflow.form_instance", "update", c.updateFormDraft, PathParam(PathParamID)))
	workflows.DELETE("/forms/:id", c.routes.Handle("workflow.form_instance", "delete", c.deleteFormDraft, PathParam(PathParamID)))
	workflows.POST("/forms/:id/submit", c.routes.Handle("workflow.form_instance", "submit", c.submitForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/approve", c.routes.Handle("workflow.form_instance", "read", c.approveForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/reject", c.routes.Handle("workflow.form_instance", "read", c.rejectForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/return", c.routes.Handle("workflow.form_instance", "read", c.returnForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/cancel", c.routes.Handle("workflow.form_instance", "update", c.cancelForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/duplicate", c.routes.Handle("workflow.form_instance", "submit", c.duplicateForm, PathParam(PathParamID)))
}

// formBuilderCapabilities 回傳 Agent 創作所需的統一能力目錄。
func (c WorkflowCtrl) formBuilderCapabilities(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.FormBuilderCapabilities(ctx)
	if err != nil {
		return err
	}
	path := r.URL.Path
	if strings.HasSuffix(path, "/data-sources") {
		item.WorkflowTargets = nil
		item.FieldTypes = nil
		item.Widgets = nil
	}
	if strings.HasSuffix(path, "/workflow-targets") {
		item.DataSources = nil
		item.FieldTypes = nil
		item.Widgets = nil
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listFormDefinitionDrafts 列出 Agent/管理員可見的表單定義草稿。
func (c WorkflowCtrl) listFormDefinitionDrafts(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	owner := ""
	if r.URL.Query().Get("owner") == "mine" {
		owner = ctx.AccountID
	}
	items, err := c.svc.ListFormDefinitionDrafts(ctx, owner, strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

// createFormDefinitionDraft 建立受控表單定義草稿。
func (c WorkflowCtrl) createFormDefinitionDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateFormDefinitionDraftInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateFormDefinitionDraft(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// getFormDefinitionDraft 取得單個草稿。
func (c WorkflowCtrl) getFormDefinitionDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetFormDefinitionDraft(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// updateFormDefinitionDraft 更新草稿並執行 revision 檢查。
func (c WorkflowCtrl) updateFormDefinitionDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateFormDefinitionDraftInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateFormDefinitionDraft(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// validateFormDefinitionDraft 執行確定性驗證。
func (c WorkflowCtrl) validateFormDefinitionDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.ValidateFormDefinitionDraft(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// previewFormDefinitionDraft 回傳預覽 contract。
func (c WorkflowCtrl) previewFormDefinitionDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.PreviewFormDefinitionDraft(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// simulateFormDefinitionWorkflow 靜態模擬審批路徑。
func (c WorkflowCtrl) simulateFormDefinitionWorkflow(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.SimulateFormDefinitionWorkflow(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// submitFormDefinitionDraftForReview 把草稿送入管理員審核隊列。
func (c WorkflowCtrl) submitFormDefinitionDraftForReview(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.SubmitFormDefinitionDraftForReview(ctx, r.PathValue(PathParamID), input.Revision)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// publishFormDefinitionDraft 在管理員確認後發佈 compiled schema。
func (c WorkflowCtrl) publishFormDefinitionDraft(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.PublishFormDefinitionDraft(ctx, r.PathValue(PathParamID), input.Revision)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// formDataSources 回傳表單設計與填寫共用的受控資料源目錄。
func (c WorkflowCtrl) formDataSources(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.FormDataSources(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// getRuntimeFormTemplate returns the published versioned schema used by form fillers.
func (c WorkflowCtrl) getRuntimeFormTemplate(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetRuntimeFormTemplate(ctx, r.PathValue("key"), strings.TrimSpace(r.URL.Query().Get("version_id")))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
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

// getFormInstance returns one authorized submitted form with template metadata.
func (c WorkflowCtrl) getFormInstance(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetFormInstanceDetail(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
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

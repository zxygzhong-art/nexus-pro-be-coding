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

const pathParamFormFileID = "file_id"

// WorkflowCtrl 定義流程 ctrl 的資料結構。
type WorkflowCtrl struct {
	routes routeBinder
	svc    service.WorkflowFacade
}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
func (c WorkflowCtrl) RegisterRoutes(router *gin.RouterGroup) {
	builder := router.Group("/form-builder")
	builder.GET("/drafts", c.routes.Handle("workflow.form_definition_draft", "read", c.listFormDefinitionDrafts))
	builder.POST("/drafts/:id/submit-review", c.routes.Handle("workflow.form_definition_draft", "submit", c.submitFormDefinitionDraftForReview, ResourceID(PathParamID)))
	builder.POST("/drafts/:id/publish", c.routes.Handle("workflow.form_definition_draft", "approve", c.publishFormDefinitionDraft, ResourceID(PathParamID)))

	workflows := router.Group("/workflows")
	workflows.GET("/form-data-sources", c.routes.Handle("workflow.form_instance", "read", c.formDataSources))
	workflows.GET("/form-templates/:key", c.routes.Handle("workflow.form_instance", "read", c.getRuntimeFormTemplate, PathParam("key")))
	workflows.GET("/forms/:id", c.routes.Handle("workflow.form_instance", "read", c.getFormInstance, PathParam(PathParamID)))
	workflows.GET("/reviews", c.routes.Handle("workflow.form_instance", "read", c.reviewQueue))
	workflows.POST("/reviews/bulk-action", c.routes.Handle("workflow.form_instance", "approve", c.bulkReviewForms))
	workflows.POST("/forms/drafts", c.routes.Handle("workflow.form_instance", "submit", c.saveFormDraft))
	workflows.GET("/forms/:id/workflow", c.routes.Handle("workflow.form_instance", "read", c.getWorkflowFormState, PathParam(PathParamID)))
	workflows.GET("/forms/:id/export", c.routes.Handle("workflow.form_instance", "read", c.exportForm, PathParam(PathParamID)))
	workflows.PATCH("/forms/:id", c.routes.Handle("workflow.form_instance", "update", c.updateFormDraft, PathParam(PathParamID)))
	workflows.DELETE("/forms/:id", c.routes.Handle("workflow.form_instance", "delete", c.deleteFormDraft, PathParam(PathParamID)))
	workflows.POST("/forms/:id/submit", c.routes.Handle("workflow.form_instance", "submit", c.submitForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/approve", c.routes.Handle("workflow.form_instance", "approve", c.approveForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/reject", c.routes.Handle("workflow.form_instance", "approve", c.rejectForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/return", c.routes.Handle("workflow.form_instance", "approve", c.returnForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/cancel", c.routes.Handle("workflow.form_instance", "update", c.cancelForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/duplicate", c.routes.Handle("workflow.form_instance", "submit", c.duplicateForm, PathParam(PathParamID)))
	workflows.POST("/forms/:id/files", c.routes.Handle("workflow.form_instance", "submit", c.uploadFormInstanceFile, PathParam(PathParamID)))
	workflows.GET("/forms/:id/files/:file_id", c.routes.Handle("workflow.form_instance", "read", c.downloadFormInstanceFile, PathParam(PathParamID), PathParam(pathParamFormFileID)))
	workflows.DELETE("/forms/:id/files/:file_id", c.routes.Handle("workflow.form_instance", "submit", c.deleteFormInstanceFile, PathParam(PathParamID), PathParam(pathParamFormFileID)))
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

// uploadFormInstanceFile stages a multipart attachment into the object store for a draft form.
func (c WorkflowCtrl) uploadFormInstanceFile(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(11 << 20); err != nil {
		return domain.BadRequest("invalid multipart form: " + err.Error())
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return domain.BadRequest("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		return domain.BadRequest("read form attachment: " + err.Error())
	}
	item, err := c.svc.UploadFormInstanceFile(ctx, r.PathValue(PathParamID), domain.UploadFormInstanceFileInput{
		FieldID:     r.FormValue("field_id"),
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Content:     content,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// downloadFormInstanceFile proxies authorized object bytes without exposing SFTPGo credentials.
func (c WorkflowCtrl) downloadFormInstanceFile(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	download, err := c.svc.DownloadFormInstanceFile(ctx, r.PathValue(PathParamID), r.PathValue(pathParamFormFileID))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", download.File.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.File.OriginalFilename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(download.Content)
	return err
}

// deleteFormInstanceFile deletes only a draft attachment that has not been submitted.
func (c WorkflowCtrl) deleteFormInstanceFile(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteFormInstanceFile(ctx, r.PathValue(PathParamID), r.PathValue(pathParamFormFileID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
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

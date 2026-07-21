package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	maxFormInstanceFileBytes   = 10 << 20
	maxFormInstanceFilesPerField = 5
)

var formAttachmentExtensions = map[string]struct{}{
	".pdf": {}, ".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {},
	".doc": {}, ".docx": {}, ".xls": {}, ".xlsx": {}, ".ppt": {}, ".pptx": {},
	".txt": {}, ".csv": {}, ".zip": {},
}

// UploadFormInstanceFile stores a binary attachment via the configured object store (SFTPGo).
func (c WorkflowService) UploadFormInstanceFile(ctx RequestContext, formInstanceID string, input domain.UploadFormInstanceFileInput) (domain.FormInstanceFile, error) {
	formInstanceID = strings.TrimSpace(formInstanceID)
	fieldID := strings.TrimSpace(input.FieldID)
	if formInstanceID == "" {
		return domain.FormInstanceFile{}, BadRequest("form instance id is required")
	}
	if fieldID == "" {
		return domain.FormInstanceFile{}, BadRequest("field_id is required")
	}
	account, decision, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, "")
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	if !ok {
		return domain.FormInstanceFile{}, NotFound("form instance", formInstanceID)
	}
	if err := requireFormInstanceVisible(instance, account, decision); err != nil {
		return domain.FormInstanceFile{}, err
	}
	status := normalizeWorkflowStatus(instance.Status)
	if status != WorkflowFormStatusDraft && status != workflowFormStatusReturned {
		return domain.FormInstanceFile{}, BadRequest("attachments can only be uploaded on draft or returned forms")
	}
	if instance.ApplicantAccountID != account.ID {
		return domain.FormInstanceFile{}, Forbidden("only the applicant can upload attachments")
	}
	count, err := c.store.CountFormInstanceFilesByField(goContext(ctx), ctx.TenantID, formInstanceID, fieldID)
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	if count >= maxFormInstanceFilesPerField {
		return domain.FormInstanceFile{}, BadRequest(fmt.Sprintf("field allows at most %d attachments", maxFormInstanceFilesPerField))
	}
	filename, contentType, err := normalizeFormInstanceFileInput(input)
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	now := c.Now()
	fileID := utils.NewID("ffile")
	objectKey := fmt.Sprintf("tenants/%s/forms/%s/%s/%s", ctx.TenantID, formInstanceID, fieldID, fileID)
	if err := c.ObjectStore().PutObject(goContext(ctx), objectKey, contentType, input.Content); err != nil {
		c.logWarn(ctx, "store form attachment failed", "object_key", objectKey, "error", err)
		return domain.FormInstanceFile{}, domain.E(502, "object_store_error", "form attachment storage failed")
	}
	committed := false
	defer func() {
		if !committed {
			c.deleteFormObjectIfSupported(ctx, objectKey)
		}
	}()
	hash := sha256.Sum256(input.Content)
	file := domain.FormInstanceFile{
		ID: fileID, TenantID: ctx.TenantID, FormInstanceID: formInstanceID, FieldID: fieldID,
		CreatedByAccountID: account.ID, OriginalFilename: filename,
		ObjectProvider: ObjectStoreProvider(c.ObjectStore()), ObjectBucket: ObjectStoreBucket(c.ObjectStore()), ObjectKey: objectKey,
		ContentType: contentType, SizeBytes: int64(len(input.Content)), SHA256: hex.EncodeToString(hash[:]),
		ScanStatus: "not_configured", ParseStatus: "unsupported", RetentionClass: "permanent", State: "draft",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		locked, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", formInstanceID)
		}
		lockedStatus := normalizeWorkflowStatus(locked.Status)
		if lockedStatus != WorkflowFormStatusDraft && lockedStatus != workflowFormStatusReturned {
			return BadRequest("attachments can only be uploaded on draft or returned forms")
		}
		if err := tx.store.UpsertFormFileAsset(goContext(ctx), file); err != nil {
			return err
		}
		return tx.store.InsertFormInstanceFile(goContext(ctx), file)
	}); err != nil {
		return domain.FormInstanceFile{}, err
	}
	committed = true
	return file, nil
}

// ListFormInstanceFiles lists attachments visible to the caller for one form instance.
func (c WorkflowService) ListFormInstanceFiles(ctx RequestContext, formInstanceID string) ([]domain.FormInstanceFile, error) {
	formInstanceID = strings.TrimSpace(formInstanceID)
	account, decision, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return nil, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, NotFound("form instance", formInstanceID)
	}
	if err := requireFormInstanceVisible(instance, account, decision); err != nil {
		return nil, err
	}
	return c.store.ListFormInstanceFiles(goContext(ctx), ctx.TenantID, formInstanceID)
}

// DownloadFormInstanceFile proxies authorized object bytes without exposing SFTPGo credentials.
func (c WorkflowService) DownloadFormInstanceFile(ctx RequestContext, formInstanceID, fileID string) (domain.FormInstanceFileDownload, error) {
	formInstanceID = strings.TrimSpace(formInstanceID)
	fileID = strings.TrimSpace(fileID)
	account, decision, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return domain.FormInstanceFileDownload{}, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstanceFileDownload{}, err
	}
	if !ok {
		return domain.FormInstanceFileDownload{}, NotFound("form instance", formInstanceID)
	}
	if err := requireFormInstanceVisible(instance, account, decision); err != nil {
		return domain.FormInstanceFileDownload{}, err
	}
	file, ok, err := c.store.GetFormInstanceFile(goContext(ctx), ctx.TenantID, formInstanceID, fileID)
	if err != nil {
		return domain.FormInstanceFileDownload{}, err
	}
	if !ok {
		return domain.FormInstanceFileDownload{}, NotFound("form attachment", fileID)
	}
	content, err := c.ObjectStore().GetObject(goContext(ctx), file.ObjectKey)
	if err != nil {
		return domain.FormInstanceFileDownload{}, err
	}
	return domain.FormInstanceFileDownload{File: file, Content: content}, nil
}

// DeleteFormInstanceFile removes only a draft attachment owned by the applicant.
func (c WorkflowService) DeleteFormInstanceFile(ctx RequestContext, formInstanceID, fileID string) (domain.FormInstanceFile, error) {
	formInstanceID = strings.TrimSpace(formInstanceID)
	fileID = strings.TrimSpace(fileID)
	account, decision, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, "")
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	if !ok {
		return domain.FormInstanceFile{}, NotFound("form instance", formInstanceID)
	}
	if err := requireFormInstanceVisible(instance, account, decision); err != nil {
		return domain.FormInstanceFile{}, err
	}
	if instance.ApplicantAccountID != account.ID {
		return domain.FormInstanceFile{}, Forbidden("only the applicant can delete attachments")
	}
	status := normalizeWorkflowStatus(instance.Status)
	if status != WorkflowFormStatusDraft && status != workflowFormStatusReturned {
		return domain.FormInstanceFile{}, BadRequest("attachments can only be deleted on draft or returned forms")
	}
	file, ok, err := c.store.GetFormInstanceFile(goContext(ctx), ctx.TenantID, formInstanceID, fileID)
	if err != nil {
		return domain.FormInstanceFile{}, err
	}
	if !ok {
		return domain.FormInstanceFile{}, NotFound("form attachment", fileID)
	}
	if file.State != "draft" {
		return domain.FormInstanceFile{}, Conflict("attached form files cannot be deleted independently")
	}
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		deleted, err := tx.store.DeleteDraftFormInstanceFile(goContext(ctx), ctx.TenantID, formInstanceID, fileID)
		if err != nil {
			return err
		}
		if !deleted {
			return NotFound("form attachment", fileID)
		}
		return tx.store.DeleteFormFileAsset(goContext(ctx), ctx.TenantID, fileID)
	}); err != nil {
		return domain.FormInstanceFile{}, err
	}
	c.deleteFormObjectIfSupported(ctx, file.ObjectKey)
	return file, nil
}

func normalizeFormInstanceFileInput(input domain.UploadFormInstanceFileInput) (string, string, error) {
	filename := path.Base(strings.ReplaceAll(strings.TrimSpace(input.Filename), "\\", "/"))
	if filename == "" || filename == "." {
		return "", "", BadRequest("file name is required")
	}
	if utf8.RuneCountInString(filename) > 255 || strings.IndexFunc(filename, unicode.IsControl) >= 0 {
		return "", "", BadRequest("file name is invalid")
	}
	if len(input.Content) == 0 {
		return "", "", BadRequest("file is empty")
	}
	if len(input.Content) > maxFormInstanceFileBytes {
		return "", "", BadRequest("attachment exceeds 10MB limit")
	}
	extension := strings.ToLower(path.Ext(filename))
	if _, ok := formAttachmentExtensions[extension]; !ok {
		return "", "", BadRequest("attachment file type is not supported")
	}
	contentType := strings.TrimSpace(input.ContentType)
	mediaType, _, mediaTypeErr := mime.ParseMediaType(contentType)
	if mediaTypeErr != nil || mediaType == "" || mediaType == "application/octet-stream" {
		contentType = http.DetectContentType(input.Content)
	} else {
		contentType = mediaType
	}
	return filename, contentType, nil
}

func (c WorkflowService) deleteFormObjectIfSupported(ctx RequestContext, key string) {
	deleter, ok := c.ObjectStore().(ObjectDeleter)
	if !ok || strings.TrimSpace(key) == "" {
		return
	}
	if err := deleter.DeleteObject(goContext(ctx), key); err != nil {
		c.logWarn(ctx, "delete form attachment object failed", "object_key", key, "error", err)
	}
}

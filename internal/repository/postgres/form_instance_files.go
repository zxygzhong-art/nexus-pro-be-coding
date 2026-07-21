package postgres

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"

	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

// UpsertFormFileAsset persists object metadata without exposing storage keys through the API.
func (s *Store) UpsertFormFileAsset(execCtx context.Context, file domain.FormInstanceFile) error {
	_, err := s.q.UpsertFileAsset(tenantContext(execCtx, file.TenantID), sqlc.UpsertFileAssetParams{
		ID: file.ID, TenantID: file.TenantID, CreatedByAccountID: file.CreatedByAccountID,
		OriginalFilename: file.OriginalFilename, ObjectProvider: file.ObjectProvider, ObjectBucket: file.ObjectBucket,
		ObjectKey: file.ObjectKey, ContentType: file.ContentType, SizeBytes: file.SizeBytes, Sha256: file.SHA256,
		ScanStatus: file.ScanStatus, ParseStatus: file.ParseStatus, RetentionClass: file.RetentionClass,
		ExpiresAt: nullableTimestamptz(file.ExpiresAt), CreatedAt: timestamptz(file.CreatedAt), UpdatedAt: timestamptz(file.UpdatedAt),
	})
	return err
}

// InsertFormInstanceFile binds a file asset to a form instance field.
func (s *Store) InsertFormInstanceFile(execCtx context.Context, file domain.FormInstanceFile) error {
	_, err := s.q.InsertFormInstanceFile(tenantContext(execCtx, file.TenantID), sqlc.InsertFormInstanceFileParams{
		TenantID: file.TenantID, FormInstanceID: file.FormInstanceID, FileID: file.ID,
		FieldID: file.FieldID, State: file.State, CreatedAt: timestamptz(file.CreatedAt), UpdatedAt: timestamptz(file.UpdatedAt),
	})
	return err
}

// GetFormInstanceFile returns one attachment belonging to the form instance.
func (s *Store) GetFormInstanceFile(execCtx context.Context, tenantID, formInstanceID, fileID string) (domain.FormInstanceFile, bool, error) {
	item, err := s.q.GetFormInstanceFile(tenantContext(execCtx, tenantID), sqlc.GetFormInstanceFileParams{
		TenantID: tenantID, FormInstanceID: formInstanceID, FileID: fileID,
	})
	if isNotFound(err) {
		return domain.FormInstanceFile{}, false, nil
	}
	if err != nil {
		return domain.FormInstanceFile{}, false, err
	}
	return formInstanceFileFromGetRow(item), true, nil
}

// ListFormInstanceFiles lists all attachments for a form instance.
func (s *Store) ListFormInstanceFiles(execCtx context.Context, tenantID, formInstanceID string) ([]domain.FormInstanceFile, error) {
	items, err := s.q.ListFormInstanceFiles(tenantContext(execCtx, tenantID), sqlc.ListFormInstanceFilesParams{
		TenantID: tenantID, FormInstanceID: formInstanceID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.FormInstanceFile, 0, len(items))
	for _, item := range items {
		out = append(out, formInstanceFileFromListRow(item))
	}
	return out, nil
}

// ListFormInstanceFilesByField lists attachments for one field.
func (s *Store) ListFormInstanceFilesByField(execCtx context.Context, tenantID, formInstanceID, fieldID string) ([]domain.FormInstanceFile, error) {
	items, err := s.q.ListFormInstanceFilesByField(tenantContext(execCtx, tenantID), sqlc.ListFormInstanceFilesByFieldParams{
		TenantID: tenantID, FormInstanceID: formInstanceID, FieldID: fieldID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.FormInstanceFile, 0, len(items))
	for _, item := range items {
		out = append(out, formInstanceFileFromFieldRow(item))
	}
	return out, nil
}

// CountFormInstanceFilesByField returns how many files are bound to a field.
func (s *Store) CountFormInstanceFilesByField(execCtx context.Context, tenantID, formInstanceID, fieldID string) (int, error) {
	count, err := s.q.CountFormInstanceFilesByField(tenantContext(execCtx, tenantID), sqlc.CountFormInstanceFilesByFieldParams{
		TenantID: tenantID, FormInstanceID: formInstanceID, FieldID: fieldID,
	})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// MarkFormInstanceFilesAttached promotes draft attachments after form submit.
func (s *Store) MarkFormInstanceFilesAttached(execCtx context.Context, tenantID, formInstanceID string, updatedAt time.Time) error {
	return s.q.MarkFormInstanceFilesAttached(tenantContext(execCtx, tenantID), sqlc.MarkFormInstanceFilesAttachedParams{
		UpdatedAt: timestamptz(updatedAt), TenantID: tenantID, FormInstanceID: formInstanceID,
	})
}

// DeleteDraftFormInstanceFile removes only an unattached draft binding.
func (s *Store) DeleteDraftFormInstanceFile(execCtx context.Context, tenantID, formInstanceID, fileID string) (bool, error) {
	_, err := s.q.DeleteDraftFormInstanceFile(tenantContext(execCtx, tenantID), sqlc.DeleteDraftFormInstanceFileParams{
		TenantID: tenantID, FormInstanceID: formInstanceID, FileID: fileID,
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// DeleteFormFileAsset removes metadata after its draft binding is gone.
func (s *Store) DeleteFormFileAsset(execCtx context.Context, tenantID, fileID string) error {
	return s.q.DeleteFileAsset(tenantContext(execCtx, tenantID), sqlc.DeleteFileAssetParams{TenantID: tenantID, FileID: fileID})
}

func formInstanceFileFromGetRow(item sqlc.GetFormInstanceFileRow) domain.FormInstanceFile {
	return domain.FormInstanceFile{
		ID: item.ID, TenantID: item.TenantID, FormInstanceID: item.FormInstanceID, FieldID: item.FieldID,
		CreatedByAccountID: item.CreatedByAccountID, OriginalFilename: item.OriginalFilename,
		ObjectProvider: item.ObjectProvider, ObjectBucket: item.ObjectBucket, ObjectKey: item.ObjectKey,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, SHA256: item.Sha256,
		ScanStatus: item.ScanStatus, ParseStatus: item.ParseStatus, RetentionClass: item.RetentionClass,
		State: item.State, ExpiresAt: timePtrFrom(item.ExpiresAt), CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt),
	}
}

func formInstanceFileFromListRow(item sqlc.ListFormInstanceFilesRow) domain.FormInstanceFile {
	return domain.FormInstanceFile{
		ID: item.ID, TenantID: item.TenantID, FormInstanceID: item.FormInstanceID, FieldID: item.FieldID,
		CreatedByAccountID: item.CreatedByAccountID, OriginalFilename: item.OriginalFilename,
		ObjectProvider: item.ObjectProvider, ObjectBucket: item.ObjectBucket, ObjectKey: item.ObjectKey,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, SHA256: item.Sha256,
		ScanStatus: item.ScanStatus, ParseStatus: item.ParseStatus, RetentionClass: item.RetentionClass,
		State: item.State, ExpiresAt: timePtrFrom(item.ExpiresAt), CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt),
	}
}

func formInstanceFileFromFieldRow(item sqlc.ListFormInstanceFilesByFieldRow) domain.FormInstanceFile {
	return domain.FormInstanceFile{
		ID: item.ID, TenantID: item.TenantID, FormInstanceID: item.FormInstanceID, FieldID: item.FieldID,
		CreatedByAccountID: item.CreatedByAccountID, OriginalFilename: item.OriginalFilename,
		ObjectProvider: item.ObjectProvider, ObjectBucket: item.ObjectBucket, ObjectKey: item.ObjectKey,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, SHA256: item.Sha256,
		ScanStatus: item.ScanStatus, ParseStatus: item.ParseStatus, RetentionClass: item.RetentionClass,
		State: item.State, ExpiresAt: timePtrFrom(item.ExpiresAt), CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt),
	}
}

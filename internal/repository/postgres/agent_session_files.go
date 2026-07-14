package postgres

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"

	sqlc "nexus-pro-be/internal/platform/postgres/db"
)

// UpsertAgentFileAsset persists object metadata without exposing its storage key through the API.
func (s *Store) UpsertAgentFileAsset(execCtx context.Context, file domain.AgentSessionFile) error {
	_, err := s.q.UpsertFileAsset(tenantContext(execCtx, file.TenantID), sqlc.UpsertFileAssetParams{
		ID: file.ID, TenantID: file.TenantID, CreatedByAccountID: file.CreatedByAccountID,
		OriginalFilename: file.OriginalFilename, ObjectProvider: file.ObjectProvider, ObjectBucket: file.ObjectBucket,
		ObjectKey: file.ObjectKey, ContentType: file.ContentType, SizeBytes: file.SizeBytes, Sha256: file.SHA256,
		ScanStatus: file.ScanStatus, ParseStatus: file.ParseStatus, RetentionClass: file.RetentionClass,
		ExpiresAt: nullableTimestamptz(file.ExpiresAt), CreatedAt: timestamptz(file.CreatedAt), UpdatedAt: timestamptz(file.UpdatedAt),
	})
	return err
}

// InsertAgentFileChunks stores bounded text chunks used to assemble conversation context.
func (s *Store) InsertAgentFileChunks(execCtx context.Context, tenantID, fileID string, chunks []string, createdAt time.Time) error {
	for ordinal, content := range chunks {
		if _, err := s.q.InsertFileChunk(tenantContext(execCtx, tenantID), sqlc.InsertFileChunkParams{
			ID: utils.NewID("fchunk"), TenantID: tenantID, FileID: fileID, Ordinal: int32(ordinal),
			Content: content, CreatedAt: timestamptz(createdAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListAgentFileChunks returns parsed text in source order.
func (s *Store) ListAgentFileChunks(execCtx context.Context, tenantID, fileID string) ([]string, error) {
	items, err := s.q.ListFileChunks(tenantContext(execCtx, tenantID), sqlc.ListFileChunksParams{TenantID: tenantID, FileID: fileID})
	if err != nil {
		return nil, err
	}
	chunks := make([]string, 0, len(items))
	for _, item := range items {
		chunks = append(chunks, item.Content)
	}
	return chunks, nil
}

// InsertAgentSessionFile stages a file inside the session's current context version.
func (s *Store) InsertAgentSessionFile(execCtx context.Context, file domain.AgentSessionFile) error {
	_, err := s.q.InsertAgentSessionFile(tenantContext(execCtx, file.TenantID), sqlc.InsertAgentSessionFileParams{
		TenantID: file.TenantID, SessionID: file.SessionID, FileID: file.ID, ContextVersion: file.ContextVersion,
		State: file.State, CreatedAt: timestamptz(file.CreatedAt), UpdatedAt: timestamptz(file.UpdatedAt),
	})
	return err
}

// GetCurrentAgentSessionFile resolves a file only when it belongs to the visible context version.
func (s *Store) GetCurrentAgentSessionFile(execCtx context.Context, tenantID, sessionID, fileID string) (domain.AgentSessionFile, bool, error) {
	item, err := s.q.GetCurrentAgentSessionFile(tenantContext(execCtx, tenantID), sqlc.GetCurrentAgentSessionFileParams{
		TenantID: tenantID, SessionID: sessionID, FileID: fileID,
	})
	if isNotFound(err) {
		return domain.AgentSessionFile{}, false, nil
	}
	if err != nil {
		return domain.AgentSessionFile{}, false, err
	}
	return agentSessionFileFromRow(item), true, nil
}

// ListCurrentAgentSessionFiles lists files visible in the active context version.
func (s *Store) ListCurrentAgentSessionFiles(execCtx context.Context, tenantID, sessionID string) ([]domain.AgentSessionFile, error) {
	items, err := s.q.ListCurrentAgentSessionFiles(tenantContext(execCtx, tenantID), sqlc.ListCurrentAgentSessionFilesParams{
		TenantID: tenantID, SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentSessionFile, 0, len(items))
	for _, item := range items {
		out = append(out, agentSessionFileFromListRow(item))
	}
	return out, nil
}

// MarkAgentSessionFileAttached moves a staged file into persisted message history.
func (s *Store) MarkAgentSessionFileAttached(execCtx context.Context, tenantID, sessionID, fileID string, updatedAt time.Time) error {
	_, err := s.q.MarkAgentSessionFileAttached(tenantContext(execCtx, tenantID), sqlc.MarkAgentSessionFileAttachedParams{
		UpdatedAt: timestamptz(updatedAt), TenantID: tenantID, SessionID: sessionID, FileID: fileID,
	})
	return err
}

// InsertAgentMessageAttachment records exact turn-level file provenance.
func (s *Store) InsertAgentMessageAttachment(execCtx context.Context, tenantID, messageID, fileID string, ordinal int, createdAt time.Time) error {
	_, err := s.q.InsertAgentMessageAttachment(tenantContext(execCtx, tenantID), sqlc.InsertAgentMessageAttachmentParams{
		TenantID: tenantID, MessageID: messageID, FileID: fileID, Ordinal: int32(ordinal), CreatedAt: timestamptz(createdAt),
	})
	return err
}

// ListCurrentAgentMessageAttachments returns attachments only for visible messages.
func (s *Store) ListCurrentAgentMessageAttachments(execCtx context.Context, tenantID, sessionID string) ([]domain.AgentMessageAttachment, error) {
	items, err := s.q.ListCurrentAgentMessageAttachments(tenantContext(execCtx, tenantID), sqlc.ListCurrentAgentMessageAttachmentsParams{
		TenantID: tenantID, SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentMessageAttachment, 0, len(items))
	for _, item := range items {
		out = append(out, domain.AgentMessageAttachment{MessageID: item.MessageID, Ordinal: int(item.Ordinal), File: agentSessionFileFromAttachmentRow(item)})
	}
	return out, nil
}

// DeleteCurrentDraftAgentSessionFile removes only an unsent file from the active context version.
func (s *Store) DeleteCurrentDraftAgentSessionFile(execCtx context.Context, tenantID, sessionID, fileID string) (bool, error) {
	_, err := s.q.DeleteCurrentDraftAgentSessionFile(tenantContext(execCtx, tenantID), sqlc.DeleteCurrentDraftAgentSessionFileParams{
		TenantID: tenantID, SessionID: sessionID, FileID: fileID,
	})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// DeleteAgentFileAsset removes metadata and parsed chunks after its draft binding is gone.
func (s *Store) DeleteAgentFileAsset(execCtx context.Context, tenantID, fileID string) error {
	return s.q.DeleteFileAsset(tenantContext(execCtx, tenantID), sqlc.DeleteFileAssetParams{TenantID: tenantID, FileID: fileID})
}

func agentSessionFileFromRow(item sqlc.GetCurrentAgentSessionFileRow) domain.AgentSessionFile {
	return domain.AgentSessionFile{
		ID: item.ID, TenantID: item.TenantID, SessionID: item.SessionID, ContextVersion: item.ContextVersion,
		CreatedByAccountID: item.CreatedByAccountID, OriginalFilename: item.OriginalFilename,
		ObjectProvider: item.ObjectProvider, ObjectBucket: item.ObjectBucket, ObjectKey: item.ObjectKey,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, SHA256: item.Sha256,
		ScanStatus: item.ScanStatus, ParseStatus: item.ParseStatus, RetentionClass: item.RetentionClass,
		State: item.State, ExpiresAt: timePtrFrom(item.ExpiresAt), CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt),
	}
}

func agentSessionFileFromListRow(item sqlc.ListCurrentAgentSessionFilesRow) domain.AgentSessionFile {
	return domain.AgentSessionFile{
		ID: item.ID, TenantID: item.TenantID, SessionID: item.SessionID, ContextVersion: item.ContextVersion,
		CreatedByAccountID: item.CreatedByAccountID, OriginalFilename: item.OriginalFilename,
		ObjectProvider: item.ObjectProvider, ObjectBucket: item.ObjectBucket, ObjectKey: item.ObjectKey,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, SHA256: item.Sha256,
		ScanStatus: item.ScanStatus, ParseStatus: item.ParseStatus, RetentionClass: item.RetentionClass,
		State: item.State, ExpiresAt: timePtrFrom(item.ExpiresAt), CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt),
	}
}

func agentSessionFileFromAttachmentRow(item sqlc.ListCurrentAgentMessageAttachmentsRow) domain.AgentSessionFile {
	return domain.AgentSessionFile{
		ID: item.ID, TenantID: item.TenantID, SessionID: item.SessionID, ContextVersion: item.ContextVersion,
		CreatedByAccountID: item.CreatedByAccountID, OriginalFilename: item.OriginalFilename,
		ObjectProvider: item.ObjectProvider, ObjectBucket: item.ObjectBucket, ObjectKey: item.ObjectKey,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, SHA256: item.Sha256,
		ScanStatus: item.ScanStatus, ParseStatus: item.ParseStatus, RetentionClass: item.RetentionClass,
		State: item.State, ExpiresAt: timePtrFrom(item.ExpiresAt), CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt),
	}
}

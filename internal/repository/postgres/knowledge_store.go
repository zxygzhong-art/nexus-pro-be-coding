package postgres

import (
	"context"

	"github.com/pgvector/pgvector-go"

	"nexus-pro-api/internal/domain"
	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

// UpsertKnowledgeBase persists a tenant knowledge base.
func (s *Store) UpsertKnowledgeBase(execCtx context.Context, v domain.KnowledgeBase) error {
	chunkMode, chunkSize, chunkOverlap := v.ChunkMode, v.ChunkSize, v.ChunkOverlap
	if chunkMode == "" {
		chunkMode = "auto"
	}
	if chunkSize == 0 {
		chunkSize, chunkOverlap = 1200, 200
	}
	_, err := s.q.UpsertKnowledgeBase(tenantContext(execCtx, v.TenantID), sqlc.UpsertKnowledgeBaseParams{
		ID: v.ID, TenantID: v.TenantID, Name: v.Name, Description: v.Description,
		ChunkMode: chunkMode, ChunkSize: int32(chunkSize), ChunkOverlap: int32(chunkOverlap),
		CreatedByAccountID: nullableText(v.CreatedByAccountID), UpdatedByAccountID: nullableText(v.UpdatedByAccountID),
		CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

// GetKnowledgeBase loads a tenant knowledge base.
func (s *Store) GetKnowledgeBase(execCtx context.Context, tenantID, id string) (domain.KnowledgeBase, bool, error) {
	v, err := s.q.GetKnowledgeBase(tenantContext(execCtx, tenantID), sqlc.GetKnowledgeBaseParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.KnowledgeBase{}, false, nil
	}
	if err != nil {
		return domain.KnowledgeBase{}, false, err
	}
	return fromKnowledgeBase(v), true, nil
}

// ListKnowledgeBases lists tenant knowledge bases.
func (s *Store) ListKnowledgeBases(execCtx context.Context, tenantID string) ([]domain.KnowledgeBase, error) {
	items, err := s.q.ListKnowledgeBases(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromKnowledgeBase), nil
}

// DeleteKnowledgeBase deletes a tenant knowledge base.
func (s *Store) DeleteKnowledgeBase(execCtx context.Context, tenantID, id string) (domain.KnowledgeBase, bool, error) {
	v, err := s.q.DeleteKnowledgeBase(tenantContext(execCtx, tenantID), sqlc.DeleteKnowledgeBaseParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.KnowledgeBase{}, false, nil
	}
	if err != nil {
		return domain.KnowledgeBase{}, false, err
	}
	return fromKnowledgeBase(v), true, nil
}

// UpsertKnowledgeDocument persists a manual text document.
func (s *Store) UpsertKnowledgeDocument(execCtx context.Context, v domain.KnowledgeDocument) error {
	parseStatus := v.ParseStatus
	if parseStatus == "" {
		parseStatus = "ready"
	}
	_, err := s.q.UpsertKnowledgeDocument(tenantContext(execCtx, v.TenantID), sqlc.UpsertKnowledgeDocumentParams{
		ID: v.ID, TenantID: v.TenantID, KnowledgeBaseID: v.KnowledgeBaseID, Title: v.Title, Content: v.Content, SourceType: v.SourceType,
		OriginalFilename: v.OriginalFilename, ContentType: v.ContentType, SizeBytes: v.SizeBytes, Sha256: v.SHA256,
		ObjectProvider: v.ObjectProvider, ObjectBucket: v.ObjectBucket, ObjectKey: v.ObjectKey, ParseStatus: parseStatus, ParseError: v.ParseError,
		CreatedByAccountID: nullableText(v.CreatedByAccountID), UpdatedByAccountID: nullableText(v.UpdatedByAccountID),
		CreatedAt: timestamptz(v.CreatedAt), UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

// GetKnowledgeDocument loads a document within its knowledge base.
func (s *Store) GetKnowledgeDocument(execCtx context.Context, tenantID, knowledgeBaseID, id string) (domain.KnowledgeDocument, bool, error) {
	v, err := s.q.GetKnowledgeDocument(tenantContext(execCtx, tenantID), sqlc.GetKnowledgeDocumentParams{TenantID: tenantID, KnowledgeBaseID: knowledgeBaseID, ID: id})
	if isNotFound(err) {
		return domain.KnowledgeDocument{}, false, nil
	}
	if err != nil {
		return domain.KnowledgeDocument{}, false, err
	}
	return fromKnowledgeDocument(v), true, nil
}

// ListKnowledgeDocuments lists documents in one knowledge base.
func (s *Store) ListKnowledgeDocuments(execCtx context.Context, tenantID, knowledgeBaseID string) ([]domain.KnowledgeDocument, error) {
	items, err := s.q.ListKnowledgeDocuments(tenantContext(execCtx, tenantID), sqlc.ListKnowledgeDocumentsParams{TenantID: tenantID, KnowledgeBaseID: knowledgeBaseID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromKnowledgeDocument), nil
}

// DeleteKnowledgeDocument deletes a document within its knowledge base.
func (s *Store) DeleteKnowledgeDocument(execCtx context.Context, tenantID, knowledgeBaseID, id string) (domain.KnowledgeDocument, bool, error) {
	v, err := s.q.DeleteKnowledgeDocument(tenantContext(execCtx, tenantID), sqlc.DeleteKnowledgeDocumentParams{TenantID: tenantID, KnowledgeBaseID: knowledgeBaseID, ID: id})
	if isNotFound(err) {
		return domain.KnowledgeDocument{}, false, nil
	}
	if err != nil {
		return domain.KnowledgeDocument{}, false, err
	}
	return fromKnowledgeDocument(v), true, nil
}

// ReplaceKnowledgeDocumentChunks replaces one document's embeddings inside the caller's transaction.
func (s *Store) ReplaceKnowledgeDocumentChunks(execCtx context.Context, tenantID, documentID string, chunks []domain.KnowledgeDocumentChunk) error {
	ctx := tenantContext(execCtx, tenantID)
	if err := s.q.DeleteKnowledgeDocumentChunks(ctx, sqlc.DeleteKnowledgeDocumentChunksParams{TenantID: tenantID, DocumentID: documentID}); err != nil {
		return err
	}
	for _, chunk := range chunks {
		if err := s.q.CreateKnowledgeDocumentChunk(ctx, sqlc.CreateKnowledgeDocumentChunkParams{
			ID: chunk.ID, TenantID: chunk.TenantID, KnowledgeBaseID: chunk.KnowledgeBaseID,
			DocumentID: chunk.DocumentID, Ordinal: int32(chunk.Ordinal), Content: chunk.Content,
			EmbeddingModel: chunk.EmbeddingModel, EmbeddingDimensions: int32(chunk.EmbeddingDimensions),
			Embedding: pgvector.NewVector(chunk.Embedding),
			CreatedAt: timestamptz(chunk.CreatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

// SearchKnowledgeDocumentChunks performs an exact cosine nearest-neighbor search in pgvector.
func (s *Store) SearchKnowledgeDocumentChunks(execCtx context.Context, tenantID string, knowledgeBaseIDs []string, embeddingModel string, queryEmbedding []float32, limit int) ([]domain.KnowledgeDocumentChunkMatch, error) {
	rows, err := s.q.SearchKnowledgeDocumentChunks(tenantContext(execCtx, tenantID), sqlc.SearchKnowledgeDocumentChunksParams{
		QueryEmbedding: pgvector.NewVector(queryEmbedding), TenantID: tenantID, KnowledgeBaseIds: knowledgeBaseIDs,
		EmbeddingModel: embeddingModel, EmbeddingDimensions: int32(len(queryEmbedding)), ResultLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeDocumentChunkMatch, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.KnowledgeDocumentChunkMatch{
			ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, KnowledgeBaseName: row.KnowledgeBaseName,
			DocumentID: row.DocumentID, DocumentTitle: row.Title, Ordinal: int(row.Ordinal),
			Content: row.Content, Score: row.Score,
		})
	}
	return out, nil
}

// CountAgentDefinitionsByKnowledgeBase counts current and versioned bindings.
func (s *Store) CountAgentDefinitionsByKnowledgeBase(execCtx context.Context, tenantID, knowledgeBaseID string) (int, error) {
	count, err := s.q.CountAgentDefinitionsByKnowledgeBase(tenantContext(execCtx, tenantID), sqlc.CountAgentDefinitionsByKnowledgeBaseParams{TenantID: tenantID, KnowledgeBaseID: knowledgeBaseID})
	return int(count), err
}

// fromKnowledgeBase maps a generated row to the domain model.
func fromKnowledgeBase(v sqlc.KnowledgeBasis) domain.KnowledgeBase {
	return domain.KnowledgeBase{
		ID: v.ID, TenantID: v.TenantID, Name: v.Name, Description: v.Description,
		ChunkMode: v.ChunkMode, ChunkSize: int(v.ChunkSize), ChunkOverlap: int(v.ChunkOverlap),
		CreatedByAccountID: textFrom(v.CreatedByAccountID), UpdatedByAccountID: textFrom(v.UpdatedByAccountID),
		CreatedAt: timeFrom(v.CreatedAt), UpdatedAt: timeFrom(v.UpdatedAt),
	}
}

// fromKnowledgeDocument maps a generated row to the domain model.
func fromKnowledgeDocument(v sqlc.KnowledgeDocument) domain.KnowledgeDocument {
	return domain.KnowledgeDocument{
		ID: v.ID, TenantID: v.TenantID, KnowledgeBaseID: v.KnowledgeBaseID, Title: v.Title, Content: v.Content, SourceType: v.SourceType,
		OriginalFilename: v.OriginalFilename, ContentType: v.ContentType, SizeBytes: v.SizeBytes, SHA256: v.Sha256,
		ObjectProvider: v.ObjectProvider, ObjectBucket: v.ObjectBucket, ObjectKey: v.ObjectKey, ParseStatus: v.ParseStatus, ParseError: v.ParseError,
		CreatedByAccountID: textFrom(v.CreatedByAccountID), UpdatedByAccountID: textFrom(v.UpdatedByAccountID),
		CreatedAt: timeFrom(v.CreatedAt), UpdatedAt: timeFrom(v.UpdatedAt),
	}
}

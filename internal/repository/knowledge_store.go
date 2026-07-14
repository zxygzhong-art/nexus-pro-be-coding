package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// KnowledgeStore persists tenant knowledge bases and manual documents.
type KnowledgeStore interface {
	UpsertKnowledgeBase(context.Context, domain.KnowledgeBase) error
	GetKnowledgeBase(ctx context.Context, tenantID, id string) (domain.KnowledgeBase, bool, error)
	ListKnowledgeBases(ctx context.Context, tenantID string) ([]domain.KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (domain.KnowledgeBase, bool, error)
	UpsertKnowledgeDocument(context.Context, domain.KnowledgeDocument) error
	GetKnowledgeDocument(ctx context.Context, tenantID, knowledgeBaseID, id string) (domain.KnowledgeDocument, bool, error)
	ListKnowledgeDocuments(ctx context.Context, tenantID, knowledgeBaseID string) ([]domain.KnowledgeDocument, error)
	DeleteKnowledgeDocument(ctx context.Context, tenantID, knowledgeBaseID, id string) (domain.KnowledgeDocument, bool, error)
	ReplaceKnowledgeDocumentChunks(ctx context.Context, tenantID, documentID string, chunks []domain.KnowledgeDocumentChunk) error
	SearchKnowledgeDocumentChunks(ctx context.Context, tenantID string, knowledgeBaseIDs []string, embeddingModel string, queryEmbedding []float32, limit int) ([]domain.KnowledgeDocumentChunkMatch, error)
	CountAgentDefinitionsByKnowledgeBase(ctx context.Context, tenantID, knowledgeBaseID string) (int, error)
}

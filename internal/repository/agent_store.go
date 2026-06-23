package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

type KnowledgeStore interface {
	UpsertKnowledgeArticle(context.Context, domain.KnowledgeArticle) error
	ListKnowledgeArticles(ctx context.Context, tenantID string) ([]domain.KnowledgeArticle, error)
}

type AgentStore interface {
	UpsertAgentRun(context.Context, domain.AgentRun) error
	GetAgentRun(ctx context.Context, tenantID, id string) (domain.AgentRun, bool, error)
	ListAgentRuns(ctx context.Context, tenantID string) ([]domain.AgentRun, error)
	ListAgentRunPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AgentRun, int, error)
}

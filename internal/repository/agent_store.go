package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// KnowledgeStore persists knowledge articles used by agent runs.
type KnowledgeStore interface {
	UpsertKnowledgeArticle(context.Context, domain.KnowledgeArticle) error
	ListKnowledgeArticles(ctx context.Context, tenantID string) ([]domain.KnowledgeArticle, error)
}

// AgentStore persists agent run state and list queries.
type AgentStore interface {
	UpsertAgentRun(context.Context, domain.AgentRun) error
	GetAgentRun(ctx context.Context, tenantID, id string) (domain.AgentRun, bool, error)
	ListAgentRuns(ctx context.Context, tenantID string) ([]domain.AgentRun, error)
	ListAgentRunsByAccount(ctx context.Context, tenantID, accountID string) ([]domain.AgentRun, error)
	ListAgentRunPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AgentRun, int, error)
	ListAgentRunPageByAccount(ctx context.Context, tenantID, accountID string, page domain.PageRequest) ([]domain.AgentRun, int, error)
}

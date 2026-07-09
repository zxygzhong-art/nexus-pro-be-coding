package repository

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
)

// AgentStore 定義 agent 儲存層的行為契約。
type AgentStore interface {
	UpsertAgentRun(context.Context, domain.AgentRun) error
	ListAgentRuns(ctx context.Context, tenantID string) ([]domain.AgentRun, error)
	ListAgentRunsByAccount(ctx context.Context, tenantID, accountID string) ([]domain.AgentRun, error)
	ListAgentRunPage(ctx context.Context, tenantID string, page domain.PageRequest) ([]domain.AgentRun, int, error)
	ListAgentRunPageByAccount(ctx context.Context, tenantID, accountID string, page domain.PageRequest) ([]domain.AgentRun, int, error)
	UpsertAgentModel(context.Context, domain.AgentModel) error
	GetAgentModel(ctx context.Context, tenantID, id string) (domain.AgentModel, bool, error)
	ListAgentModels(ctx context.Context, tenantID string) ([]domain.AgentModel, error)
	DeleteAgentModel(ctx context.Context, tenantID, id string) (domain.AgentModel, bool, error)
	ClearDefaultAgentModel(ctx context.Context, tenantID, exceptID string) error
	UpdateAgentModelTestResult(ctx context.Context, tenantID, id, status, message string, testedAt time.Time) (domain.AgentModel, bool, error)
	CountAgentDefinitionsByModel(ctx context.Context, tenantID, modelID string) (int, error)
	UpsertAgentDefinition(context.Context, domain.AgentDefinition) error
	GetAgentDefinition(ctx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error)
	ListAgentDefinitions(ctx context.Context, tenantID string) ([]domain.AgentDefinition, error)
	ListPublishedAgentDefinitions(ctx context.Context, tenantID string) ([]domain.AgentDefinition, error)
	DeleteAgentDefinition(ctx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error)
	UpdateAgentDefinitionUsage(ctx context.Context, tenantID, id string, success bool, latencyMs int, prompt string, runAt time.Time) (domain.AgentDefinition, bool, error)
	InsertAgentDefinitionVersion(context.Context, domain.AgentDefinitionVersion) error
	ListAgentDefinitionVersions(ctx context.Context, tenantID, agentID string) ([]domain.AgentDefinitionVersion, error)
	GetAgentDefinitionVersion(ctx context.Context, tenantID, agentID string, version int) (domain.AgentDefinitionVersion, bool, error)
	InsertAgentAudit(context.Context, domain.AgentAudit) error
	ListAgentAudits(ctx context.Context, tenantID string) ([]domain.AgentAudit, error)
	UpsertAgentSession(context.Context, domain.AgentSession) error
	GetAgentSession(ctx context.Context, tenantID, id string) (domain.AgentSession, bool, error)
	ListAgentSessionsByAccount(ctx context.Context, tenantID, accountID, status, agentID string) ([]domain.AgentSession, error)
	DeleteAgentSession(ctx context.Context, tenantID, id string) (domain.AgentSession, bool, error)
	InsertAgentSessionMessage(context.Context, domain.AgentSessionMessage) error
	ListAgentSessionMessages(ctx context.Context, tenantID, sessionID string) ([]domain.AgentSessionMessage, error)
	ListRecentAgentSessionMessages(ctx context.Context, tenantID, sessionID string, limit int) ([]domain.AgentSessionMessage, error)
	CountActiveAgentRunsBySession(ctx context.Context, tenantID, sessionID string) (int, error)
	UpsertAgentMemory(context.Context, domain.AgentMemory) error
	GetAgentMemory(ctx context.Context, tenantID, id string) (domain.AgentMemory, bool, error)
	ListAgentMemoriesByAccount(ctx context.Context, tenantID, accountID, agentID, sessionID string, limit int) ([]domain.AgentMemory, error)
	DeleteAgentMemory(ctx context.Context, tenantID, id string) (domain.AgentMemory, bool, error)
}

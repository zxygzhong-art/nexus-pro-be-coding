package repository

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"
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
	UpdateAgentModelTestResult(ctx context.Context, tenantID, id, status, message string, testedAt time.Time) (domain.AgentModel, bool, error)
	UpdateAgentModelSyncResult(ctx context.Context, tenantID, id string, status domain.AgentModelSyncStatus, lastError, configHash string, syncedAt *time.Time, updatedAt time.Time) (domain.AgentModel, bool, error)
	ListAgentDefinitionRefsByModel(ctx context.Context, tenantID, modelID string) ([]domain.AgentDefinitionRef, error)
	InsertAgentExternalTool(context.Context, domain.AgentExternalTool) error
	ListAgentExternalTools(ctx context.Context, tenantID string) ([]domain.AgentExternalTool, error)
	DeleteAgentExternalTool(ctx context.Context, tenantID, id string) (domain.AgentExternalTool, bool, error)
	UpsertAgentDefinition(context.Context, domain.AgentDefinition) error
	GetAgentDefinition(ctx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error)
	ListAgentDefinitions(ctx context.Context, tenantID string) ([]domain.AgentDefinition, error)
	ListPublishedAgentDefinitions(ctx context.Context, tenantID string) ([]domain.AgentDefinition, error)
	DeleteAgentDefinition(ctx context.Context, tenantID, id string) (domain.AgentDefinition, bool, error)
	UpdateAgentDefinitionUsage(ctx context.Context, tenantID, id string, success bool, latencyMs int, prompt string, runAt time.Time) (domain.AgentDefinition, bool, error)
	InsertAgentDefinitionVersion(context.Context, domain.AgentDefinitionVersion) error
	ListAgentDefinitionVersions(ctx context.Context, tenantID, agentID string) ([]domain.AgentDefinitionVersion, error)
	GetAgentDefinitionVersion(ctx context.Context, tenantID, agentID string, version int) (domain.AgentDefinitionVersion, bool, error)
	UpsertAgentSession(context.Context, domain.AgentSession) error
	GetAgentSession(ctx context.Context, tenantID, id string) (domain.AgentSession, bool, error)
	GetAgentSessionForUpdate(ctx context.Context, tenantID, id string) (domain.AgentSession, bool, error)
	ListAgentSessionsByAccount(ctx context.Context, tenantID, accountID, status, agentID string, page domain.KeysetPage) ([]domain.AgentSession, error)
	ListAgentUsageByAccount(ctx context.Context, tenantID string, query domain.AgentAccountUsageQuery, page domain.PageRequest) ([]domain.AgentAccountUsage, int, error)
	GetAgentUsageByAccount(ctx context.Context, tenantID, accountID string) (domain.AgentAccountUsage, bool, error)
	GetAgentUsageSummary(ctx context.Context, tenantID string) (domain.AgentUsageSummary, error)
	ListAgentUsageBySession(ctx context.Context, tenantID, accountID string, page domain.PageRequest) ([]domain.AgentSessionUsage, int, error)
	DeleteAgentSession(ctx context.Context, tenantID, id string) (domain.AgentSession, bool, error)
	InsertAgentSessionMessage(context.Context, domain.AgentSessionMessage) error
	ListAgentSessionMessages(ctx context.Context, tenantID, sessionID string, page domain.KeysetPage) ([]domain.AgentSessionMessage, error)
	ListRecentAgentSessionMessages(ctx context.Context, tenantID, sessionID string, limit int) ([]domain.AgentSessionMessage, error)
	UpsertAgentFileAsset(context.Context, domain.AgentSessionFile) error
	InsertAgentFileChunks(ctx context.Context, tenantID, fileID string, chunks []string, createdAt time.Time) error
	ListAgentFileChunks(ctx context.Context, tenantID, fileID string) ([]string, error)
	InsertAgentSessionFile(context.Context, domain.AgentSessionFile) error
	GetCurrentAgentSessionFile(ctx context.Context, tenantID, sessionID, fileID string) (domain.AgentSessionFile, bool, error)
	ListCurrentAgentSessionFiles(ctx context.Context, tenantID, sessionID string) ([]domain.AgentSessionFile, error)
	MarkAgentSessionFileAttached(ctx context.Context, tenantID, sessionID, fileID, messageID string, ordinal int, updatedAt time.Time) error
	ListCurrentAgentMessageAttachments(ctx context.Context, tenantID, sessionID string) ([]domain.AgentMessageAttachment, error)
	DeleteCurrentDraftAgentSessionFile(ctx context.Context, tenantID, sessionID, fileID string) (bool, error)
	DeleteAgentFileAsset(ctx context.Context, tenantID, fileID string) error
	FailStaleAgentRunsBySession(ctx context.Context, tenantID, sessionID string, staleBefore, failedAt time.Time, reason string) (int, error)
	CountActiveAgentRunsBySession(ctx context.Context, tenantID, sessionID string) (int, error)
	UpsertAgentMemory(context.Context, domain.AgentMemory) error
	GetAgentMemory(ctx context.Context, tenantID, id string) (domain.AgentMemory, bool, error)
	ListAgentMemoriesByAccount(ctx context.Context, tenantID, accountID, agentID, sessionID string, limit int) ([]domain.AgentMemory, error)
	DeleteAgentMemory(ctx context.Context, tenantID, id string) (domain.AgentMemory, bool, error)
}

// AgentV2Store defines persistence capabilities introduced by the normalized
// agent data model. It intentionally remains separate from AgentStore while
// legacy and v2 runtime paths coexist, so non-PostgreSQL compatibility stores
// do not have to implement unused v2 aggregates.
type AgentV2Store interface {
	GetAgentExternalTool(ctx context.Context, tenantID, id string) (domain.AgentExternalTool, bool, error)
	ReplaceAgentExternalToolCapabilities(ctx context.Context, tenantID, connectionID string, capabilities []domain.ExternalToolCapability) error
	UpdateAgentExternalToolTestResult(ctx context.Context, tenantID, id, status, message string, testedAt time.Time) (domain.AgentExternalTool, bool, error)
	GetAgentExternalToolCapability(ctx context.Context, tenantID, capabilityID string) (domain.ExternalToolCapability, bool, error)
	ListAgentExternalToolCapabilities(ctx context.Context, tenantID, connectionID string) ([]domain.ExternalToolCapability, error)
	ListAgentExternalToolCapabilitiesByIDs(ctx context.Context, tenantID string, capabilityIDs []string) ([]domain.ExternalToolCapability, error)
	ListAgentRevisionExternalToolBindings(ctx context.Context, tenantID, revisionID string) ([]domain.AgentRevisionExternalTool, error)

	UpsertExecutionStep(context.Context, domain.ExecutionStep) error
	AppendExecutionStep(context.Context, domain.ExecutionStep) (domain.ExecutionStep, error)
	GetExecutionStep(ctx context.Context, tenantID, id string) (domain.ExecutionStep, bool, error)
	ListExecutionSteps(ctx context.Context, tenantID, executionID string) ([]domain.ExecutionStep, error)

	UpsertAgentConfirmation(context.Context, domain.AgentConfirmationRecord) error
	ListPendingAgentConfirmations(ctx context.Context, tenantID, accountID, conversationID, segmentID string, now time.Time) ([]domain.AgentConfirmationRecord, error)
	ClaimAgentConfirmation(ctx context.Context, tenantID, accountID, id string, now time.Time) (domain.AgentConfirmationRecord, bool, error)
	UpdateAgentConfirmation(ctx context.Context, confirmation domain.AgentConfirmationRecord) (domain.AgentConfirmationRecord, bool, error)
}

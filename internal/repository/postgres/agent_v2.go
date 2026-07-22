package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus-pro-api/internal/domain"
	sqlc "nexus-pro-api/internal/platform/postgres/db"
	"nexus-pro-api/internal/repository"
)

var _ repository.AgentV2Store = (*Store)(nil)

// UpsertCredentialSecret persists encrypted credential material independently
// from the connection that references it.
func (s *Store) UpsertCredentialSecret(execCtx context.Context, item domain.CredentialSecret) error {
	_, err := s.q.UpsertCredentialSecretV2(tenantContext(execCtx, item.TenantID), sqlc.UpsertCredentialSecretV2Params{
		ID:                 item.ID,
		TenantID:           item.TenantID,
		Name:               item.Name,
		SecretType:         string(item.SecretType),
		Ciphertext:         item.Ciphertext,
		Preview:            item.Preview,
		Status:             string(item.Status),
		CreatedByAccountID: nullableText(item.CreatedByAccountID),
		CreatedAt:          timestamptz(item.CreatedAt),
		UpdatedAt:          timestamptz(item.UpdatedAt),
		RevokedAt:          nullableTimestamptz(item.RevokedAt),
	})
	return err
}

// GetCredentialSecret returns one tenant-owned encrypted secret.
func (s *Store) GetCredentialSecret(execCtx context.Context, tenantID, id string) (domain.CredentialSecret, bool, error) {
	item, err := s.q.GetCredentialSecretV2(tenantContext(execCtx, tenantID), sqlc.GetCredentialSecretV2Params{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.CredentialSecret{}, false, nil
	}
	if err != nil {
		return domain.CredentialSecret{}, false, err
	}
	return credentialSecretFromRow(item), true, nil
}

// RevokeCredentialSecret makes active credential material permanently unusable.
func (s *Store) RevokeCredentialSecret(execCtx context.Context, tenantID, id string, revokedAt time.Time) (domain.CredentialSecret, bool, error) {
	item, err := s.q.RevokeCredentialSecretV2(tenantContext(execCtx, tenantID), sqlc.RevokeCredentialSecretV2Params{
		TenantID: tenantID, ID: id, RevokedAt: timestamptz(revokedAt),
	})
	if isNotFound(err) {
		return domain.CredentialSecret{}, false, nil
	}
	if err != nil {
		return domain.CredentialSecret{}, false, err
	}
	return credentialSecretFromRow(item), true, nil
}

// GetAgentExternalTool returns one connection together with its active,
// persisted capability catalogue.
func (s *Store) GetAgentExternalTool(execCtx context.Context, tenantID, id string) (domain.AgentExternalTool, bool, error) {
	item, err := s.q.GetAgentExternalToolV2(tenantContext(execCtx, tenantID), sqlc.GetAgentExternalToolV2Params{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentExternalTool{}, false, nil
	}
	if err != nil {
		return domain.AgentExternalTool{}, false, err
	}
	out := agentExternalToolFromGetV2Row(item)
	out.Capabilities, err = s.listAgentExternalToolCapabilitiesAll(execCtx, tenantID, id)
	if err != nil {
		return domain.AgentExternalTool{}, false, err
	}
	return out, true, nil
}

// ReplaceAgentExternalToolCapabilities archives the previous active catalogue
// and upserts the freshly discovered operations. Callers that require the
// connection test update in the same unit of work use WithTenantTransaction.
func (s *Store) ReplaceAgentExternalToolCapabilities(execCtx context.Context, tenantID, connectionID string, capabilities []domain.ExternalToolCapability) error {
	archivedAt := time.Now().UTC()
	for _, capability := range capabilities {
		if capability.UpdatedAt.After(archivedAt) {
			archivedAt = capability.UpdatedAt.UTC()
		}
	}
	ctx := tenantContext(execCtx, tenantID)
	if err := s.q.ArchiveAgentExternalToolCapabilitiesV2(ctx, sqlc.ArchiveAgentExternalToolCapabilitiesV2Params{
		TenantID: tenantID, ConnectionID: connectionID, ArchivedAt: timestamptz(archivedAt),
	}); err != nil {
		return err
	}
	for _, capability := range capabilities {
		if _, err := s.q.UpsertAgentExternalToolCapabilityV2(ctx, sqlc.UpsertAgentExternalToolCapabilityV2Params{
			ID:             capability.ID,
			TenantID:       tenantID,
			ConnectionID:   connectionID,
			ToolName:       capability.ToolName,
			Description:    capability.Description,
			HttpMethod:     capability.HTTPMethod,
			HttpPath:       capability.HTTPPath,
			InputSchema:    mustJSON(capability.InputSchema),
			OutputSchema:   mustJSON(capability.OutputSchema),
			Readonly:       capability.Readonly,
			Enabled:        capability.Enabled,
			SchemaChecksum: capability.SchemaChecksum,
			DiscoveredAt:   timestamptz(capability.DiscoveredAt),
			UpdatedAt:      timestamptz(capability.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

// UpdateAgentExternalToolTestResult persists the latest endpoint test.
func (s *Store) UpdateAgentExternalToolTestResult(execCtx context.Context, tenantID, id, status, message string, testedAt time.Time) (domain.AgentExternalTool, bool, error) {
	item, err := s.q.UpdateAgentExternalToolTestResultV2(tenantContext(execCtx, tenantID), sqlc.UpdateAgentExternalToolTestResultV2Params{
		TenantID: tenantID, ID: id, LastTestStatus: status, LastTestMessage: message, LastTestedAt: timestamptz(testedAt),
	})
	if isNotFound(err) {
		return domain.AgentExternalTool{}, false, nil
	}
	if err != nil {
		return domain.AgentExternalTool{}, false, err
	}
	out := agentExternalToolFromTestV2Row(item)
	out.Capabilities, err = s.listAgentExternalToolCapabilitiesAll(execCtx, tenantID, id)
	if err != nil {
		return domain.AgentExternalTool{}, false, err
	}
	return out, true, nil
}

// GetAgentExternalToolCapability returns one current discovered operation.
func (s *Store) GetAgentExternalToolCapability(execCtx context.Context, tenantID, capabilityID string) (domain.ExternalToolCapability, bool, error) {
	item, err := s.q.GetAgentExternalToolCapabilityV2(tenantContext(execCtx, tenantID), sqlc.GetAgentExternalToolCapabilityV2Params{TenantID: tenantID, ID: capabilityID})
	if isNotFound(err) {
		return domain.ExternalToolCapability{}, false, nil
	}
	if err != nil {
		return domain.ExternalToolCapability{}, false, err
	}
	return externalToolCapabilityFromRow(item), true, nil
}

// ListAgentExternalToolCapabilities lists current operations for one connection.
func (s *Store) ListAgentExternalToolCapabilities(execCtx context.Context, tenantID, connectionID string) ([]domain.ExternalToolCapability, error) {
	items, err := s.q.ListAgentExternalToolCapabilitiesV2(tenantContext(execCtx, tenantID), sqlc.ListAgentExternalToolCapabilitiesV2Params{
		TenantID: tenantID, ConnectionID: connectionID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, externalToolCapabilityFromRow), nil
}

func (s *Store) listAgentExternalToolCapabilitiesAll(execCtx context.Context, tenantID, connectionID string) ([]domain.ExternalToolCapability, error) {
	items, err := s.q.ListAgentExternalToolCapabilitiesAllV2(tenantContext(execCtx, tenantID), sqlc.ListAgentExternalToolCapabilitiesAllV2Params{
		TenantID: tenantID, ConnectionID: connectionID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, externalToolCapabilityFromRow), nil
}

// ListAgentExternalToolCapabilitiesByIDs batch-loads revision-bound operations.
func (s *Store) ListAgentExternalToolCapabilitiesByIDs(execCtx context.Context, tenantID string, capabilityIDs []string) ([]domain.ExternalToolCapability, error) {
	if len(capabilityIDs) == 0 {
		return []domain.ExternalToolCapability{}, nil
	}
	items, err := s.q.ListAgentExternalToolCapabilitiesByIDsV2(tenantContext(execCtx, tenantID), sqlc.ListAgentExternalToolCapabilitiesByIDsV2Params{
		TenantID: tenantID, Ids: capabilityIDs,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, externalToolCapabilityFromRow), nil
}

// ListAgentRevisionExternalToolBindings returns immutable root bindings and
// their publish-time schema checksums.
func (s *Store) ListAgentRevisionExternalToolBindings(execCtx context.Context, tenantID, revisionID string) ([]domain.AgentRevisionExternalTool, error) {
	items, err := s.q.ListAgentRevisionExternalToolBindingsV2(tenantContext(execCtx, tenantID), sqlc.ListAgentRevisionExternalToolBindingsV2Params{
		TenantID: tenantID, RevisionID: revisionID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentRevisionExternalTool, 0, len(items))
	for _, item := range items {
		out = append(out, domain.AgentRevisionExternalTool{
			TenantID: item.TenantID, RevisionID: item.RevisionID, ExternalToolID: item.ExternalToolID,
			ToolSchemaChecksum: item.ToolSchemaChecksum, Ordinal: int(item.Ordinal), Config: jsonMap(item.Config),
		})
	}
	return out, nil
}

// ListAgentRevisionMemberExternalToolBindings returns immutable member
// bindings and their publish-time schema checksums.
func (s *Store) ListAgentRevisionMemberExternalToolBindings(execCtx context.Context, tenantID, revisionID string) ([]domain.AgentRevisionMemberExternalTool, error) {
	items, err := s.q.ListAgentRevisionMemberExternalToolBindingsV2(tenantContext(execCtx, tenantID), sqlc.ListAgentRevisionMemberExternalToolBindingsV2Params{
		TenantID: tenantID, RevisionID: revisionID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentRevisionMemberExternalTool, 0, len(items))
	for _, item := range items {
		out = append(out, domain.AgentRevisionMemberExternalTool{
			TenantID: item.TenantID, RevisionID: item.RevisionID, MemberID: item.MemberID,
			ExternalToolID: item.ExternalToolID, ToolSchemaChecksum: item.ToolSchemaChecksum,
			Ordinal: int(item.Ordinal), Config: jsonMap(item.Config),
		})
	}
	return out, nil
}

// UpsertExecutionStep persists one observable orchestration step.
func (s *Store) UpsertExecutionStep(execCtx context.Context, item domain.ExecutionStep) error {
	_, err := s.q.UpsertExecutionStepV2(tenantContext(execCtx, item.TenantID), sqlc.UpsertExecutionStepV2Params{
		ID: item.ID, TenantID: item.TenantID, ExecutionID: item.ExecutionID,
		ParentStepID: item.ParentStepID, SequenceNo: int32(item.SequenceNo), StepType: string(item.StepType), Name: item.Name,
		ModelConnectionID: item.ModelConnectionID, ExternalToolID: item.ExternalToolID, Status: string(item.Status),
		InputSummary: mustJSON(item.InputSummary), OutputSummary: mustJSON(item.OutputSummary),
		InputTokens: item.InputTokens, CachedTokens: item.CachedTokens, OutputTokens: item.OutputTokens,
		StartedAt: nullableTimestamptz(item.StartedAt), CompletedAt: nullableTimestamptz(item.CompletedAt),
		ErrorCode: item.ErrorCode, CreatedAt: timestamptz(item.CreatedAt),
	})
	return err
}

// AppendExecutionStep locks the owning execution, allocates the next sequence,
// and inserts a running step in one statement.
func (s *Store) AppendExecutionStep(execCtx context.Context, item domain.ExecutionStep) (domain.ExecutionStep, error) {
	stored, err := s.q.AppendExecutionStepV2(tenantContext(execCtx, item.TenantID), sqlc.AppendExecutionStepV2Params{
		ID: item.ID, TenantID: item.TenantID, ExecutionID: item.ExecutionID,
		ParentStepID: item.ParentStepID, StepType: string(item.StepType), Name: item.Name,
		ModelConnectionID: item.ModelConnectionID, ExternalToolID: item.ExternalToolID,
		InputSummary: mustJSON(item.InputSummary), StartedAt: nullableTimestamptz(item.StartedAt),
		CreatedAt: timestamptz(item.CreatedAt),
	})
	if err != nil {
		return domain.ExecutionStep{}, err
	}
	return executionStepFromRow(stored), nil
}

// GetExecutionStep returns one step within the tenant boundary.
func (s *Store) GetExecutionStep(execCtx context.Context, tenantID, id string) (domain.ExecutionStep, bool, error) {
	item, err := s.q.GetExecutionStepV2(tenantContext(execCtx, tenantID), sqlc.GetExecutionStepV2Params{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.ExecutionStep{}, false, nil
	}
	if err != nil {
		return domain.ExecutionStep{}, false, err
	}
	return executionStepFromRow(item), true, nil
}

// ListExecutionSteps returns ordered steps for one execution.
func (s *Store) ListExecutionSteps(execCtx context.Context, tenantID, executionID string) ([]domain.ExecutionStep, error) {
	items, err := s.q.ListExecutionStepsV2(tenantContext(execCtx, tenantID), sqlc.ListExecutionStepsV2Params{TenantID: tenantID, ExecutionID: executionID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, executionStepFromRow), nil
}

// UpsertAgentConfirmation stores a one-time action and its protected payload.
func (s *Store) UpsertAgentConfirmation(execCtx context.Context, item domain.AgentConfirmationRecord) error {
	stored, err := s.q.UpsertAgentConfirmationV2(tenantContext(execCtx, item.TenantID), sqlc.UpsertAgentConfirmationV2Params{
		ID: item.ID, TenantID: item.TenantID, AccountID: item.AccountID,
		ConversationID: item.ConversationID, SegmentID: item.SegmentID,
		ExecutionID: item.ExecutionID, SourceMessageID: item.SourceMessageID,
		Kind: item.Kind, Title: item.Title, Action: item.Action,
		PublicPayload: mustJSON(item.PublicPayload), ActionPayload: mustJSON(item.ActionPayload), ResultPayload: mustJSON(item.ResultPayload),
		Status: string(item.Status), LastError: item.LastError, ExpiresAt: timestamptz(item.ExpiresAt),
		ConsumedAt: nullableTimestamptz(item.ConsumedAt), CreatedAt: timestamptz(item.CreatedAt), UpdatedAt: timestamptz(item.UpdatedAt),
	})
	if err != nil {
		return err
	}
	if stored.AccountID != item.AccountID || stored.ConversationID != item.ConversationID || stored.SegmentID != item.SegmentID || stored.Kind != item.Kind || stored.Action != item.Action {
		return domain.Conflict("agent confirmation id already belongs to a different action")
	}
	return nil
}

// ListPendingAgentConfirmations restores only current-segment, unexpired work.
func (s *Store) ListPendingAgentConfirmations(execCtx context.Context, tenantID, accountID, conversationID, segmentID string, now time.Time) ([]domain.AgentConfirmationRecord, error) {
	items, err := s.q.ListPendingAgentConfirmationsV2(tenantContext(execCtx, tenantID), sqlc.ListPendingAgentConfirmationsV2Params{
		TenantID: tenantID, AccountID: accountID, ConversationID: conversationID, SegmentID: segmentID, NowAt: timestamptz(now),
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, agentConfirmationFromRow), nil
}

// ClaimAgentConfirmation atomically expires or claims one pending record, and
// only while its bound segment remains the conversation's current segment.
func (s *Store) ClaimAgentConfirmation(execCtx context.Context, tenantID, accountID, id string, now time.Time) (domain.AgentConfirmationRecord, bool, error) {
	item, err := s.q.ClaimAgentConfirmationV2(tenantContext(execCtx, tenantID), sqlc.ClaimAgentConfirmationV2Params{
		TenantID: tenantID, AccountID: accountID, ID: id, NowAt: timestamptz(now),
	})
	if isNotFound(err) {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	if err != nil {
		return domain.AgentConfirmationRecord{}, false, err
	}
	return agentConfirmationFromFields(
		item.ID, item.TenantID, item.AccountID, item.ConversationID, item.SegmentID,
		item.ExecutionID, item.SourceMessageID, item.Kind, item.Title, item.Action,
		item.PublicPayload, item.ActionPayload, item.ResultPayload, item.Status, item.LastError,
		item.ExpiresAt, item.ConsumedAt, item.CreatedAt, item.UpdatedAt,
	), true, nil
}

// UpdateAgentConfirmation applies the SQL-enforced confirmation state machine.
func (s *Store) UpdateAgentConfirmation(execCtx context.Context, item domain.AgentConfirmationRecord) (domain.AgentConfirmationRecord, bool, error) {
	stored, err := s.q.UpdateAgentConfirmationV2(tenantContext(execCtx, item.TenantID), sqlc.UpdateAgentConfirmationV2Params{
		TenantID: item.TenantID, AccountID: item.AccountID, ID: item.ID,
		ResultPayload: mustJSON(item.ResultPayload), Status: string(item.Status), LastError: item.LastError,
		ConsumedAt: nullableTimestamptz(item.ConsumedAt), UpdatedAt: timestamptz(item.UpdatedAt),
	})
	if isNotFound(err) {
		return domain.AgentConfirmationRecord{}, false, nil
	}
	if err != nil {
		return domain.AgentConfirmationRecord{}, false, err
	}
	return agentConfirmationFromRow(stored), true, nil
}

func credentialSecretFromRow(item sqlc.CredentialSecret) domain.CredentialSecret {
	return domain.CredentialSecret{
		ID: item.ID, TenantID: item.TenantID, Name: item.Name,
		SecretType: domain.CredentialSecretType(item.SecretType), Ciphertext: item.Ciphertext, Preview: item.Preview,
		Status: domain.CredentialSecretStatus(item.Status), CreatedByAccountID: textFrom(item.CreatedByAccountID),
		CreatedAt: timeFrom(item.CreatedAt), UpdatedAt: timeFrom(item.UpdatedAt), RevokedAt: timePtrFrom(item.RevokedAt),
	}
}

func agentExternalToolFromGetV2Row(item sqlc.GetAgentExternalToolV2Row) domain.AgentExternalTool {
	return agentExternalToolFromFields(
		item.ID, item.TenantID, item.Name, item.Description, item.Kind, item.Transport, item.EndpointUrl,
		item.AuthType, item.AuthHeaderName, item.AuthUsername, item.AuthSecretCiphertext, textFrom(item.CredentialSecretID),
		item.TimeoutSeconds, item.Status, item.LastTestedAt, item.LastTestStatus, item.LastTestMessage,
		textFrom(item.CreatedByAccountID), item.CreatedAt, item.UpdatedAt, item.ArchivedAt,
	)
}

func agentExternalToolFromTestV2Row(item sqlc.UpdateAgentExternalToolTestResultV2Row) domain.AgentExternalTool {
	return agentExternalToolFromFields(
		item.ID, item.TenantID, item.Name, item.Description, item.Kind, item.Transport, item.EndpointUrl,
		item.AuthType, item.AuthHeaderName, item.AuthUsername, item.AuthSecretCiphertext, textFrom(item.CredentialSecretID),
		item.TimeoutSeconds, item.Status, item.LastTestedAt, item.LastTestStatus, item.LastTestMessage,
		textFrom(item.CreatedByAccountID), item.CreatedAt, item.UpdatedAt, item.ArchivedAt,
	)
}

func externalToolCapabilityFromRow(item sqlc.ExternalTool) domain.ExternalToolCapability {
	return domain.ExternalToolCapability{
		ID: item.ID, TenantID: item.TenantID, ConnectionID: item.ConnectionID,
		ToolName: item.ToolName, Description: item.Description, HTTPMethod: item.HttpMethod, HTTPPath: item.HttpPath,
		InputSchema: jsonMap(item.InputSchema), OutputSchema: jsonMap(item.OutputSchema),
		Readonly: item.Readonly, Enabled: item.Enabled, SchemaChecksum: item.SchemaChecksum,
		DiscoveredAt: timeFrom(item.DiscoveredAt), UpdatedAt: timeFrom(item.UpdatedAt), ArchivedAt: timePtrFrom(item.ArchivedAt),
	}
}

func executionStepFromRow(item sqlc.ExecutionStep) domain.ExecutionStep {
	return domain.ExecutionStep{
		ID: item.ID, TenantID: item.TenantID, ExecutionID: item.ExecutionID,
		ParentStepID: textFrom(item.ParentStepID), SequenceNo: int(item.SequenceNo), StepType: domain.ExecutionStepType(item.StepType),
		Name: item.Name, ModelConnectionID: textFrom(item.ModelConnectionID), ExternalToolID: textFrom(item.ExternalToolID),
		Status: domain.ExecutionStepStatus(item.Status), InputSummary: jsonMap(item.InputSummary), OutputSummary: jsonMap(item.OutputSummary),
		InputTokens: item.InputTokens, CachedTokens: item.CachedTokens, OutputTokens: item.OutputTokens,
		StartedAt: timePtrFrom(item.StartedAt), CompletedAt: timePtrFrom(item.CompletedAt), ErrorCode: item.ErrorCode,
		CreatedAt: timeFrom(item.CreatedAt),
	}
}

func agentConfirmationFromRow(item sqlc.AgentConfirmation) domain.AgentConfirmationRecord {
	return agentConfirmationFromFields(
		item.ID, item.TenantID, item.AccountID, item.ConversationID, item.SegmentID,
		item.ExecutionID, item.SourceMessageID, item.Kind, item.Title, item.Action,
		item.PublicPayload, item.ActionPayload, item.ResultPayload, item.Status, item.LastError,
		item.ExpiresAt, item.ConsumedAt, item.CreatedAt, item.UpdatedAt,
	)
}

func agentConfirmationFromFields(
	id, tenantID, accountID, conversationID, segmentID string,
	executionID, sourceMessageID pgtype.Text,
	kind, title, action string,
	publicPayload, actionPayload, resultPayload []byte,
	status, lastError string,
	expiresAt, consumedAt, createdAt, updatedAt pgtype.Timestamptz,
) domain.AgentConfirmationRecord {
	return domain.AgentConfirmationRecord{
		ID: id, TenantID: tenantID, AccountID: accountID, ConversationID: conversationID, SegmentID: segmentID,
		ExecutionID: textFrom(executionID), SourceMessageID: textFrom(sourceMessageID), Kind: kind, Title: title, Action: action,
		PublicPayload: jsonMap(publicPayload), ActionPayload: jsonMap(actionPayload), ResultPayload: jsonMap(resultPayload),
		Status: domain.AgentConfirmationStatus(status), LastError: lastError,
		ExpiresAt: timeFrom(expiresAt), ConsumedAt: timePtrFrom(consumedAt), CreatedAt: timeFrom(createdAt), UpdatedAt: timeFrom(updatedAt),
	}
}

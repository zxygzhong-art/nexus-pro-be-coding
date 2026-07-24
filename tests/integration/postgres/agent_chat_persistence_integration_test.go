package postgres_integration_test

import (
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	postgresrepo "nexus-pro-api/internal/repository/postgres"
)

// TestPostgresAgentChatPersistsOneInputMessageAndExecution covers the exact
// write order used by AgentService.Chat: persist the user message first, then
// create the execution that references that message in the same transaction.
func TestPostgresAgentChatPersistsOneInputMessageAndExecution(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)

	store := postgresrepo.NewStore(pool)
	now := time.Now().UTC()
	suffix := strings.ToLower(t.Name()) + "_" + now.Format("150405000000")
	tenantID := "tenant_" + suffix
	accountID := "acct_" + suffix
	modelID := "amodel_" + suffix
	agentID := "agent_" + suffix
	revisionID := "arev_" + suffix
	sessionID := "asess_" + suffix
	segmentID := "aseg_" + suffix
	messageID := "amsg_" + suffix
	answerMessageID := "amsg_" + suffix + "_answer"
	runID := "arun_" + suffix
	ctx := tenantScopedContext(tenantID)
	defer func() {
		if _, err := pool.Exec(ctx, "DELETE FROM tenants WHERE id = $1", tenantID); err != nil {
			t.Errorf("clean up test tenant: %v", err)
		}
	}()

	if _, err := pool.Exec(ctx, `
INSERT INTO tenants (id, name, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO NOTHING
`, tenantID, "Agent persistence tenant", now); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{
		ID: accountID, TenantID: tenantID, DisplayName: "Agent User",
		Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentModel(ctx, domain.AgentModel{
		ID: modelID, TenantID: tenantID, Name: "Test model",
		Provider: "openai", ModelName: "gpt-4o-mini",
		Status: domain.AgentModelStatusActive, TimeoutSeconds: 30,
		LastTestStatus: "untested", SyncStatus: domain.AgentModelSyncStatusSynced,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentDefinition(ctx, domain.AgentDefinition{
		ID: agentID, TenantID: tenantID,
		DraftRevisionID: revisionID, PublishedRevisionID: revisionID,
		Name: "Persistence Agent", Category: domain.AgentCategoryIT,
		ModelID: modelID, SystemPrompt: "Be helpful.",
		Status:         domain.AgentDefinitionStatusPublished,
		Visibility:     domain.AgentVisibilityAll,
		TimeoutSeconds: 30, Version: 1, PublishedVersion: 1,
		CreatedByAccountID: accountID,
		CreatedAt:          now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgentSession(ctx, domain.AgentSession{
		ID: sessionID, TenantID: tenantID, AccountID: accountID,
		AgentID: agentID, SegmentID: segmentID, Title: "Persistence",
		Status: domain.AgentSessionStatusActive, ContextVersion: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	err := store.WithTenantTransaction(ctx, tenantID, func(tx repository.Store) error {
		if err := tx.InsertAgentSessionMessage(ctx, domain.AgentSessionMessage{
			ID: messageID, TenantID: tenantID, SessionID: sessionID,
			SegmentID: segmentID, Role: domain.AgentMessageRoleUser,
			Content: "hello", ContextVersion: 1, CreatedAt: now,
		}); err != nil {
			return err
		}
		return tx.UpsertAgentRun(ctx, domain.AgentRun{
			ID: runID, TenantID: tenantID, AccountID: accountID,
			AgentID: agentID, AgentRevisionID: revisionID,
			ModelConnectionID: modelID, SessionID: sessionID,
			SegmentID: segmentID, InputMessageID: messageID,
			Mode: "assistant", Prompt: "hello",
			Status:    string(domain.AgentRunStatusQueued),
			CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("persist message and execution: %v", err)
	}

	messages, err := store.ListAgentSessionMessages(ctx, tenantID, sessionID, domain.KeysetPage{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != messageID {
		t.Fatalf("expected exactly the original input message, got %+v", messages)
	}
	runs, err := store.ListAgentRunsByAccount(ctx, tenantID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != runID || runs[0].InputMessageID != messageID {
		t.Fatalf("expected execution to reference the original input message, got %+v", runs)
	}

	completedAt := now.Add(time.Second)
	err = store.WithTenantTransaction(ctx, tenantID, func(tx repository.Store) error {
		if err := tx.InsertAgentSessionMessage(ctx, domain.AgentSessionMessage{
			ID: answerMessageID, TenantID: tenantID, SessionID: sessionID,
			SegmentID: segmentID, Role: domain.AgentMessageRoleAssistant,
			Content: "hello back", RunID: runID,
			ContextVersion: 1, CreatedAt: completedAt,
		}); err != nil {
			return err
		}
		return tx.UpsertAgentRun(ctx, domain.AgentRun{
			ID: runID, TenantID: tenantID, AccountID: accountID,
			AgentID: agentID, AgentRevisionID: revisionID,
			ModelConnectionID: modelID, SessionID: sessionID,
			SegmentID: segmentID, InputMessageID: messageID,
			Mode: "assistant", Prompt: "hello", Answer: "hello back",
			Status:       string(domain.AgentRunStatusCompleted),
			LLMCallCount: 1, InputTokens: 3, OutputTokens: 2, TotalTokens: 5,
			UsageComplete: true,
			CreatedAt:     now, UpdatedAt: completedAt,
		})
	})
	if err != nil {
		t.Fatalf("persist answer and complete execution: %v", err)
	}

	messages, err = store.ListAgentSessionMessages(ctx, tenantID, sessionID, domain.KeysetPage{})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != messageID || messages[1].ID != answerMessageID {
		t.Fatalf("expected one user and one assistant message, got %+v", messages)
	}
	runs, err = store.ListAgentRunsByAccount(ctx, tenantID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != string(domain.AgentRunStatusCompleted) || runs[0].TotalTokens != 5 {
		t.Fatalf("expected completed execution with final usage, got %+v", runs)
	}
	recovered, err := store.FailStaleAgentRunsBySession(
		ctx, tenantID, sessionID, completedAt.Add(time.Hour), completedAt.Add(2*time.Hour), "interrupted",
	)
	if err != nil {
		t.Fatalf("scan stale executions: %v", err)
	}
	if recovered != 0 {
		t.Fatalf("completed execution must not be recovered as stale, got %d", recovered)
	}
}

package db_test

import (
	"os"
	"strings"
	"testing"
)

func readSchemaSQL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func readPostInitMigration(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../../db/migrations/000002_post_init_updates.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestAgentV2SchemaReplacesLegacyAgentTables keeps the squashed schema free of
// the pre-v2 agent persistence tables while retaining shared knowledge/files.
func TestAgentV2SchemaReplacesLegacyAgentTables(t *testing.T) {
	schema := readSchemaSQL(t)
	for _, legacy := range []string{
		"CREATE TABLE agent_message_attachments",
		"CREATE TABLE agent_session_files",
		"CREATE TABLE agent_session_messages",
		"CREATE TABLE agent_runs",
		"CREATE TABLE agent_sessions",
		"CREATE TABLE agent_definition_versions",
		"CREATE TABLE agent_definitions",
		"CREATE TABLE agent_external_tools",
		"CREATE TABLE agent_models",
	} {
		if strings.Contains(schema, legacy) {
			t.Fatalf("final schema must not retain legacy agent table %q", legacy)
		}
	}
	for _, required := range []string{
		"CREATE TABLE agents (",
		"CREATE TABLE agent_revisions (",
		"CREATE TABLE conversations (",
		"CREATE TABLE agent_memories (",
		"CREATE TABLE knowledge_bases (",
		"CREATE TABLE file_assets (",
		"CREATE TABLE file_chunks (",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("final schema is missing %q", required)
		}
	}
	migration := readPostInitMigration(t)
	if !strings.Contains(migration, "is irreversible because it is a squashed net schema snapshot") {
		t.Fatal("squashed post-init migration must reject a misleading down migration")
	}
}

func TestAgentV2QueriesPreservePublishedBindingsAndMemoryScopes(t *testing.T) {
	adminRaw, err := os.ReadFile("../../../db/queries/agent_admin.sql")
	if err != nil {
		t.Fatal(err)
	}
	admin := string(adminRaw)
	for _, fragment := range []string{
		"agent_revision_external_tools",
		"target_child_revisions",
		"':member:'",
		"external_tools.schema_checksum",
		"model_config_checksum = EXCLUDED.model_config_checksum",
		"published_agent.published_revision_id = agent_revisions.id",
	} {
		if !strings.Contains(admin, fragment) {
			t.Fatalf("agent revision persistence is missing %q", fragment)
		}
	}

	sessionRaw, err := os.ReadFile("../../../db/queries/agent_sessions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sessions := string(sessionRaw)
	for _, fragment := range []string{
		"agent_memories.scope_type = 'global'",
		"agent_memories.scope_type = 'agent'",
		"agent_memories.scope_type = 'conversation'",
		"conversations.current_segment_id",
		"source_message_id = EXCLUDED.source_message_id",
		"confidence = EXCLUDED.confidence",
		"ON CONFLICT (tenant_id, account_id, scope_type, agent_id, conversation_id, segment_id, key)",
	} {
		if !strings.Contains(sessions, fragment) {
			t.Fatalf("agent memory persistence is missing %q", fragment)
		}
	}
}

func TestConversationFilesInlineAttachmentColumnsStayInSchema(t *testing.T) {
	schema := readSchemaSQL(t)
	for _, fragment := range []string{
		"CREATE TABLE conversation_files (",
		"message_id text",
		"ordinal integer",
		"conversation_files_attachment_state_check",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("conversation file attachment schema is missing %q", fragment)
		}
	}
	if strings.Contains(schema, "CREATE TABLE conversation_message_attachments") {
		t.Fatal("final schema must not retain conversation_message_attachments")
	}
}

func TestModelAndToolSecretsAreInlinedInSchema(t *testing.T) {
	schema := readSchemaSQL(t)
	for _, fragment := range []string{
		"CREATE TABLE conversation_messages (",
		"CREATE TABLE conversation_executions (",
		"CREATE TABLE conversation_execution_steps (",
		"api_key_ciphertext text NOT NULL DEFAULT ''",
		"auth_secret_ciphertext text NOT NULL DEFAULT ''",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("inlined secret schema is missing %q", fragment)
		}
	}
	for _, removed := range []string{
		"CREATE TABLE credential_secrets",
		"credential_secret_id",
	} {
		if strings.Contains(schema, removed) {
			t.Fatalf("final schema must not retain %q", removed)
		}
	}
}

func TestAgentMemoriesAreCanonicalMemoryTable(t *testing.T) {
	schema := readSchemaSQL(t)
	for _, fragment := range []string{
		"CREATE TABLE agent_memories (",
		"CREATE POLICY tenant_isolation_agent_memories ON agent_memories",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("agent_memories schema is missing %q", fragment)
		}
	}
	if strings.Contains(schema, "CREATE TABLE memories (") {
		t.Fatal("final schema must not retain legacy memories table name")
	}
}

func TestAgentMemberUnificationRemovesMemberTables(t *testing.T) {
	schema := readSchemaSQL(t)
	for _, required := range []string{
		"parent_agent_id text",
		"FOREIGN KEY (tenant_id, parent_agent_id)",
		"ordinal integer CHECK (ordinal >= 0)",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("final schema is missing unified agent hierarchy fragment %q", required)
		}
	}
	for _, removed := range []string{
		"CREATE TABLE agent_revision_members",
		"CREATE TABLE agent_revision_member_builtin_tools",
		"CREATE TABLE agent_revision_member_external_tools",
		"CREATE TABLE agent_revision_member_knowledge_bases",
	} {
		if strings.Contains(schema, removed) {
			t.Fatalf("final schema must not retain %q", removed)
		}
	}
}

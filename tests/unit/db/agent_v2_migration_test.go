package db_test

import (
	"os"
	"strings"
	"testing"
)

// TestAgentV2MigrationIsExplicitlyDestructive documents the accepted cutover:
// legacy Agent rows are discarded while shared knowledge and file assets stay.
func TestAgentV2MigrationIsExplicitlyDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../../db/migrations/000020_agent_data_model_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	for _, table := range []string{
		"agent_message_attachments",
		"agent_session_files",
		"agent_memories",
		"agent_session_messages",
		"agent_runs",
		"agent_sessions",
		"agent_definition_versions",
		"agent_definitions",
		"agent_external_tools",
		"agent_models",
	} {
		if !strings.Contains(migration, "DROP TABLE IF EXISTS "+table+";") {
			t.Fatalf("Agent v2 cutover must explicitly drop %s", table)
		}
	}
	for _, sharedTable := range []string{"knowledge_bases", "file_assets", "file_chunks"} {
		if strings.Contains(migration, "DROP TABLE IF EXISTS "+sharedTable) {
			t.Fatalf("Agent v2 cutover must preserve shared table %s", sharedTable)
		}
	}
	if !strings.Contains(migration, "is irreversible because legacy Agent data was discarded") {
		t.Fatal("destructive Agent v2 migration must reject a misleading down migration")
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
		"agent_revision_member_external_tools",
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
		"memories.scope_type = 'global'",
		"memories.scope_type = 'agent'",
		"memories.scope_type = 'conversation'",
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

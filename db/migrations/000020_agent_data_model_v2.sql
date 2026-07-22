-- +goose Up
-- Agent data model v2 intentionally discards the legacy Agent control-plane,
-- conversation, execution, and memory rows. Shared knowledge and file assets
-- remain intact.

DROP TABLE IF EXISTS agent_message_attachments;
DROP TABLE IF EXISTS agent_session_files;
DROP TABLE IF EXISTS agent_memories;
DROP TABLE IF EXISTS agent_session_messages;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS agent_definition_versions;
DROP TABLE IF EXISTS agent_definitions;
DROP TABLE IF EXISTS agent_external_tools;
DROP TABLE IF EXISTS agent_models;

CREATE TABLE credential_secrets (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL DEFAULT '',
    secret_type text NOT NULL CHECK (secret_type IN ('api_key', 'bearer', 'basic_password')),
    ciphertext text NOT NULL,
    preview text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT credential_secrets_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT credential_secrets_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT credential_secrets_revoked_at_check CHECK (
        (status = 'active' AND revoked_at IS NULL) OR
        (status = 'revoked' AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX credential_secrets_tenant_status_idx ON credential_secrets (tenant_id, status, updated_at DESC, id);

CREATE TABLE model_connections (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    provider text NOT NULL DEFAULT 'openai',
    upstream_model text NOT NULL,
    api_base_url text NOT NULL DEFAULT '',
    credential_secret_id text,
    rate_limit_rpm integer NOT NULL DEFAULT 0 CHECK (rate_limit_rpm >= 0),
    timeout_ms integer NOT NULL DEFAULT 60000 CHECK (timeout_ms > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'archived')),
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT model_connections_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT model_connections_secret_fk FOREIGN KEY (tenant_id, credential_secret_id) REFERENCES credential_secrets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT model_connections_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT model_connections_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT model_connections_archived_at_check CHECK (
        (status <> 'archived' AND archived_at IS NULL) OR
        (status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE INDEX model_connections_tenant_status_idx ON model_connections (tenant_id, status, updated_at DESC, id);
CREATE INDEX model_connections_tenant_name_idx ON model_connections (tenant_id, name);

CREATE TABLE model_connection_state (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    model_connection_id text NOT NULL,
    sync_status text NOT NULL DEFAULT 'pending' CHECK (sync_status IN ('pending', 'synced', 'failed')),
    synced_config_checksum text NOT NULL DEFAULT '',
    last_synced_at timestamptz,
    last_sync_error text NOT NULL DEFAULT '',
    last_tested_at timestamptz,
    last_test_status text NOT NULL DEFAULT 'untested' CHECK (last_test_status IN ('ok', 'failed', 'untested')),
    last_test_message text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, model_connection_id),
    CONSTRAINT model_connection_state_connection_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE external_tool_connections (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK (kind IN ('mcp', 'http')),
    transport text NOT NULL CHECK (transport IN ('sse', 'streamable_http', 'http')),
    endpoint_url text NOT NULL,
    auth_type text NOT NULL DEFAULT 'none' CHECK (auth_type IN ('none', 'bearer', 'api_key', 'basic')),
    auth_header_name text NOT NULL DEFAULT '',
    auth_username text NOT NULL DEFAULT '',
    credential_secret_id text,
    timeout_ms integer NOT NULL DEFAULT 30000 CHECK (timeout_ms BETWEEN 1000 AND 120000),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'archived')),
    last_tested_at timestamptz,
    last_test_status text NOT NULL DEFAULT 'untested' CHECK (last_test_status IN ('ok', 'failed', 'untested')),
    last_test_message text NOT NULL DEFAULT '',
    created_by_account_id text,
    updated_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT external_tool_connections_transport_kind_check CHECK (
        (kind = 'mcp' AND transport IN ('sse', 'streamable_http')) OR
        (kind = 'http' AND transport = 'http')
    ),
    CONSTRAINT external_tool_connections_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT external_tool_connections_secret_fk FOREIGN KEY (tenant_id, credential_secret_id) REFERENCES credential_secrets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT external_tool_connections_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT external_tool_connections_updated_by_fk FOREIGN KEY (tenant_id, updated_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT external_tool_connections_archived_at_check CHECK (
        (status <> 'archived' AND archived_at IS NULL) OR
        (status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE INDEX external_tool_connections_tenant_status_idx ON external_tool_connections (tenant_id, status, updated_at DESC, id);

CREATE TABLE external_tools (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    connection_id text NOT NULL,
    tool_name text NOT NULL,
    description text NOT NULL DEFAULT '',
    http_method text NOT NULL DEFAULT '' CHECK (http_method IN ('', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
    http_path text NOT NULL DEFAULT '',
    input_schema jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input_schema) = 'object'),
    output_schema jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_schema) = 'object'),
    readonly boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    schema_checksum text NOT NULL DEFAULT '',
    discovered_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT external_tools_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT external_tools_connection_name_idx UNIQUE (tenant_id, connection_id, tool_name),
    CONSTRAINT external_tools_connection_fk FOREIGN KEY (tenant_id, connection_id) REFERENCES external_tool_connections (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX external_tools_tenant_connection_idx ON external_tools (tenant_id, connection_id, enabled, tool_name) WHERE archived_at IS NULL;

CREATE TABLE agents (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    lifecycle_status text NOT NULL DEFAULT 'active' CHECK (lifecycle_status IN ('active', 'archived')),
    draft_revision_id text,
    published_revision_id text,
    next_revision_no integer NOT NULL DEFAULT 1 CHECK (next_revision_no > 0),
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT agents_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agents_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agents_archived_at_check CHECK (
        (lifecycle_status <> 'archived' AND archived_at IS NULL) OR
        (lifecycle_status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE INDEX agents_tenant_status_idx ON agents (tenant_id, lifecycle_status, updated_at DESC, id);

CREATE TABLE agent_revisions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id text NOT NULL,
    revision_no integer NOT NULL CHECK (revision_no > 0),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    icon text NOT NULL DEFAULT 'AI',
    category text NOT NULL DEFAULT 'workflow' CHECK (category IN ('workflow', 'doc', 'analytics', 'it')),
    visibility text NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'department', 'role')),
    visibility_targets jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(visibility_targets) = 'array'),
    main_agent_role text NOT NULL DEFAULT '',
    system_prompt text NOT NULL DEFAULT '',
    welcome_message text NOT NULL DEFAULT '',
    suggested_questions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(suggested_questions) = 'array'),
    suggested_question_translations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(suggested_question_translations) = 'array'),
    model_connection_id text NOT NULL,
    model_config_checksum text NOT NULL DEFAULT '',
    timeout_ms integer NOT NULL DEFAULT 60000 CHECK (timeout_ms > 0),
    config_schema_version integer NOT NULL DEFAULT 1 CHECK (config_schema_version > 0),
    checksum text NOT NULL,
    revision_note text NOT NULL DEFAULT '',
    created_by_account_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_revisions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_revisions_agent_revision_no_idx UNIQUE (tenant_id, agent_id, revision_no),
    CONSTRAINT agent_revisions_agent_id_id_idx UNIQUE (tenant_id, agent_id, id),
    CONSTRAINT agent_revisions_execution_binding_idx UNIQUE (tenant_id, agent_id, id, model_connection_id),
    CONSTRAINT agent_revisions_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revisions_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_revisions_created_by_fk FOREIGN KEY (tenant_id, created_by_account_id) REFERENCES accounts (tenant_id, id)
);

ALTER TABLE agents
    ADD CONSTRAINT agents_draft_revision_fk
    FOREIGN KEY (tenant_id, id, draft_revision_id)
    REFERENCES agent_revisions (tenant_id, agent_id, id) ON DELETE RESTRICT;

ALTER TABLE agents
    ADD CONSTRAINT agents_published_revision_fk
    FOREIGN KEY (tenant_id, id, published_revision_id)
    REFERENCES agent_revisions (tenant_id, agent_id, id) ON DELETE RESTRICT;

CREATE INDEX agent_revisions_agent_idx ON agent_revisions (tenant_id, agent_id, revision_no DESC);
CREATE INDEX agent_revisions_model_idx ON agent_revisions (tenant_id, model_connection_id, created_at DESC);

CREATE TABLE agent_revision_members (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    id text NOT NULL,
    name text NOT NULL,
    role text NOT NULL DEFAULT '',
    model_connection_id text NOT NULL,
    model_config_checksum text NOT NULL DEFAULT '',
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (tenant_id, revision_id, id),
    CONSTRAINT agent_revision_members_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_members_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_members_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE agent_revision_builtin_tools (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    tool_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    PRIMARY KEY (tenant_id, revision_id, tool_key),
    CONSTRAINT agent_revision_builtin_tools_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_builtin_tools_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE agent_revision_external_tools (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    external_tool_id text NOT NULL,
    tool_schema_checksum text NOT NULL DEFAULT '',
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    PRIMARY KEY (tenant_id, revision_id, external_tool_id),
    CONSTRAINT agent_revision_external_tools_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_external_tools_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_external_tools_tool_fk FOREIGN KEY (tenant_id, external_tool_id) REFERENCES external_tools (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE agent_revision_knowledge_bases (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    knowledge_base_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (tenant_id, revision_id, knowledge_base_id),
    CONSTRAINT agent_revision_knowledge_bases_ordinal_idx UNIQUE (tenant_id, revision_id, ordinal),
    CONSTRAINT agent_revision_knowledge_bases_revision_fk FOREIGN KEY (tenant_id, revision_id) REFERENCES agent_revisions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_knowledge_bases_base_fk FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES knowledge_bases (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE agent_revision_member_builtin_tools (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    member_id text NOT NULL,
    tool_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    PRIMARY KEY (tenant_id, revision_id, member_id, tool_key),
    CONSTRAINT agent_revision_member_builtin_tools_ordinal_idx UNIQUE (tenant_id, revision_id, member_id, ordinal),
    CONSTRAINT agent_revision_member_builtin_tools_member_fk FOREIGN KEY (tenant_id, revision_id, member_id) REFERENCES agent_revision_members (tenant_id, revision_id, id) ON DELETE CASCADE
);

CREATE TABLE agent_revision_member_external_tools (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    member_id text NOT NULL,
    external_tool_id text NOT NULL,
    tool_schema_checksum text NOT NULL DEFAULT '',
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    PRIMARY KEY (tenant_id, revision_id, member_id, external_tool_id),
    CONSTRAINT agent_revision_member_external_tools_ordinal_idx UNIQUE (tenant_id, revision_id, member_id, ordinal),
    CONSTRAINT agent_revision_member_external_tools_member_fk FOREIGN KEY (tenant_id, revision_id, member_id) REFERENCES agent_revision_members (tenant_id, revision_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_member_external_tools_tool_fk FOREIGN KEY (tenant_id, external_tool_id) REFERENCES external_tools (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE agent_revision_member_knowledge_bases (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    revision_id text NOT NULL,
    member_id text NOT NULL,
    knowledge_base_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (tenant_id, revision_id, member_id, knowledge_base_id),
    CONSTRAINT agent_revision_member_knowledge_bases_ordinal_idx UNIQUE (tenant_id, revision_id, member_id, ordinal),
    CONSTRAINT agent_revision_member_knowledge_bases_member_fk FOREIGN KEY (tenant_id, revision_id, member_id) REFERENCES agent_revision_members (tenant_id, revision_id, id) ON DELETE CASCADE,
    CONSTRAINT agent_revision_member_knowledge_bases_base_fk FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES knowledge_bases (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE conversations (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    owner_account_id text NOT NULL,
    agent_id text,
    current_segment_id text,
    next_message_sequence bigint NOT NULL DEFAULT 1 CHECK (next_message_sequence > 0),
    title text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    last_message_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    archived_at timestamptz,
    CONSTRAINT conversations_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversations_agent_id_idx UNIQUE (tenant_id, id, agent_id),
    CONSTRAINT conversations_owner_fk FOREIGN KEY (tenant_id, owner_account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT conversations_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversations_archived_at_check CHECK (
        (status <> 'archived' AND archived_at IS NULL) OR
        (status = 'archived' AND archived_at IS NOT NULL)
    )
);

CREATE TABLE conversation_segments (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    start_reason text NOT NULL DEFAULT 'initial' CHECK (start_reason IN ('initial', 'context_reset')),
    created_at timestamptz NOT NULL,
    CONSTRAINT conversation_segments_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_segments_conversation_ordinal_idx UNIQUE (tenant_id, conversation_id, ordinal),
    CONSTRAINT conversation_segments_conversation_id_idx UNIQUE (tenant_id, conversation_id, id),
    CONSTRAINT conversation_segments_conversation_fk FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE conversations
    ADD CONSTRAINT conversations_current_segment_fk
    FOREIGN KEY (tenant_id, id, current_segment_id)
    REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT;

CREATE INDEX conversations_tenant_owner_status_idx ON conversations (tenant_id, owner_account_id, status, updated_at DESC, id DESC);
CREATE INDEX conversations_tenant_agent_idx ON conversations (tenant_id, agent_id, updated_at DESC, id DESC);

CREATE TABLE messages (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    sequence_no bigint NOT NULL CHECK (sequence_no > 0),
    role text NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content text NOT NULL DEFAULT '',
    content_json jsonb,
    execution_id text,
    execution_step_id text,
    created_at timestamptz NOT NULL,
    CONSTRAINT messages_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT messages_conversation_segment_id_idx UNIQUE (tenant_id, conversation_id, segment_id, id),
    CONSTRAINT messages_conversation_sequence_idx UNIQUE (tenant_id, conversation_id, sequence_no),
    CONSTRAINT messages_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE CASCADE,
    CONSTRAINT messages_execution_step_requires_execution CHECK (execution_step_id IS NULL OR execution_id IS NOT NULL)
);

CREATE INDEX messages_conversation_segment_sequence_idx ON messages (tenant_id, conversation_id, segment_id, sequence_no ASC);

CREATE TABLE executions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    input_message_id text NOT NULL,
    agent_id text,
    agent_revision_id text,
    model_connection_id text,
    mode text NOT NULL DEFAULT '',
    trigger_type text NOT NULL DEFAULT 'chat' CHECK (trigger_type IN ('chat', 'api', 'trial', 'system')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    queued_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    error_code text NOT NULL DEFAULT '',
    error_category text NOT NULL DEFAULT '',
    safe_error_message text NOT NULL DEFAULT '',
    llm_call_count bigint NOT NULL DEFAULT 0 CHECK (llm_call_count >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cached_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    usage_complete boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT executions_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT executions_conversation_segment_id_idx UNIQUE (tenant_id, conversation_id, segment_id, id),
    CONSTRAINT executions_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT executions_conversation_agent_fk FOREIGN KEY (tenant_id, conversation_id, agent_id) REFERENCES conversations (tenant_id, id, agent_id) ON DELETE RESTRICT,
    CONSTRAINT executions_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT,
    CONSTRAINT executions_input_message_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, input_message_id) REFERENCES messages (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT,
    CONSTRAINT executions_agent_revision_fk FOREIGN KEY (tenant_id, agent_id, agent_revision_id, model_connection_id) REFERENCES agent_revisions (tenant_id, agent_id, id, model_connection_id) ON DELETE RESTRICT,
    CONSTRAINT executions_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT executions_agent_binding_check CHECK (
        (agent_id IS NULL AND agent_revision_id IS NULL AND model_connection_id IS NULL) OR
        (agent_id IS NOT NULL AND agent_revision_id IS NOT NULL AND model_connection_id IS NOT NULL)
    ),
    CONSTRAINT executions_cached_tokens_check CHECK (cached_tokens <= input_tokens),
    CONSTRAINT executions_timestamps_check CHECK (
        (status = 'queued' AND started_at IS NULL AND completed_at IS NULL) OR
        (status = 'running' AND started_at IS NOT NULL AND completed_at IS NULL) OR
        (status IN ('completed', 'failed', 'cancelled') AND completed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX executions_active_conversation_unique
    ON executions (tenant_id, conversation_id)
    WHERE status IN ('queued', 'running');
CREATE INDEX executions_tenant_account_created_idx ON executions (tenant_id, account_id, created_at DESC, id DESC);
CREATE INDEX executions_tenant_revision_created_idx ON executions (tenant_id, agent_revision_id, created_at DESC, id DESC);

CREATE TABLE execution_steps (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    execution_id text NOT NULL,
    parent_step_id text,
    sequence_no integer NOT NULL CHECK (sequence_no > 0),
    step_type text NOT NULL CHECK (step_type IN ('llm', 'tool', 'sub_agent', 'retrieval')),
    name text NOT NULL DEFAULT '',
    model_connection_id text,
    external_tool_id text,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    input_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(input_summary) = 'object'),
    output_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_summary) = 'object'),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cached_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    started_at timestamptz,
    completed_at timestamptz,
    error_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    CONSTRAINT execution_steps_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT execution_steps_execution_id_idx UNIQUE (tenant_id, execution_id, id),
    CONSTRAINT execution_steps_execution_sequence_idx UNIQUE (tenant_id, execution_id, sequence_no),
    CONSTRAINT execution_steps_execution_fk FOREIGN KEY (tenant_id, execution_id) REFERENCES executions (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT execution_steps_parent_fk FOREIGN KEY (tenant_id, execution_id, parent_step_id) REFERENCES execution_steps (tenant_id, execution_id, id) ON DELETE RESTRICT,
    CONSTRAINT execution_steps_model_fk FOREIGN KEY (tenant_id, model_connection_id) REFERENCES model_connections (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT execution_steps_external_tool_fk FOREIGN KEY (tenant_id, external_tool_id) REFERENCES external_tools (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT execution_steps_cached_tokens_check CHECK (cached_tokens <= input_tokens)
);

ALTER TABLE messages
    ADD CONSTRAINT messages_execution_fk
    FOREIGN KEY (tenant_id, conversation_id, segment_id, execution_id)
    REFERENCES executions (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT;

ALTER TABLE messages
    ADD CONSTRAINT messages_execution_step_fk
    FOREIGN KEY (tenant_id, execution_id, execution_step_id)
    REFERENCES execution_steps (tenant_id, execution_id, id) ON DELETE RESTRICT;

CREATE TABLE conversation_files (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    file_asset_id text NOT NULL,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'attached')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT conversation_files_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT conversation_files_conversation_segment_id_idx UNIQUE (tenant_id, conversation_id, segment_id, id),
    CONSTRAINT conversation_files_asset_idx UNIQUE (tenant_id, conversation_id, segment_id, file_asset_id),
    CONSTRAINT conversation_files_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_files_asset_fk FOREIGN KEY (tenant_id, file_asset_id) REFERENCES file_assets (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX conversation_files_segment_idx
    ON conversation_files (tenant_id, conversation_id, segment_id, state, created_at ASC, id ASC);

CREATE TABLE message_attachments (
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    message_id text NOT NULL,
    conversation_file_id text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, message_id, conversation_file_id),
    CONSTRAINT message_attachments_ordinal_idx UNIQUE (tenant_id, message_id, ordinal),
    CONSTRAINT message_attachments_message_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, message_id) REFERENCES messages (tenant_id, conversation_id, segment_id, id) ON DELETE CASCADE,
    CONSTRAINT message_attachments_conversation_file_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, conversation_file_id) REFERENCES conversation_files (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT
);

CREATE INDEX message_attachments_message_idx
    ON message_attachments (tenant_id, conversation_id, segment_id, message_id, ordinal ASC);

CREATE TABLE memories (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('global', 'agent', 'conversation')),
    agent_id text,
    conversation_id text,
    segment_id text,
    key text NOT NULL,
    content text NOT NULL,
    source_type text NOT NULL DEFAULT 'extracted' CHECK (source_type IN ('manual', 'extracted')),
    source_message_id text,
    confidence numeric(5,4) NOT NULL DEFAULT 1 CHECK (confidence >= 0 AND confidence <= 1),
    importance integer NOT NULL DEFAULT 1 CHECK (importance >= 1 AND importance <= 5),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT memories_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT memories_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT memories_agent_fk FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT memories_conversation_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT,
    CONSTRAINT memories_source_message_fk FOREIGN KEY (tenant_id, source_message_id) REFERENCES messages (tenant_id, id) ON DELETE SET NULL (source_message_id),
    CONSTRAINT memories_scope_check CHECK (
        (scope_type = 'global' AND agent_id IS NULL AND conversation_id IS NULL AND segment_id IS NULL) OR
        (scope_type = 'agent' AND agent_id IS NOT NULL AND conversation_id IS NULL AND segment_id IS NULL) OR
        (scope_type = 'conversation' AND agent_id IS NULL AND conversation_id IS NOT NULL AND segment_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX memories_active_scope_key_idx
    ON memories (tenant_id, account_id, scope_type, agent_id, conversation_id, segment_id, key) NULLS NOT DISTINCT
    WHERE status = 'active';
CREATE INDEX memories_tenant_account_idx
    ON memories (tenant_id, account_id, scope_type, importance DESC, updated_at DESC, id DESC)
    WHERE status = 'active';

CREATE TABLE agent_confirmations (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id text NOT NULL,
    conversation_id text NOT NULL,
    segment_id text NOT NULL,
    execution_id text,
    source_message_id text,
    kind text NOT NULL,
    title text NOT NULL,
    action text NOT NULL,
    public_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(public_payload) = 'object'),
    action_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(action_payload) = 'object'),
    result_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_payload) = 'object'),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'executing', 'completed', 'failed', 'cancelled', 'expired')),
    last_error text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT agent_confirmations_tenant_id_id_idx UNIQUE (tenant_id, id),
    CONSTRAINT agent_confirmations_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id),
    CONSTRAINT agent_confirmations_segment_fk FOREIGN KEY (tenant_id, conversation_id, segment_id) REFERENCES conversation_segments (tenant_id, conversation_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_confirmations_execution_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, execution_id) REFERENCES executions (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_confirmations_source_message_fk FOREIGN KEY (tenant_id, conversation_id, segment_id, source_message_id) REFERENCES messages (tenant_id, conversation_id, segment_id, id) ON DELETE RESTRICT
);

CREATE INDEX agent_confirmations_pending_idx
    ON agent_confirmations (tenant_id, account_id, conversation_id, segment_id, expires_at ASC, id)
    WHERE status = 'pending';

ALTER TABLE credential_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE credential_secrets FORCE ROW LEVEL SECURITY;
ALTER TABLE model_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE model_connection_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_connection_state FORCE ROW LEVEL SECURITY;
ALTER TABLE external_tool_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_tool_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE external_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revisions FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_members FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_builtin_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_builtin_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_external_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_external_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_knowledge_bases ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_knowledge_bases FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_member_builtin_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_member_builtin_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_member_external_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_member_external_tools FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_member_knowledge_bases ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_revision_member_knowledge_bases FORCE ROW LEVEL SECURITY;
ALTER TABLE conversations ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversations FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_segments FORCE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages FORCE ROW LEVEL SECURITY;
ALTER TABLE executions ENABLE ROW LEVEL SECURITY;
ALTER TABLE executions FORCE ROW LEVEL SECURITY;
ALTER TABLE execution_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE execution_steps FORCE ROW LEVEL SECURITY;
ALTER TABLE conversation_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_files FORCE ROW LEVEL SECURITY;
ALTER TABLE message_attachments ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_attachments FORCE ROW LEVEL SECURITY;
ALTER TABLE memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE memories FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_confirmations ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_confirmations FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_credential_secrets ON credential_secrets USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_model_connections ON model_connections USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_model_connection_state ON model_connection_state USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_external_tool_connections ON external_tool_connections USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_external_tools ON external_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agents ON agents USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revisions ON agent_revisions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_members ON agent_revision_members USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_builtin_tools ON agent_revision_builtin_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_external_tools ON agent_revision_external_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_knowledge_bases ON agent_revision_knowledge_bases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_member_builtin_tools ON agent_revision_member_builtin_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_member_external_tools ON agent_revision_member_external_tools USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_revision_member_knowledge_bases ON agent_revision_member_knowledge_bases USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversations ON conversations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_segments ON conversation_segments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_messages ON messages USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_executions ON executions USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_execution_steps ON execution_steps USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_conversation_files ON conversation_files USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_message_attachments ON message_attachments USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_memories ON memories USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
CREATE POLICY tenant_isolation_agent_confirmations ON agent_confirmations USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down
-- The Up migration destroys legacy Agent data and cannot reconstruct it safely.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION '000020_agent_data_model_v2 is irreversible because legacy Agent data was discarded';
END $$;
-- +goose StatementEnd

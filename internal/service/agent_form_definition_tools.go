package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"nexus-pro-api/internal/domain"
)

// ToolFormGetCapabilities 只讀取表單創作能力，不返回租戶業務記錄。
func (c *Service) ToolFormGetCapabilities(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	capabilities, err := c.Workflow().FormBuilderCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"capabilities": capabilities}, nil
}

// ToolFormGetDataSourceSchema 返回 metadata-only 數據源 schema，避免 Agent 讀取整租戶記錄。
func (c *Service) ToolFormGetDataSourceSchema(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	capabilities, err := c.Workflow().FormBuilderCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"data_sources": capabilities.DataSources}, nil
}

// ToolFormCreateDraft 把自然語言產出的結構化 schema 保存為受控草稿，不提供發佈能力。
func (c *Service) ToolFormCreateDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	schema, err := formDefinitionSchemaArg(args)
	if err != nil {
		return nil, err
	}
	draft, err := c.Workflow().CreateFormDefinitionDraft(ctx, domain.CreateFormDefinitionDraftInput{
		BaseTemplateID: stringFromAny(args["base_template_id"]), Schema: schema, Source: firstNonEmpty(stringFromAny(args["source"]), "agent"), AgentID: stringFromAny(args["agent_id"]), AgentRunID: stringFromAny(args["agent_run_id"]), AgentSessionID: stringFromAny(args["agent_session_id"]), ToolCallID: stringFromAny(args["tool_call_id"]),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"draft": draft, "next_step": "call form.validate_draft, then ask the employee to submit the draft for review"}, nil
}

// ToolFormUpdateDraft 更新 Agent 自己創建的定義草稿並要求 revision。
func (c *Service) ToolFormUpdateDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringFromAny(args["draft_id"]))
	if id == "" {
		return nil, BadRequest("draft_id is required")
	}
	schema, err := formDefinitionSchemaArg(args)
	if err != nil {
		return nil, err
	}
	revision := int64FromAny(args["revision"])
	if revision <= 0 {
		return nil, BadRequest("revision is required")
	}
	draft, err := c.Workflow().UpdateFormDefinitionDraft(ctx, id, domain.UpdateFormDefinitionDraftInput{Revision: revision, Schema: schema, Source: stringFromAny(args["source"]), AgentRunID: stringFromAny(args["agent_run_id"]), AgentSessionID: stringFromAny(args["agent_session_id"]), ToolCallID: stringFromAny(args["tool_call_id"])})
	if err != nil {
		return nil, err
	}
	return map[string]any{"draft": draft, "next_step": "call form.validate_draft before asking for review"}, nil
}

// ToolFormValidateDraft 返回結構化錯誤與編譯結果，不修改發佈狀態。
func (c *Service) ToolFormValidateDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringFromAny(args["draft_id"]))
	if id == "" {
		return nil, BadRequest("draft_id is required")
	}
	preview, err := c.Workflow().ValidateFormDefinitionDraft(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"draft": preview.Draft, "validation": preview.Validation, "compiled_schema": preview.CompiledSchema}, nil
}

// ToolFormPreviewDraft 提供前端可複用的預覽數據。
func (c *Service) ToolFormPreviewDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringFromAny(args["draft_id"]))
	if id == "" {
		return nil, BadRequest("draft_id is required")
	}
	preview, err := c.Workflow().PreviewFormDefinitionDraft(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"preview": preview}, nil
}

// ToolFormSimulateWorkflow 返回審批路徑模擬，不啟動真實 Temporal/workflow run。
func (c *Service) ToolFormSimulateWorkflow(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringFromAny(args["draft_id"]))
	if id == "" {
		return nil, BadRequest("draft_id is required")
	}
	simulation, err := c.Workflow().SimulateFormDefinitionWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"simulation": simulation}, nil
}

// formDefinitionSchemaArg 解析 Agent 傳入的 schema object，拒絕自由文本 schema。
func formDefinitionSchemaArg(args map[string]any) (domain.FormDefinitionSchemaV2, error) {
	raw, ok := args["schema"]
	if !ok {
		raw = args["authoring_schema"]
	}
	if raw == nil {
		return domain.FormDefinitionSchemaV2{}, BadRequest("schema is required")
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return domain.FormDefinitionSchemaV2{}, BadRequest("schema must be an object")
	}
	var schema domain.FormDefinitionSchemaV2
	if err := json.Unmarshal(bytes, &schema); err != nil {
		return domain.FormDefinitionSchemaV2{}, BadRequest("schema must be an object")
	}
	return schema, nil
}

// int64FromAny 解析 JSON number 與字符串形式的 revision。
func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		value, _ := typed.Int64()
		return value
	case string:
		var out int64
		_, _ = fmt.Sscan(strings.TrimSpace(typed), &out)
		return out
	}
	return 0
}

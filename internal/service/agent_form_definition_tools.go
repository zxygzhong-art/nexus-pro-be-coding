package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"nexus-pro-be/internal/domain"
)

// toolFormGetCapabilities 只读取表单创作能力，不返回租户业务记录。
func (c AgentService) toolFormGetCapabilities(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	capabilities, err := c.Workflow().FormBuilderCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"capabilities": capabilities}, nil
}

// toolFormGetDataSourceSchema 返回 metadata-only 数据源 schema，避免 Agent 读取整租户记录。
func (c AgentService) toolFormGetDataSourceSchema(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	capabilities, err := c.Workflow().FormBuilderCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"data_sources": capabilities.DataSources}, nil
}

// toolFormCreateDraft 把自然语言产出的结构化 schema 保存为受控草稿，不提供发布能力。
func (c AgentService) toolFormCreateDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
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

// toolFormUpdateDraft 更新 Agent 自己创建的定义草稿并要求 revision。
func (c AgentService) toolFormUpdateDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
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

// toolFormValidateDraft 返回结构化错误与编译结果，不修改发布状态。
func (c AgentService) toolFormValidateDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
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

// toolFormPreviewDraft 提供前端可复用的预览数据。
func (c AgentService) toolFormPreviewDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
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

// toolFormSimulateWorkflow 返回审批路径模拟，不启动真实 Temporal/workflow run。
func (c AgentService) toolFormSimulateWorkflow(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
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

// formDefinitionSchemaArg 解析 Agent 传入的 schema object，拒绝自由文本 schema。
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

// int64FromAny 解析 JSON number 与字符串形式的 revision。
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

package service

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"

	"nexus-pro-be/internal/domain"
)

const agentChatInstruction = `你是 Nexus Pro 的中文 HR/OA 助理。
必须通过提供的工具读取员工、考勤、表单、审批和工作区数据，不要编造业务资料。
你可以创建或更新可撤销的表单草稿。提交表单只能调用 preview_form_submission 生成确认卡；批准、拒绝或退回只能调用 prepare_bulk_review 生成确认卡。确认卡出现前后都不得声称操作已经完成。
高影响操作由使用者在确认卡上明确点击后由服务端执行，你不能绕过确认、扩大批次或代替非当前审批人操作。
工具不可用、资料不足或权限不足时，请明确说明缺少的信息或能力边界。
最终答复使用简洁的 GitHub Flavored Markdown。查询或摘要型答复先给一句结论；多条记录使用有序列表，每条记录的标题单独成行，字段使用缩进的无序列表；少量汇总指标使用无序列表或表格。
每个列表项必须实际换行，不要把多条记录或多个字段挤在同一段。除非使用者明确要求或后续操作必需，不要展示内部 ID。`

// ADKAgentChatRuntime runs an ADK root agent and its optional delegated sub-agents.
type ADKAgentChatRuntime struct {
	model    model.LLM
	sessions session.Service
}

// NewADKAgentChatRuntime creates the production agent runtime.
func NewADKAgentChatRuntime(llm model.LLM) (*ADKAgentChatRuntime, error) {
	if llm == nil {
		return nil, fmt.Errorf("agent model is required")
	}
	return &ADKAgentChatRuntime{model: llm, sessions: session.InMemoryService()}, nil
}

func (r *ADKAgentChatRuntime) RunAgentChat(ctx context.Context, req AgentChatRuntimeRequest, emit AgentChatEmitFunc) error {
	tools, err := adkTools(req.Tools)
	if err != nil {
		return err
	}
	llm := model.LLM(r.model)
	if modelName := strings.TrimSpace(req.ModelName); modelName != "" {
		llm = modelOverrideLLM{base: r.model, name: modelName}
	}
	subAgents := make([]adkagent.Agent, 0, len(req.SubAgents))
	agentLabels := map[string]string{}
	for index, member := range req.SubAgents {
		memberTools, toolErr := adkTools(member.Tools)
		if toolErr != nil {
			return toolErr
		}
		memberLLM := model.LLM(r.model)
		if modelName := strings.TrimSpace(member.ModelName); modelName != "" {
			memberLLM = modelOverrideLLM{base: r.model, name: modelName}
		}
		technicalName := fmt.Sprintf("sub_agent_%d", index+1)
		child, childErr := llmagent.New(llmagent.Config{
			Name:        technicalName,
			Description: strings.TrimSpace(member.Role),
			Model:       memberLLM,
			Mode:        llmagent.ModeSingleTurn,
			Instruction: subAgentInstruction(member),
			Tools:       memberTools,
		})
		if childErr != nil {
			return childErr
		}
		subAgents = append(subAgents, child)
		agentLabels[technicalName] = strings.TrimSpace(member.Name)
	}
	rootName := "nexus_team_root"
	rootLabel := strings.TrimSpace(req.AgentName)
	if rootLabel == "" {
		rootLabel = "Nexus Pro 助理"
	}
	agentLabels[rootName] = rootLabel
	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        rootName,
		Description: "Nexus Pro HR/OA Team coordinator",
		Model:       llm,
		Mode:        llmagent.ModeChat,
		Instruction: RootAgentInstruction(req.AgentRole, len(subAgents)),
		Tools:       tools,
		SubAgents:   subAgents,
	})
	if err != nil {
		return err
	}
	run, err := runner.New(runner.Config{
		AppName:           "nexus-pro-be",
		Agent:             rootAgent,
		SessionService:    r.sessions,
		AutoCreateSession: true,
	})
	if err != nil {
		return err
	}
	userID := strings.TrimSpace(req.RequestContext.AccountID)
	if userID == "" {
		userID = "anonymous"
	}
	message := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{genai.NewPartFromText(req.Message)},
	}
	for event, err := range run.Run(ctx, userID, req.SessionID, message, adkagent.RunConfig{}) {
		if err != nil {
			return err
		}
		if event == nil {
			continue
		}
		if event.UsageMetadata != nil && req.RecordUsage != nil {
			req.RecordUsage(domain.AgentTokenUsage{
				InputTokens:  int64(event.UsageMetadata.PromptTokenCount),
				CachedTokens: int64(event.UsageMetadata.CachedContentTokenCount),
				OutputTokens: int64(event.UsageMetadata.CandidatesTokenCount),
				TotalTokens:  int64(event.UsageMetadata.TotalTokenCount),
			})
		}
		if event.Content == nil {
			continue
		}
		if err := emitADKEvent(ctx, event, agentLabels, emit); err != nil {
			return err
		}
	}
	return nil
}

// RootAgentInstruction builds the user-facing instruction contract for the root agent.
func RootAgentInstruction(role string, subAgentCount int) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "理解使用者目标，选择合适的能力完成任务，并验证最终答案。"
	}
	instruction := agentChatInstruction + "\n\n主 Agent 职责：" + role
	if subAgentCount > 0 {
		instruction += "\n你有可调用的子 Agent。根据各自 Description 选择必要成员；收到结果后必须由你验证、整合并向使用者给出最终答复。不要声称并行执行，也不要虚构未返回的子 Agent 结果。"
	}
	return instruction
}

// subAgentInstruction 將成員職責與平台安全邊界組合為獨立任務提示。
func subAgentInstruction(member AgentChatSubAgentRuntimeRequest) string {
	return agentChatInstruction + "\n\n你是 Team 中的子 Agent「" + strings.TrimSpace(member.Name) + "」。你的职责是：" + strings.TrimSpace(member.Role) + "\n只处理被主 Agent 委派的任务，使用可用工具取得事实，并把可验证结果返回主 Agent。"
}

type modelOverrideLLM struct {
	base model.LLM
	name string
}

func (m modelOverrideLLM) Name() string {
	if strings.TrimSpace(m.name) != "" {
		return strings.TrimSpace(m.name)
	}
	return m.base.Name()
}

func (m modelOverrideLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if req != nil && strings.TrimSpace(m.name) != "" {
		cloned := *req
		cloned.Model = strings.TrimSpace(m.name)
		req = &cloned
	}
	return m.base.GenerateContent(ctx, req, stream)
}

func adkTools(src map[string]AgentTool) ([]tool.Tool, error) {
	out := make([]tool.Tool, 0, len(src))
	for name, fn := range src {
		name, fn := name, fn
		t, err := functiontool.New[map[string]any, map[string]any](functiontool.Config{
			Name:        name,
			Description: agentToolDescription(name),
			InputSchema: agentToolInputSchema(name),
		}, func(ctx adkagent.Context, args map[string]any) (map[string]any, error) {
			return fn(ctx, args)
		})
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// agentToolInputSchema gives the model required fields and enums for high-value business tools.
func agentToolInputSchema(name string) *jsonschema.Schema {
	stringProperty := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "string", Description: description}
	}
	numberProperty := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "number", Description: description}
	}
	integerProperty := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "integer", Description: description}
	}
	objectProperty := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "object", Description: description}
	}
	arrayProperty := func(items *jsonschema.Schema, description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "array", Items: items, Description: description}
	}
	object := func(properties map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "object", Properties: properties, Required: required}
	}

	switch name {
	case "get_my_profile", "my_leave_balances", "my_attendance_summary", "my_pending_reviews", "list_published_form_templates", "form.get_capabilities", "form.get_data_source_schema":
		return object(map[string]*jsonschema.Schema{})
	case "knowledge.search":
		return object(map[string]*jsonschema.Schema{"query": stringProperty("Tenant knowledge search query.")}, "query")
	case "list_employees", "my_clock_records":
		return object(map[string]*jsonschema.Schema{"limit": integerProperty("Maximum number of rows to return.")})
	case "my_form_history":
		return object(map[string]*jsonschema.Schema{
			"template_key": stringProperty("Optional form template key, for example leave-request."),
			"status":       stringProperty("Optional application status such as draft, pending, approved, rejected, returned, or cancelled."),
			"limit":        integerProperty("Maximum number of rows to return."),
		})
	case "get_employee":
		return object(map[string]*jsonschema.Schema{"employee_id": stringProperty("Employee ID.")}, "employee_id")
	case "workspace_insights":
		return object(map[string]*jsonschema.Schema{"month": stringProperty("Optional month in YYYY-MM format.")})
	case "check_leave_eligibility":
		return object(map[string]*jsonschema.Schema{
			"leave_type": stringProperty("Leave type code returned by the active policy."),
			"date":       stringProperty("Requested date in YYYY-MM-DD or RFC3339 format."),
			"hours":      numberProperty("Requested leave hours; must be greater than zero."),
		}, "leave_type", "date", "hours")
	case "get_published_form_template":
		return object(map[string]*jsonschema.Schema{"template_key": stringProperty("Published form template key.")}, "template_key")
	case "create_form_draft":
		return object(map[string]*jsonschema.Schema{
			"template_key": stringProperty("Published form template key."),
			"payload":      objectProperty("Field ID to value map matching the published template."),
		}, "template_key", "payload")
	case "update_form_draft":
		return object(map[string]*jsonschema.Schema{
			"draft_id": stringProperty("Existing form instance draft ID."),
			"payload":  objectProperty("Complete field ID to value map matching the published template."),
		}, "draft_id", "payload")
	case "preview_form_submission":
		return object(map[string]*jsonschema.Schema{"draft_id": stringProperty("Form instance draft ID.")}, "draft_id")
	case "prepare_bulk_review":
		action := stringProperty("Review action.")
		action.Enum = []any{"approve", "reject", "return"}
		return object(map[string]*jsonschema.Schema{
			"action":            action,
			"form_instance_ids": arrayProperty(stringProperty("Form instance ID."), "Optional fixed review batch."),
			"reason":            stringProperty("Required business reason for reject or return."),
		}, "action")
	case "form.create_draft":
		return object(map[string]*jsonschema.Schema{
			"schema":       objectProperty("Controlled form definition schema."),
			"agent_run_id": stringProperty("Optional Agent run provenance ID."),
			"tool_call_id": stringProperty("Optional tool call provenance ID."),
		}, "schema")
	case "form.update_draft":
		return object(map[string]*jsonschema.Schema{
			"draft_id": stringProperty("Form definition draft ID."),
			"revision": integerProperty("Expected current revision."),
			"schema":   objectProperty("Complete controlled form definition schema."),
		}, "draft_id", "revision", "schema")
	case "form.validate_draft", "form.preview_draft", "form.simulate_workflow":
		return object(map[string]*jsonschema.Schema{"draft_id": stringProperty("Form definition draft ID.")}, "draft_id")
	default:
		return object(map[string]*jsonschema.Schema{})
	}
}

func emitADKEvent(ctx context.Context, event *session.Event, agentLabels map[string]string, emit AgentChatEmitFunc) error {
	agentName := strings.TrimSpace(agentLabels[event.Author])
	if agentName == "" {
		agentName = strings.TrimSpace(event.Author)
	}
	for _, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		if part.FunctionCall != nil {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolCall, AgentName: agentName, AgentBranch: event.Branch, Name: part.FunctionCall.Name, Status: "started"}); err != nil {
				return err
			}
			continue
		}
		if part.FunctionResponse != nil {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, AgentName: agentName, AgentBranch: event.Branch, Name: part.FunctionResponse.Name, Status: "ok"}); err != nil {
				return err
			}
			continue
		}
		if part.Text != "" {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, AgentName: agentName, AgentBranch: event.Branch, Delta: part.Text}); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ AgentChatRuntime = (*ADKAgentChatRuntime)(nil)

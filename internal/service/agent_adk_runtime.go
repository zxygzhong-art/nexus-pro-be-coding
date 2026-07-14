package service

import (
	"context"
	"fmt"
	"iter"
	"strings"

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
工具不可用、资料不足或权限不足时，请明确说明缺少的信息或能力边界。`

// ADKAgentChatRuntime runs an ADK root agent and its optional task sub-agents.
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
			Mode:        llmagent.ModeTask,
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
		Instruction: rootAgentInstruction(req.AgentRole, len(subAgents)),
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
		if event == nil || event.Content == nil {
			continue
		}
		if err := emitADKEvent(ctx, event, agentLabels, emit); err != nil {
			return err
		}
	}
	return nil
}

// rootAgentInstruction 让主 Agent 只在需要时委派，并始终负责最终验证与汇总。
func rootAgentInstruction(role string, subAgentCount int) string {
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

// subAgentInstruction 将成员职责与平台安全边界组合为独立任务提示。
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

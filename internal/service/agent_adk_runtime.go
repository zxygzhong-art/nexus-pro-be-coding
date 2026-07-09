//go:build adk

package service

import (
	"context"
	"fmt"
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
只能通过提供的只读工具读取员工、考勤、审批和工作区数据。
不要编造员工、考勤、请假或审批数据；工具不可用或权限不足时，请明确说明能力边界。
禁止承诺或执行任何写操作。`

// ADKAgentChatRuntime runs a single ADK LlmAgent with in-memory sessions.
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
	agent, err := llmagent.New(llmagent.Config{
		Name:        "nexus_hr_assistant",
		Description: "Nexus Pro HR/OA assistant",
		Model:       r.model,
		Instruction: agentChatInstruction,
		Tools:       tools,
	})
	if err != nil {
		return err
	}
	run, err := runner.New(runner.Config{
		AppName:           "nexus-pro-be",
		Agent:             agent,
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
		if err := emitADKEvent(ctx, event, emit); err != nil {
			return err
		}
	}
	return nil
}

func adkTools(src map[string]AgentReadOnlyTool) ([]tool.Tool, error) {
	out := make([]tool.Tool, 0, len(src))
	for name, fn := range src {
		name, fn := name, fn
		t, err := functiontool.New[map[string]any, map[string]any](functiontool.Config{
			Name:        name,
			Description: "Nexus Pro read-only tool: " + name,
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

func emitADKEvent(ctx context.Context, event *session.Event, emit AgentChatEmitFunc) error {
	for _, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		if part.FunctionCall != nil {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolCall, Name: part.FunctionCall.Name, Status: "started"}); err != nil {
				return err
			}
			continue
		}
		if part.FunctionResponse != nil {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: part.FunctionResponse.Name, Status: "ok"}); err != nil {
				return err
			}
			continue
		}
		if part.Text != "" {
			if err := emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: part.Text}); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ AgentChatRuntime = (*ADKAgentChatRuntime)(nil)

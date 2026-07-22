package service

import (
	"context"

	"nexus-pro-api/internal/domain"
)

// agent_runtime_contract.go — Agent 對話 runtime 的 DI 契約（基底 Service 依賴，實作在 platform/adk 與 service/agent）。

// AgentTool 定義 agent runtime 可呼叫、且受工具與業務權限雙重檢查的工具。
type AgentTool func(context.Context, map[string]any) (map[string]any, error)

// AgentToolSpec carries the model-facing contract for dynamically resolved
// tools. Tools without a spec continue to use the built-in catalog contract.
type AgentToolSpec struct {
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type AgentChatEmitFunc func(context.Context, domain.AgentChatEvent) error

// AgentChatRuntimeRequest 定義 agent runtime 輸入。
type AgentChatRuntimeRequest struct {
	RequestContext domain.RequestContext
	RunID          string
	SessionID      string
	AgentName      string
	AgentRole      string
	ModelName      string
	Message        string
	History        []domain.AgentSessionMessage
	Memories       []domain.AgentMemory
	Mode           string
	Tools          map[string]AgentTool
	ToolSpecs      map[string]AgentToolSpec
	SubAgents      []AgentChatSubAgentRuntimeRequest
	RecordUsage    func(domain.AgentTokenUsage)
}

// AgentChatSubAgentRuntimeRequest 定義一個可由主 Agent 委派的運行時成員。
type AgentChatSubAgentRuntimeRequest struct {
	ID        string
	Name      string
	Role      string
	ModelName string
	Tools     map[string]AgentTool
	ToolSpecs map[string]AgentToolSpec
}

type ResolvedAgentTeamMember struct {
	ID               string
	Name             string
	Role             string
	ModelName        string
	ToolNames        []string
	ExternalToolIDs  []string
	KnowledgeBaseIDs []string
}

// AgentChatRuntime 定義 agent chat runtime 行為。
type AgentChatRuntime interface {
	RunAgentChat(context.Context, AgentChatRuntimeRequest, AgentChatEmitFunc) error
}

type AgentChatExecutionContext struct {
	AgentID        string
	SessionID      string
	SegmentID      string
	RunID          string
	InputMessageID string
	ContextVersion int64
}

type AgentChatExecutionContextKey struct{}

// WithAgentChatExecutionContext associates tool side effects with the exact visible conversation partition.
func WithAgentChatExecutionContext(ctx context.Context, execution AgentChatExecutionContext) context.Context {
	return context.WithValue(ctx, AgentChatExecutionContextKey{}, execution)
}

// AgentChatExecutionContextFromContext restores the session identity used by artifact and confirmation persistence.
func AgentChatExecutionContextFromContext(ctx context.Context) (AgentChatExecutionContext, bool) {
	if ctx == nil {
		return AgentChatExecutionContext{}, false
	}
	execution, ok := ctx.Value(AgentChatExecutionContextKey{}).(AgentChatExecutionContext)
	return execution, ok
}

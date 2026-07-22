package agent

import "nexus-pro-api/internal/service"

// service_aliases.go — 拆包過渡別名：讓 agent 包既有識別字直接指向基底 service 包。
// 型別別名（=）保證跨包型別完全相同，介面滿足與函式簽名不受影響。

type (
	Service                         = service.Service
	AgentTool                       = service.AgentTool
	AgentToolSpec                   = service.AgentToolSpec
	AgentChatEmitFunc               = service.AgentChatEmitFunc
	AgentChatRuntime                = service.AgentChatRuntime
	AgentChatRuntimeRequest         = service.AgentChatRuntimeRequest
	AgentChatSubAgentRuntimeRequest = service.AgentChatSubAgentRuntimeRequest
	ResolvedAgentTeamMember         = service.ResolvedAgentTeamMember
)

// 常數與函式別名：指向基底 service 包的匯出符號。
const LeaveEvaluationUnsupported = service.LeaveEvaluationUnsupported

var (
	ValidateWorkflowTemplateSubmittable = service.ValidateWorkflowTemplateSubmittable
	FormTemplateAtVersion               = service.FormTemplateAtVersion
)

const WorkflowFormStatusDraft = service.WorkflowFormStatusDraft

type AgentChatExecutionContext = service.AgentChatExecutionContext

var (
	WithAgentChatExecutionContext        = service.WithAgentChatExecutionContext
	AgentChatExecutionContextFromContext = service.AgentChatExecutionContextFromContext
)

const AgentConfirmationMemoryKey = service.AgentConfirmationMemoryKey

type (
	AgentToolCall   = service.AgentToolCall
	AgentToolResult = service.AgentToolResult
	AgentToolCaller = service.AgentToolCaller
)

type ObjectDeleter = service.ObjectDeleter

var (
	ObjectStoreProvider = service.ObjectStoreProvider
	ObjectStoreBucket   = service.ObjectStoreBucket
	ForbiddenDataScope  = service.ForbiddenDataScope
)

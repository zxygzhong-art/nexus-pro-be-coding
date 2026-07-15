package service

import (
	"strings"

	"nexus-pro-be/internal/domain"
)

const (
	AgentRuntimeFailureReasonCode = "agent_runtime_unavailable"
	AgentRuntimeFailureMessage    = "agent runtime is temporarily unavailable"
)

// agentRuntimeFailureTraceID returns the safest available request correlation ID.
func agentRuntimeFailureTraceID(ctx RequestContext) string {
	if traceID := strings.TrimSpace(ctx.TraceID); traceID != "" {
		return traceID
	}
	return strings.TrimSpace(ctx.RequestID)
}

// agentRuntimeFailureAnswer builds the stable, non-sensitive failure text stored in run history.
func agentRuntimeFailureAnswer(ctx RequestContext) string {
	answer := AgentRuntimeFailureMessage + "; reason_code=" + AgentRuntimeFailureReasonCode
	if traceID := agentRuntimeFailureTraceID(ctx); traceID != "" {
		answer += "; trace_id=" + traceID
	}
	return answer
}

// agentRuntimeFailureError returns a public-safe service error for non-stream callers.
func agentRuntimeFailureError(ctx RequestContext) *domain.AppError {
	err := domain.E(503, "service_unavailable", AgentRuntimeFailureMessage).
		WithReasonCode(AgentRuntimeFailureReasonCode)
	err.TraceID = agentRuntimeFailureTraceID(ctx)
	return err
}

// AgentRuntimeFailureEvent returns a stable public SSE event without the raw runtime cause.
func AgentRuntimeFailureEvent(ctx RequestContext, runID string) domain.AgentChatEvent {
	data := map[string]any{"reason_code": AgentRuntimeFailureReasonCode}
	if traceID := agentRuntimeFailureTraceID(ctx); traceID != "" {
		data["trace_id"] = traceID
	}
	return domain.AgentChatEvent{
		Event:   domain.AgentChatEventError,
		RunID:   strings.TrimSpace(runID),
		Message: AgentRuntimeFailureMessage,
		Data:    data,
	}
}

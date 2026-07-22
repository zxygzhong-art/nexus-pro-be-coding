package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/platform/mcpclient"
	"nexus-pro-api/internal/utils"
)

const externalToolAuditTimeout = 3 * time.Second

var errExternalToolReportedFailure = errors.New("external tool reported a failure")

// externalToolRuntimeStore is intentionally narrower than repository.Store:
// only deployments with the normalized v2 bindings and step audit may expose
// a published external capability to the model.
type externalToolRuntimeStore interface {
	ListAgentRevisionExternalToolBindings(context.Context, string, string) ([]domain.AgentRevisionExternalTool, error)
	ListAgentRevisionMemberExternalToolBindings(context.Context, string, string) ([]domain.AgentRevisionMemberExternalTool, error)
	ListAgentExternalToolCapabilitiesByIDs(context.Context, string, []string) ([]domain.ExternalToolCapability, error)
	GetAgentExternalToolCapability(context.Context, string, string) (domain.ExternalToolCapability, bool, error)
	GetAgentExternalTool(context.Context, string, string) (domain.AgentExternalTool, bool, error)
	AppendExecutionStep(context.Context, domain.ExecutionStep) (domain.ExecutionStep, error)
	UpsertExecutionStep(context.Context, domain.ExecutionStep) error
}

// externalAgentRuntimeTools resolves the exact immutable revision bindings.
// Stale, disabled, archived, and schema-drifted operations are deliberately
// absent. Mutating operations remain visible but can only create a one-time
// confirmation; their remote call is performed by the confirmation executor.
func (c AgentService) externalAgentRuntimeTools(
	reqCtx RequestContext,
	revisionID string,
	memberID string,
	configuredIDs []string,
	emit AgentChatEmitFunc,
) (map[string]AgentTool, map[string]AgentToolSpec, error) {
	tools := map[string]AgentTool{}
	specs := map[string]AgentToolSpec{}
	configured := externalRuntimeStringSet(configuredIDs)
	if len(configured) == 0 {
		return tools, specs, nil
	}
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return nil, nil, domain.E(503, "service_unavailable", "published agent revision is unavailable")
	}
	store, ok := any(c.store).(externalToolRuntimeStore)
	if !ok {
		return nil, nil, domain.E(503, "service_unavailable", "external tool runtime persistence is not configured")
	}

	type binding struct {
		capabilityID string
		checksum     string
		ordinal      int
	}
	bindings := make([]binding, 0, len(configured))
	if memberID == "" {
		items, err := store.ListAgentRevisionExternalToolBindings(goContext(reqCtx), reqCtx.TenantID, revisionID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			bindings = append(bindings, binding{item.ExternalToolID, item.ToolSchemaChecksum, item.Ordinal})
		}
	} else {
		items, err := store.ListAgentRevisionMemberExternalToolBindings(goContext(reqCtx), reqCtx.TenantID, revisionID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			if item.MemberID == memberID {
				bindings = append(bindings, binding{item.ExternalToolID, item.ToolSchemaChecksum, item.Ordinal})
			}
		}
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].ordinal == bindings[j].ordinal {
			return bindings[i].capabilityID < bindings[j].capabilityID
		}
		return bindings[i].ordinal < bindings[j].ordinal
	})
	wantedIDs := make([]string, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, item := range bindings {
		id := strings.TrimSpace(item.capabilityID)
		if _, allowed := configured[id]; !allowed || id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		wantedIDs = append(wantedIDs, id)
	}
	capabilities, err := store.ListAgentExternalToolCapabilitiesByIDs(goContext(reqCtx), reqCtx.TenantID, wantedIDs)
	if err != nil {
		return nil, nil, err
	}
	capabilityByID := make(map[string]domain.ExternalToolCapability, len(capabilities))
	for _, capability := range capabilities {
		capabilityByID[capability.ID] = capability
	}
	connectionByID := map[string]domain.AgentExternalTool{}
	for _, item := range bindings {
		capabilityID := strings.TrimSpace(item.capabilityID)
		if _, allowed := configured[capabilityID]; !allowed {
			continue
		}
		capability, found := capabilityByID[capabilityID]
		if !found || !capability.Enabled || capability.ArchivedAt != nil {
			continue
		}
		if strings.TrimSpace(item.checksum) == "" || item.checksum != capability.SchemaChecksum {
			c.Logger().Warn("published external tool schema no longer matches discovery", "tenant_id", reqCtx.TenantID, "revision_id", revisionID, "external_tool_id", capabilityID)
			continue
		}
		connection, cached := connectionByID[capability.ConnectionID]
		if !cached {
			var ok bool
			connection, ok, err = store.GetAgentExternalTool(goContext(reqCtx), reqCtx.TenantID, capability.ConnectionID)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				continue
			}
			connectionByID[capability.ConnectionID] = connection
		}
		if connection.Status != string(domain.ExternalToolConnectionStatusActive) || connection.ArchivedAt != nil {
			continue
		}
		runtimeName := externalToolRuntimeName(capability.ID)
		if _, collision := tools[runtimeName]; collision {
			return nil, nil, fmt.Errorf("external tool runtime name collision for capability %q", capability.ID)
		}
		tools[runtimeName] = c.externalToolInvocation(reqCtx, runtimeName, connection, capability, emit)
		description := strings.TrimSpace(capability.Description)
		if description == "" {
			description = "Invoke the configured operation " + capability.ToolName + "."
		}
		if !capability.Readonly {
			description += " This operation requires explicit user confirmation before the external system is changed."
		}
		specs[runtimeName] = AgentToolSpec{
			Description: strings.TrimSpace(connection.Name + ": " + description),
			InputSchema: capability.InputSchema,
		}
	}
	return tools, specs, nil
}

func mergeRuntimeTools(dst map[string]AgentTool, dstSpecs map[string]AgentToolSpec, src map[string]AgentTool, srcSpecs map[string]AgentToolSpec) error {
	for name, tool := range src {
		if _, exists := dst[name]; exists {
			return fmt.Errorf("agent runtime tool name collision: %s", name)
		}
		dst[name] = tool
		if spec, ok := srcSpecs[name]; ok {
			dstSpecs[name] = spec
		}
	}
	return nil
}

// externalToolRuntimeName is always ADK-compatible and does not depend on a
// remote server choosing a globally unique or syntactically safe tool name.
func externalToolRuntimeName(capabilityID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(capabilityID)))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:])
	return "ext_" + strings.ToLower(encoded)
}

func (c AgentService) externalToolInvocation(
	reqCtx RequestContext,
	runtimeName string,
	connection domain.AgentExternalTool,
	capability domain.ExternalToolCapability,
	emit AgentChatEmitFunc,
) AgentTool {
	return func(ctx context.Context, args map[string]any) (map[string]any, error) {
		actualCtx, ok := AgentRequestContextFromContext(ctx)
		if !ok {
			actualCtx = reqCtx
		}
		actualCtx.Context = ctx
		execution, ok := AgentChatExecutionContextFromContext(ctx)
		if !ok || strings.TrimSpace(execution.RunID) == "" {
			return nil, domain.E(500, "internal_error", "external tool execution context is unavailable")
		}
		startedAt := c.Now()
		step := domain.ExecutionStep{
			ID: utils.NewID("astep"), TenantID: actualCtx.TenantID, ExecutionID: execution.RunID,
			StepType: domain.ExecutionStepTypeTool,
			Name:     capability.ToolName, ExternalToolID: capability.ID,
			Status: domain.ExecutionStepStatusRunning, InputSummary: safeExternalToolInputSummary(args), OutputSummary: map[string]any{},
			StartedAt: &startedAt, CreatedAt: startedAt,
		}
		store, ok := any(c.store).(interface {
			AppendExecutionStep(context.Context, domain.ExecutionStep) (domain.ExecutionStep, error)
			UpsertExecutionStep(context.Context, domain.ExecutionStep) error
		})
		if !ok {
			return nil, domain.E(503, "service_unavailable", "external tool execution audit is not configured")
		}
		step, err := store.AppendExecutionStep(ctx, step)
		if err != nil {
			return nil, err
		}
		if emit != nil {
			_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolCall, Name: runtimeName, Status: "started"})
		}

		result, callErr := c.AgentToolGateway().Call(actualCtx, AgentToolCall{
			Name: runtimeName,
			Authz: CheckRequest{
				ApplicationCode: AppAgent,
				ResourceType:    ResourceTool,
				ResourceID:      capability.ID,
				Action:          ActionCall,
			},
			Execute: func() (AgentToolResult, error) {
				if !capability.Readonly {
					confirmation, err := c.Service.CreateExternalToolConfirmation(actualCtx, domain.CreateExternalToolConfirmationInput{
						ConnectionID: connection.ID, CapabilityID: capability.ID,
						SchemaChecksum: capability.SchemaChecksum, Arguments: args,
					})
					if err != nil {
						return AgentToolResult{}, err
					}
					return AgentToolResult{Data: map[string]any{
						"status": "confirmation_required", "confirmation": confirmation,
					}}, nil
				}
				currentConnection, currentCapability, err := c.reloadReadonlyExternalToolTarget(
					actualCtx, connection.ID, capability.ID, capability.SchemaChecksum,
				)
				if err != nil {
					return AgentToolResult{}, err
				}
				data, err := c.callExternalTool(actualCtx, currentConnection, currentCapability, args)
				if err != nil {
					return AgentToolResult{}, err
				}
				return AgentToolResult{Data: data}, nil
			},
		})
		completedAt := c.Now()
		step.CompletedAt = &completedAt
		if callErr != nil {
			step.Status = domain.ExecutionStepStatusFailed
			step.ErrorCode = externalToolErrorCode(ctx, callErr)
			step.OutputSummary = map[string]any{"status": "failed"}
		} else {
			step.Status = domain.ExecutionStepStatusCompleted
			step.OutputSummary = safeExternalToolOutputSummary(result.Data)
		}
		auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), externalToolAuditTimeout)
		persistErr := store.UpsertExecutionStep(auditCtx, step)
		cancelAudit()
		if persistErr != nil {
			callErr = errors.Join(callErr, fmt.Errorf("persist external tool completion audit: %w", persistErr))
		}
		if callErr != nil {
			if emit != nil {
				_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: runtimeName, Status: "denied"})
			}
			return nil, callErr
		}
		if confirmation, ok := result.Data["confirmation"].(*domain.AgentConfirmation); ok && confirmation != nil && emit != nil {
			_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventConfirmation, Confirmation: confirmation})
		}
		if emit != nil {
			_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: runtimeName, Status: "ok", Data: result.Data})
		}
		return result.Data, nil
	}
}

// reloadReadonlyExternalToolTarget closes the discovery-to-invocation TOCTOU
// window. Endpoint/auth state is read fresh, while the immutable published
// capability contract must still match exactly.
func (c AgentService) reloadReadonlyExternalToolTarget(
	reqCtx RequestContext,
	connectionID string,
	capabilityID string,
	schemaChecksum string,
) (domain.AgentExternalTool, domain.ExternalToolCapability, error) {
	store, ok := any(c.store).(interface {
		GetAgentExternalToolCapability(context.Context, string, string) (domain.ExternalToolCapability, bool, error)
		GetAgentExternalTool(context.Context, string, string) (domain.AgentExternalTool, bool, error)
	})
	if !ok {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, domain.E(503, "service_unavailable", "external tool runtime persistence is not configured")
	}
	capability, found, err := store.GetAgentExternalToolCapability(goContext(reqCtx), reqCtx.TenantID, capabilityID)
	if err != nil {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, err
	}
	if !found || capability.ConnectionID != connectionID || !capability.Readonly || !capability.Enabled || capability.ArchivedAt != nil || strings.TrimSpace(capability.SchemaChecksum) == "" || capability.SchemaChecksum != schemaChecksum {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, Conflict("external tool capability changed since the agent revision was published").WithReasonCode("agent_external_tool_drift")
	}
	connection, found, err := store.GetAgentExternalTool(goContext(reqCtx), reqCtx.TenantID, connectionID)
	if err != nil {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, err
	}
	if !found || connection.Status != string(domain.ExternalToolConnectionStatusActive) || connection.ArchivedAt != nil {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, Conflict("external tool connection is no longer active").WithReasonCode("agent_external_tool_unavailable")
	}
	return connection, capability, nil
}

func (c AgentService) callExternalTool(reqCtx RequestContext, connection domain.AgentExternalTool, capability domain.ExternalToolCapability, args map[string]any) (map[string]any, error) {
	headers, timeout, err := c.externalToolRequestConfig(reqCtx, connection)
	if err != nil {
		return nil, err
	}
	if connection.Kind == string(domain.ExternalToolConnectionKindMCP) {
		client, err := c.MCPConnector().Connect(goContext(reqCtx), mcpclient.Config{
			Endpoint: connection.EndpointURL, Transport: mcpclient.Transport(connection.Transport), Headers: headers, Timeout: timeout,
		})
		if err != nil {
			return nil, err
		}
		defer client.Close()
		result, err := client.CallTool(goContext(reqCtx), capability.ToolName, args)
		if err != nil {
			return nil, err
		}
		if result.IsError {
			return nil, errExternalToolReportedFailure
		}
		content := make([]map[string]any, 0, len(result.Content))
		for _, block := range result.Content {
			item := map[string]any{"type": block.Type}
			if block.Text != "" {
				item["text"] = block.Text
			}
			if block.Data != "" {
				item["data"] = block.Data
			}
			if block.MIMEType != "" {
				item["mime_type"] = block.MIMEType
			}
			content = append(content, item)
		}
		return map[string]any{"text": result.Text, "structured_content": result.StructuredContent, "content": content}, nil
	}
	if connection.Kind != string(domain.ExternalToolConnectionKindHTTP) {
		return nil, fmt.Errorf("unsupported external tool kind")
	}
	result, err := c.ExternalHTTPExecutor().Call(goContext(reqCtx), externaltool.Request{
		Endpoint: connection.EndpointURL, Method: capability.HTTPMethod, Path: capability.HTTPPath,
		Headers: headers, Arguments: args, Timeout: timeout,
	})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"status_code": result.StatusCode, "content_type": result.ContentType}
	if result.JSON != nil {
		out["data"] = result.JSON
	}
	if result.Text != "" {
		out["text"] = result.Text
	}
	return out, nil
}

func safeExternalToolInputSummary(args map[string]any) map[string]any {
	raw, _ := json.Marshal(args)
	return map[string]any{"argument_count": len(args), "encoded_bytes": len(raw)}
}

func safeExternalToolOutputSummary(result map[string]any) map[string]any {
	raw, _ := json.Marshal(result)
	return map[string]any{"status": "completed", "field_count": len(result), "encoded_bytes": len(raw)}
}

func externalToolErrorCode(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "external_tool_timeout"
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "external_tool_cancelled"
	}
	var statusErr *externaltool.StatusError
	if errors.As(err, &statusErr) {
		return "external_tool_http_status"
	}
	if errors.Is(err, errExternalToolReportedFailure) {
		return "external_tool_reported_failure"
	}
	return "external_tool_call_failed"
}

func externalRuntimeStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

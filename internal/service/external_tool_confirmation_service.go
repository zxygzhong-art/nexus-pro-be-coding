package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/platform/mcpclient"
	"nexus-pro-api/internal/utils"
)

const (
	externalToolConfirmationDefaultTimeout = 30 * time.Second
	externalToolConfirmationAuditTimeout   = 5 * time.Second
)

// errAgentConfirmationNonRetryable prevents replay after a deterministic
// protocol/configuration failure or after the remote side effect succeeded but
// its terminal step audit could not be persisted.
var errAgentConfirmationNonRetryable = errors.New("agent confirmation failure is not retryable")

type externalToolConfirmationCatalogStore interface {
	GetAgentExternalTool(context.Context, string, string) (domain.AgentExternalTool, bool, error)
	GetAgentExternalToolCapability(context.Context, string, string) (domain.ExternalToolCapability, bool, error)
}

type externalToolConfirmationStepStore interface {
	AppendExecutionStep(context.Context, domain.ExecutionStep) (domain.ExecutionStep, error)
	UpsertExecutionStep(context.Context, domain.ExecutionStep) error
}

// CreateExternalToolConfirmation creates a one-time confirmation for a
// mutating external capability. It deliberately performs no remote call.
func (c *Service) CreateExternalToolConfirmation(ctx RequestContext, input domain.CreateExternalToolConfirmationInput) (*domain.AgentConfirmation, error) {
	input.ConnectionID = strings.TrimSpace(input.ConnectionID)
	input.CapabilityID = strings.TrimSpace(input.CapabilityID)
	input.SchemaChecksum = strings.TrimSpace(input.SchemaChecksum)
	if input.ConnectionID == "" || input.CapabilityID == "" || input.SchemaChecksum == "" {
		return nil, BadRequest("connection_id, capability_id, and schema_checksum are required")
	}
	arguments, err := externalToolConfirmationArguments(input.Arguments)
	if err != nil {
		return nil, err
	}
	connection, capability, err := c.loadExternalToolConfirmationTarget(ctx, input.ConnectionID, input.CapabilityID, input.SchemaChecksum)
	if err != nil {
		return nil, err
	}
	if capability.Readonly {
		return nil, Conflict("read-only external tools do not require confirmation")
	}

	var confirmation *domain.AgentConfirmation
	_, err = c.AgentToolGateway().Call(ctx, AgentToolCall{
		Name: "external_tool_confirmation:" + capability.ToolName,
		Authz: CheckRequest{
			ApplicationCode: AppAgent,
			ResourceType:    ResourceTool,
			ResourceID:      capability.ID,
			Action:          ActionCall,
		},
		Execute: func() (AgentToolResult, error) {
			created, createErr := c.newExternalToolConfirmation(ctx, connection, capability, input.SchemaChecksum, arguments)
			if createErr != nil {
				return AgentToolResult{}, createErr
			}
			confirmation = created
			return AgentToolResult{}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return confirmation, nil
}

func (c *Service) newExternalToolConfirmation(
	ctx RequestContext,
	connection domain.AgentExternalTool,
	capability domain.ExternalToolCapability,
	schemaChecksum string,
	arguments map[string]any,
) (*domain.AgentConfirmation, error) {
	id, err := utils.NewSecretID("aconf")
	if err != nil {
		return nil, err
	}
	argumentNames := externalToolConfirmationArgumentNames(arguments)
	argumentLabel := "（無）"
	if len(argumentNames) > 0 {
		argumentLabel = strings.Join(argumentNames, "、")
	}
	confirmation := domain.AgentConfirmation{
		ID:          id,
		Kind:        agentConfirmationExternalTool,
		Title:       "確認執行外部操作「" + capability.ToolName + "」",
		Description: "此操作可能修改外部系統資料；執行前會再次檢查連線、工具版本與權限。",
		Action:      "execute",
		ActionLabel: "確認並執行",
		Rows: []domain.AgentAnalysisRow{
			{Label: "連線", Value: connection.Name},
			{Label: "工具", Value: capability.ToolName},
			{Label: "參數欄位", Value: argumentLabel},
		},
		ExpiresAt: c.Now().Add(agentConfirmationTTL),
	}
	if err := c.saveAgentConfirmation(ctx, agentConfirmationAction{
		Public:                 confirmation,
		ExternalConnectionID:   connection.ID,
		ExternalCapabilityID:   capability.ID,
		ExternalSchemaChecksum: schemaChecksum,
		ExternalArguments:      arguments,
	}); err != nil {
		return nil, err
	}
	return &confirmation, nil
}

func (c *Service) executeConfirmedExternalToolCall(
	ctx RequestContext,
	action agentConfirmationAction,
	record domain.AgentConfirmationRecord,
) (domain.AgentConfirmationExecution, error) {
	connection, capability, err := c.loadExternalToolConfirmationTarget(
		ctx,
		action.ExternalConnectionID,
		action.ExternalCapabilityID,
		action.ExternalSchemaChecksum,
	)
	if err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	if capability.Readonly {
		return domain.AgentConfirmationExecution{}, Conflict("external tool mutation contract changed after confirmation preview")
	}
	if strings.TrimSpace(record.ExecutionID) == "" {
		return domain.AgentConfirmationExecution{}, externalToolConfirmationNonRetryable(
			domain.E(500, "external_tool_execution_missing", "external tool execution audit context is unavailable"),
		)
	}
	stepStore, ok := any(c.store).(externalToolConfirmationStepStore)
	if !ok {
		return domain.AgentConfirmationExecution{}, externalToolConfirmationNonRetryable(
			domain.E(503, "external_tool_audit_unavailable", "external tool execution audit is not configured"),
		)
	}

	startedAt := c.Now()
	step, err := stepStore.AppendExecutionStep(goContext(ctx), domain.ExecutionStep{
		ID:             utils.NewID("astep"),
		TenantID:       ctx.TenantID,
		ExecutionID:    record.ExecutionID,
		StepType:       domain.ExecutionStepTypeTool,
		Name:           capability.ToolName,
		ExternalToolID: capability.ID,
		Status:         domain.ExecutionStepStatusRunning,
		InputSummary:   safeConfirmedExternalToolInputSummary(action.ExternalArguments),
		OutputSummary:  map[string]any{},
		StartedAt:      &startedAt,
		CreatedAt:      startedAt,
	})
	if err != nil {
		return domain.AgentConfirmationExecution{}, errors.Join(
			err,
			domain.E(503, "external_tool_audit_unavailable", "external tool execution audit is temporarily unavailable"),
		)
	}

	result, callErr := c.AgentToolGateway().Call(ctx, AgentToolCall{
		Name: "external_tool:" + capability.ToolName,
		Authz: CheckRequest{
			ApplicationCode: AppAgent,
			ResourceType:    ResourceTool,
			ResourceID:      capability.ID,
			Action:          ActionCall,
		},
		Execute: func() (AgentToolResult, error) {
			data, executeErr := c.callConfirmedExternalTool(ctx, connection, capability, action.ExternalArguments)
			if executeErr != nil {
				return AgentToolResult{}, classifyConfirmedExternalToolCallError(executeErr)
			}
			return AgentToolResult{Data: data}, nil
		},
	})

	completedAt := c.Now()
	step.CompletedAt = &completedAt
	if callErr != nil {
		step.Status = domain.ExecutionStepStatusFailed
		step.ErrorCode = confirmedExternalToolErrorCode(goContext(ctx), callErr)
		step.OutputSummary = map[string]any{"status": "failed"}
	} else {
		step.Status = domain.ExecutionStepStatusCompleted
		step.ErrorCode = ""
		step.OutputSummary = safeConfirmedExternalToolOutputSummary(result.Data)
	}
	auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(goContext(ctx)), externalToolConfirmationAuditTimeout)
	persistErr := stepStore.UpsertExecutionStep(auditCtx, step)
	cancelAudit()
	if persistErr != nil {
		if callErr == nil {
			return domain.AgentConfirmationExecution{}, externalToolConfirmationNonRetryable(errors.Join(
				persistErr,
				domain.E(500, "external_tool_audit_failed", "external tool completed but its execution audit could not be finalized"),
			))
		}
		callErr = errors.Join(callErr, fmt.Errorf("persist external tool completion audit: %w", persistErr))
	}
	if callErr != nil {
		return domain.AgentConfirmationExecution{}, callErr
	}
	return domain.AgentConfirmationExecution{
		ConfirmationID: action.Public.ID,
		Kind:           action.Public.Kind,
		Status:         agentConfirmationStatusDone,
		Data:           result.Data,
	}, nil
}

func (c *Service) loadExternalToolConfirmationTarget(
	ctx RequestContext,
	connectionID string,
	capabilityID string,
	schemaChecksum string,
) (domain.AgentExternalTool, domain.ExternalToolCapability, error) {
	connectionID = strings.TrimSpace(connectionID)
	capabilityID = strings.TrimSpace(capabilityID)
	schemaChecksum = strings.TrimSpace(schemaChecksum)
	if connectionID == "" || capabilityID == "" || schemaChecksum == "" {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, Conflict("external tool confirmation payload is incomplete")
	}
	store, ok := any(c.store).(externalToolConfirmationCatalogStore)
	if !ok {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, domain.E(503, "external_tool_store_unavailable", "external tool persistence is unavailable")
	}
	capability, found, err := store.GetAgentExternalToolCapability(goContext(ctx), ctx.TenantID, capabilityID)
	if err != nil {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, errors.Join(
			err,
			domain.E(503, "external_tool_store_unavailable", "external tool persistence is temporarily unavailable"),
		)
	}
	if !found || capability.ConnectionID != connectionID {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, Conflict("external tool capability changed after confirmation preview")
	}
	if !capability.Enabled || capability.ArchivedAt != nil || strings.TrimSpace(capability.SchemaChecksum) == "" || capability.SchemaChecksum != schemaChecksum {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, Conflict("external tool capability changed after confirmation preview")
	}
	connection, found, err := store.GetAgentExternalTool(goContext(ctx), ctx.TenantID, connectionID)
	if err != nil {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, errors.Join(
			err,
			domain.E(503, "external_tool_store_unavailable", "external tool persistence is temporarily unavailable"),
		)
	}
	if !found || connection.ArchivedAt != nil || connection.Status != string(domain.ExternalToolConnectionStatusActive) {
		return domain.AgentExternalTool{}, domain.ExternalToolCapability{}, Conflict("external tool connection is no longer active")
	}
	return connection, capability, nil
}

func (c *Service) callConfirmedExternalTool(
	ctx RequestContext,
	connection domain.AgentExternalTool,
	capability domain.ExternalToolCapability,
	arguments map[string]any,
) (map[string]any, error) {
	headers, timeout, err := c.confirmedExternalToolRequestConfig(ctx, connection)
	if err != nil {
		return nil, externalToolConfirmationNonRetryable(err)
	}
	if connection.Kind == string(domain.ExternalToolConnectionKindMCP) {
		client, err := c.mcpConnector.Connect(goContext(ctx), mcpclient.Config{
			Endpoint: connection.EndpointURL, Transport: mcpclient.Transport(connection.Transport), Headers: headers, Timeout: timeout,
		})
		if err != nil {
			return nil, err
		}
		defer client.Close()
		result, err := client.CallTool(goContext(ctx), capability.ToolName, arguments)
		if err != nil {
			return nil, err
		}
		if result.IsError {
			return nil, externalToolConfirmationNonRetryable(errors.New("external tool reported a protocol failure"))
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
		return nil, externalToolConfirmationNonRetryable(errors.New("unsupported external tool kind"))
	}
	result, err := c.externalHTTPExecutor.Call(goContext(ctx), externaltool.Request{
		Endpoint:  connection.EndpointURL,
		Method:    capability.HTTPMethod,
		Path:      capability.HTTPPath,
		Headers:   headers,
		Arguments: arguments,
		Timeout:   timeout,
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

func (c *Service) confirmedExternalToolRequestConfig(ctx RequestContext, connection domain.AgentExternalTool) (http.Header, time.Duration, error) {
	headers := make(http.Header)
	if connection.AuthType != string(domain.ExternalToolAuthTypeNone) {
		if connection.AuthSecretCiphertext == "" || connection.CredentialSecretID == "" {
			return nil, 0, domain.E(409, "external_tool_credential_unavailable", "external tool credential is unavailable")
		}
		if c.credentialCipher == nil {
			return nil, 0, domain.E(503, "external_tool_credential_unavailable", "external tool credential storage is not configured")
		}
		plaintext, err := c.credentialCipher.Decrypt(
			connection.AuthSecretCiphertext,
			domain.CredentialSecretAAD(ctx.TenantID, connection.CredentialSecretID),
		)
		if err != nil {
			return nil, 0, domain.E(500, "external_tool_credential_invalid", "failed to open external tool credential")
		}
		secret := string(plaintext)
		switch connection.AuthType {
		case string(domain.ExternalToolAuthTypeBearer):
			headers.Set("Authorization", "Bearer "+secret)
		case string(domain.ExternalToolAuthTypeAPIKey):
			if strings.TrimSpace(connection.AuthHeaderName) == "" {
				return nil, 0, domain.E(409, "external_tool_auth_invalid", "external tool authentication configuration is invalid")
			}
			headers.Set(connection.AuthHeaderName, secret)
		case string(domain.ExternalToolAuthTypeBasic):
			token := base64.StdEncoding.EncodeToString([]byte(connection.AuthUsername + ":" + secret))
			headers.Set("Authorization", "Basic "+token)
		default:
			return nil, 0, domain.E(409, "external_tool_auth_invalid", "external tool authentication configuration is invalid")
		}
	}
	timeout := time.Duration(connection.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = externalToolConfirmationDefaultTimeout
	}
	return headers, timeout, nil
}

func externalToolConfirmationArguments(arguments map[string]any) (map[string]any, error) {
	if arguments == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return nil, BadRequest("external tool arguments must be JSON-compatible")
	}
	copy := map[string]any{}
	if err := json.Unmarshal(raw, &copy); err != nil {
		return nil, BadRequest("external tool arguments must be a JSON object")
	}
	return copy, nil
}

func externalToolConfirmationArgumentNames(arguments map[string]any) []string {
	names := make([]string, 0, len(arguments))
	for name := range arguments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func safeConfirmedExternalToolInputSummary(arguments map[string]any) map[string]any {
	raw, _ := json.Marshal(arguments)
	return map[string]any{
		"argument_count": len(arguments),
		"encoded_bytes":  len(raw),
	}
}

func safeConfirmedExternalToolOutputSummary(result map[string]any) map[string]any {
	raw, _ := json.Marshal(result)
	return map[string]any{"status": "completed", "field_count": len(result), "encoded_bytes": len(raw)}
}

func classifyConfirmedExternalToolCallError(err error) error {
	if err == nil || errors.Is(err, errAgentConfirmationNonRetryable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var statusErr *externaltool.StatusError
	if errors.As(err, &statusErr) {
		if statusErr.StatusCode >= http.StatusInternalServerError {
			return errors.Join(err, domain.E(502, "external_tool_upstream_unavailable", "external tool is temporarily unavailable"))
		}
		return externalToolConfirmationNonRetryable(err)
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return err
	}
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		return err
	}
	return externalToolConfirmationNonRetryable(err)
}

func externalToolConfirmationNonRetryable(err error) error {
	if err == nil || errors.Is(err, errAgentConfirmationNonRetryable) {
		return err
	}
	return errors.Join(errAgentConfirmationNonRetryable, err)
}

func confirmedExternalToolErrorCode(ctx context.Context, err error) string {
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
	if errors.Is(err, errAgentConfirmationNonRetryable) {
		return "external_tool_protocol_failure"
	}
	return "external_tool_call_failed"
}

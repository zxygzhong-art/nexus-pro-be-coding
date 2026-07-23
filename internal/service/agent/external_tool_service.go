package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/platform/mcpclient"
)

const (
	externalToolDefaultTimeout  = 30 * time.Second
	externalToolMaxSchemaBytes  = 512 << 10
	externalToolMaxCapabilities = 256
)

// externalToolLifecycleStore is the persistence slice needed by connection
// testing and capability discovery. It deliberately stays smaller than the
// complete v2 repository contract.
type externalToolLifecycleStore interface {
	GetAgentExternalTool(context.Context, string, string) (domain.AgentExternalTool, bool, error)
	ReplaceAgentExternalToolCapabilities(context.Context, string, string, []domain.ExternalToolCapability) error
	UpdateAgentExternalToolTestResult(context.Context, string, string, string, string, time.Time) (domain.AgentExternalTool, bool, error)
}

// TestExternalTool checks one active connection without invoking a configured
// business operation. MCP performs initialize + tools/list; manual HTTP uses a
// bounded HEAD probe.
func (c AgentService) TestExternalTool(ctx RequestContext, id string) (domain.AgentExternalTool, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceTool, ActionUpdate, id)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	item, err := c.currentExternalTool(ctx, id)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	if item.Status != "active" {
		return domain.AgentExternalTool{}, Conflict("agent external tool is not active")
	}

	header, timeout, err := c.externalToolRequestConfig(ctx, item)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	message := "connection succeeded"
	var testErr error
	if item.Kind == "mcp" {
		client, connectErr := c.MCPConnector().Connect(goContext(ctx), mcpclient.Config{
			Endpoint:  item.EndpointURL,
			Transport: mcpclient.Transport(item.Transport),
			Headers:   header,
			Timeout:   timeout,
		})
		if connectErr != nil {
			testErr = connectErr
		} else {
			defer client.Close()
			tools, listErr := client.ListTools(goContext(ctx))
			if listErr != nil {
				testErr = listErr
			} else {
				message = fmt.Sprintf("connection succeeded; %d capabilities available", len(tools))
			}
		}
	} else {
		probe, probeErr := c.ExternalHTTPExecutor().Probe(goContext(ctx), externaltool.ProbeRequest{
			Endpoint: item.EndpointURL,
			Headers:  header,
			Timeout:  timeout,
		})
		if probeErr != nil {
			testErr = probeErr
		} else if probe.StatusCode == http.StatusMethodNotAllowed {
			message = "endpoint is reachable; HEAD is not supported"
		} else if probe.StatusCode < http.StatusOK || probe.StatusCode >= http.StatusBadRequest {
			testErr = fmt.Errorf("endpoint returned HTTP %d", probe.StatusCode)
		} else {
			message = fmt.Sprintf("connection succeeded; HTTP %d", probe.StatusCode)
		}
	}

	status := string(domain.ConnectionTestStatusOK)
	if testErr != nil {
		status = string(domain.ConnectionTestStatusFailed)
		message = externalToolFailureMessage(testErr)
	}
	updated, err := c.persistExternalToolTest(ctx, account, item, status, message)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	if testErr != nil {
		c.LogWarn(ctx, "external tool connection test failed", "external_tool_id", item.ID, "error", testErr)
		return updated, domain.E(502, "bad_gateway", "external tool connection test failed").WithReasonCode("agent_runtime_unavailable")
	}
	return updated, nil
}

// DiscoverExternalTool refreshes the persisted MCP capability catalogue while
// retaining IDs for tools whose protocol-level names have not changed.
func (c AgentService) DiscoverExternalTool(ctx RequestContext, id string) (domain.AgentExternalTool, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceTool, ActionUpdate, id)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	item, err := c.currentExternalTool(ctx, id)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	if item.Status != "active" {
		return domain.AgentExternalTool{}, Conflict("agent external tool is not active")
	}
	if item.Kind != "mcp" {
		return domain.AgentExternalTool{}, BadRequest("manual HTTP connections do not support capability discovery")
	}
	header, timeout, err := c.externalToolRequestConfig(ctx, item)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	client, err := c.MCPConnector().Connect(goContext(ctx), mcpclient.Config{
		Endpoint:  item.EndpointURL,
		Transport: mcpclient.Transport(item.Transport),
		Headers:   header,
		Timeout:   timeout,
	})
	if err != nil {
		_, _ = c.persistExternalToolTest(ctx, account, item, string(domain.ConnectionTestStatusFailed), externalToolFailureMessage(err))
		return domain.AgentExternalTool{}, domain.E(502, "bad_gateway", "external tool discovery failed").WithReasonCode("agent_runtime_unavailable")
	}
	defer client.Close()
	tools, err := client.ListTools(goContext(ctx))
	if err != nil {
		_, _ = c.persistExternalToolTest(ctx, account, item, string(domain.ConnectionTestStatusFailed), externalToolFailureMessage(err))
		return domain.AgentExternalTool{}, domain.E(502, "bad_gateway", "external tool discovery failed").WithReasonCode("agent_runtime_unavailable")
	}
	capabilities, err := discoveredExternalToolCapabilities(item, tools, c.Now())
	if err != nil {
		_, _ = c.persistExternalToolTest(ctx, account, item, string(domain.ConnectionTestStatusFailed), externalToolFailureMessage(err))
		return domain.AgentExternalTool{}, BadRequest(err.Error())
	}

	store, err := c.externalToolLifecycleStore()
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	message := fmt.Sprintf("discovered %d capabilities", len(capabilities))
	var updated domain.AgentExternalTool
	err = c.withTransaction(ctx, func(tx AgentService) error {
		txStore, storeErr := tx.externalToolLifecycleStore()
		if storeErr != nil {
			return storeErr
		}
		if replaceErr := txStore.ReplaceAgentExternalToolCapabilities(goContext(ctx), ctx.TenantID, item.ID, capabilities); replaceErr != nil {
			return replaceErr
		}
		var ok bool
		updated, ok, storeErr = txStore.UpdateAgentExternalToolTestResult(goContext(ctx), ctx.TenantID, item.ID, string(domain.ConnectionTestStatusOK), message, c.Now())
		if storeErr != nil {
			return storeErr
		}
		if !ok {
			return NotFound("agent external tool", item.ID)
		}
		updated.Capabilities = capabilities
		return tx.recordAgentAdminAudit(ctx, account, "external_tool", item.ID, item.Name, "discover", message)
	})
	_ = store // the early assertion gives a deterministic configuration error before opening the transaction.
	return updated, err
}

func (c AgentService) currentExternalTool(ctx RequestContext, id string) (domain.AgentExternalTool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentExternalTool{}, BadRequest("id is required")
	}
	store, err := c.externalToolLifecycleStore()
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	item, ok, err := store.GetAgentExternalTool(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	if !ok || item.Status == "archived" {
		return domain.AgentExternalTool{}, NotFound("agent external tool", id)
	}
	item.CredentialSet = item.AuthSecretCiphertext != ""
	return item, nil
}

func (c AgentService) externalToolLifecycleStore() (externalToolLifecycleStore, error) {
	store, ok := any(c.store).(externalToolLifecycleStore)
	if !ok {
		return nil, domain.E(503, "service_unavailable", "external tool persistence is not configured")
	}
	return store, nil
}

func (c AgentService) persistExternalToolTest(ctx RequestContext, account Account, item domain.AgentExternalTool, status, message string) (domain.AgentExternalTool, error) {
	var updated domain.AgentExternalTool
	err := c.withTransaction(ctx, func(tx AgentService) error {
		store, err := tx.externalToolLifecycleStore()
		if err != nil {
			return err
		}
		var ok bool
		updated, ok, err = store.UpdateAgentExternalToolTestResult(goContext(ctx), ctx.TenantID, item.ID, status, message, c.Now())
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent external tool", item.ID)
		}
		return tx.recordAgentAdminAudit(ctx, account, "external_tool", item.ID, item.Name, "test", status)
	})
	return updated, err
}

func (c AgentService) externalToolRequestConfig(ctx RequestContext, item domain.AgentExternalTool) (http.Header, time.Duration, error) {
	header := make(http.Header)
	if item.AuthType != "none" {
		if item.AuthSecretCiphertext == "" {
			return nil, 0, domain.E(503, "service_unavailable", "external tool credential is unavailable")
		}
		if c.CredentialCipher() == nil {
			return nil, 0, domain.E(503, "service_unavailable", "external tool credential storage is not configured")
		}
		plaintext, err := c.CredentialCipher().Decrypt(item.AuthSecretCiphertext, domain.ExternalToolCredentialAAD(ctx.TenantID, item.ID))
		if err != nil {
			return nil, 0, domain.E(500, "internal_error", "failed to open external tool credential")
		}
		secret := string(plaintext)
		switch item.AuthType {
		case "bearer":
			header.Set("Authorization", "Bearer "+secret)
		case "api_key":
			header.Set(item.AuthHeaderName, secret)
		case "basic":
			token := base64.StdEncoding.EncodeToString([]byte(item.AuthUsername + ":" + secret))
			header.Set("Authorization", "Basic "+token)
		default:
			return nil, 0, domain.E(500, "internal_error", "external tool authentication type is invalid")
		}
	}
	timeout := time.Duration(item.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = externalToolDefaultTimeout
	}
	return header, timeout, nil
}

func discoveredExternalToolCapabilities(connection domain.AgentExternalTool, tools []mcpclient.Tool, now time.Time) ([]domain.ExternalToolCapability, error) {
	if len(tools) > externalToolMaxCapabilities {
		return nil, fmt.Errorf("external tool exposes more than %d capabilities", externalToolMaxCapabilities)
	}
	existing := make(map[string]string, len(connection.Capabilities))
	for _, capability := range connection.Capabilities {
		existing[capability.ToolName] = capability.ID
	}
	seen := make(map[string]struct{}, len(tools))
	capabilities := make([]domain.ExternalToolCapability, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return nil, fmt.Errorf("discovered capability name is required")
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("external tool returned duplicate capability %q", name)
		}
		seen[name] = struct{}{}
		inputSchema, err := externalToolSchemaMap(tool.InputSchema, true)
		if err != nil {
			return nil, fmt.Errorf("capability %q input schema is invalid: %w", name, err)
		}
		outputSchema, err := externalToolSchemaMap(tool.OutputSchema, false)
		if err != nil {
			return nil, fmt.Errorf("capability %q output schema is invalid: %w", name, err)
		}
		id := existing[name]
		if id == "" {
			id = externalToolCapabilityID(connection.ID, name)
		}
		capability := domain.ExternalToolCapability{
			ID:           id,
			TenantID:     connection.TenantID,
			ConnectionID: connection.ID,
			ToolName:     name,
			Description:  strings.TrimSpace(tool.Description),
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Readonly:     externalToolReadonlyHint(tool.Annotations),
			Enabled:      true,
			DiscoveredAt: now,
			UpdatedAt:    now,
		}
		capability.SchemaChecksum, err = externalToolCapabilityChecksum(capability)
		if err != nil {
			return nil, err
		}
		capabilities = append(capabilities, capability)
	}
	return capabilities, nil
}

func manualHTTPExternalToolCapability(connection domain.AgentExternalTool, input domain.CreateAgentExternalToolInput, now time.Time) (domain.ExternalToolCapability, error) {
	name := strings.TrimSpace(input.ToolName)
	if name == "" {
		return domain.ExternalToolCapability{}, BadRequest("tool_name is required for a manual HTTP connection")
	}
	if len([]rune(name)) > 200 {
		return domain.ExternalToolCapability{}, BadRequest("tool_name must not exceed 200 characters")
	}
	method := strings.ToUpper(strings.TrimSpace(input.HTTPMethod))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return domain.ExternalToolCapability{}, BadRequest("http_method must be GET, POST, PUT, PATCH, or DELETE")
	}
	path := strings.TrimSpace(input.HTTPPath)
	parsedPath, err := url.Parse(path)
	if err != nil || parsedPath.IsAbs() || parsedPath.Host != "" || parsedPath.User != nil || parsedPath.Fragment != "" {
		return domain.ExternalToolCapability{}, BadRequest("http_path must be a same-origin relative path without credentials or fragments")
	}
	inputSchema, err := externalToolSchemaMap(input.InputSchema, true)
	if err != nil {
		return domain.ExternalToolCapability{}, BadRequest("input_schema is invalid")
	}
	outputSchema, err := externalToolSchemaMap(input.OutputSchema, false)
	if err != nil {
		return domain.ExternalToolCapability{}, BadRequest("output_schema is invalid")
	}
	capability := domain.ExternalToolCapability{
		ID:           externalToolCapabilityID(connection.ID, name),
		TenantID:     connection.TenantID,
		ConnectionID: connection.ID,
		ToolName:     name,
		Description:  connection.Description,
		HTTPMethod:   method,
		HTTPPath:     path,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Readonly:     input.Readonly || method == http.MethodGet,
		Enabled:      true,
		DiscoveredAt: now,
		UpdatedAt:    now,
	}
	capability.SchemaChecksum, err = externalToolCapabilityChecksum(capability)
	return capability, err
}

func externalToolSchemaMap(value any, input bool) (map[string]any, error) {
	if schema, ok := value.(map[string]any); ok && schema == nil {
		value = nil
	}
	if value == nil {
		if input {
			return map[string]any{"type": "object", "additionalProperties": true}, nil
		}
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > externalToolMaxSchemaBytes {
		return nil, fmt.Errorf("schema must be valid JSON smaller than %d bytes", externalToolMaxSchemaBytes)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return nil, fmt.Errorf("schema must be a JSON object")
	}
	return schema, nil
}

func externalToolReadonlyHint(annotations map[string]any) bool {
	for _, key := range []string{"readOnlyHint", "read_only_hint", "readonly"} {
		if value, ok := annotations[key].(bool); ok && value {
			return true
		}
	}
	return false
}

func externalToolCapabilityChecksum(capability domain.ExternalToolCapability) (string, error) {
	payload := struct {
		ToolName     string         `json:"tool_name"`
		Description  string         `json:"description"`
		HTTPMethod   string         `json:"http_method,omitempty"`
		HTTPPath     string         `json:"http_path,omitempty"`
		InputSchema  map[string]any `json:"input_schema"`
		OutputSchema map[string]any `json:"output_schema"`
		Readonly     bool           `json:"readonly"`
	}{capability.ToolName, capability.Description, capability.HTTPMethod, capability.HTTPPath, capability.InputSchema, capability.OutputSchema, capability.Readonly}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode external tool capability checksum: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// externalToolCapabilityID keeps protocol-name identity stable even when a
// capability disappears for one discovery cycle and is later reintroduced.
func externalToolCapabilityID(connectionID, toolName string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(connectionID) + "\x00" + strings.TrimSpace(toolName)))
	return "exttool-" + hex.EncodeToString(digest[:12])
}

func externalToolFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500])
	}
	return message
}

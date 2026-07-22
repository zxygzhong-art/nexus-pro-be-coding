// Package mcpclient provides a bounded remote MCP client adapter.
package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultTimeout          = 30 * time.Second
	defaultMaxResponseBytes = int64(4 << 20)
	defaultClientName       = "nexus-pro-api"
	defaultClientVersion    = "dev"
)

// Transport identifies a remote MCP transport.
type Transport string

const (
	// TransportAuto tries Streamable HTTP first and falls back to legacy SSE.
	TransportAuto Transport = "auto"
	// TransportStreamableHTTP uses the current Streamable HTTP transport.
	TransportStreamableHTTP Transport = "streamable_http"
	// TransportSSE uses the legacy HTTP+SSE transport.
	TransportSSE Transport = "sse"
)

var (
	// ErrResponseTooLarge indicates that an MCP HTTP response crossed the configured limit.
	ErrResponseTooLarge = errors.New("mcp response exceeds size limit")
	// ErrUnsafeEndpoint indicates that an endpoint failed the outbound network policy.
	ErrUnsafeEndpoint = errors.New("unsafe mcp endpoint")
	// ErrPaginationLoop indicates that a server repeated a non-empty tools/list cursor.
	ErrPaginationLoop = errors.New("mcp tools pagination cursor repeated")
)

// Config configures one remote MCP session.
type Config struct {
	Endpoint         string
	Transport        Transport
	Headers          http.Header
	Timeout          time.Duration
	MaxResponseBytes int64
	ClientName       string
	ClientVersion    string
	// EndpointValidator replaces the public-network-only default. Use
	// AllowPrivateEndpoints explicitly for trusted intranet endpoints and tests.
	EndpointValidator EndpointValidator
}

// Tool is the transport-independent representation needed by agent services.
type Tool struct {
	Name         string
	Title        string
	Description  string
	InputSchema  any
	OutputSchema any
	Annotations  map[string]any
}

// ContentBlock is a normalized MCP result content block. Raw preserves fields
// belonging to non-text content types without exposing SDK-specific structs.
type ContentBlock struct {
	Type     string
	Text     string
	Data     string
	MIMEType string
	Raw      map[string]any
}

// ToolResult is a transport-independent tools/call result.
type ToolResult struct {
	IsError           bool
	Text              string
	StructuredContent map[string]any
	Content           []ContentBlock
}

// Client is the narrow interface services can inject or fake in tests.
type Client interface {
	ListTools(context.Context) ([]Tool, error)
	CallTool(context.Context, string, any) (ToolResult, error)
	Close() error
}

// Connector creates MCP sessions and is intentionally small for service tests.
type Connector interface {
	Connect(context.Context, Config) (Client, error)
}

// ConnectorFunc adapts a function into a Connector.
type ConnectorFunc func(context.Context, Config) (Client, error)

// Connect implements Connector.
func (f ConnectorFunc) Connect(ctx context.Context, cfg Config) (Client, error) {
	return f(ctx, cfg)
}

// DefaultConnector connects through this package's production adapter.
type DefaultConnector struct{}

// Connect implements Connector.
func (DefaultConnector) Connect(ctx context.Context, cfg Config) (Client, error) {
	return Connect(ctx, cfg)
}

type remoteClient struct {
	session         *mcp.ClientSession
	timeout         time.Duration
	lifecycleCancel context.CancelCauseFunc

	closeOnce sync.Once
	closeErr  error
}

// Connect validates the endpoint and establishes an initialized MCP session.
func Connect(ctx context.Context, cfg Config) (Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("mcp connect: nil context")
	}
	resolved, _, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(resolved.Timeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	validationCtx, cancelValidation := context.WithDeadline(ctx, deadline)
	httpClient, endpoint, err := NewBoundedHTTPClient(
		validationCtx,
		resolved.Endpoint,
		resolved.Headers,
		resolved.Timeout,
		resolved.MaxResponseBytes,
		resolved.EndpointValidator,
	)
	cancelValidation()
	if err != nil {
		return nil, err
	}
	// A global http.Client timeout would terminate legacy SSE's session-long GET.
	// MCP connection and operation deadlines are enforced explicitly below.
	httpClient.Timeout = 0
	resolved.Endpoint = endpoint.String()
	connectOne := func(transport Transport) (Client, error) {
		session, lifecycleCancel, connectErr := connectSession(ctx, deadline, resolved, httpClient, transport)
		if connectErr != nil {
			return nil, connectErr
		}
		return &remoteClient{session: session, timeout: resolved.Timeout, lifecycleCancel: lifecycleCancel}, nil
	}

	switch resolved.Transport {
	case TransportStreamableHTTP, TransportSSE:
		return connectOne(resolved.Transport)
	case TransportAuto:
		streamable, streamableErr := connectOne(TransportStreamableHTTP)
		if streamableErr == nil {
			return streamable, nil
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return nil, streamableErr
		}
		legacy, legacyErr := connectOne(TransportSSE)
		if legacyErr == nil {
			return legacy, nil
		}
		return nil, fmt.Errorf("mcp auto transport failed: %w", errors.Join(streamableErr, legacyErr))
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", resolved.Transport)
	}
}

func normalizeConfig(cfg Config) (Config, *url.URL, error) {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	if cfg.Endpoint == "" {
		return Config{}, nil, fmt.Errorf("mcp endpoint is required")
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return Config{}, nil, fmt.Errorf("parse mcp endpoint: %w", err)
	}
	if err := validateEndpointSyntax(endpoint); err != nil {
		return Config{}, nil, err
	}
	if cfg.Transport == "" {
		cfg.Transport = TransportAuto
	}
	if cfg.Transport != TransportAuto && cfg.Transport != TransportStreamableHTTP && cfg.Transport != TransportSSE {
		return Config{}, nil, fmt.Errorf("unsupported mcp transport %q", cfg.Transport)
	}
	if cfg.Timeout < 0 {
		return Config{}, nil, fmt.Errorf("mcp timeout must not be negative")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxResponseBytes < 0 {
		return Config{}, nil, fmt.Errorf("mcp max response bytes must not be negative")
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = defaultMaxResponseBytes
	}
	if strings.TrimSpace(cfg.ClientName) == "" {
		cfg.ClientName = defaultClientName
	}
	if strings.TrimSpace(cfg.ClientVersion) == "" {
		cfg.ClientVersion = defaultClientVersion
	}
	if cfg.EndpointValidator == nil {
		cfg.EndpointValidator = ValidatePublicEndpoint
	}
	cfg.Headers = cfg.Headers.Clone()
	cfg.Endpoint = endpoint.String()
	return cfg, endpoint, nil
}

func connectSession(
	parent context.Context,
	deadline time.Time,
	cfg Config,
	httpClient *http.Client,
	transport Transport,
) (*mcp.ClientSession, context.CancelCauseFunc, error) {
	if !time.Now().Before(deadline) {
		return nil, nil, fmt.Errorf("mcp %s connect: %w", transport, context.DeadlineExceeded)
	}

	// Legacy SSE keeps its initial GET request alive for the entire session. A
	// normal WithTimeout followed by defer cancel would therefore kill a valid
	// session. This cancellable context enforces only the establishment deadline;
	// its timer and parent propagation are stopped after a successful initialize.
	connectCtx, cancel := context.WithCancelCause(context.WithoutCancel(parent))
	stopParent := context.AfterFunc(parent, func() { cancel(context.Cause(parent)) })
	timer := time.AfterFunc(time.Until(deadline), func() { cancel(context.DeadlineExceeded) })

	client := mcp.NewClient(&mcp.Implementation{
		Name:    cfg.ClientName,
		Version: cfg.ClientVersion,
	}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})

	var sdkTransport mcp.Transport
	switch transport {
	case TransportStreamableHTTP:
		sdkTransport = &mcp.StreamableClientTransport{
			Endpoint:             cfg.Endpoint,
			HTTPClient:           httpClient,
			MaxRetries:           -1,
			DisableStandaloneSSE: true,
		}
	case TransportSSE:
		sdkTransport = &mcp.SSEClientTransport{Endpoint: cfg.Endpoint, HTTPClient: httpClient}
	default:
		stopParent()
		timer.Stop()
		cancel(context.Canceled)
		return nil, nil, fmt.Errorf("unsupported mcp transport %q", transport)
	}

	session, err := client.Connect(connectCtx, sdkTransport, nil)
	if err != nil {
		stopParent()
		timer.Stop()
		cause := context.Cause(connectCtx)
		cancel(context.Canceled)
		if cause != nil {
			return nil, nil, fmt.Errorf("mcp %s connect: %w", transport, cause)
		}
		return nil, nil, fmt.Errorf("mcp %s connect: %w", transport, preserveAdapterError(err))
	}
	stopParent()
	timer.Stop()
	if parentErr := parent.Err(); parentErr != nil {
		cancel(parentErr)
	} else if !time.Now().Before(deadline) {
		cancel(context.DeadlineExceeded)
	}
	if cause := context.Cause(connectCtx); cause != nil {
		_ = session.Close()
		cancel(context.Canceled)
		return nil, nil, fmt.Errorf("mcp %s connect: %w", transport, cause)
	}
	return session, cancel, nil
}

// ListTools follows all opaque pagination cursors and returns a normalized list.
func (c *remoteClient) ListTools(ctx context.Context) ([]Tool, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("mcp client is not connected")
	}
	if ctx == nil {
		return nil, fmt.Errorf("mcp tools/list: nil context")
	}
	opCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var tools []Tool
	seen := make(map[string]struct{})
	var cursor string
	for {
		result, err := c.session.ListTools(opCtx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			if opErr := opCtx.Err(); opErr != nil {
				return nil, fmt.Errorf("mcp tools/list: %w", opErr)
			}
			return nil, fmt.Errorf("mcp tools/list: %w", preserveAdapterError(err))
		}
		if result == nil {
			return nil, fmt.Errorf("mcp tools/list returned a nil result")
		}
		for _, sdkTool := range result.Tools {
			tool, err := normalizeTool(sdkTool)
			if err != nil {
				return nil, err
			}
			tools = append(tools, tool)
		}
		if result.NextCursor == "" {
			return tools, nil
		}
		if _, duplicate := seen[result.NextCursor]; duplicate {
			return nil, fmt.Errorf("%w: %q", ErrPaginationLoop, result.NextCursor)
		}
		seen[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}
}

// CallTool invokes a named tool and normalizes text, structured, and raw content.
func (c *remoteClient) CallTool(ctx context.Context, name string, arguments any) (ToolResult, error) {
	if c == nil || c.session == nil {
		return ToolResult{}, fmt.Errorf("mcp client is not connected")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ToolResult{}, fmt.Errorf("mcp tool name is required")
	}
	if ctx == nil {
		return ToolResult{}, fmt.Errorf("mcp tools/call %q: nil context", name)
	}
	opCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result, err := c.session.CallTool(opCtx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		if opErr := opCtx.Err(); opErr != nil {
			return ToolResult{}, fmt.Errorf("mcp tools/call %q: %w", name, opErr)
		}
		return ToolResult{}, fmt.Errorf("mcp tools/call %q: %w", name, preserveAdapterError(err))
	}
	normalized, err := normalizeToolResult(result)
	if err != nil {
		return ToolResult{}, fmt.Errorf("normalize mcp tools/call %q: %w", name, err)
	}
	return normalized, nil
}

// Close gracefully terminates the MCP session. It is idempotent.
func (c *remoteClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.session != nil {
			c.closeErr = c.session.Close()
		}
		if c.lifecycleCancel != nil {
			c.lifecycleCancel(context.Canceled)
		}
	})
	return c.closeErr
}

func normalizeTool(tool *mcp.Tool) (Tool, error) {
	if tool == nil {
		return Tool{}, fmt.Errorf("mcp tools/list returned a nil tool")
	}
	input, err := normalizeJSON(tool.InputSchema)
	if err != nil {
		return Tool{}, fmt.Errorf("normalize input schema for tool %q: %w", tool.Name, err)
	}
	output, err := normalizeJSON(tool.OutputSchema)
	if err != nil {
		return Tool{}, fmt.Errorf("normalize output schema for tool %q: %w", tool.Name, err)
	}
	annotations, err := normalizeJSONObject(tool.Annotations)
	if err != nil {
		return Tool{}, fmt.Errorf("normalize annotations for tool %q: %w", tool.Name, err)
	}
	return Tool{
		Name:         tool.Name,
		Title:        tool.Title,
		Description:  tool.Description,
		InputSchema:  input,
		OutputSchema: output,
		Annotations:  annotations,
	}, nil
}

func normalizeToolResult(result *mcp.CallToolResult) (ToolResult, error) {
	if result == nil {
		return ToolResult{}, fmt.Errorf("server returned a nil tool result")
	}
	structured, err := normalizeJSONObject(result.StructuredContent)
	if err != nil {
		return ToolResult{}, fmt.Errorf("structured content must be a JSON object: %w", err)
	}

	content := make([]ContentBlock, 0, len(result.Content))
	texts := make([]string, 0, len(result.Content))
	for i, block := range result.Content {
		if block == nil {
			return ToolResult{}, fmt.Errorf("content block %d is nil", i)
		}
		encoded, err := block.MarshalJSON()
		if err != nil {
			return ToolResult{}, fmt.Errorf("marshal content block %d: %w", i, err)
		}
		var raw map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(encoded)))
		decoder.UseNumber()
		if err := decoder.Decode(&raw); err != nil {
			return ToolResult{}, fmt.Errorf("decode content block %d: %w", i, err)
		}
		normalized := ContentBlock{
			Type:     stringValue(raw["type"]),
			Text:     stringValue(raw["text"]),
			Data:     stringValue(raw["data"]),
			MIMEType: stringValue(raw["mimeType"]),
			Raw:      raw,
		}
		if normalized.Type == "text" {
			texts = append(texts, normalized.Text)
		}
		content = append(content, normalized)
	}
	return ToolResult{
		IsError:           result.IsError,
		Text:              strings.Join(texts, "\n"),
		StructuredContent: structured,
		Content:           content,
	}, nil
}

func normalizeJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeJSONObject(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	normalized, err := normalizeJSON(value)
	if err != nil {
		return nil, err
	}
	if normalized == nil {
		return nil, nil
	}
	object, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("got %T", normalized)
	}
	return object, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

// The upstream SDK preserves most causes, but a few asynchronous HTTP reader
// paths currently format errors with %v. Reattach adapter-owned sentinels so
// callers can still classify policy and size failures with errors.Is.
func preserveAdapterError(err error) error {
	if err == nil {
		return nil
	}
	for _, sentinel := range []error{ErrResponseTooLarge, ErrUnsafeEndpoint} {
		if errors.Is(err, sentinel) {
			return err
		}
		if strings.Contains(err.Error(), sentinel.Error()) {
			return fmt.Errorf("%w: %v", sentinel, err)
		}
	}
	return err
}

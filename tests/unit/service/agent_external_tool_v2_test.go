package service_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/platform/mcpclient"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

type externalToolFakeMCPClient struct {
	tools []mcpclient.Tool
}

func (c *externalToolFakeMCPClient) ListTools(context.Context) ([]mcpclient.Tool, error) {
	return append([]mcpclient.Tool(nil), c.tools...), nil
}

func (*externalToolFakeMCPClient) CallTool(context.Context, string, any) (mcpclient.ToolResult, error) {
	return mcpclient.ToolResult{}, nil
}

func (*externalToolFakeMCPClient) Close() error { return nil }

type externalToolFakeHTTPExecutor struct {
	probeResult externaltool.ProbeResult
	probeCalls  int
	callCalls   int
}

func (e *externalToolFakeHTTPExecutor) Probe(context.Context, externaltool.ProbeRequest) (externaltool.ProbeResult, error) {
	e.probeCalls++
	return e.probeResult, nil
}

func (e *externalToolFakeHTTPExecutor) Call(context.Context, externaltool.Request) (externaltool.Result, error) {
	e.callCalls++
	return externaltool.Result{}, nil
}

func TestExternalToolMCPDiscoveryPersistsStableCapabilities(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	client := &externalToolFakeMCPClient{tools: []mcpclient.Tool{{
		Name:        "lookup_ticket",
		Description: "Read one support ticket",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
		Annotations: map[string]any{"readOnlyHint": true},
	}}}
	var configs []mcpclient.Config
	connector := mcpclient.ConnectorFunc(func(_ context.Context, cfg mcpclient.Config) (mcpclient.Client, error) {
		configs = append(configs, cfg)
		return client, nil
	})
	svc := service.New(store, service.Options{
		Now:              func() time.Time { return now },
		CredentialCipher: newTestCredentialCipher(t),
		MCPConnector:     connector,
	})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	created, err := agentservice.New(svc).CreateExternalTool(ctx, domain.CreateAgentExternalToolInput{
		Name: "Support", Kind: "mcp", Transport: "streamable_http",
		EndpointURL: "https://tools.example.com/mcp", AuthType: "bearer", AuthSecret: "mcp-secret",
		TimeoutSeconds: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	discovered, err := agentservice.New(svc).DiscoverExternalTool(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Capabilities) != 1 || discovered.Capabilities[0].ToolName != "lookup_ticket" || !discovered.Capabilities[0].Readonly {
		t.Fatalf("unexpected discovered capabilities: %+v", discovered.Capabilities)
	}
	firstID := discovered.Capabilities[0].ID
	firstChecksum := discovered.Capabilities[0].SchemaChecksum
	if firstID == "" || firstChecksum == "" {
		t.Fatalf("expected stable capability identity and checksum: %+v", discovered.Capabilities[0])
	}
	if len(configs) != 1 || configs[0].Headers.Get("Authorization") != "Bearer mcp-secret" || configs[0].Timeout != 12*time.Second {
		t.Fatalf("unexpected MCP connection config: %+v", configs)
	}

	client.tools[0].Description = "Read a support ticket without mutation"
	discovered, err = agentservice.New(svc).DiscoverExternalTool(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if discovered.Capabilities[0].ID != firstID {
		t.Fatalf("capability ID changed across discovery: before=%s after=%s", firstID, discovered.Capabilities[0].ID)
	}
	if discovered.Capabilities[0].SchemaChecksum == firstChecksum {
		t.Fatal("expected capability checksum to change with the discovered contract")
	}
	if discovered.LastTestStatus != string(domain.ConnectionTestStatusOK) || discovered.LastTestedAt == nil {
		t.Fatalf("expected discovery to update connection health: %+v", discovered)
	}
}

func TestManualHTTPExternalToolCreatesCapabilityAndUsesSafeProbe(t *testing.T) {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	seedAgentAdminAccount(t, store, now)
	executor := &externalToolFakeHTTPExecutor{probeResult: externaltool.ProbeResult{StatusCode: http.StatusNoContent}}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }, ExternalHTTPExecutor: executor})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}

	created, err := agentservice.New(svc).CreateExternalTool(ctx, domain.CreateAgentExternalToolInput{
		Name: "Customer API", Kind: "http", EndpointURL: "https://api.example.com/v1", AuthType: "none",
		ToolName: "get_customer", HTTPMethod: "GET", HTTPPath: "customers/get",
		InputSchema: map[string]any{"type": "object"}, Readonly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Capabilities) != 1 || created.Capabilities[0].HTTPMethod != http.MethodGet || created.Capabilities[0].SchemaChecksum == "" {
		t.Fatalf("unexpected manual HTTP capability: %+v", created.Capabilities)
	}
	tested, err := agentservice.New(svc).TestExternalTool(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tested.LastTestStatus != string(domain.ConnectionTestStatusOK) || executor.probeCalls != 1 || executor.callCalls != 0 {
		t.Fatalf("connection test invoked business operation or did not persist health: tested=%+v probe=%d call=%d", tested, executor.probeCalls, executor.callCalls)
	}
	if _, err := agentservice.New(svc).DiscoverExternalTool(ctx, created.ID); err == nil {
		t.Fatal("manual HTTP capability discovery should be rejected")
	}
	archived, err := agentservice.New(svc).DeleteExternalTool(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != string(domain.ExternalToolConnectionStatusArchived) || archived.ArchivedAt == nil {
		t.Fatalf("expected archive semantics, got %+v", archived)
	}
	items, err := agentservice.New(svc).ListExternalTools(ctx)
	if err != nil || len(items) != 1 || items[0].Status != string(domain.ExternalToolConnectionStatusArchived) {
		t.Fatalf("archived connection history is not visible: items=%+v err=%v", items, err)
	}
}

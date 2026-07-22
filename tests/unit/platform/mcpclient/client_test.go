package mcpclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"nexus-pro-api/internal/platform/mcpclient"
)

func TestStreamableHTTPListsAllPagesCallsToolAndAddsHeaders(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "v1"},
		&mcp.ServerOptions{PageSize: 1},
	)
	server.AddTool(&mcp.Tool{
		Name:        "alpha",
		Title:       "Alpha tool",
		Description: "first tool",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "hello " + fmt.Sprint(arguments["name"])},
				&mcp.TextContent{Text: "done"},
			},
			StructuredContent: map[string]any{"ok": true, "count": 2},
		}, nil
	})
	server.AddTool(&mcp.Tool{
		Name:        "beta",
		Description: "second tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "beta"}}}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	var authenticatedRequests atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer top-secret" || r.Header.Get("X-Tenant") != "tenant-1" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		authenticatedRequests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	client, err := mcpclient.Connect(context.Background(), mcpclient.Config{
		Endpoint:  httpServer.URL,
		Transport: mcpclient.TransportStreamableHTTP,
		Headers: http.Header{
			"Authorization": {"Bearer top-secret"},
			"X-Tenant":      {"tenant-1"},
		},
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
		Timeout:           2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("ListTools() returned %d tools, want 2: %+v", len(tools), tools)
	}
	names := []string{tools[0].Name, tools[1].Name}
	sort.Strings(names)
	if strings.Join(names, ",") != "alpha,beta" {
		t.Fatalf("tool names = %v", names)
	}
	var alpha mcpclient.Tool
	for _, tool := range tools {
		if tool.Name == "alpha" {
			alpha = tool
		}
	}
	if alpha.Title != "Alpha tool" || alpha.Annotations["readOnlyHint"] != true {
		t.Fatalf("normalized alpha tool = %+v", alpha)
	}

	result, err := client.CallTool(context.Background(), "alpha", map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "hello Ada\ndone" || result.IsError {
		t.Fatalf("normalized text result = %+v", result)
	}
	if result.StructuredContent["ok"] != true || fmt.Sprint(result.StructuredContent["count"]) != "2" {
		t.Fatalf("normalized structured result = %+v", result.StructuredContent)
	}
	if len(result.Content) != 2 || result.Content[0].Type != "text" {
		t.Fatalf("normalized content = %+v", result.Content)
	}
	if authenticatedRequests.Load() < 5 { // initialize, initialized, two list pages, call
		t.Fatalf("only %d authenticated requests observed", authenticatedRequests.Load())
	}
}

func TestAutoTransportFallsBackToLegacySSE(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "legacy", Version: "v1"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "legacy-tool",
		Description: "served over SSE",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "legacy-ok"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()

	client, err := mcpclient.Connect(context.Background(), mcpclient.Config{
		Endpoint:          httpServer.URL,
		Transport:         mcpclient.TransportAuto,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
		Timeout:           2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "legacy-tool" {
		t.Fatalf("legacy tools = %+v", tools)
	}
	result, err := client.CallTool(context.Background(), "legacy-tool", map[string]any{})
	if err != nil || result.Text != "legacy-ok" {
		t.Fatalf("legacy call result=%+v err=%v", result, err)
	}
}

func TestDefaultValidatorRejectsLocalAndPrivateDestinations(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1/mcp",
		"http://[::1]/mcp",
		"http://10.2.3.4/mcp",
		"http://172.16.0.1/mcp",
		"http://192.168.1.10/mcp",
		"http://169.254.169.254/latest/meta-data",
	} {
		t.Run(rawURL, func(t *testing.T) {
			endpoint, err := url.Parse(rawURL)
			if err != nil {
				t.Fatal(err)
			}
			err = mcpclient.ValidatePublicEndpoint(context.Background(), endpoint)
			if !errors.Is(err, mcpclient.ErrUnsafeEndpoint) {
				t.Fatalf("ValidatePublicEndpoint(%q) error = %v, want ErrUnsafeEndpoint", rawURL, err)
			}
		})
	}
}

func TestNewBoundedHTTPClientSupportsManualHTTPWithoutDuplicatingSafetyLogic(t *testing.T) {
	var gotHeader atomic.Bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Manual-Auth") == "secret" {
			gotHeader.Store(true)
		}
		w.Header().Set("Content-Length", "64")
		_, _ = w.Write([]byte(strings.Repeat("x", 64)))
	}))
	defer httpServer.Close()

	client, endpoint, err := mcpclient.NewBoundedHTTPClient(
		context.Background(),
		httpServer.URL,
		http.Header{"X-Manual-Auth": {"secret"}},
		time.Second,
		32,
		mcpclient.AllowPrivateEndpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.String() != httpServer.URL {
		t.Fatalf("normalized endpoint = %q, want %q", endpoint, httpServer.URL)
	}
	_, err = client.Get(endpoint.String())
	if !errors.Is(err, mcpclient.ErrResponseTooLarge) {
		t.Fatalf("manual GET error = %v, want ErrResponseTooLarge", err)
	}
	if !gotHeader.Load() {
		t.Fatal("manual HTTP request did not receive configured same-origin header")
	}
}

func TestResponseSizeLimitIsEnforced(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "large", Version: "v1"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "large-tool",
		Description: strings.Repeat("x", 8<<10),
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client, err := mcpclient.Connect(context.Background(), mcpclient.Config{
		Endpoint:          httpServer.URL,
		Transport:         mcpclient.TransportStreamableHTTP,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
		MaxResponseBytes:  1024,
		Timeout:           2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.ListTools(context.Background())
	if !errors.Is(err, mcpclient.ErrResponseTooLarge) {
		t.Fatalf("ListTools() error = %v, want ErrResponseTooLarge", err)
	}
}

func TestOperationTimeoutCancelsSlowToolCall(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "slow", Version: "v1"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "slow-tool",
		Description: "waits for cancellation",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client, err := mcpclient.Connect(context.Background(), mcpclient.Config{
		Endpoint:          httpServer.URL,
		Transport:         mcpclient.TransportStreamableHTTP,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
		Timeout:           75 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.CallTool(context.Background(), "slow-tool", map[string]any{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CallTool() error = %v, want context deadline exceeded", err)
	}
}

func TestRedirectPolicyBlocksNonHTTPAndDoesNotLeakHeadersAcrossOrigins(t *testing.T) {
	t.Run("non-http redirect", func(t *testing.T) {
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "file:///tmp/not-allowed")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer redirector.Close()

		_, err := mcpclient.Connect(context.Background(), mcpclient.Config{
			Endpoint:          redirector.URL,
			Transport:         mcpclient.TransportStreamableHTTP,
			EndpointValidator: mcpclient.AllowPrivateEndpoints,
			Timeout:           time.Second,
		})
		if !errors.Is(err, mcpclient.ErrUnsafeEndpoint) {
			t.Fatalf("Connect() error = %v, want ErrUnsafeEndpoint", err)
		}
	})

	t.Run("cross-origin header", func(t *testing.T) {
		var leaked atomic.Bool
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-MCP-Secret") != "" {
				leaked.Store(true)
			}
			http.Error(w, "not an MCP server", http.StatusBadGateway)
		}))
		defer target.Close()
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer redirector.Close()

		_, _ = mcpclient.Connect(context.Background(), mcpclient.Config{
			Endpoint:          redirector.URL,
			Transport:         mcpclient.TransportStreamableHTTP,
			Headers:           http.Header{"X-MCP-Secret": {"do-not-leak"}},
			EndpointValidator: mcpclient.AllowPrivateEndpoints,
			Timeout:           time.Second,
		})
		if leaked.Load() {
			t.Fatal("custom MCP header leaked to a redirected origin")
		}
	})
}

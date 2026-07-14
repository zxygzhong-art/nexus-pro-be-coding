package v1_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexus-pro-be/internal/domain"
)

func TestWorkspaceAgentExternalToolLifecycle(t *testing.T) {
	api := newTestAPI(true)
	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/workspace/agents/external-tools",
		strings.NewReader(`{"name":"Support MCP","description":"Support knowledge","kind":"mcp","transport":"streamable_http","endpoint_url":"https://tools.example.com/mcp","auth_type":"none"}`),
	)
	createRequest.Header.Set("Content-Type", "application/json")
	createResponse := httptest.NewRecorder()
	api.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	created := decodeData[domain.AgentExternalTool](t, createResponse.Body.Bytes())
	if created.ID == "" || created.Name != "Support MCP" || created.Transport != "streamable_http" || created.AuthType != "none" || created.CredentialSet {
		t.Fatalf("unexpected created external tool: %+v", created)
	}
	if strings.Contains(createResponse.Body.String(), "auth_secret") {
		t.Fatalf("credential field leaked in response: %s", createResponse.Body.String())
	}

	listResponse := httptest.NewRecorder()
	api.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/v1/workspace/agents/external-tools", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
	listed := decodeData[struct {
		Items []domain.AgentExternalTool `json:"items"`
		Total int                        `json:"total"`
	}](t, listResponse.Body.Bytes())
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected external tool list: %+v", listed)
	}

	deleteResponse := httptest.NewRecorder()
	api.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/v1/workspace/agents/external-tools/"+created.ID, nil))
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d: %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	deleted := decodeData[domain.AgentExternalTool](t, deleteResponse.Body.Bytes())
	if deleted.ID != created.ID {
		t.Fatalf("unexpected deleted external tool: %+v", deleted)
	}
}

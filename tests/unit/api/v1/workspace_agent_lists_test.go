package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// newTypedListTestAPI builds an authenticated API with the requested permission boundary and service dependencies.
func newTypedListTestAPI(
	t *testing.T,
	permissions []domain.Permission,
	svcOptions service.Options,
	mutateStore func(*memory.Store, time.Time),
) http.Handler {
	t.Helper()
	now := time.Date(2026, 7, 17, 9, 30, 0, 0, time.UTC)
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "demo", Name: "Demo", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	permissionSetIDs := []string{}
	if len(permissions) > 0 {
		permissionSetIDs = []string{"ps-typed-list"}
		if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
			ID: "ps-typed-list", TenantID: "demo", Name: "Typed List", Permissions: permissions, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-typed-list", TenantID: "demo", DisplayName: "Typed List", Status: "active",
		DirectPermissionSetIDs: permissionSetIDs, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
		ID: "identity-typed-list", TenantID: "demo", AccountID: "acct-typed-list",
		Provider: domain.IdentityProviderKeycloak, Subject: "acct-typed-list", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if mutateStore != nil {
		mutateStore(store, now)
	}
	svcOptions.Now = func() time.Time { return now }
	return v1api.New(service.New(store, svcOptions), nil, v1api.Options{
		TokenResolver: staticTokenResolver{ctx: v1api.TokenContext{
			Provider: "keycloak", Subject: "acct-typed-list", TenantID: "demo", AccountID: "acct-typed-list",
		}, ok: true},
	}).Routes()
}

// assertDataEnvelopeKeys verifies the success envelope carries exactly the documented data keys.
func assertDataEnvelopeKeys(t *testing.T, body []byte, keys ...string) {
	t.Helper()
	var payload struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != len(keys) {
		t.Fatalf("expected data keys %v, got %s", keys, body)
	}
	for _, key := range keys {
		if _, ok := payload.Data[key]; !ok {
			t.Fatalf("expected data key %q, got %s", key, body)
		}
	}
}

// staticKnowledgeEmbedder returns one deterministic unit vector per input so memory-store cosine search scores 1.0.
type staticKnowledgeEmbedder struct{}

func (staticKnowledgeEmbedder) Model() string { return "nexus-pro-embedding" }

func (staticKnowledgeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, len(inputs))
	for index := range inputs {
		vectors[index] = []float32{1, 0, 0}
	}
	return vectors, nil
}

// TestWorkspaceAgentListEndpointsReturnTypedResponses keeps workspace agent list payloads on their typed items/total contract.
func TestWorkspaceAgentListEndpointsReturnTypedResponses(t *testing.T) {
	handler := newTypedListTestAPI(t, []domain.Permission{
		{Resource: "agent.model", Action: "read", Scope: "all"},
		{Resource: "agent.definition", Action: "read", Scope: "all"},
		{Resource: "agent.tool", Action: "read", Scope: "all"},
	}, service.Options{}, func(store *memory.Store, now time.Time) {
		if err := store.UpsertAgentModel(context.Background(), domain.AgentModel{
			ID: "amodel-list", TenantID: "demo", Name: "List Model", Provider: "openai", ModelName: "gpt-4o-mini",
			LiteLLMModel: "amodel-list", APIKeySet: true, RateLimitRPM: 60, Status: domain.AgentModelStatusActive,
			TimeoutSeconds: 60, LastTestStatus: "untested", SyncStatus: domain.AgentModelSyncStatusPending,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertAgentDefinition(context.Background(), domain.AgentDefinition{
			ID: "agent-list", TenantID: "demo", Name: "List Agent", ModelID: "amodel-list",
			Status: domain.AgentDefinitionStatusDraft, Visibility: domain.AgentVisibilityAll, Version: 1,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("agent models", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/agent-models", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected list status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		assertDataEnvelopeKeys(t, recorder.Body.Bytes(), "items", "total")
		listed := decodeData[domain.AgentModelListResponse](t, recorder.Body.Bytes())
		if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != "amodel-list" {
			t.Fatalf("unexpected agent model list: %+v", listed)
		}
	})

	t.Run("agent definitions", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/agents", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected list status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		assertDataEnvelopeKeys(t, recorder.Body.Bytes(), "items", "total")
		listed := decodeData[domain.AgentDefinitionListResponse](t, recorder.Body.Bytes())
		if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != "agent-list" {
			t.Fatalf("unexpected agent definition list: %+v", listed)
		}
	})

	t.Run("agent tools", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/agents/tools", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected list status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		assertDataEnvelopeKeys(t, recorder.Body.Bytes(), "items", "total")
		listed := decodeData[domain.AgentToolMetaListResponse](t, recorder.Body.Bytes())
		if listed.Total != 24 || listed.Total != len(listed.Items) || listed.Items[0].Value == "" {
			t.Fatalf("unexpected agent tool list: %+v", listed)
		}
		for _, tool := range listed.Items {
			if tool.Category == "" || tool.DescriptionZhTW == "" || tool.RequiredPermission == "" {
				t.Fatalf("agent tool is missing display or authorization metadata: %+v", tool)
			}
		}
	})
}

// TestWorkspaceKnowledgeListAndSearchReturnTypedResponses keeps knowledge list and search payloads on their typed contracts.
func TestWorkspaceKnowledgeListAndSearchReturnTypedResponses(t *testing.T) {
	handler := newTypedListTestAPI(t, []domain.Permission{
		{Resource: "agent.knowledge_base", Action: "read", Scope: "all"},
		{Resource: "agent.knowledge_base", Action: "create", Scope: "all"},
		{Resource: "agent.knowledge_base", Action: "update", Scope: "all"},
	}, service.Options{KnowledgeEmbedder: staticKnowledgeEmbedder{}}, nil)

	createRequest := httptest.NewRequest(http.MethodPost, "/v1/workspace/knowledge-bases", strings.NewReader(`{"name":"支援知識庫"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected create status 201, got %d: %s", createRecorder.Code, createRecorder.Body.String())
	}
	base := decodeData[domain.KnowledgeBase](t, createRecorder.Body.Bytes())

	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/knowledge-bases", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listRecorder.Code, listRecorder.Body.String())
	}
	assertDataEnvelopeKeys(t, listRecorder.Body.Bytes(), "items", "total", "page", "page_size", "sort")
	bases := decodeData[domain.PageResponse[domain.KnowledgeBase]](t, listRecorder.Body.Bytes())
	if bases.Total != 1 || bases.Page != 1 || bases.PageSize != domain.DefaultPageSize || len(bases.Items) != 1 || bases.Items[0].ID != base.ID || bases.Items[0].DocumentCount != 0 {
		t.Fatalf("unexpected knowledge base list: %+v", bases)
	}

	documentRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/workspace/knowledge-bases/"+base.ID+"/documents",
		strings.NewReader(`{"title":"加班規則","content":"加班需事先申請。"}`),
	)
	documentRequest.Header.Set("Content-Type", "application/json")
	documentRecorder := httptest.NewRecorder()
	handler.ServeHTTP(documentRecorder, documentRequest)
	if documentRecorder.Code != http.StatusCreated {
		t.Fatalf("expected document create status 201, got %d: %s", documentRecorder.Code, documentRecorder.Body.String())
	}
	createdDocument := decodeData[domain.KnowledgeDocument](t, documentRecorder.Body.Bytes())

	documentsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(documentsRecorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/knowledge-bases/"+base.ID+"/documents", nil))
	if documentsRecorder.Code != http.StatusOK {
		t.Fatalf("expected documents list status 200, got %d: %s", documentsRecorder.Code, documentsRecorder.Body.String())
	}
	assertDataEnvelopeKeys(t, documentsRecorder.Body.Bytes(), "items", "total", "page", "page_size", "sort")
	documents := decodeData[domain.PageResponse[domain.KnowledgeDocument]](t, documentsRecorder.Body.Bytes())
	if documents.Total != 1 || len(documents.Items) != 1 || documents.Items[0].Title != "加班規則" {
		t.Fatalf("unexpected knowledge document list: %+v", documents)
	}
	var rawDocumentList struct {
		Data struct {
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(documentsRecorder.Body.Bytes(), &rawDocumentList); err != nil {
		t.Fatal(err)
	}
	if _, ok := rawDocumentList.Data.Items[0]["content"]; ok {
		t.Fatalf("document list items must not include full content: %s", documentsRecorder.Body.String())
	}

	detailRecorder := httptest.NewRecorder()
	handler.ServeHTTP(detailRecorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/knowledge-bases/"+base.ID+"/documents/"+createdDocument.ID, nil))
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("expected document detail status 200, got %d: %s", detailRecorder.Code, detailRecorder.Body.String())
	}
	detail := decodeData[domain.KnowledgeDocument](t, detailRecorder.Body.Bytes())
	if detail.ID != createdDocument.ID || detail.Content != "加班需事先申請。" {
		t.Fatalf("document detail did not return full content: %+v", detail)
	}

	searchRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/workspace/knowledge-bases/"+base.ID+"/search",
		strings.NewReader(`{"query":"加班"}`),
	)
	searchRequest.Header.Set("Content-Type", "application/json")
	searchRecorder := httptest.NewRecorder()
	handler.ServeHTTP(searchRecorder, searchRequest)
	if searchRecorder.Code != http.StatusOK {
		t.Fatalf("expected search status 200, got %d: %s", searchRecorder.Code, searchRecorder.Body.String())
	}
	assertDataEnvelopeKeys(t, searchRecorder.Body.Bytes(), "items", "total", "query", "semantics")
	result := decodeData[domain.KnowledgeSearchResponse](t, searchRecorder.Body.Bytes())
	if result.Query != "加班" || result.Semantics == "" || result.Total != len(result.Items) || result.Total != 1 {
		t.Fatalf("unexpected knowledge search response: %+v", result)
	}
	if result.Items[0].KnowledgeBaseID != base.ID || result.Items[0].Source == "" {
		t.Fatalf("unexpected knowledge search hit: %+v", result.Items[0])
	}

	secondDocumentRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/workspace/knowledge-bases/"+base.ID+"/documents",
		strings.NewReader(`{"title":"請假規則","content":"請假需提前一天提出。"}`),
	)
	secondDocumentRequest.Header.Set("Content-Type", "application/json")
	secondDocumentRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondDocumentRecorder, secondDocumentRequest)
	if secondDocumentRecorder.Code != http.StatusCreated {
		t.Fatalf("expected second document create status 201, got %d: %s", secondDocumentRecorder.Code, secondDocumentRecorder.Body.String())
	}
	pagedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(pagedRecorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/knowledge-bases/"+base.ID+"/documents?page=2&page_size=1", nil))
	if pagedRecorder.Code != http.StatusOK {
		t.Fatalf("expected paged documents status 200, got %d: %s", pagedRecorder.Code, pagedRecorder.Body.String())
	}
	paged := decodeData[domain.PageResponse[domain.KnowledgeDocument]](t, pagedRecorder.Body.Bytes())
	if paged.Total != 2 || paged.Page != 2 || paged.PageSize != 1 || len(paged.Items) != 1 {
		t.Fatalf("unexpected paged document list: %+v", paged)
	}
	firstPageRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstPageRecorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/knowledge-bases/"+base.ID+"/documents?page=1&page_size=1", nil))
	firstPage := decodeData[domain.PageResponse[domain.KnowledgeDocument]](t, firstPageRecorder.Body.Bytes())
	if len(firstPage.Items) != 1 || firstPage.Items[0].ID == paged.Items[0].ID {
		t.Fatalf("paged document lists must be disjoint: first=%+v second=%+v", firstPage.Items, paged.Items)
	}
	seenTitles := map[string]bool{firstPage.Items[0].Title: true, paged.Items[0].Title: true}
	if !seenTitles["加班規則"] || !seenTitles["請假規則"] {
		t.Fatalf("paged document lists did not cover both documents: %v", seenTitles)
	}

	basesRecorder := httptest.NewRecorder()
	handler.ServeHTTP(basesRecorder, httptest.NewRequest(http.MethodGet, "/v1/workspace/knowledge-bases?page=1&page_size=10", nil))
	counted := decodeData[domain.PageResponse[domain.KnowledgeBase]](t, basesRecorder.Body.Bytes())
	if len(counted.Items) != 1 || counted.Items[0].DocumentCount != 2 {
		t.Fatalf("knowledge base list did not aggregate document counts: %+v", counted)
	}
}

// TestAgentSessionFileListReturnsTypedResponse keeps the session file list payload on its typed items/total contract.
func TestAgentSessionFileListReturnsTypedResponse(t *testing.T) {
	handler := newTypedListTestAPI(t, []domain.Permission{
		{Resource: "agent.run", Action: "read", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
	}, service.Options{ObjectStore: service.NewMemoryObjectStore()}, nil)

	createRequest := httptest.NewRequest(http.MethodPost, "/v1/agents/sessions", strings.NewReader(`{"title":"附件對話"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected session creation, got %d: %s", createRecorder.Code, createRecorder.Body.String())
	}
	session := decodeData[domain.AgentSession](t, createRecorder.Body.Bytes())

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("typed list payload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadRequest := httptest.NewRequest(http.MethodPost, "/v1/agents/sessions/"+session.ID+"/files", &form)
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRecorder := httptest.NewRecorder()
	handler.ServeHTTP(uploadRecorder, uploadRequest)
	if uploadRecorder.Code != http.StatusCreated {
		t.Fatalf("expected file upload, got %d: %s", uploadRecorder.Code, uploadRecorder.Body.String())
	}
	file := decodeData[domain.AgentSessionFile](t, uploadRecorder.Body.Bytes())

	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/v1/agents/sessions/"+session.ID+"/files", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("expected file list status 200, got %d: %s", listRecorder.Code, listRecorder.Body.String())
	}
	assertDataEnvelopeKeys(t, listRecorder.Body.Bytes(), "items", "total")
	listed := decodeData[domain.AgentSessionFileListResponse](t, listRecorder.Body.Bytes())
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != file.ID {
		t.Fatalf("unexpected session file list: %+v", listed)
	}
}

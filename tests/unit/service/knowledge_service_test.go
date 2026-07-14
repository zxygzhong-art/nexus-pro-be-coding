package service_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestKnowledgeCRUDSearchAndAgentBindings covers the in-memory MVP lifecycle and runtime citations.
func TestKnowledgeCRUDSearchAndAgentBindings(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store, svc, ctx := newKnowledgeFixture(t, now)

	base, err := svc.Agent().CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{Name: "HR Policies", Description: "Current HR policies"})
	if err != nil {
		t.Fatal(err)
	}
	document, err := svc.Agent().CreateKnowledgeDocument(ctx, base.ID, domain.CreateKnowledgeDocumentInput{
		Title: "Annual Leave Policy", Content: "Employees have 12 annual leave days each calendar year.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if document.SourceType != "manual" {
		t.Fatalf("expected manual source type, got %+v", document)
	}
	result, err := svc.Agent().SearchKnowledge(ctx, domain.KnowledgeSearchInput{
		Query: "ANNUAL employees", KnowledgeBaseIDs: []string{base.ID}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Hits[0].DocumentID != document.ID || !strings.HasPrefix(result.Hits[0].Source, "knowledge://") {
		t.Fatalf("unexpected AND search result: %+v", result)
	}
	if miss, err := svc.Agent().SearchKnowledge(ctx, domain.KnowledgeSearchInput{Query: "annual contractor", KnowledgeBaseIDs: []string{base.ID}}); err != nil || miss.Total != 0 {
		t.Fatalf("expected AND semantics to reject partial matches: result=%+v err=%v", miss, err)
	}

	model, err := svc.Agent().CreateModel(ctx, domain.CreateAgentModelInput{Name: "Knowledge Model", ModelName: "knowledge-model", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := svc.Agent().CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name: "Policy Helper", ModelID: model.ID, Tools: []string{"knowledge.search"}, KnowledgeBaseIDs: []string{base.ID},
		SubAgents: []domain.AgentTeamMember{{ID: "policy", Name: "Policy", Role: "search policy", ModelID: model.ID, Tools: []string{"knowledge.search"}, KnowledgeBaseIDs: []string{base.ID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.Agent().GetDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.KnowledgeBaseIDs) != 1 || len(listed.SubAgents) != 1 || len(listed.SubAgents[0].KnowledgeBaseIDs) != 1 || len(listed.Versions) != 1 || len(listed.Versions[0].KnowledgeBaseIDs) != 1 {
		t.Fatalf("knowledge bindings did not round-trip through agent and version snapshots: %+v", listed)
	}
	agent, err = svc.Agent().PublishDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.Agent().CreateRun(ctx, domain.CreateAgentRunInput{AgentID: agent.ID, Prompt: "annual employees"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.References) != 1 || run.References[0].Source != result.Hits[0].Source {
		t.Fatalf("runtime knowledge search did not return a bound citation: %+v", run)
	}
	if _, err := svc.Agent().DeleteKnowledgeBase(ctx, base.ID); err == nil {
		t.Fatal("expected bound knowledge base deletion to conflict")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 409 {
		t.Fatalf("expected conflict deleting bound knowledge base, got %v", err)
	}

	updatedTitle := "Annual Leave Rules"
	if _, err := svc.Agent().UpdateKnowledgeDocument(ctx, base.ID, document.ID, domain.UpdateKnowledgeDocumentInput{Title: &updatedTitle}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Agent().DeleteKnowledgeDocument(ctx, base.ID, document.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := store.ListKnowledgeDocuments(context.Background(), "tenant-1", base.ID)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("expected document CRUD to remove the document: items=%+v err=%v", remaining, err)
	}
}

// TestKnowledgeUploadsAndChunkConfiguration covers text/PDF extraction, storage cleanup, and reindex settings.
func TestKnowledgeUploadsAndChunkConfiguration(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store, _, ctx := newKnowledgeFixture(t, now)
	objects := service.NewMemoryObjectStore()
	svc := service.New(store, service.Options{
		Now: func() time.Time { return now }, KnowledgeEmbedder: deterministicKnowledgeEmbedder{}, ObjectStore: objects,
	})
	chunkSize, overlap := 600, 60
	base, err := svc.Agent().CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{
		Name: "Uploaded policies", ChunkMode: "paragraph", ChunkSize: &chunkSize, ChunkOverlap: &overlap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if base.ChunkMode != "paragraph" || base.ChunkSize != chunkSize || base.ChunkOverlap != overlap {
		t.Fatalf("chunk configuration did not persist: %+v", base)
	}

	textDocument, err := svc.Agent().UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
		Filename: "leave-policy.md", ContentType: "text/markdown", Content: []byte("# Leave\n\nEmployees must submit leave before the start date."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if textDocument.SourceType != "text" || textDocument.OriginalFilename != "leave-policy.md" || textDocument.ParseStatus != "ready" || textDocument.SHA256 == "" {
		t.Fatalf("unexpected uploaded text metadata: %+v", textDocument)
	}
	if stored, getErr := objects.GetObject(context.Background(), textDocument.ObjectKey); getErr != nil || !bytes.Equal(stored, []byte("# Leave\n\nEmployees must submit leave before the start date.")) {
		t.Fatalf("uploaded source object did not round-trip: content=%q err=%v", stored, getErr)
	}

	pdfDocument, err := svc.Agent().UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
		Filename: "attendance.pdf", ContentType: "application/pdf", Content: minimalTextPDF("Attendance policy requires clock records."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if pdfDocument.SourceType != "pdf" || !strings.Contains(pdfDocument.Content, "Attendance policy") {
		t.Fatalf("PDF text was not extracted: %+v", pdfDocument)
	}

	if _, err := svc.Agent().UpdateKnowledgeDocument(ctx, base.ID, textDocument.ID, domain.UpdateKnowledgeDocumentInput{}); err == nil {
		t.Fatal("expected uploaded sources to reject manual edits")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 409 {
		t.Fatalf("expected uploaded-source edit conflict, got %v", err)
	}

	fixedSize, fixedOverlap, fixedMode := 200, 20, "fixed"
	updated, err := svc.Agent().UpdateKnowledgeBase(ctx, base.ID, domain.UpdateKnowledgeBaseInput{
		ChunkMode: &fixedMode, ChunkSize: &fixedSize, ChunkOverlap: &fixedOverlap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ChunkMode != fixedMode || updated.ChunkSize != fixedSize || updated.ChunkOverlap != fixedOverlap {
		t.Fatalf("updated chunk configuration did not persist: %+v", updated)
	}

	if _, err := svc.Agent().DeleteKnowledgeDocument(ctx, base.ID, textDocument.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := objects.GetObject(context.Background(), textDocument.ObjectKey); err == nil {
		t.Fatal("expected deleting an uploaded source to remove its object")
	}
}

// TestAgentKnowledgeBindingRequiresReadableExistingBase closes guessed-ID binding paths.
func TestAgentKnowledgeBindingRequiresReadableExistingBase(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store, svc, adminCtx := newKnowledgeFixture(t, now)
	base, err := svc.Agent().CreateKnowledgeBase(adminCtx, domain.CreateKnowledgeBaseInput{Name: "Private"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := svc.Agent().CreateModel(adminCtx, domain.CreateAgentModelInput{Name: "Model", ModelName: "model", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID: "ps-agent-editor", TenantID: "tenant-1", Name: "Agent Editor", CreatedAt: now,
		Permissions: []domain.Permission{
			{Resource: "agent.model", Action: "read", Scope: "all"},
			{Resource: "agent.definition", Action: "create", Scope: "all"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID: "acct-editor", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-agent-editor"}, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Agent().CreateDefinition(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-editor"}, domain.CreateAgentDefinitionInput{
		Name: "Guessed Binding", ModelID: model.ID, KnowledgeBaseIDs: []string{base.ID},
	})
	if err == nil {
		t.Fatal("expected binding without knowledge-base read permission to be denied")
	}
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 403 {
		t.Fatalf("expected forbidden guessed binding, got %v", err)
	}
	_, err = svc.Agent().CreateDefinition(adminCtx, domain.CreateAgentDefinitionInput{
		Name: "Missing Binding", ModelID: model.ID, KnowledgeBaseIDs: []string{"kb-missing"},
	})
	if err == nil {
		t.Fatal("expected missing knowledge base binding to fail")
	}
}

// TestKnowledgeDocumentCreateRequiresEmbeddingRuntime keeps unindexed documents out of storage.
func TestKnowledgeDocumentCreateRequiresEmbeddingRuntime(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store, _, ctx := newKnowledgeFixture(t, now)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	base, err := svc.Agent().CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{Name: "Policies"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Agent().CreateKnowledgeDocument(ctx, base.ID, domain.CreateKnowledgeDocumentInput{Title: "Policy", Content: "Indexed content"})
	if err == nil {
		t.Fatal("expected document creation to fail without an embedding runtime")
	}
	appErr, ok := domain.AsAppError(err)
	if !ok || appErr.Status != 503 || appErr.ReasonCode != "knowledge_embedding_unavailable" {
		t.Fatalf("unexpected missing embedding error: %v", err)
	}
	documents, listErr := store.ListKnowledgeDocuments(context.Background(), ctx.TenantID, base.ID)
	if listErr != nil || len(documents) != 0 {
		t.Fatalf("unindexed document was persisted: documents=%+v err=%v", documents, listErr)
	}
}

type deterministicKnowledgeEmbedder struct{}

// Model returns the stable public alias used by production configuration.
func (deterministicKnowledgeEmbedder) Model() string { return "nexus-pro-embedding" }

// Embed maps the fixture's policy and unrelated terms to deterministic vectors.
func (deterministicKnowledgeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, len(inputs))
	for i, input := range inputs {
		if strings.Contains(strings.ToLower(input), "contractor") {
			vectors[i] = []float32{0, 1}
		} else {
			vectors[i] = []float32{1, 0}
		}
	}
	return vectors, nil
}

// newKnowledgeFixture seeds a tenant administrator with the knowledge MVP permissions.
func newKnowledgeFixture(t *testing.T, now time.Time) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	permissions := []domain.Permission{
		{Resource: "agent.knowledge_base", Action: "read", Scope: "all"},
		{Resource: "agent.knowledge_base", Action: "create", Scope: "all"},
		{Resource: "agent.knowledge_base", Action: "update", Scope: "all"},
		{Resource: "agent.knowledge_base", Action: "delete", Scope: "all"},
		{Resource: "agent.model", Action: "read", Scope: "all"},
		{Resource: "agent.model", Action: "create", Scope: "all"},
		{Resource: "agent.definition", Action: "read", Scope: "all"},
		{Resource: "agent.definition", Action: "create", Scope: "all"},
		{Resource: "agent.definition", Action: "update", Scope: "all"},
		{Resource: "agent.run", Action: "create", Scope: "all"},
		{Resource: "agent.tool", Action: "call", Target: "knowledge.search", Scope: "all"},
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{ID: "ps-knowledge-admin", TenantID: "tenant-1", Name: "Knowledge Admin", Permissions: permissions, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{ID: "acct-admin", TenantID: "tenant-1", Status: "active", DirectPermissionSetIDs: []string{"ps-knowledge-admin"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return store, service.New(store, service.Options{
		Now: func() time.Time { return now }, KnowledgeEmbedder: deterministicKnowledgeEmbedder{},
	}), domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-admin"}
}

// minimalTextPDF builds a deterministic one-page PDF fixture without a production dependency.
func minimalTextPDF(text string) []byte {
	escaped := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len("BT /F1 12 Tf 72 720 Td ("+escaped+") Tj ET"), "BT /F1 12 Tf 72 720 Td ("+escaped+") Tj ET"),
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xrefOffset := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return output.Bytes()
}

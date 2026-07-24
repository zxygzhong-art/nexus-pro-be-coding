package service_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
	agentservice "nexus-pro-api/internal/service/agent"
)

// TestKnowledgeCRUDSearchAndAgentBindings covers the in-memory MVP lifecycle and runtime citations.
func TestKnowledgeCRUDSearchAndAgentBindings(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	store, svc, ctx := newKnowledgeFixture(t, now)

	base, err := agentservice.New(svc).CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{Name: "HR Policies", Description: "Current HR policies"})
	if err != nil {
		t.Fatal(err)
	}
	document, err := agentservice.New(svc).CreateKnowledgeDocument(ctx, base.ID, domain.CreateKnowledgeDocumentInput{
		Title: "Annual Leave Policy", Content: "Employees have 12 annual leave days each calendar year.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if document.SourceType != "manual" {
		t.Fatalf("expected manual source type, got %+v", document)
	}
	result, err := agentservice.New(svc).SearchKnowledge(ctx, domain.KnowledgeSearchInput{
		Query: "ANNUAL employees", KnowledgeBaseIDs: []string{base.ID}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Hits[0].DocumentID != document.ID || !strings.HasPrefix(result.Hits[0].Source, "knowledge://") {
		t.Fatalf("unexpected AND search result: %+v", result)
	}
	if miss, err := agentservice.New(svc).SearchKnowledge(ctx, domain.KnowledgeSearchInput{Query: "annual contractor", KnowledgeBaseIDs: []string{base.ID}}); err != nil || miss.Total != 0 {
		t.Fatalf("expected AND semantics to reject partial matches: result=%+v err=%v", miss, err)
	}

	model, err := agentservice.New(svc).CreateModel(ctx, domain.CreateAgentModelInput{Name: "Knowledge Model", ModelName: "knowledge-model", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentservice.New(svc).CreateDefinition(ctx, domain.CreateAgentDefinitionInput{
		Name: "Policy Helper", ModelID: model.ID, Tools: []string{"knowledge.search"}, KnowledgeBaseIDs: []string{base.ID},
		SubAgents: []domain.AgentTeamMember{{ID: "policy", Name: "Policy", Role: "search policy", ModelID: model.ID, Tools: []string{"knowledge.search"}, KnowledgeBaseIDs: []string{base.ID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := agentservice.New(svc).GetDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.KnowledgeBaseIDs) != 1 || len(listed.SubAgents) != 1 || len(listed.SubAgents[0].KnowledgeBaseIDs) != 1 || len(listed.Versions) != 1 || len(listed.Versions[0].KnowledgeBaseIDs) != 1 {
		t.Fatalf("knowledge bindings did not round-trip through agent and version snapshots: %+v", listed)
	}
	agent, err = agentservice.New(svc).PublishDefinition(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := agentservice.New(svc).CreateRun(ctx, domain.CreateAgentRunInput{AgentID: agent.ID, Prompt: "annual employees"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.References) != 1 || run.References[0].Source != result.Hits[0].Source {
		t.Fatalf("runtime knowledge search did not return a bound citation: %+v", run)
	}
	if _, err := agentservice.New(svc).DeleteKnowledgeBase(ctx, base.ID); err == nil {
		t.Fatal("expected bound knowledge base deletion to conflict")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 409 {
		t.Fatalf("expected conflict deleting bound knowledge base, got %v", err)
	}

	updatedTitle := "Annual Leave Rules"
	if _, err := agentservice.New(svc).UpdateKnowledgeDocument(ctx, base.ID, document.ID, domain.UpdateKnowledgeDocumentInput{Title: &updatedTitle}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(svc).DeleteKnowledgeDocument(ctx, base.ID, document.ID); err != nil {
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
	base, err := agentservice.New(svc).CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{
		Name: "Uploaded policies", ChunkMode: "paragraph", ChunkSize: &chunkSize, ChunkOverlap: &overlap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if base.ChunkMode != "paragraph" || base.ChunkSize != chunkSize || base.ChunkOverlap != overlap {
		t.Fatalf("chunk configuration did not persist: %+v", base)
	}

	textDocument, err := agentservice.New(svc).UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
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

	pdfDocument, err := agentservice.New(svc).UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
		Filename: "attendance.pdf", ContentType: "application/pdf", Content: minimalTextPDF("Attendance policy requires clock records."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if pdfDocument.SourceType != "pdf" || !strings.Contains(pdfDocument.Content, "Attendance policy") {
		t.Fatalf("PDF text was not extracted: %+v", pdfDocument)
	}

	cjkContent := "特休有十四天，請假前須先提出申請。"
	cjkDocument, err := agentservice.New(svc).UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
		Filename: "leave-policy-cjk.pdf", ContentType: "application/pdf", Content: minimalCIDTextPDF(cjkContent, "UniGB-UCS2-H"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cjkDocument.Content != cjkContent {
		t.Fatalf("CJK PDF text was not decoded from its predefined CMap: got %q", cjkDocument.Content)
	}

	toUnicodeContent := "特休有十四天"
	toUnicodeDocument, err := agentservice.New(svc).UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
		Filename: "leave-policy-type3.pdf", ContentType: "application/pdf", Content: minimalToUnicodeTextPDF(toUnicodeContent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if toUnicodeDocument.Content != toUnicodeContent {
		t.Fatalf("Type 3 PDF text was not decoded from its ToUnicode CMap: got %q", toUnicodeDocument.Content)
	}

	_, err = agentservice.New(svc).UploadKnowledgeDocument(ctx, base.ID, domain.UploadKnowledgeDocumentInput{
		Filename: "unsupported-cjk.pdf", ContentType: "application/pdf", Content: minimalCIDTextPDF(cjkContent, "Identity-H"),
	})
	if err == nil {
		t.Fatal("expected unsupported PDF encoding to be rejected instead of indexed as garbled text")
	}
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 400 || !strings.Contains(appErr.Message, "encoding is not supported") {
		t.Fatalf("expected unsupported PDF encoding error, got %v", err)
	}

	if _, err := agentservice.New(svc).UpdateKnowledgeDocument(ctx, base.ID, textDocument.ID, domain.UpdateKnowledgeDocumentInput{}); err == nil {
		t.Fatal("expected uploaded sources to reject manual edits")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 409 {
		t.Fatalf("expected uploaded-source edit conflict, got %v", err)
	}

	fixedSize, fixedOverlap, fixedMode := 200, 20, "fixed"
	updated, err := agentservice.New(svc).UpdateKnowledgeBase(ctx, base.ID, domain.UpdateKnowledgeBaseInput{
		ChunkMode: &fixedMode, ChunkSize: &fixedSize, ChunkOverlap: &fixedOverlap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ChunkMode != fixedMode || updated.ChunkSize != fixedSize || updated.ChunkOverlap != fixedOverlap {
		t.Fatalf("updated chunk configuration did not persist: %+v", updated)
	}

	if _, err := agentservice.New(svc).DeleteKnowledgeDocument(ctx, base.ID, textDocument.ID); err != nil {
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
	base, err := agentservice.New(svc).CreateKnowledgeBase(adminCtx, domain.CreateKnowledgeBaseInput{Name: "Private"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := agentservice.New(svc).CreateModel(adminCtx, domain.CreateAgentModelInput{Name: "Model", ModelName: "model", APIKey: "sk-test"})
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
	_, err = agentservice.New(svc).CreateDefinition(domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-editor"}, domain.CreateAgentDefinitionInput{
		Name: "Guessed Binding", ModelID: model.ID, KnowledgeBaseIDs: []string{base.ID},
	})
	if err == nil {
		t.Fatal("expected binding without knowledge-base read permission to be denied")
	}
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 403 {
		t.Fatalf("expected forbidden guessed binding, got %v", err)
	}
	_, err = agentservice.New(svc).CreateDefinition(adminCtx, domain.CreateAgentDefinitionInput{
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
	base, err := agentservice.New(svc).CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{Name: "Policies"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agentservice.New(svc).CreateKnowledgeDocument(ctx, base.ID, domain.CreateKnowledgeDocumentInput{Title: "Policy", Content: "Indexed content"})
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

// TestKnowledgeListPaginationAndMetadata covers paged listing, aggregate document counts, and metadata-only list items.
func TestKnowledgeListPaginationAndMetadata(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	_, svc, ctx := newKnowledgeFixture(t, now)

	baseA, err := agentservice.New(svc).CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{Name: "Base A"})
	if err != nil {
		t.Fatal(err)
	}
	baseB, err := agentservice.New(svc).CreateKnowledgeBase(ctx, domain.CreateKnowledgeBaseInput{Name: "Base B"})
	if err != nil {
		t.Fatal(err)
	}
	firstDocument, err := agentservice.New(svc).CreateKnowledgeDocument(ctx, baseA.ID, domain.CreateKnowledgeDocumentInput{Title: "Doc 1", Content: "first body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(svc).CreateKnowledgeDocument(ctx, baseA.ID, domain.CreateKnowledgeDocumentInput{Title: "Doc 2", Content: "second body"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentservice.New(svc).CreateKnowledgeDocument(ctx, baseB.ID, domain.CreateKnowledgeDocumentInput{Title: "Doc 3", Content: "third body"}); err != nil {
		t.Fatal(err)
	}

	allBases, err := agentservice.New(svc).ListKnowledgeBases(ctx, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if allBases.Total != 2 || len(allBases.Items) != 2 || allBases.Page != 1 || allBases.PageSize != 10 {
		t.Fatalf("unexpected knowledge base page: %+v", allBases)
	}
	countsByID := map[string]int{}
	for _, item := range allBases.Items {
		countsByID[item.ID] = item.DocumentCount
	}
	if countsByID[baseA.ID] != 2 || countsByID[baseB.ID] != 1 {
		t.Fatalf("document counts were not aggregated per base: %+v", countsByID)
	}

	basesPageOne, err := agentservice.New(svc).ListKnowledgeBases(ctx, domain.PageRequest{Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	basesPageTwo, err := agentservice.New(svc).ListKnowledgeBases(ctx, domain.PageRequest{Page: 2, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if basesPageOne.Total != 2 || len(basesPageOne.Items) != 1 || len(basesPageTwo.Items) != 1 || basesPageOne.Items[0].ID == basesPageTwo.Items[0].ID {
		t.Fatalf("knowledge base pages must be disjoint: page1=%+v page2=%+v", basesPageOne.Items, basesPageTwo.Items)
	}

	documents, err := agentservice.New(svc).ListKnowledgeDocuments(ctx, baseA.ID, domain.PageRequest{Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if documents.Total != 2 || len(documents.Items) != 1 {
		t.Fatalf("unexpected knowledge document page: %+v", documents)
	}
	for _, item := range documents.Items {
		if item.Content != "" {
			t.Fatalf("document list items must not include full content: %+v", item)
		}
	}

	detail, err := agentservice.New(svc).GetKnowledgeDocument(ctx, baseA.ID, firstDocument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Content != "first body" {
		t.Fatalf("document detail did not return full content: %+v", detail)
	}
	if _, err := agentservice.New(svc).GetKnowledgeDocument(ctx, baseB.ID, firstDocument.ID); err == nil {
		t.Fatal("expected cross-base document lookup to be not found")
	} else if appErr, ok := domain.AsAppError(err); !ok || appErr.Status != 404 {
		t.Fatalf("expected not found for cross-base document lookup, got %v", err)
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
		Now: func() time.Time { return now }, KnowledgeEmbedder: deterministicKnowledgeEmbedder{}, CredentialCipher: newTestCredentialCipher(t),
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
	return buildMinimalPDF(objects)
}

// minimalCIDTextPDF reproduces CJK PDFs that rely on a predefined Unicode CMap
// instead of embedding a ToUnicode stream.
func minimalCIDTextPDF(text, encoding string) []byte {
	var encoded strings.Builder
	for _, character := range utf16.Encode([]rune(text)) {
		fmt.Fprintf(&encoded, "%04X", character)
	}
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td <%s> Tj ET", encoded.String())
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /%s /DescendantFonts [6 0 R] >>", encoding),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> >>",
	}
	return buildMinimalPDF(objects)
}

func minimalToUnicodeTextPDF(text string) []byte {
	var encoded strings.Builder
	var mappings strings.Builder
	for index, character := range []rune(text) {
		code := index + 1
		fmt.Fprintf(&encoded, "%02X", code)
		var destination strings.Builder
		for _, codeUnit := range utf16.Encode([]rune{character}) {
			fmt.Fprintf(&destination, "%04X", codeUnit)
		}
		fmt.Fprintf(&mappings, "<%02X> <%s>\n", code, destination.String())
	}
	cmap := fmt.Sprintf(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
1 begincodespacerange
<00> <FF>
endcodespacerange
%d beginbfchar
%sendbfchar
endcmap
CMapName currentdict /CMap defineresource pop
end
end`, len([]rune(text)), mappings.String())
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td <%s> Tj ET", encoded.String())
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		fmt.Sprintf("<< /Type /Font /Subtype /Type3 /Name /F1 /FontBBox [0 0 1000 1000] /FontMatrix [0.001 0 0 0.001 0 0] /FirstChar 1 /LastChar %d /Widths [] /Encoding << /Type /Encoding /Differences [] >> /CharProcs << >> /Resources << >> /ToUnicode 6 0 R >>", len([]rune(text))),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
	}
	return buildMinimalPDF(objects)
}

func buildMinimalPDF(objects []string) []byte {
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

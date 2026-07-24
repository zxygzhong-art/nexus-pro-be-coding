package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"mime"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	knowledgeSearchDefaultLimit  = 10
	knowledgeSearchMaxLimit      = 50
	knowledgeDocumentMaxRunes    = 200000
	knowledgeUploadMaxBytes      = 20 << 20
	knowledgeUploadMaxRunes      = 1_000_000
	knowledgeChunkDefaultMode    = "auto"
	knowledgeChunkDefaultRunes   = 1200
	knowledgeChunkDefaultOverlap = 200
	knowledgeChunkMinRunes       = 200
	knowledgeChunkMaxRunes       = 4000
	knowledgeEmbeddingBatchSize  = 64
	knowledgeSearchCandidateMax  = 200
	knowledgeSearchMinScore      = 0.25
)

var knowledgeTextExtensions = map[string]struct{}{
	".txt": {}, ".md": {}, ".markdown": {}, ".csv": {}, ".json": {}, ".yaml": {}, ".yml": {},
	".xml": {}, ".html": {}, ".htm": {}, ".log": {}, ".sql": {}, ".ini": {}, ".toml": {},
}

var knowledgeBaseResource = ResourceType("knowledge_base")

// ListKnowledgeBases 列出租戶知識庫分頁並以單次聚合查詢補齊文件數。
func (c AgentService) ListKnowledgeBases(ctx RequestContext, page PageRequest) (PageResponse[domain.KnowledgeBase], error) {
	if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionRead, ""); err != nil {
		return PageResponse[domain.KnowledgeBase]{}, err
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListKnowledgeBasePage(goContext(ctx), ctx.TenantID, page)
	if err != nil {
		return PageResponse[domain.KnowledgeBase]{}, err
	}
	baseIDs := make([]string, len(items))
	for index := range items {
		baseIDs[index] = items[index].ID
	}
	counts, err := c.store.CountKnowledgeDocumentsByBase(goContext(ctx), ctx.TenantID, baseIDs)
	if err != nil {
		return PageResponse[domain.KnowledgeBase]{}, err
	}
	for index := range items {
		items[index].DocumentCount = counts[items[index].ID]
	}
	return utils.PageResponseFromStore(items, total, page), nil
}

// GetKnowledgeBase 取得租戶知識庫並補齊文件數。
func (c AgentService) GetKnowledgeBase(ctx RequestContext, id string) (domain.KnowledgeBase, error) {
	if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionRead, id); err != nil {
		return domain.KnowledgeBase{}, err
	}
	return c.currentKnowledgeBase(ctx, id)
}

// CreateKnowledgeBase 建立租戶知識庫。
func (c AgentService) CreateKnowledgeBase(ctx RequestContext, input domain.CreateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	account, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionCreate, "")
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.KnowledgeBase{}, BadRequest("name is required")
	}
	config, err := knowledgeChunkConfigFromCreate(input)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	now := c.Now()
	item := domain.KnowledgeBase{
		ID: utils.NewID("kb"), TenantID: ctx.TenantID, Name: name, Description: strings.TrimSpace(input.Description),
		ChunkMode: config.Mode, ChunkSize: config.Size, ChunkOverlap: config.Overlap,
		CreatedByAccountID: account.ID, UpdatedByAccountID: account.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := c.store.UpsertKnowledgeBase(goContext(ctx), item); err != nil {
		return domain.KnowledgeBase{}, err
	}
	return item, nil
}

// UpdateKnowledgeBase 更新租戶知識庫。
func (c AgentService) UpdateKnowledgeBase(ctx RequestContext, id string, input domain.UpdateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	account, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionUpdate, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	item, err := c.currentKnowledgeBase(ctx, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	if input.Name != nil {
		item.Name = strings.TrimSpace(*input.Name)
		if item.Name == "" {
			return domain.KnowledgeBase{}, BadRequest("name is required")
		}
	}
	if input.Description != nil {
		item.Description = strings.TrimSpace(*input.Description)
	}
	currentConfig := knowledgeChunkConfigForBase(item)
	candidateConfig := currentConfig
	if input.ChunkMode != nil {
		candidateConfig.Mode = strings.TrimSpace(*input.ChunkMode)
	}
	if input.ChunkSize != nil {
		candidateConfig.Size = *input.ChunkSize
	}
	if input.ChunkOverlap != nil {
		candidateConfig.Overlap = *input.ChunkOverlap
	}
	candidateConfig, err = validateKnowledgeChunkConfig(candidateConfig)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	configChanged := candidateConfig != currentConfig
	item.ChunkMode, item.ChunkSize, item.ChunkOverlap = candidateConfig.Mode, candidateConfig.Size, candidateConfig.Overlap
	item.UpdatedByAccountID = account.ID
	item.UpdatedAt = c.Now()
	if !configChanged {
		if err := c.store.UpsertKnowledgeBase(goContext(ctx), item); err != nil {
			return domain.KnowledgeBase{}, err
		}
		return item, nil
	}
	documents, err := c.store.ListKnowledgeDocuments(goContext(ctx), ctx.TenantID, item.ID)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	reindexed := make(map[string][]domain.KnowledgeDocumentChunk, len(documents))
	for _, document := range documents {
		chunks, chunkErr := c.knowledgeChunksForDocument(ctx, document, candidateConfig)
		if chunkErr != nil {
			return domain.KnowledgeBase{}, chunkErr
		}
		reindexed[document.ID] = chunks
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertKnowledgeBase(goContext(ctx), item); err != nil {
			return err
		}
		for _, document := range documents {
			if err := tx.store.ReplaceKnowledgeDocumentChunks(goContext(ctx), ctx.TenantID, document.ID, reindexed[document.ID]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return domain.KnowledgeBase{}, err
	}
	return item, nil
}

// DeleteKnowledgeBase 刪除未被任何 Agent 版本綁定的知識庫。
func (c AgentService) DeleteKnowledgeBase(ctx RequestContext, id string) (domain.KnowledgeBase, error) {
	if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionDelete, id); err != nil {
		return domain.KnowledgeBase{}, err
	}
	id = strings.TrimSpace(id)
	count, err := c.store.CountAgentDefinitionsByKnowledgeBase(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	if count > 0 {
		return domain.KnowledgeBase{}, Conflict("knowledge base is used by agent definitions").WithReasonCode("knowledge_base_in_use")
	}
	documents, err := c.store.ListKnowledgeDocuments(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	item, ok, err := c.store.DeleteKnowledgeBase(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	if !ok {
		return domain.KnowledgeBase{}, NotFound("knowledge base", id)
	}
	for _, document := range documents {
		c.deleteKnowledgeObjectIfSupported(ctx, document.ObjectKey)
	}
	return item, nil
}

// ListKnowledgeDocuments 列出指定知識庫的文件元資料分頁（不含 content 全文）。
func (c AgentService) ListKnowledgeDocuments(ctx RequestContext, knowledgeBaseID string, page PageRequest) (PageResponse[domain.KnowledgeDocument], error) {
	if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionRead, knowledgeBaseID); err != nil {
		return PageResponse[domain.KnowledgeDocument]{}, err
	}
	if _, err := c.currentKnowledgeBase(ctx, knowledgeBaseID); err != nil {
		return PageResponse[domain.KnowledgeDocument]{}, err
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListKnowledgeDocumentPage(goContext(ctx), ctx.TenantID, strings.TrimSpace(knowledgeBaseID), page)
	if err != nil {
		return PageResponse[domain.KnowledgeDocument]{}, err
	}
	return utils.PageResponseFromStore(items, total, page), nil
}

// GetKnowledgeDocument 取得單一文件的完整內容。
func (c AgentService) GetKnowledgeDocument(ctx RequestContext, knowledgeBaseID, id string) (domain.KnowledgeDocument, error) {
	if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionRead, knowledgeBaseID); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	item, ok, err := c.store.GetKnowledgeDocument(goContext(ctx), ctx.TenantID, strings.TrimSpace(knowledgeBaseID), strings.TrimSpace(id))
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if !ok {
		return domain.KnowledgeDocument{}, NotFound("knowledge document", id)
	}
	return item, nil
}

// CreateKnowledgeDocument 建立手動文字文件。
func (c AgentService) CreateKnowledgeDocument(ctx RequestContext, knowledgeBaseID string, input domain.CreateKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	account, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionUpdate, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	base, err := c.currentKnowledgeBase(ctx, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	title, content, err := normalizeKnowledgeDocumentInput(input.Title, input.Content)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	now := c.Now()
	item := domain.KnowledgeDocument{
		ID: utils.NewID("kdoc"), TenantID: ctx.TenantID, KnowledgeBaseID: base.ID, Title: title, Content: content, SourceType: "manual",
		ParseStatus:        "ready",
		CreatedByAccountID: account.ID, UpdatedByAccountID: account.ID, CreatedAt: now, UpdatedAt: now,
	}
	chunks, err := c.knowledgeChunksForDocument(ctx, item, knowledgeChunkConfigForBase(base))
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertKnowledgeDocument(goContext(ctx), item); err != nil {
			return err
		}
		return tx.store.ReplaceKnowledgeDocumentChunks(goContext(ctx), ctx.TenantID, item.ID, chunks)
	}); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	return item, nil
}

// UploadKnowledgeDocument stores and indexes one validated text or PDF source.
func (c AgentService) UploadKnowledgeDocument(ctx RequestContext, knowledgeBaseID string, input domain.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	account, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionUpdate, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	base, err := c.currentKnowledgeBase(ctx, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	upload, err := normalizeKnowledgeUpload(input)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	now := c.Now()
	item := domain.KnowledgeDocument{
		ID: utils.NewID("kdoc"), TenantID: ctx.TenantID, KnowledgeBaseID: base.ID,
		Title: upload.Title, Content: upload.Content, SourceType: upload.SourceType,
		OriginalFilename: upload.Filename, ContentType: upload.ContentType, SizeBytes: int64(len(input.Content)), SHA256: upload.SHA256,
		ObjectProvider: ObjectStoreProvider(c.ObjectStore()), ObjectBucket: ObjectStoreBucket(c.ObjectStore()), ParseStatus: "ready",
		CreatedByAccountID: account.ID, UpdatedByAccountID: account.ID, CreatedAt: now, UpdatedAt: now,
	}
	item.ObjectKey = fmt.Sprintf("tenants/%s/knowledge-bases/%s/documents/%s/source", ctx.TenantID, base.ID, item.ID)
	if err := c.ObjectStore().PutObject(goContext(ctx), item.ObjectKey, item.ContentType, input.Content); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	stored := true
	defer func() {
		if stored {
			c.deleteKnowledgeObjectIfSupported(ctx, item.ObjectKey)
		}
	}()
	chunks, err := c.knowledgeChunksForDocument(ctx, item, knowledgeChunkConfigForBase(base))
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertKnowledgeDocument(goContext(ctx), item); err != nil {
			return err
		}
		return tx.store.ReplaceKnowledgeDocumentChunks(goContext(ctx), ctx.TenantID, item.ID, chunks)
	}); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	stored = false
	return item, nil
}

// UpdateKnowledgeDocument 更新手動文字文件。
func (c AgentService) UpdateKnowledgeDocument(ctx RequestContext, knowledgeBaseID, id string, input domain.UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	account, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionUpdate, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	item, ok, err := c.store.GetKnowledgeDocument(goContext(ctx), ctx.TenantID, strings.TrimSpace(knowledgeBaseID), strings.TrimSpace(id))
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if !ok {
		return domain.KnowledgeDocument{}, NotFound("knowledge document", id)
	}
	if item.SourceType != "manual" {
		return domain.KnowledgeDocument{}, Conflict("uploaded knowledge sources cannot be edited; upload a replacement")
	}
	base, err := c.currentKnowledgeBase(ctx, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if input.Title != nil {
		item.Title = *input.Title
	}
	if input.Content != nil {
		item.Content = *input.Content
	}
	item.Title, item.Content, err = normalizeKnowledgeDocumentInput(item.Title, item.Content)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	item.UpdatedByAccountID = account.ID
	item.UpdatedAt = c.Now()
	chunks, err := c.knowledgeChunksForDocument(ctx, item, knowledgeChunkConfigForBase(base))
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertKnowledgeDocument(goContext(ctx), item); err != nil {
			return err
		}
		return tx.store.ReplaceKnowledgeDocumentChunks(goContext(ctx), ctx.TenantID, item.ID, chunks)
	}); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	return item, nil
}

// DeleteKnowledgeDocument 刪除手動文字文件。
func (c AgentService) DeleteKnowledgeDocument(ctx RequestContext, knowledgeBaseID, id string) (domain.KnowledgeDocument, error) {
	if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionUpdate, knowledgeBaseID); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	item, ok, err := c.store.DeleteKnowledgeDocument(goContext(ctx), ctx.TenantID, strings.TrimSpace(knowledgeBaseID), strings.TrimSpace(id))
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	if !ok {
		return domain.KnowledgeDocument{}, NotFound("knowledge document", id)
	}
	c.deleteKnowledgeObjectIfSupported(ctx, item.ObjectKey)
	return item, nil
}

// SearchKnowledge performs a tenant-scoped cosine search over bound knowledge bases.
func (c AgentService) SearchKnowledge(ctx RequestContext, input domain.KnowledgeSearchInput) (domain.KnowledgeSearchResult, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return domain.KnowledgeSearchResult{}, BadRequest("query is required")
	}
	baseIDs := uniqueStrings(input.KnowledgeBaseIDs)
	limit := input.Limit
	if limit <= 0 {
		limit = knowledgeSearchDefaultLimit
	}
	if limit > knowledgeSearchMaxLimit {
		limit = knowledgeSearchMaxLimit
	}
	for _, baseID := range baseIDs {
		_, ok, err := c.store.GetKnowledgeBase(goContext(ctx), ctx.TenantID, baseID)
		if err != nil {
			return domain.KnowledgeSearchResult{}, err
		}
		if !ok {
			return domain.KnowledgeSearchResult{}, BadRequest("knowledge base does not exist: " + baseID)
		}
	}
	semantics := "cosine similarity over LiteLLM alias nexus-pro-embedding in PostgreSQL pgvector"
	if c.KnowledgeEmbedder() != nil && strings.TrimSpace(c.KnowledgeEmbedder().Model()) != "" {
		semantics = fmt.Sprintf("cosine similarity over LiteLLM alias %s in PostgreSQL pgvector", strings.TrimSpace(c.KnowledgeEmbedder().Model()))
	}
	if len(baseIDs) == 0 {
		return domain.KnowledgeSearchResult{Query: query, Semantics: semantics, Hits: []domain.KnowledgeSearchHit{}, Total: 0}, nil
	}
	queryVector, model, err := c.embedKnowledgeQuery(ctx, query)
	if err != nil {
		return domain.KnowledgeSearchResult{}, err
	}
	candidateLimit := limit * 4
	if candidateLimit > knowledgeSearchCandidateMax {
		candidateLimit = knowledgeSearchCandidateMax
	}
	matches, err := c.store.SearchKnowledgeDocumentChunks(goContext(ctx), ctx.TenantID, baseIDs, model, queryVector, candidateLimit)
	if err != nil {
		return domain.KnowledgeSearchResult{}, err
	}
	hits := make([]domain.KnowledgeSearchHit, 0, limit)
	seenDocuments := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if match.Score < knowledgeSearchMinScore {
			continue
		}
		if _, seen := seenDocuments[match.DocumentID]; seen {
			continue
		}
		seenDocuments[match.DocumentID] = struct{}{}
		hits = append(hits, domain.KnowledgeSearchHit{
			KnowledgeBaseID: match.KnowledgeBaseID, KnowledgeBaseName: match.KnowledgeBaseName,
			DocumentID: match.DocumentID, Title: match.DocumentTitle, Snippet: knowledgeSnippet(match.Content),
			Source: fmt.Sprintf("knowledge://%s/%s#chunk=%d", match.KnowledgeBaseID, match.DocumentID, match.Ordinal+1), Score: match.Score,
		})
		if len(hits) == limit {
			break
		}
	}
	return domain.KnowledgeSearchResult{
		Query: query, Semantics: semantics, Hits: hits, Total: len(hits),
	}, nil
}

// knowledgeChunksForDocument chunks and embeds a document before its transactional write.
func (c AgentService) knowledgeChunksForDocument(ctx RequestContext, document domain.KnowledgeDocument, config domain.KnowledgeChunkConfig) ([]domain.KnowledgeDocumentChunk, error) {
	if c.KnowledgeEmbedder() == nil || strings.TrimSpace(c.KnowledgeEmbedder().Model()) == "" {
		return nil, knowledgeEmbeddingUnavailable()
	}
	contents := splitKnowledgeContent(document.Content, config)
	inputs := make([]string, len(contents))
	for i, content := range contents {
		inputs[i] = document.Title + "\n\n" + content
	}
	vectors := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += knowledgeEmbeddingBatchSize {
		end := start + knowledgeEmbeddingBatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch, err := c.KnowledgeEmbedder().Embed(goContext(ctx), inputs[start:end])
		if err != nil {
			c.LogWarn(ctx, "knowledge document embedding failed", "document_id", document.ID, "model", c.KnowledgeEmbedder().Model(), "error", err)
			return nil, knowledgeEmbeddingFailed()
		}
		if err := validateKnowledgeVectors(batch, end-start); err != nil {
			c.LogWarn(ctx, "knowledge document embedding response invalid", "document_id", document.ID, "model", c.KnowledgeEmbedder().Model(), "error", err)
			return nil, knowledgeEmbeddingFailed()
		}
		if len(vectors) > 0 && len(batch[0]) != len(vectors[0]) {
			return nil, knowledgeEmbeddingFailed()
		}
		vectors = append(vectors, batch...)
	}
	chunks := make([]domain.KnowledgeDocumentChunk, len(contents))
	for i, content := range contents {
		chunks[i] = domain.KnowledgeDocumentChunk{
			ID: utils.NewID("kchunk"), TenantID: document.TenantID, KnowledgeBaseID: document.KnowledgeBaseID,
			DocumentID: document.ID, Ordinal: i, Content: content, EmbeddingModel: strings.TrimSpace(c.KnowledgeEmbedder().Model()),
			EmbeddingDimensions: len(vectors[i]), Embedding: vectors[i], CreatedAt: document.UpdatedAt,
		}
	}
	return chunks, nil
}

// embedKnowledgeQuery embeds and validates one semantic search query.
func (c AgentService) embedKnowledgeQuery(ctx RequestContext, query string) ([]float32, string, error) {
	if c.KnowledgeEmbedder() == nil || strings.TrimSpace(c.KnowledgeEmbedder().Model()) == "" {
		return nil, "", knowledgeEmbeddingUnavailable()
	}
	vectors, err := c.KnowledgeEmbedder().Embed(goContext(ctx), []string{query})
	if err != nil {
		c.LogWarn(ctx, "knowledge query embedding failed", "model", c.KnowledgeEmbedder().Model(), "error", err)
		return nil, "", knowledgeEmbeddingFailed()
	}
	if err := validateKnowledgeVectors(vectors, 1); err != nil {
		c.LogWarn(ctx, "knowledge query embedding response invalid", "model", c.KnowledgeEmbedder().Model(), "error", err)
		return nil, "", knowledgeEmbeddingFailed()
	}
	return vectors[0], strings.TrimSpace(c.KnowledgeEmbedder().Model()), nil
}

// splitKnowledgeContent applies the selected bounded segmentation strategy.
func splitKnowledgeContent(content string, config domain.KnowledgeChunkConfig) []string {
	config, err := validateKnowledgeChunkConfig(config)
	if err != nil {
		config = defaultKnowledgeChunkConfig()
	}
	if config.Mode == "paragraph" {
		return splitKnowledgeParagraphs(content, config)
	}
	return splitKnowledgeWindows(content, config, config.Mode == "auto")
}

// splitKnowledgeWindows creates fixed or boundary-aware rune chunks with overlap.
func splitKnowledgeWindows(content string, config domain.KnowledgeChunkConfig, preferBoundary bool) []string {
	runes := []rune(strings.TrimSpace(content))
	chunks := make([]string, 0, (len(runes)/config.Size)+1)
	for start := 0; start < len(runes); {
		end := start + config.Size
		if end > len(runes) {
			end = len(runes)
		} else if preferBoundary {
			minimum := start + config.Size/2
			for candidate := end; candidate > minimum; candidate-- {
				if knowledgeChunkBoundary(runes, candidate-1) {
					end = candidate
					break
				}
			}
		}
		if chunk := strings.TrimSpace(string(runes[start:end])); chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
		next := end - config.Overlap
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return chunks
}

// splitKnowledgeParagraphs keeps paragraphs isolated while bounding oversized blocks.
func splitKnowledgeParagraphs(content string, config domain.KnowledgeChunkConfig) []string {
	normalized := strings.ReplaceAll(strings.TrimSpace(content), "\r\n", "\n")
	paragraphs := strings.Split(normalized, "\n\n")
	chunks := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if len([]rune(paragraph)) <= config.Size {
			chunks = append(chunks, paragraph)
			continue
		}
		chunks = append(chunks, splitKnowledgeWindows(paragraph, config, false)...)
	}
	return chunks
}

// knowledgeChunkBoundary identifies natural sentence or line endings for auto mode.
func knowledgeChunkBoundary(runes []rune, index int) bool {
	if index < 0 || index >= len(runes) {
		return false
	}
	switch runes[index] {
	case '\n', '。', '！', '？', '!', '?':
		return true
	case '.':
		return index+1 >= len(runes) || runes[index+1] == ' ' || runes[index+1] == '\n'
	default:
		return false
	}
}

// validateKnowledgeVectors rejects partial, mixed-dimension, or non-finite responses.
func validateKnowledgeVectors(vectors [][]float32, expected int) error {
	if len(vectors) != expected || expected == 0 {
		return fmt.Errorf("expected %d embeddings, got %d", expected, len(vectors))
	}
	dimension := len(vectors[0])
	if dimension == 0 {
		return fmt.Errorf("embedding dimension is empty")
	}
	for _, vector := range vectors {
		if len(vector) != dimension {
			return fmt.Errorf("embedding dimensions are inconsistent")
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return fmt.Errorf("embedding contains a non-finite value")
			}
		}
	}
	return nil
}

// knowledgeEmbeddingUnavailable reports missing runtime configuration without exposing secrets.
func knowledgeEmbeddingUnavailable() error {
	return domain.E(503, "service_unavailable", "knowledge embedding is temporarily unavailable").WithReasonCode("knowledge_embedding_unavailable")
}

// knowledgeEmbeddingFailed maps upstream failures to a stable gateway error.
func knowledgeEmbeddingFailed() error {
	return domain.E(502, "bad_gateway", "knowledge embedding failed").WithReasonCode("knowledge_embedding_unavailable")
}

// currentKnowledgeBase 取得目前租戶知識庫並補齊文件數。
func (c AgentService) currentKnowledgeBase(ctx RequestContext, id string) (domain.KnowledgeBase, error) {
	id = strings.TrimSpace(id)
	item, ok, err := c.store.GetKnowledgeBase(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	if !ok {
		return domain.KnowledgeBase{}, NotFound("knowledge base", id)
	}
	count, err := c.store.CountKnowledgeDocuments(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	item.DocumentCount = count
	return item, nil
}

// defaultKnowledgeChunkConfig returns the stable indexing defaults for new and legacy bases.
func defaultKnowledgeChunkConfig() domain.KnowledgeChunkConfig {
	return domain.KnowledgeChunkConfig{Mode: knowledgeChunkDefaultMode, Size: knowledgeChunkDefaultRunes, Overlap: knowledgeChunkDefaultOverlap}
}

// knowledgeChunkConfigFromCreate merges optional creation fields with stable defaults.
func knowledgeChunkConfigFromCreate(input domain.CreateKnowledgeBaseInput) (domain.KnowledgeChunkConfig, error) {
	config := defaultKnowledgeChunkConfig()
	if strings.TrimSpace(input.ChunkMode) != "" {
		config.Mode = strings.TrimSpace(input.ChunkMode)
	}
	if input.ChunkSize != nil {
		config.Size = *input.ChunkSize
	}
	if input.ChunkOverlap != nil {
		config.Overlap = *input.ChunkOverlap
	}
	return validateKnowledgeChunkConfig(config)
}

// knowledgeChunkConfigForBase normalizes persisted and pre-migration in-memory values.
func knowledgeChunkConfigForBase(base domain.KnowledgeBase) domain.KnowledgeChunkConfig {
	config := defaultKnowledgeChunkConfig()
	if strings.TrimSpace(base.ChunkMode) != "" {
		config.Mode = strings.TrimSpace(base.ChunkMode)
	}
	if base.ChunkSize != 0 {
		config.Size = base.ChunkSize
	}
	if base.ChunkOverlap != 0 || base.ChunkSize != 0 {
		config.Overlap = base.ChunkOverlap
	}
	validated, err := validateKnowledgeChunkConfig(config)
	if err != nil {
		return defaultKnowledgeChunkConfig()
	}
	return validated
}

// validateKnowledgeChunkConfig enforces bounded settings accepted by the database.
func validateKnowledgeChunkConfig(config domain.KnowledgeChunkConfig) (domain.KnowledgeChunkConfig, error) {
	config.Mode = strings.TrimSpace(config.Mode)
	if config.Mode != "auto" && config.Mode != "paragraph" && config.Mode != "fixed" {
		return domain.KnowledgeChunkConfig{}, BadRequest("chunk_mode must be auto, paragraph, or fixed")
	}
	if config.Size < knowledgeChunkMinRunes || config.Size > knowledgeChunkMaxRunes {
		return domain.KnowledgeChunkConfig{}, BadRequest("chunk_size must be between 200 and 4000")
	}
	if config.Overlap < 0 || config.Overlap >= config.Size {
		return domain.KnowledgeChunkConfig{}, BadRequest("chunk_overlap must be non-negative and smaller than chunk_size")
	}
	return config, nil
}

type normalizedKnowledgeUpload struct {
	Title       string
	Filename    string
	ContentType string
	Content     string
	SourceType  string
	SHA256      string
}

// normalizeKnowledgeUpload validates file metadata and extracts indexable UTF-8 text.
func normalizeKnowledgeUpload(input domain.UploadKnowledgeDocumentInput) (normalizedKnowledgeUpload, error) {
	filename := path.Base(strings.ReplaceAll(strings.TrimSpace(input.Filename), "\\", "/"))
	if filename == "" || filename == "." {
		return normalizedKnowledgeUpload{}, BadRequest("file name is required")
	}
	if len(input.Content) == 0 {
		return normalizedKnowledgeUpload{}, BadRequest("file is empty")
	}
	if len(input.Content) > knowledgeUploadMaxBytes {
		return normalizedKnowledgeUpload{}, BadRequest("knowledge source exceeds 20MB limit")
	}
	extension := strings.ToLower(path.Ext(filename))
	sourceType := "text"
	contentType := strings.TrimSpace(input.ContentType)
	var content string
	if extension == ".pdf" {
		sourceType = "pdf"
		contentType = "application/pdf"
		extractedContent, err := extractKnowledgePDFText(input.Content)
		if err != nil {
			return normalizedKnowledgeUpload{}, BadRequest(err.Error())
		}
		content = extractedContent
	} else {
		if _, ok := knowledgeTextExtensions[extension]; !ok {
			return normalizedKnowledgeUpload{}, BadRequest("knowledge source type is not supported")
		}
		data := input.Content
		if len(data) >= 3 && string(data[:3]) == "\xef\xbb\xbf" {
			data = data[3:]
		}
		if !utf8.Valid(data) {
			return normalizedKnowledgeUpload{}, BadRequest("text source must use UTF-8 encoding")
		}
		content = strings.TrimSpace(string(data))
		if content == "" {
			return normalizedKnowledgeUpload{}, BadRequest("text source is empty")
		}
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = mime.TypeByExtension(extension)
			if contentType == "" {
				contentType = http.DetectContentType(input.Content)
			}
		}
	}
	if len([]rune(content)) > knowledgeUploadMaxRunes {
		return normalizedKnowledgeUpload{}, BadRequest("knowledge source exceeds 1000000 extracted characters")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(filename, path.Ext(filename)))
	}
	if title == "" {
		return normalizedKnowledgeUpload{}, BadRequest("title is required")
	}
	if len([]rune(title)) > 120 {
		return normalizedKnowledgeUpload{}, BadRequest("title exceeds 120 characters")
	}
	digest := sha256.Sum256(input.Content)
	return normalizedKnowledgeUpload{
		Title: title, Filename: filename, ContentType: contentType, Content: content,
		SourceType: sourceType, SHA256: hex.EncodeToString(digest[:]),
	}, nil
}

// deleteKnowledgeObjectIfSupported performs best-effort cleanup after metadata deletion or rollback.
func (c AgentService) deleteKnowledgeObjectIfSupported(ctx RequestContext, key string) {
	deleter, ok := c.ObjectStore().(ObjectDeleter)
	if !ok || strings.TrimSpace(key) == "" {
		return
	}
	if err := deleter.DeleteObject(goContext(ctx), key); err != nil {
		c.LogWarn(ctx, "delete knowledge source object failed", "object_key", key, "error", err)
	}
}

// normalizeKnowledgeDocumentInput 驗證手動文字文件的必要內容及大小上限。
func normalizeKnowledgeDocumentInput(title, content string) (string, string, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" {
		return "", "", BadRequest("title is required")
	}
	if len([]rune(title)) > 120 {
		return "", "", BadRequest("title exceeds 120 characters")
	}
	if content == "" {
		return "", "", BadRequest("content is required")
	}
	if len([]rune(content)) > knowledgeDocumentMaxRunes {
		return "", "", BadRequest("content exceeds 200000 characters")
	}
	return title, content, nil
}

// knowledgeSnippet 產生固定上限的引用摘要。
func knowledgeSnippet(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= 220 {
		return string(runes)
	}
	return string(runes[:217]) + "..."
}

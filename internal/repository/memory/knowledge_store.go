package memory

import (
	"context"
	"math"
	"sort"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// UpsertKnowledgeBase persists a tenant knowledge base.
func (s *Store) UpsertKnowledgeBase(_ context.Context, v KnowledgeBase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.knowledgeBases, v.TenantID, v.ID, v)
	return nil
}

// GetKnowledgeBase loads a tenant knowledge base.
func (s *Store) GetKnowledgeBase(_ context.Context, tenantID, id string) (KnowledgeBase, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.knowledgeBases, tenantID, id)
	return v, ok, nil
}

// ListKnowledgeBases lists tenant knowledge bases.
func (s *Store) ListKnowledgeBases(_ context.Context, tenantID string) ([]KnowledgeBase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := copyNestedValues(s.knowledgeBases[tenantID], func(v KnowledgeBase) KnowledgeBase { return v })
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// DeleteKnowledgeBase deletes a knowledge base and its documents.
func (s *Store) DeleteKnowledgeBase(_ context.Context, tenantID, id string) (KnowledgeBase, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.knowledgeBases, tenantID, id)
	if !ok {
		return KnowledgeBase{}, false, nil
	}
	delete(s.knowledgeBases[tenantID], id)
	for documentID, document := range s.knowledgeDocuments[tenantID] {
		if document.KnowledgeBaseID == id {
			delete(s.knowledgeDocuments[tenantID], documentID)
		}
	}
	for chunkID, chunk := range s.knowledgeDocumentChunks[tenantID] {
		if chunk.KnowledgeBaseID == id {
			delete(s.knowledgeDocumentChunks[tenantID], chunkID)
		}
	}
	return v, true, nil
}

// UpsertKnowledgeDocument persists a manual text document.
func (s *Store) UpsertKnowledgeDocument(_ context.Context, v KnowledgeDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.knowledgeDocuments, v.TenantID, v.ID, v)
	return nil
}

// GetKnowledgeDocument loads a document within its knowledge base.
func (s *Store) GetKnowledgeDocument(_ context.Context, tenantID, knowledgeBaseID, id string) (KnowledgeDocument, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := getNested(s.knowledgeDocuments, tenantID, id)
	if !ok || v.KnowledgeBaseID != knowledgeBaseID {
		return KnowledgeDocument{}, false, nil
	}
	return v, true, nil
}

// ListKnowledgeDocuments lists documents in one knowledge base.
func (s *Store) ListKnowledgeDocuments(_ context.Context, tenantID, knowledgeBaseID string) ([]KnowledgeDocument, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]KnowledgeDocument, 0)
	for _, document := range s.knowledgeDocuments[tenantID] {
		if document.KnowledgeBaseID == knowledgeBaseID {
			out = append(out, document)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// DeleteKnowledgeDocument deletes a document within its knowledge base.
func (s *Store) DeleteKnowledgeDocument(_ context.Context, tenantID, knowledgeBaseID, id string) (KnowledgeDocument, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := getNested(s.knowledgeDocuments, tenantID, id)
	if !ok || v.KnowledgeBaseID != knowledgeBaseID {
		return KnowledgeDocument{}, false, nil
	}
	delete(s.knowledgeDocuments[tenantID], id)
	for chunkID, chunk := range s.knowledgeDocumentChunks[tenantID] {
		if chunk.DocumentID == id {
			delete(s.knowledgeDocumentChunks[tenantID], chunkID)
		}
	}
	return v, true, nil
}

// ReplaceKnowledgeDocumentChunks atomically replaces one document's retrievable chunks.
func (s *Store) ReplaceKnowledgeDocumentChunks(_ context.Context, tenantID, documentID string, chunks []KnowledgeDocumentChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for chunkID, chunk := range s.knowledgeDocumentChunks[tenantID] {
		if chunk.DocumentID == documentID {
			delete(s.knowledgeDocumentChunks[tenantID], chunkID)
		}
	}
	for _, chunk := range chunks {
		putNested(s.knowledgeDocumentChunks, tenantID, chunk.ID, copyKnowledgeDocumentChunk(chunk))
	}
	return nil
}

// SearchKnowledgeDocumentChunks returns cosine-ranked chunks for the selected bases and model alias.
func (s *Store) SearchKnowledgeDocumentChunks(_ context.Context, tenantID string, knowledgeBaseIDs []string, embeddingModel string, queryEmbedding []float32, limit int) ([]domain.KnowledgeDocumentChunkMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	allowed := make(map[string]struct{}, len(knowledgeBaseIDs))
	for _, id := range knowledgeBaseIDs {
		allowed[id] = struct{}{}
	}
	out := make([]domain.KnowledgeDocumentChunkMatch, 0)
	for _, chunk := range s.knowledgeDocumentChunks[tenantID] {
		if _, ok := allowed[chunk.KnowledgeBaseID]; !ok || chunk.EmbeddingModel != embeddingModel || chunk.EmbeddingDimensions != len(queryEmbedding) {
			continue
		}
		score, ok := cosineSimilarity(queryEmbedding, chunk.Embedding)
		if !ok {
			continue
		}
		base, baseOK := getNested(s.knowledgeBases, tenantID, chunk.KnowledgeBaseID)
		document, documentOK := getNested(s.knowledgeDocuments, tenantID, chunk.DocumentID)
		if !baseOK || !documentOK {
			continue
		}
		out = append(out, domain.KnowledgeDocumentChunkMatch{
			ID: chunk.ID, KnowledgeBaseID: chunk.KnowledgeBaseID, KnowledgeBaseName: base.Name,
			DocumentID: chunk.DocumentID, DocumentTitle: document.Title, Ordinal: chunk.Ordinal,
			Content: chunk.Content, Score: score,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// cosineSimilarity computes cosine similarity for equal non-zero vectors.
func cosineSimilarity(left, right []float32) (float64, bool) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, false
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l, r := float64(left[i]), float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, false
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), true
}

// CountAgentDefinitionsByKnowledgeBase counts current and versioned bindings.
func (s *Store) CountAgentDefinitionsByKnowledgeBase(_ context.Context, tenantID, knowledgeBaseID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, agent := range s.agentDefinitions[tenantID] {
		if agentUsesKnowledgeBase(agent.KnowledgeBaseIDs, agent.SubAgents, knowledgeBaseID) {
			count++
		}
	}
	for _, version := range s.agentDefinitionVersions[tenantID] {
		if agentUsesKnowledgeBase(version.KnowledgeBaseIDs, version.SubAgents, knowledgeBaseID) {
			count++
		}
	}
	return count, nil
}

// agentUsesKnowledgeBase checks main and sub-agent bindings.
func agentUsesKnowledgeBase(ids []string, members []AgentTeamMember, knowledgeBaseID string) bool {
	if utils.ContainsString(ids, knowledgeBaseID) {
		return true
	}
	for _, member := range members {
		if utils.ContainsString(member.KnowledgeBaseIDs, knowledgeBaseID) {
			return true
		}
	}
	return false
}

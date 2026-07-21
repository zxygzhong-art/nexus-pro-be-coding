package domain

import "time"

// KnowledgeBase defines tenant-owned knowledge that agents can bind.
type KnowledgeBase struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	ChunkMode          string    `json:"chunk_mode"`
	ChunkSize          int       `json:"chunk_size"`
	ChunkOverlap       int       `json:"chunk_overlap"`
	DocumentCount      int       `json:"document_count"`
	CreatedByAccountID string    `json:"created_by_account_id,omitempty"`
	UpdatedByAccountID string    `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// KnowledgeDocument defines one manual or uploaded knowledge source.
type KnowledgeDocument struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	KnowledgeBaseID    string    `json:"knowledge_base_id"`
	Title              string    `json:"title"`
	Content            string    `json:"content,omitempty"`
	SourceType         string    `json:"source_type"`
	OriginalFilename   string    `json:"original_filename"`
	ContentType        string    `json:"content_type"`
	SizeBytes          int64     `json:"size_bytes"`
	SHA256             string    `json:"sha256"`
	ObjectProvider     string    `json:"object_provider"`
	ObjectBucket       string    `json:"object_bucket"`
	ObjectKey          string    `json:"-"`
	ParseStatus        string    `json:"parse_status"`
	ParseError         string    `json:"parse_error"`
	CreatedByAccountID string    `json:"created_by_account_id,omitempty"`
	UpdatedByAccountID string    `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// KnowledgeDocumentChunk stores one retrievable document segment and its embedding.
type KnowledgeDocumentChunk struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	KnowledgeBaseID     string    `json:"knowledge_base_id"`
	DocumentID          string    `json:"document_id"`
	Ordinal             int       `json:"ordinal"`
	Content             string    `json:"content"`
	EmbeddingModel      string    `json:"embedding_model"`
	EmbeddingDimensions int       `json:"embedding_dimensions"`
	Embedding           []float32 `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
}

// KnowledgeDocumentChunkMatch represents one semantic nearest-neighbor match.
type KnowledgeDocumentChunkMatch struct {
	ID                string
	KnowledgeBaseID   string
	KnowledgeBaseName string
	DocumentID        string
	DocumentTitle     string
	Ordinal           int
	Content           string
	Score             float64
}

// CreateKnowledgeBaseInput defines knowledge-base creation fields.
type CreateKnowledgeBaseInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ChunkMode    string `json:"chunk_mode"`
	ChunkSize    *int   `json:"chunk_size"`
	ChunkOverlap *int   `json:"chunk_overlap"`
}

// UpdateKnowledgeBaseInput defines partial knowledge-base updates.
type UpdateKnowledgeBaseInput struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	ChunkMode    *string `json:"chunk_mode"`
	ChunkSize    *int    `json:"chunk_size"`
	ChunkOverlap *int    `json:"chunk_overlap"`
}

// KnowledgeChunkConfig controls how source text is segmented before embedding.
type KnowledgeChunkConfig struct {
	Mode    string
	Size    int
	Overlap int
}

// CreateKnowledgeDocumentInput defines manual document creation fields.
type CreateKnowledgeDocumentInput struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UpdateKnowledgeDocumentInput defines partial manual document updates.
type UpdateKnowledgeDocumentInput struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

// UploadKnowledgeDocumentInput defines one uploaded text or PDF source.
type UploadKnowledgeDocumentInput struct {
	Title       string
	Filename    string
	ContentType string
	Content     []byte
}

// KnowledgeSearchInput defines a bounded semantic vector search.
type KnowledgeSearchInput struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	Limit            int      `json:"limit,omitempty"`
}

// KnowledgeSearchHit identifies a traceable document match.
type KnowledgeSearchHit struct {
	KnowledgeBaseID   string  `json:"knowledge_base_id"`
	KnowledgeBaseName string  `json:"knowledge_base_name"`
	DocumentID        string  `json:"document_id"`
	Title             string  `json:"title"`
	Snippet           string  `json:"snippet"`
	Source            string  `json:"source"`
	Score             float64 `json:"score"`
}

// KnowledgeSearchResult returns matches with explicit search semantics.
type KnowledgeSearchResult struct {
	Query     string               `json:"query"`
	Semantics string               `json:"semantics"`
	Hits      []KnowledgeSearchHit `json:"hits"`
	Total     int                  `json:"total"`
}

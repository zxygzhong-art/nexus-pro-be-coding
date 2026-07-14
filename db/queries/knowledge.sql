-- name: UpsertKnowledgeBase :one
INSERT INTO knowledge_bases (
    id, tenant_id, name, description, chunk_mode, chunk_size, chunk_overlap,
    created_by_account_id, updated_by_account_id, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(chunk_mode), sqlc.arg(chunk_size), sqlc.arg(chunk_overlap),
    sqlc.arg(created_by_account_id), sqlc.arg(updated_by_account_id), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    chunk_mode = EXCLUDED.chunk_mode,
    chunk_size = EXCLUDED.chunk_size,
    chunk_overlap = EXCLUDED.chunk_overlap,
    updated_by_account_id = EXCLUDED.updated_by_account_id,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetKnowledgeBase :one
SELECT * FROM knowledge_bases
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: ListKnowledgeBases :many
SELECT * FROM knowledge_bases
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY updated_at DESC, id ASC;

-- name: DeleteKnowledgeBase :one
DELETE FROM knowledge_bases
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: UpsertKnowledgeDocument :one
INSERT INTO knowledge_documents (
    id, tenant_id, knowledge_base_id, title, content, source_type, original_filename,
    content_type, size_bytes, sha256, object_provider, object_bucket, object_key, parse_status, parse_error,
    created_by_account_id, updated_by_account_id, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(knowledge_base_id), sqlc.arg(title), sqlc.arg(content), sqlc.arg(source_type), sqlc.arg(original_filename),
    sqlc.arg(content_type), sqlc.arg(size_bytes), sqlc.arg(sha256), sqlc.arg(object_provider), sqlc.arg(object_bucket), sqlc.arg(object_key), sqlc.arg(parse_status), sqlc.arg(parse_error),
    sqlc.arg(created_by_account_id), sqlc.arg(updated_by_account_id), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    content = EXCLUDED.content,
    source_type = EXCLUDED.source_type,
    original_filename = EXCLUDED.original_filename,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    sha256 = EXCLUDED.sha256,
    object_provider = EXCLUDED.object_provider,
    object_bucket = EXCLUDED.object_bucket,
    object_key = EXCLUDED.object_key,
    parse_status = EXCLUDED.parse_status,
    parse_error = EXCLUDED.parse_error,
    updated_by_account_id = EXCLUDED.updated_by_account_id,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetKnowledgeDocument :one
SELECT * FROM knowledge_documents
WHERE tenant_id = sqlc.arg(tenant_id)
  AND knowledge_base_id = sqlc.arg(knowledge_base_id)
  AND id = sqlc.arg(id);

-- name: ListKnowledgeDocuments :many
SELECT * FROM knowledge_documents
WHERE tenant_id = sqlc.arg(tenant_id)
  AND knowledge_base_id = sqlc.arg(knowledge_base_id)
ORDER BY updated_at DESC, id ASC;

-- name: DeleteKnowledgeDocument :one
DELETE FROM knowledge_documents
WHERE tenant_id = sqlc.arg(tenant_id)
  AND knowledge_base_id = sqlc.arg(knowledge_base_id)
  AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteKnowledgeDocumentChunks :exec
DELETE FROM knowledge_document_chunks
WHERE tenant_id = sqlc.arg(tenant_id)
  AND document_id = sqlc.arg(document_id);

-- name: CreateKnowledgeDocumentChunk :exec
INSERT INTO knowledge_document_chunks (
    id, tenant_id, knowledge_base_id, document_id, ordinal, content,
    embedding_model, embedding_dimensions, embedding, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(knowledge_base_id), sqlc.arg(document_id),
    sqlc.arg(ordinal), sqlc.arg(content), sqlc.arg(embedding_model), sqlc.arg(embedding_dimensions), sqlc.arg(embedding)::vector,
    sqlc.arg(created_at)
);

-- name: SearchKnowledgeDocumentChunks :many
SELECT
    c.id,
    c.knowledge_base_id,
    b.name AS knowledge_base_name,
    c.document_id,
    d.title,
    c.ordinal,
    c.content,
    (1 - (c.embedding <=> sqlc.arg(query_embedding)::vector))::double precision AS score
FROM knowledge_document_chunks c
JOIN knowledge_bases b
  ON b.tenant_id = c.tenant_id AND b.id = c.knowledge_base_id
JOIN knowledge_documents d
  ON d.tenant_id = c.tenant_id AND d.id = c.document_id
WHERE c.tenant_id = sqlc.arg(tenant_id)
  AND c.knowledge_base_id = ANY(sqlc.arg(knowledge_base_ids)::text[])
  AND c.embedding_model = sqlc.arg(embedding_model)
  AND c.embedding_dimensions = sqlc.arg(embedding_dimensions)
ORDER BY c.embedding <=> sqlc.arg(query_embedding)::vector, c.id
LIMIT sqlc.arg(result_limit);

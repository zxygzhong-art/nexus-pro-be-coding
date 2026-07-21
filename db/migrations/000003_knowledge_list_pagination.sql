-- +goose Up
-- 知識庫列表分頁改為 (created_at, id) 排序，補上對應索引避免全表排序。
CREATE INDEX IF NOT EXISTS knowledge_bases_tenant_created_idx
    ON knowledge_bases (tenant_id, created_at DESC, id);

CREATE INDEX IF NOT EXISTS knowledge_documents_base_created_idx
    ON knowledge_documents (tenant_id, knowledge_base_id, created_at DESC, id);

-- +goose Down

DROP INDEX IF EXISTS knowledge_documents_base_created_idx;
DROP INDEX IF EXISTS knowledge_bases_tenant_created_idx;

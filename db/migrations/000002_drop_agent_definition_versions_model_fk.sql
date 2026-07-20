-- +goose Up
-- 歷史版本是不可變審計快照，其 model_id 僅作歷史記錄，不應以參照完整性鎖死模型刪除。
ALTER TABLE agent_definition_versions DROP CONSTRAINT agent_definition_versions_model_fk;

-- +goose Down
-- 回填前僅當所有歷史版本引用的模型仍存在時才能重建約束。
ALTER TABLE agent_definition_versions ADD CONSTRAINT agent_definition_versions_model_fk FOREIGN KEY (tenant_id, model_id) REFERENCES agent_models (tenant_id, id);

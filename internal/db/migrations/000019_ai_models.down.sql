-- Rollback 000019: restore embedding_models as canonical registry.

INSERT INTO {{POSTBRAIN_SCHEMA}}.embedding_models (
    id, slug, dimensions, content_type, is_active, description, created_at,
    provider, service_url, provider_model, table_name, is_ready, provider_config
)
SELECT
    id, slug, dimensions, content_type, is_active, description, created_at,
    provider, service_url, provider_model, table_name, is_ready, provider_config
FROM {{POSTBRAIN_SCHEMA}}.ai_models
WHERE model_type = 'embedding'
ON CONFLICT (slug) DO UPDATE SET
    dimensions = EXCLUDED.dimensions,
    content_type = EXCLUDED.content_type,
    is_active = EXCLUDED.is_active,
    description = EXCLUDED.description,
    provider = EXCLUDED.provider,
    service_url = EXCLUDED.service_url,
    provider_model = EXCLUDED.provider_model,
    table_name = EXCLUDED.table_name,
    is_ready = EXCLUDED.is_ready,
    provider_config = EXCLUDED.provider_config;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.memories
    DROP CONSTRAINT IF EXISTS memories_embedding_model_id_fkey,
    ADD CONSTRAINT memories_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id);

ALTER TABLE {{POSTBRAIN_SCHEMA}}.memories
    DROP CONSTRAINT IF EXISTS memories_embedding_code_model_id_fkey,
    ADD CONSTRAINT memories_embedding_code_model_id_fkey
        FOREIGN KEY (embedding_code_model_id) REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id);

ALTER TABLE {{POSTBRAIN_SCHEMA}}.entities
    DROP CONSTRAINT IF EXISTS entities_embedding_model_id_fkey,
    ADD CONSTRAINT entities_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id);

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP CONSTRAINT IF EXISTS knowledge_artifacts_embedding_model_id_fkey,
    ADD CONSTRAINT knowledge_artifacts_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id);

ALTER TABLE {{POSTBRAIN_SCHEMA}}.skills
    DROP CONSTRAINT IF EXISTS skills_embedding_model_id_fkey,
    ADD CONSTRAINT skills_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id);

ALTER TABLE {{POSTBRAIN_SCHEMA}}.embedding_index
    DROP CONSTRAINT IF EXISTS embedding_index_model_id_fkey,
    ADD CONSTRAINT embedding_index_model_id_fkey
        FOREIGN KEY (model_id) REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id);

DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.ai_models;

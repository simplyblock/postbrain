-- Migration 000019: ai_models registry
-- Introduces ai_models as the canonical model registry with model_type.
-- Existing embedding_models rows are backfilled as model_type='embedding'.
-- embedding_models is intentionally retained for compatibility.

CREATE TABLE ai_models (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    slug            citext NOT NULL UNIQUE,
    dimensions      INT NOT NULL,
    content_type    TEXT NOT NULL DEFAULT 'text' CHECK (content_type IN ('text', 'code')),
    model_type      TEXT NOT NULL DEFAULT 'embedding' CHECK (model_type IN ('embedding', 'generation')),
    is_active       BOOLEAN NOT NULL DEFAULT false,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    provider        TEXT,
    service_url     TEXT,
    provider_model  TEXT,
    table_name      TEXT,
    is_ready        BOOLEAN NOT NULL DEFAULT false,
    provider_config TEXT NOT NULL DEFAULT 'default'
);

CREATE UNIQUE INDEX ai_models_active_embedding_text_idx
    ON ai_models (is_active)
    WHERE is_active = true AND model_type = 'embedding' AND content_type = 'text';

CREATE UNIQUE INDEX ai_models_active_embedding_code_idx
    ON ai_models (is_active)
    WHERE is_active = true AND model_type = 'embedding' AND content_type = 'code';

CREATE UNIQUE INDEX ai_models_active_generation_text_idx
    ON ai_models (is_active)
    WHERE is_active = true AND model_type = 'generation' AND content_type = 'text';

CREATE INDEX ai_models_provider_config_idx
    ON ai_models (provider_config);

INSERT INTO ai_models (
    id, slug, dimensions, content_type, model_type, is_active, description, created_at,
    provider, service_url, provider_model, table_name, is_ready, provider_config
)
SELECT
    id, slug, dimensions, content_type, 'embedding', is_active, description, created_at,
    provider, service_url, provider_model, table_name, is_ready, provider_config
FROM embedding_models
ON CONFLICT (id) DO UPDATE SET
    slug = EXCLUDED.slug,
    dimensions = EXCLUDED.dimensions,
    content_type = EXCLUDED.content_type,
    model_type = EXCLUDED.model_type,
    is_active = EXCLUDED.is_active,
    description = EXCLUDED.description,
    created_at = EXCLUDED.created_at,
    provider = EXCLUDED.provider,
    service_url = EXCLUDED.service_url,
    provider_model = EXCLUDED.provider_model,
    table_name = EXCLUDED.table_name,
    is_ready = EXCLUDED.is_ready,
    provider_config = EXCLUDED.provider_config;

ALTER TABLE memories
    DROP CONSTRAINT IF EXISTS memories_embedding_model_id_fkey,
    ADD CONSTRAINT memories_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES ai_models(id);

ALTER TABLE memories
    DROP CONSTRAINT IF EXISTS memories_embedding_code_model_id_fkey,
    ADD CONSTRAINT memories_embedding_code_model_id_fkey
        FOREIGN KEY (embedding_code_model_id) REFERENCES ai_models(id);

ALTER TABLE entities
    DROP CONSTRAINT IF EXISTS entities_embedding_model_id_fkey,
    ADD CONSTRAINT entities_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES ai_models(id);

ALTER TABLE knowledge_artifacts
    DROP CONSTRAINT IF EXISTS knowledge_artifacts_embedding_model_id_fkey,
    ADD CONSTRAINT knowledge_artifacts_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES ai_models(id);

ALTER TABLE skills
    DROP CONSTRAINT IF EXISTS skills_embedding_model_id_fkey,
    ADD CONSTRAINT skills_embedding_model_id_fkey
        FOREIGN KEY (embedding_model_id) REFERENCES ai_models(id);

ALTER TABLE embedding_index
    DROP CONSTRAINT IF EXISTS embedding_index_model_id_fkey,
    ADD CONSTRAINT embedding_index_model_id_fkey
        FOREIGN KEY (model_id) REFERENCES ai_models(id);

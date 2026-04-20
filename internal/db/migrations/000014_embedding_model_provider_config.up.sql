-- Migration 000014: embedding model provider profiles
-- Adds a named provider profile on embedding_models to resolve runtime
-- embedding backend/service credentials from config.embedding.providers.

ALTER TABLE {{POSTBRAIN_SCHEMA}}.embedding_models
    ADD COLUMN provider_config TEXT NOT NULL DEFAULT 'default';

CREATE INDEX embedding_models_provider_config_idx
    ON {{POSTBRAIN_SCHEMA}}.embedding_models (provider_config);

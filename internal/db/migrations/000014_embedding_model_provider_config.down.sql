-- Migration 000014 rollback: embedding model provider profiles

DROP INDEX IF EXISTS {{POSTBRAIN_SCHEMA}}.embedding_models_provider_config_idx;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.embedding_models
    DROP COLUMN provider_config;

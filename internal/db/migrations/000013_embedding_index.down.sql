-- Migration 000013 rollback: embedding index
-- Removes embedding_index and embedding_models metadata extension columns.
-- Legacy embedding_model_id columns remain untouched because they were not
-- dropped in the corresponding up migration.

DROP TABLE {{POSTBRAIN_SCHEMA}}.embedding_index;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.embedding_models
    DROP COLUMN is_ready,
    DROP COLUMN table_name,
    DROP COLUMN provider_model,
    DROP COLUMN service_url,
    DROP COLUMN provider;

DROP INDEX IF EXISTS {{POSTBRAIN_SCHEMA}}.memories_embedding_code_hnsw_idx;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.memories
    ALTER COLUMN embedding_code TYPE vector(1024);

CREATE INDEX memories_embedding_code_hnsw_idx
    ON {{POSTBRAIN_SCHEMA}}.memories USING hnsw (embedding_code vector_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE embedding_code IS NOT NULL;

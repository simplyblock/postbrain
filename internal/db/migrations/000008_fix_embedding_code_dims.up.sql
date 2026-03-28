-- Resize embedding_code from vector(1024) to vector(1536) to match
-- text-embedding-3-small, which is now used for both text and code embeddings.
-- All existing embedding_code values are NULL so no data is lost.

DROP INDEX IF EXISTS memories_embedding_code_hnsw_idx;

ALTER TABLE memories
    ALTER COLUMN embedding_code TYPE vector(1536);

CREATE INDEX memories_embedding_code_hnsw_idx
    ON memories USING hnsw (embedding_code vector_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE embedding_code IS NOT NULL;
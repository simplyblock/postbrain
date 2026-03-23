-- name: GetMemory :one
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at
FROM memories WHERE id = $1;

-- name: ListMemoriesByScope :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at
FROM memories WHERE scope_id=$1 AND is_active=true
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateMemory :one
INSERT INTO memories
(memory_type, scope_id, author_id, content, summary,
 embedding, embedding_model_id, embedding_code, embedding_code_model_id,
 content_kind, meta, version, is_active, confidence, importance,
 access_count, expires_at, promotion_status, promoted_to, source_ref)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
        COALESCE($12,1),true,COALESCE($13,1.0),COALESCE($14,0.5),
        0,$15,$16,$17,$18)
RETURNING id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at;

-- name: UpdateMemoryContent :one
UPDATE memories
SET content=$2, embedding=$3, embedding_model_id=$4,
    embedding_code=$5, embedding_code_model_id=$6,
    content_kind=$7, version=version+1, updated_at=now()
WHERE id=$1
RETURNING id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at;

-- name: SoftDeleteMemory :exec
UPDATE memories SET is_active=false, updated_at=now() WHERE id=$1;

-- name: HardDeleteMemory :exec
DELETE FROM memories WHERE id=$1;

-- name: IncrementMemoryAccess :exec
UPDATE memories SET access_count=access_count+1, last_accessed=now(), updated_at=now() WHERE id=$1;

-- name: FindNearDuplicates :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at
FROM memories
WHERE scope_id = $1
  AND is_active = true
  AND (embedding <=> $2) <= $3::float8
  AND ($4::uuid IS NULL OR id != $4)
LIMIT 5;

-- name: RecallMemoriesByVector :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at,
    1 - (embedding <=> $3) AS vec_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[])
ORDER BY embedding <=> $3
LIMIT $2;

-- name: RecallMemoriesByCodeVector :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at,
    1 - (embedding_code <=> $3) AS vec_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[]) AND embedding_code IS NOT NULL
ORDER BY embedding_code <=> $3
LIMIT $2;

-- name: RecallMemoriesByFTS :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at,
    ts_rank_cd(to_tsvector('postbrain_fts', content), plainto_tsquery('postbrain_fts', $3)) AS bm25_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[])
  AND to_tsvector('postbrain_fts', content) @@ plainto_tsquery('postbrain_fts', $3)
ORDER BY bm25_score DESC
LIMIT $2;

-- name: ListConsolidationCandidates :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at
FROM memories
WHERE is_active = true AND scope_id = $1 AND importance < 0.7 AND access_count < 3;

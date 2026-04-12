-- name: ListMemoriesForEntity :many
-- Returns active memories linked to a given entity, most recent first.
SELECT m.id, m.memory_type, m.scope_id, m.author_id,
    m.content, m.summary, m.embedding, m.embedding_model_id,
    m.embedding_code, m.embedding_code_model_id, m.content_kind, m.meta,
    m.version, m.is_active, m.confidence, m.importance, m.access_count, m.last_accessed,
    m.expires_at, m.promotion_status, m.promoted_to, m.source_ref, m.parent_memory_id, m.created_at, m.updated_at
FROM memories m
JOIN memory_entities me ON me.memory_id = m.id
WHERE me.entity_id = $1 AND m.is_active = true
ORDER BY m.created_at DESC
LIMIT $2;

-- name: GetMemory :one
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at
FROM memories WHERE id = $1;

-- name: ListMemoriesByScope :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at
FROM memories WHERE scope_id=$1 AND is_active=true
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateMemory :one
INSERT INTO memories
(memory_type, scope_id, author_id, content, summary,
 embedding, embedding_model_id, embedding_code, embedding_code_model_id,
 content_kind, meta, version, is_active, confidence, importance,
 access_count, expires_at, promotion_status, promoted_to, source_ref, parent_memory_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
        COALESCE($12,1),true,COALESCE($13,1.0),COALESCE($14,0.5),
        0,$15,$16,$17,$18,$19)
RETURNING id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at;

-- name: UpdateMemoryContent :one
UPDATE memories
SET content=$2, embedding=$3, embedding_model_id=$4,
    embedding_code=$5, embedding_code_model_id=$6,
    content_kind=$7, summary=$8, meta=$9, version=version+1, updated_at=now()
WHERE id=$1
RETURNING id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at;

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
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at
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
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at,
    (1 - (embedding <=> $3))::float4 AS vec_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[])
  AND created_at >= $4::timestamptz
  AND created_at <= $5::timestamptz
ORDER BY embedding <=> $3
LIMIT $2;

-- name: RecallMemoriesByCodeVector :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at,
    (1 - (embedding_code <=> $3))::float4 AS vec_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[]) AND embedding_code IS NOT NULL
  AND created_at >= $4::timestamptz
  AND created_at <= $5::timestamptz
ORDER BY embedding_code <=> $3
LIMIT $2;

-- name: RecallMemoriesByFTS :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at,
    ts_rank_cd(to_tsvector('postbrain_fts', content), plainto_tsquery('postbrain_fts', $3)) AS bm25_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[])
  AND created_at >= $4::timestamptz
  AND created_at <= $5::timestamptz
  AND to_tsvector('postbrain_fts', content) @@ plainto_tsquery('postbrain_fts', $3)
ORDER BY bm25_score DESC
LIMIT $2;

-- name: RecallMemoriesByTrigram :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at,
    similarity(content, $3) AS trgm_score
FROM memories
WHERE is_active = true AND scope_id = ANY($1::uuid[])
  AND created_at >= $4::timestamptz
  AND created_at <= $5::timestamptz
  AND similarity(content, $3) > 0.1
ORDER BY trgm_score DESC
LIMIT $2;

-- name: ListConsolidationCandidates :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at
FROM memories
WHERE is_active = true AND scope_id = $1 AND importance < 0.7 AND access_count < 3;

-- name: ListChunkMemories :many
-- Returns chunk memories (children) for a given parent memory.
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, parent_memory_id, created_at, updated_at
FROM memories
WHERE parent_memory_id = $1 AND is_active = true
ORDER BY created_at;

-- name: MarkMemoryNominated :exec
UPDATE memories SET promotion_status='nominated', updated_at=now() WHERE id=$1;

-- name: ExpireWorkingMemories :execrows
UPDATE memories SET is_active = false
WHERE expires_at < now() AND is_active = true;

-- name: GetScopesWithConsolidationCandidates :many
SELECT DISTINCT scope_id FROM memories
WHERE is_active = true AND importance < 0.7 AND access_count < 3;

-- name: GetActiveTextModel :one
SELECT id, slug, dimensions, content_type, is_active, description, created_at
FROM ai_models WHERE model_type = 'embedding' AND content_type = 'text' AND is_active = true LIMIT 1;

-- name: GetActiveCodeModel :one
SELECT id, slug, dimensions, content_type, is_active, description, created_at
FROM ai_models WHERE model_type = 'embedding' AND content_type = 'code' AND is_active = true LIMIT 1;

-- name: GetMemoriesNeedingTextReembed :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at
FROM memories
WHERE is_active = true
  AND (embedding_model_id IS NULL OR embedding_model_id != $1)
  AND content_kind = 'text'
ORDER BY created_at
LIMIT $2 OFFSET $3;

-- name: GetMemoriesNeedingCodeReembed :many
SELECT id, memory_type, scope_id, author_id,
    content, summary, embedding, embedding_model_id,
    embedding_code, embedding_code_model_id, content_kind, meta,
    version, is_active, confidence, importance, access_count, last_accessed,
    expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at
FROM memories
WHERE is_active = true
  AND content_kind = 'code'
  AND (embedding_code_model_id IS NULL OR embedding_code_model_id != $1)
ORDER BY created_at
LIMIT $2 OFFSET $3;

-- name: UpdateMemoryTextEmbedding :exec
UPDATE memories SET embedding = $2, embedding_model_id = $3, updated_at = now()
WHERE id = $1;

-- name: UpdateMemoryCodeEmbedding :exec
UPDATE memories SET embedding_code = $2, embedding_code_model_id = $3, updated_at = now()
WHERE id = $1;

-- name: GetAIModelRuntimeConfigByID :one
SELECT provider, service_url, provider_model, dimensions, provider_config
FROM ai_models WHERE id = $1;

-- name: GetActiveAIModelIDByTypeAndContent :one
SELECT id FROM ai_models
WHERE is_active = true AND model_type = $1 AND content_type = $2
LIMIT 1;

-- name: UpsertEmbeddingModel :one
INSERT INTO ai_models (slug, dimensions, content_type, model_type, is_active)
VALUES ($1, $2, $3, 'embedding', $4)
ON CONFLICT (slug) DO UPDATE SET
  dimensions = EXCLUDED.dimensions,
  is_active = EXCLUDED.is_active,
  content_type = EXCLUDED.content_type,
  model_type = EXCLUDED.model_type
RETURNING id, slug, dimensions, content_type, is_active, description, created_at;

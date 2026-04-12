-- name: GetPendingTextEmbeddingBatch :many
SELECT ei.object_type, ei.object_id, ei.retry_count,
       COALESCE(
           CASE
               WHEN ei.object_type='skill' THEN btrim(concat_ws(' ', NULLIF(s.description, ''), NULLIF(s.body, '')))
               ELSE COALESCE(m.content, ka.content, s.body)
           END,
           ''::text
       ) AS content
FROM embedding_index ei
LEFT JOIN memories m ON ei.object_type='memory' AND m.id=ei.object_id AND m.is_active=true
LEFT JOIN knowledge_artifacts ka ON ei.object_type='knowledge_artifact' AND ka.id=ei.object_id
LEFT JOIN skills s ON ei.object_type='skill' AND s.id=ei.object_id
WHERE ei.model_id = $1
  AND ei.status = 'pending'
  AND ei.object_type IN ('memory','knowledge_artifact','skill')
ORDER BY ei.updated_at, ei.object_id
LIMIT $2;

-- name: GetPendingCodeEmbeddingBatch :many
SELECT ei.object_id, ei.retry_count, m.content, m.scope_id
FROM embedding_index ei
JOIN memories m ON ei.object_type='memory' AND m.id=ei.object_id
WHERE ei.model_id = $1
  AND ei.status = 'pending'
  AND m.is_active = true
  AND m.content_kind = 'code'
ORDER BY ei.updated_at, ei.object_id
LIMIT $2;

-- name: UpdateKnowledgeArtifactEmbedding :exec
UPDATE knowledge_artifacts SET embedding = $2, embedding_model_id = $3, updated_at = now()
WHERE id = $1;

-- name: UpdateSkillEmbedding :exec
UPDATE skills SET embedding = $2, embedding_model_id = $3, updated_at = now()
WHERE id = $1;

-- name: GetMemoryScopeID :one
SELECT scope_id FROM memories WHERE id = $1;

-- name: GetSkillScopeID :one
SELECT scope_id FROM skills WHERE id = $1;

-- name: GetArtifactOwnerScopeID :one
SELECT owner_scope_id FROM knowledge_artifacts WHERE id = $1;

-- name: MarkEmbeddingIndexReady :exec
UPDATE embedding_index
SET status='ready', retry_count=0, last_error=NULL, updated_at=now()
WHERE object_type=$1 AND object_id=$2 AND model_id=$3;

-- name: MarkEmbeddingIndexFailed :exec
UPDATE embedding_index
SET status=$4, retry_count=$5, last_error=$6, updated_at=now()
WHERE object_type=$1 AND object_id=$2 AND model_id=$3;

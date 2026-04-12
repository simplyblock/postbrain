-- name: UpsertEntity :one
INSERT INTO entities (scope_id, entity_type, name, canonical, meta, embedding, embedding_model_id)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (scope_id, entity_type, canonical)
DO UPDATE SET name=EXCLUDED.name, meta=EXCLUDED.meta,
              embedding=EXCLUDED.embedding, embedding_model_id=EXCLUDED.embedding_model_id,
              updated_at=now()
RETURNING id, scope_id, entity_type, name, canonical, meta,
          embedding, embedding_model_id, created_at, updated_at;

-- name: GetEntityByCanonical :one
SELECT id, scope_id, entity_type, name, canonical, meta,
       embedding, embedding_model_id, created_at, updated_at
FROM entities WHERE scope_id=$1 AND entity_type=$2 AND canonical=$3;

-- name: LinkMemoryToEntity :exec
INSERT INTO memory_entities (memory_id, entity_id, role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: LinkArtifactToEntity :exec
INSERT INTO artifact_entities (artifact_id, entity_id, role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: DeleteArtifactEntityLinks :exec
DELETE FROM artifact_entities WHERE artifact_id = $1;

-- name: UpsertRelation :one
INSERT INTO relations (scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (scope_id, subject_id, predicate, object_id)
DO UPDATE SET confidence=EXCLUDED.confidence, source_memory=EXCLUDED.source_memory,
              source_artifact=EXCLUDED.source_artifact, source_file=EXCLUDED.source_file
RETURNING id, scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file, created_at;

-- name: ListRelationsForEntity :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file, created_at
FROM relations WHERE subject_id=$1 OR object_id=$1
ORDER BY created_at;

-- name: ListRelationsForEntityByPredicate :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file, created_at
FROM relations
WHERE (subject_id = $1 OR object_id = $1) AND predicate = $2
ORDER BY created_at;

-- name: ListEntitiesByCanonical :many
SELECT id, scope_id, entity_type, name, canonical, meta,
       embedding, embedding_model_id, created_at, updated_at
FROM entities
WHERE scope_id=$1 AND canonical=$2 AND entity_type != $3;

-- name: ListEntitiesByScope :many
SELECT id, scope_id, entity_type, name, canonical, meta,
       embedding, embedding_model_id, created_at, updated_at
FROM entities
WHERE scope_id=$1 AND ($2='' OR entity_type=$2)
ORDER BY name
LIMIT $3 OFFSET $4;

-- name: ListRelationsByScope :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file, created_at
FROM relations
WHERE scope_id=$1
ORDER BY created_at;

-- name: DeleteRelationsBySourceFile :exec
-- Used by incremental re-index to invalidate stale edges before re-extraction.
DELETE FROM relations WHERE scope_id = $1 AND source_file = $2;

-- name: GetEntityByID :one
SELECT id, scope_id, entity_type, name, canonical, meta,
       embedding, embedding_model_id, created_at, updated_at
FROM entities WHERE id = $1;

-- name: ListOutgoingRelations :many
-- Relations where this entity is the subject, optionally filtered by predicate.
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file, created_at
FROM relations
WHERE scope_id = $1 AND subject_id = $2 AND ($3 = '' OR predicate = $3)
ORDER BY confidence DESC, created_at;

-- name: ListIncomingRelations :many
-- Relations where this entity is the object, optionally filtered by predicate.
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, source_artifact, source_file, created_at
FROM relations
WHERE scope_id = $1 AND object_id = $2 AND ($3 = '' OR predicate = $3)
ORDER BY confidence DESC, created_at;

-- name: FindEntitiesBySuffix :many
-- Heuristic resolution: match call/type targets against entities whose
-- canonical name equals $2, ends with ".$2", or ends with "::$2".
-- Returns up to 5 candidates ordered by shortest name first (most specific).
SELECT id, scope_id, entity_type, name, canonical, meta,
       embedding, embedding_model_id, created_at, updated_at
FROM entities
WHERE scope_id = $1
  AND (
    canonical = $2
    OR canonical LIKE ('%.' || $2)
    OR canonical LIKE ('%::' || $2)
    OR canonical LIKE ('%#' || $2)
  )
ORDER BY length(canonical) ASC
LIMIT 5;

-- name: GetEntityBatchFirstPage :many
SELECT id, scope_id, entity_type, name, canonical, created_at
FROM entities
ORDER BY created_at, id
LIMIT $1;

-- name: GetEntityBatchCursor :many
SELECT id, scope_id, entity_type, name, canonical, created_at
FROM entities
WHERE (created_at, id) > ($1::timestamptz, $2::uuid)
ORDER BY created_at, id
LIMIT $3;

-- name: GetRelationBatchFirstPage :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, created_at
FROM relations
ORDER BY created_at, id
LIMIT $1;

-- name: GetRelationBatchCursor :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, created_at
FROM relations
WHERE (created_at, id) > ($1::timestamptz, $2::uuid)
ORDER BY created_at, id
LIMIT $3;

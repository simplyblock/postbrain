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

-- name: UpsertRelation :one
INSERT INTO relations (scope_id, subject_id, predicate, object_id, confidence, source_memory)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (scope_id, subject_id, predicate, object_id)
DO UPDATE SET confidence=EXCLUDED.confidence, source_memory=EXCLUDED.source_memory
RETURNING id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at;

-- name: ListRelationsForEntity :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at
FROM relations WHERE subject_id=$1 OR object_id=$1
ORDER BY created_at;

-- name: ListEntitiesByScope :many
SELECT id, scope_id, entity_type, name, canonical, meta,
       embedding, embedding_model_id, created_at, updated_at
FROM entities
WHERE scope_id=$1 AND ($2='' OR entity_type=$2)
ORDER BY name
LIMIT $3 OFFSET $4;

-- name: ListRelationsByScope :many
SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at
FROM relations
WHERE scope_id=$1
ORDER BY created_at
LIMIT $2 OFFSET $3;

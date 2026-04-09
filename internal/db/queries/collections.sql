-- name: CreateCollection :one
INSERT INTO knowledge_collections (scope_id, owner_id, slug, name, description, visibility, meta)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at;

-- name: GetCollection :one
SELECT id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at
FROM knowledge_collections WHERE id=$1;

-- name: GetCollectionBySlug :one
SELECT id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at
FROM knowledge_collections WHERE scope_id=$1 AND slug=$2;

-- name: ListCollections :many
SELECT id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at
FROM knowledge_collections WHERE scope_id=$1 ORDER BY name;

-- name: AddCollectionItem :exec
INSERT INTO knowledge_collection_items (collection_id, artifact_id, added_by)
VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: RemoveCollectionItem :exec
DELETE FROM knowledge_collection_items WHERE collection_id=$1 AND artifact_id=$2;

-- name: ListCollectionItems :many
SELECT ka.id, ka.knowledge_type, ka.owner_scope_id, ka.author_id,
    ka.visibility, ka.status, ka.published_at, ka.deprecated_at, ka.review_required,
    ka.title, ka.content, ka.summary, ka.embedding, ka.embedding_model_id, ka.meta,
    ka.endorsement_count, ka.access_count, ka.last_accessed,
    ka.version, ka.previous_version, ka.source_memory_id, ka.source_ref,
    ka.created_at, ka.updated_at, ka.artifact_kind
FROM knowledge_artifacts ka
JOIN knowledge_collection_items kci ON kci.artifact_id = ka.id
WHERE kci.collection_id = $1
ORDER BY kci.position, kci.added_at;

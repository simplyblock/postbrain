-- name: CreateArtifact :one
INSERT INTO knowledge_artifacts
(knowledge_type, owner_scope_id, author_id, visibility, status,
 published_at, deprecated_at, review_required,
 title, content, summary, embedding, embedding_model_id, meta,
 version, previous_version, source_memory_id, source_ref)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
        NULLIF($16, '00000000-0000-0000-0000-000000000000'::uuid),
        NULLIF($17, '00000000-0000-0000-0000-000000000000'::uuid),
        $18)
RETURNING id, knowledge_type, owner_scope_id, author_id,
    visibility, status, published_at, deprecated_at, review_required,
    title, content, summary, embedding, embedding_model_id, meta,
    endorsement_count, access_count, last_accessed,
    version, previous_version, source_memory_id, source_ref,
    created_at, updated_at;

-- name: GetArtifact :one
SELECT id, knowledge_type, owner_scope_id, author_id,
    visibility, status, published_at, deprecated_at, review_required,
    title, content, summary, embedding, embedding_model_id, meta,
    endorsement_count, access_count, last_accessed,
    version, previous_version, source_memory_id, source_ref,
    created_at, updated_at
FROM knowledge_artifacts WHERE id = $1;

-- name: UpdateArtifact :one
UPDATE knowledge_artifacts
SET title=$2, content=$3, summary=$4, embedding=$5,
    embedding_model_id=$6, version=version+1, updated_at=now()
WHERE id=$1
RETURNING id, knowledge_type, owner_scope_id, author_id,
    visibility, status, published_at, deprecated_at, review_required,
    title, content, summary, embedding, embedding_model_id, meta,
    endorsement_count, access_count, last_accessed,
    version, previous_version, source_memory_id, source_ref,
    created_at, updated_at;

-- name: UpdateArtifactStatus :exec
UPDATE knowledge_artifacts
SET status=$2, published_at=$3, deprecated_at=$4, updated_at=now()
WHERE id=$1;

-- name: IncrementArtifactEndorsementCount :exec
UPDATE knowledge_artifacts SET endorsement_count=endorsement_count+1, updated_at=now() WHERE id=$1;

-- name: IncrementArtifactAccess :exec
UPDATE knowledge_artifacts SET access_count=access_count+1, last_accessed=now(), updated_at=now() WHERE id=$1;

-- name: SnapshotArtifactVersion :exec
INSERT INTO knowledge_history (artifact_id, version, content, summary, changed_by, change_note)
VALUES ($1,$2,$3,$4,$5,$6);

-- name: CreateEndorsement :one
INSERT INTO knowledge_endorsements (artifact_id, endorser_id, note)
VALUES ($1,$2,$3)
RETURNING id, artifact_id, endorser_id, note, created_at;

-- name: GetEndorsementByEndorser :one
SELECT id, artifact_id, endorser_id, note, created_at
FROM knowledge_endorsements WHERE artifact_id=$1 AND endorser_id=$2;

-- name: ListVisibleArtifacts :many
SELECT id, knowledge_type, owner_scope_id, author_id,
    visibility, status, published_at, deprecated_at, review_required,
    title, content, summary, embedding, embedding_model_id, meta,
    endorsement_count, access_count, last_accessed,
    version, previous_version, source_memory_id, source_ref,
    created_at, updated_at
FROM knowledge_artifacts
WHERE status = 'published'
  AND (owner_scope_id = ANY($1::uuid[]) OR visibility = 'company')
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: RecallArtifactsByVector :many
SELECT id, knowledge_type, owner_scope_id, author_id,
    visibility, status, published_at, deprecated_at, review_required,
    title, content, summary, embedding, embedding_model_id, meta,
    endorsement_count, access_count, last_accessed,
    version, previous_version, source_memory_id, source_ref,
    created_at, updated_at,
    1 - (embedding <=> $3) AS vec_score
FROM knowledge_artifacts ka
WHERE ka.status = 'published' AND ka.owner_scope_id = ANY($1::uuid[])
ORDER BY ka.embedding <=> $3
LIMIT $2;

-- name: GetArtifactHistory :many
SELECT id, artifact_id, version, content, summary, changed_by, change_note, created_at
FROM knowledge_history
WHERE artifact_id = $1
ORDER BY version DESC;

-- name: RecallArtifactsByFTS :many
SELECT id, knowledge_type, owner_scope_id, author_id,
    visibility, status, published_at, deprecated_at, review_required,
    title, content, summary, embedding, embedding_model_id, meta,
    endorsement_count, access_count, last_accessed,
    version, previous_version, source_memory_id, source_ref,
    created_at, updated_at,
    ts_rank_cd(to_tsvector('postbrain_fts', content),
               plainto_tsquery('postbrain_fts', $3)) AS bm25_score
FROM knowledge_artifacts
WHERE status = 'published'
  AND owner_scope_id = ANY($1::uuid[])
  AND to_tsvector('postbrain_fts', content) @@ plainto_tsquery('postbrain_fts', $3)
ORDER BY bm25_score DESC
LIMIT $2;

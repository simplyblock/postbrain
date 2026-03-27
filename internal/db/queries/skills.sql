-- name: CreateSkill :one
INSERT INTO skills
(scope_id, author_id, source_artifact_id, slug, name, description,
 agent_types, body, parameters, visibility, status, published_at,
 deprecated_at, review_required, version, previous_version,
 embedding, embedding_model_id)
VALUES ($1,$2,NULLIF($3, '00000000-0000-0000-0000-000000000000'::uuid),
        $4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
        NULLIF($16, '00000000-0000-0000-0000-000000000000'::uuid),
        $17,$18)
RETURNING id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at;

-- name: GetSkill :one
SELECT id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at
FROM skills WHERE id = $1;

-- name: GetSkillBySlug :one
SELECT id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at
FROM skills WHERE scope_id = $1 AND slug = $2;

-- name: UpdateSkillContent :one
UPDATE skills
SET body=$2, parameters=$3, embedding=$4, embedding_model_id=$5,
    version=version+1, updated_at=now()
WHERE id=$1
RETURNING id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at;

-- name: UpdateSkillStatus :exec
UPDATE skills SET status=$2, published_at=$3, deprecated_at=$4, updated_at=now()
WHERE id=$1;

-- name: SnapshotSkillVersion :exec
INSERT INTO skill_history (skill_id, version, body, parameters, changed_by, change_note)
VALUES ($1,$2,$3,$4,$5,$6);

-- name: CreateSkillEndorsement :one
INSERT INTO skill_endorsements (skill_id, endorser_id, note)
VALUES ($1,$2,$3)
RETURNING id, skill_id, endorser_id, note, created_at;

-- name: GetSkillEndorsementByEndorser :one
SELECT id, skill_id, endorser_id, note, created_at
FROM skill_endorsements WHERE skill_id=$1 AND endorser_id=$2;

-- name: CountSkillEndorsements :one
SELECT COUNT(*) FROM skill_endorsements WHERE skill_id = $1;

-- name: RecallSkillsByVector :many
SELECT id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at,
    (embedding <=> $1) AS score
FROM skills
WHERE status = 'published'
  AND scope_id = ANY($2::uuid[])
  AND ($3 = 'any' OR 'any' = ANY(agent_types) OR $3 = ANY(agent_types))
ORDER BY embedding <=> $1
LIMIT $4;

-- name: RecallSkillsByFTS :many
SELECT id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at,
    ts_rank_cd(to_tsvector('postbrain_fts', description || ' ' || body),
               plainto_tsquery('postbrain_fts', $1)) AS score
FROM skills
WHERE status = 'published'
  AND scope_id = ANY($2::uuid[])
  AND ($3 = 'any' OR 'any' = ANY(agent_types) OR $3 = ANY(agent_types))
  AND to_tsvector('postbrain_fts', description || ' ' || body)
      @@ plainto_tsquery('postbrain_fts', $1)
ORDER BY score DESC
LIMIT $4;

-- name: RecallSkillsByTrigram :many
SELECT id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at,
    similarity(description || ' ' || body, $1) AS score
FROM skills
WHERE status = 'published'
  AND scope_id = ANY($2::uuid[])
  AND ($3 = 'any' OR 'any' = ANY(agent_types) OR $3 = ANY(agent_types))
  AND similarity(description || ' ' || body, $1) > 0.1
ORDER BY score DESC
LIMIT $4;

-- name: ListPublishedSkillsForAgent :many
SELECT id, scope_id, author_id, source_artifact_id,
    slug, name, description, agent_types, body, parameters,
    visibility, status, published_at, deprecated_at, review_required,
    version, previous_version, embedding, embedding_model_id,
    invocation_count, last_invoked_at, created_at, updated_at
FROM skills
WHERE status = 'published'
  AND scope_id = ANY($1::uuid[])
  AND ($2 = 'any' OR 'any' = ANY(agent_types) OR $2 = ANY(agent_types))
ORDER BY created_at DESC;

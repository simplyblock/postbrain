package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// scanPrincipal scans a row into a Principal struct.
func scanPrincipal(row pgx.Row) (*Principal, error) {
	var p Principal
	err := row.Scan(
		&p.ID,
		&p.Kind,
		&p.Slug,
		&p.DisplayName,
		&p.Meta,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePrincipal inserts a new principal row and returns the created record.
func CreatePrincipal(ctx context.Context, pool *pgxpool.Pool, kind, slug, displayName string, meta []byte) (*Principal, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	row := pool.QueryRow(ctx,
		`INSERT INTO principals (kind, slug, display_name, meta)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, kind, slug, display_name, meta, created_at, updated_at`,
		kind, slug, displayName, meta,
	)
	return scanPrincipal(row)
}

// GetPrincipalByID looks up a principal by its UUID.
func GetPrincipalByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Principal, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, kind, slug, display_name, meta, created_at, updated_at
		 FROM principals WHERE id = $1`,
		id,
	)
	p, err := scanPrincipal(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetPrincipalBySlug looks up a principal by its slug (case-insensitive).
func GetPrincipalBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (*Principal, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, kind, slug, display_name, meta, created_at, updated_at
		 FROM principals WHERE slug = $1`,
		slug,
	)
	p, err := scanPrincipal(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// CreateMembership inserts a membership record linking memberID to parentID.
func CreateMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID, role string, grantedBy *uuid.UUID) (*Membership, error) {
	var m Membership
	err := pool.QueryRow(ctx,
		`INSERT INTO principal_memberships (member_id, parent_id, role, granted_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING member_id, parent_id, role, granted_by, created_at`,
		memberID, parentID, role, grantedBy,
	).Scan(&m.MemberID, &m.ParentID, &m.Role, &m.GrantedBy, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteMembership removes a direct membership between memberID and parentID.
func DeleteMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`DELETE FROM principal_memberships WHERE member_id = $1 AND parent_id = $2`,
		memberID, parentID,
	)
	return err
}

// GetMemberships returns the direct parent memberships for a given principal.
func GetMemberships(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]*Membership, error) {
	rows, err := pool.Query(ctx,
		`SELECT member_id, parent_id, role, granted_by, created_at
		 FROM principal_memberships WHERE member_id = $1`,
		memberID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []*Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.MemberID, &m.ParentID, &m.Role, &m.GrantedBy, &m.CreatedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, &m)
	}
	return memberships, rows.Err()
}

// GetAllParentIDs returns all ancestor principal IDs for a given principal via recursive CTE.
// The result includes the principal itself.
func GetAllParentIDs(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`WITH RECURSIVE member_tree AS (
		     SELECT $1::uuid AS id
		     UNION ALL
		     SELECT pm.parent_id
		     FROM   principal_memberships pm
		     JOIN   member_tree mt ON pm.member_id = mt.id
		 )
		 SELECT id FROM member_tree`,
		memberID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CreateScope inserts a new scope row. The path is computed by the database trigger.
func CreateScope(ctx context.Context, pool *pgxpool.Pool, kind, externalID, name string, parentID *uuid.UUID, principalID uuid.UUID, meta []byte) (*Scope, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	var s Scope
	err := pool.QueryRow(ctx,
		`INSERT INTO scopes (kind, external_id, name, parent_id, principal_id, meta, path)
		 VALUES ($1, $2, $3, $4, $5, $6, 'placeholder')
		 RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at`,
		kind, externalID, name, parentID, principalID, meta,
	).Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID, &s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetScopeByID retrieves a scope by its UUID.
func GetScopeByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Scope, error) {
	var s Scope
	err := pool.QueryRow(ctx,
		`SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
		 FROM scopes WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID, &s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetScopeByExternalID retrieves a scope by its kind and external_id (case-insensitive).
func GetScopeByExternalID(ctx context.Context, pool *pgxpool.Pool, kind, externalID string) (*Scope, error) {
	var s Scope
	err := pool.QueryRow(ctx,
		`SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
		 FROM scopes WHERE kind = $1 AND external_id = $2`,
		kind, externalID,
	).Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID, &s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetAncestorScopeIDs returns all ancestor scope IDs of a given scope (including itself)
// using the ltree @> operator.
func GetAncestorScopeIDs(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`SELECT s2.id FROM scopes s1
		 JOIN scopes s2 ON s2.path @> s1.path
		 WHERE s1.id = $1`,
		scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// scanToken scans a token row. scopeIDs is stored as a PostgreSQL UUID array.
func scanToken(row pgx.Row) (*Token, error) {
	var t Token
	var scopeIDs []uuid.UUID
	var permissions []string
	err := row.Scan(
		&t.ID,
		&t.PrincipalID,
		&t.TokenHash,
		&t.Name,
		&scopeIDs,
		&permissions,
		&t.ExpiresAt,
		&t.LastUsedAt,
		&t.CreatedAt,
		&t.RevokedAt,
	)
	if err != nil {
		return nil, err
	}
	t.ScopeIDs = scopeIDs
	t.Permissions = permissions
	return &t, nil
}

// CreateToken inserts a new token record. The raw token is never stored; only the hash.
func CreateToken(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID, tokenHash, name string, scopeIDs []uuid.UUID, permissions []string, expiresAt *time.Time) (*Token, error) {
	if len(permissions) == 0 {
		permissions = []string{"read"}
	}

	row := pool.QueryRow(ctx,
		`INSERT INTO tokens (principal_id, token_hash, name, scope_ids, permissions, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at`,
		principalID, tokenHash, name, scopeIDs, permissions, expiresAt,
	)
	t, err := scanToken(row)
	if err != nil {
		return nil, fmt.Errorf("db: create token: %w", err)
	}
	return t, nil
}

// LookupToken finds a token by its hash. Returns nil, nil if not found.
// Does not filter by revoked_at or expires_at — callers must check those fields.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (*Token, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at
		 FROM tokens WHERE token_hash = $1`,
		tokenHash,
	)
	t, err := scanToken(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: lookup token: %w", err)
	}
	return t, nil
}

// RevokeToken soft-revokes a token by setting revoked_at = now().
func RevokeToken(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE tokens SET revoked_at = now() WHERE id = $1`,
		tokenID,
	)
	return err
}

// UpdateTokenLastUsed sets last_used_at = now() for a token.
func UpdateTokenLastUsed(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE tokens SET last_used_at = now() WHERE id = $1`,
		tokenID,
	)
	return err
}

// SkillScore pairs a skill with its retrieval score.
type SkillScore struct {
	Skill *Skill
	Score float64
}

// skillColumns is the canonical SELECT column list for skills.
const skillColumns = `id, scope_id, author_id, source_artifact_id,
	slug, name, description, agent_types, body, parameters,
	visibility, status, published_at, deprecated_at, review_required,
	version, previous_version, embedding::text, embedding_model_id,
	invocation_count, last_invoked_at, created_at, updated_at`

// scanSkill scans one skills row into a Skill struct.
// The embedding column is returned as its text representation from pg_vector
// and is stored as nil (we do not parse the vector back at this layer).
func scanSkill(row pgx.Row) (*Skill, error) {
	var s Skill
	var embeddingText *string
	err := row.Scan(
		&s.ID, &s.ScopeID, &s.AuthorID, &s.SourceArtifactID,
		&s.Slug, &s.Name, &s.Description, &s.AgentTypes, &s.Body, &s.Parameters,
		&s.Visibility, &s.Status, &s.PublishedAt, &s.DeprecatedAt, &s.ReviewRequired,
		&s.Version, &s.PreviousVersion, &embeddingText, &s.EmbeddingModelID,
		&s.InvocationCount, &s.LastInvokedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	// Embedding is not parsed back from its text form; callers use it write-only.
	return &s, nil
}

// CreateSkill inserts a new skill row and returns the created record.
func CreateSkill(ctx context.Context, pool *pgxpool.Pool, s *Skill) (*Skill, error) {
	var embedding interface{}
	if len(s.Embedding) > 0 {
		embedding = fmt.Sprintf("%v", float32SliceToVector(s.Embedding))
	}
	row := pool.QueryRow(ctx,
		`INSERT INTO skills
		 (scope_id, author_id, source_artifact_id, slug, name, description,
		  agent_types, body, parameters, visibility, status, published_at,
		  deprecated_at, review_required, version, previous_version,
		  embedding, embedding_model_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::vector,$18)
		 RETURNING `+skillColumns,
		s.ScopeID, s.AuthorID, s.SourceArtifactID, s.Slug, s.Name, s.Description,
		s.AgentTypes, s.Body, s.Parameters, s.Visibility, s.Status, s.PublishedAt,
		s.DeprecatedAt, s.ReviewRequired, s.Version, s.PreviousVersion,
		embedding, s.EmbeddingModelID,
	)
	return scanSkill(row)
}

// GetSkill retrieves a skill by its UUID. Returns nil, nil if not found.
func GetSkill(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Skill, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+skillColumns+` FROM skills WHERE id = $1`, id,
	)
	s, err := scanSkill(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// GetSkillBySlug retrieves a skill by scope and slug. Returns nil, nil if not found.
func GetSkillBySlug(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, slug string) (*Skill, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+skillColumns+` FROM skills WHERE scope_id = $1 AND slug = $2`,
		scopeID, slug,
	)
	s, err := scanSkill(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// UpdateSkillContent updates the body, parameters, embedding, and bumps the version.
func UpdateSkillContent(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, body string, parameters []byte, embedding []float32, modelID *uuid.UUID) (*Skill, error) {
	var embeddingVal interface{}
	if len(embedding) > 0 {
		embeddingVal = fmt.Sprintf("%v", float32SliceToVector(embedding))
	}
	row := pool.QueryRow(ctx,
		`UPDATE skills
		 SET body=$2, parameters=$3, embedding=$4::vector, embedding_model_id=$5,
		     version=version+1, updated_at=now()
		 WHERE id=$1
		 RETURNING `+skillColumns,
		id, body, parameters, embeddingVal, modelID,
	)
	s, err := scanSkill(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// UpdateSkillStatus updates status, published_at, and deprecated_at for a skill.
func UpdateSkillStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, publishedAt, deprecatedAt *time.Time) error {
	_, err := pool.Exec(ctx,
		`UPDATE skills SET status=$2, published_at=$3, deprecated_at=$4, updated_at=now()
		 WHERE id=$1`,
		id, status, publishedAt, deprecatedAt,
	)
	return err
}

// SnapshotSkillVersion inserts a skill_history row.
func SnapshotSkillVersion(ctx context.Context, pool *pgxpool.Pool, h *SkillHistory) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO skill_history (skill_id, version, body, parameters, changed_by, change_note)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		h.SkillID, h.Version, h.Body, h.Parameters, h.ChangedBy, h.ChangeNote,
	)
	return err
}

// CreateSkillEndorsement inserts a skill_endorsements row and returns the created record.
func CreateSkillEndorsement(ctx context.Context, pool *pgxpool.Pool, skillID, endorserID uuid.UUID, note *string) (*SkillEndorsement, error) {
	var e SkillEndorsement
	err := pool.QueryRow(ctx,
		`INSERT INTO skill_endorsements (skill_id, endorser_id, note)
		 VALUES ($1,$2,$3)
		 RETURNING id, skill_id, endorser_id, note, created_at`,
		skillID, endorserID, note,
	).Scan(&e.ID, &e.SkillID, &e.EndorserID, &e.Note, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetSkillEndorsementByEndorser finds an endorsement by (skill, endorser) pair.
// Returns nil, nil if not found.
func GetSkillEndorsementByEndorser(ctx context.Context, pool *pgxpool.Pool, skillID, endorserID uuid.UUID) (*SkillEndorsement, error) {
	var e SkillEndorsement
	err := pool.QueryRow(ctx,
		`SELECT id, skill_id, endorser_id, note, created_at
		 FROM skill_endorsements WHERE skill_id=$1 AND endorser_id=$2`,
		skillID, endorserID,
	).Scan(&e.ID, &e.SkillID, &e.EndorserID, &e.Note, &e.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CountSkillEndorsements returns the number of endorsements for a skill.
func CountSkillEndorsements(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID) (int, error) {
	var count int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM skill_endorsements WHERE skill_id = $1`, skillID,
	).Scan(&count)
	return count, err
}

// IncrementSkillEndorsementCount is a no-op placeholder.
// Skills do not have a denormalized endorsement_count column; use CountSkillEndorsements instead.
func IncrementSkillEndorsementCount(_ context.Context, _ *pgxpool.Pool, _ uuid.UUID) error {
	return nil
}

// RecallSkillsByVector retrieves published skills ordered by vector similarity.
func RecallSkillsByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, agentType string, limit int) ([]SkillScore, error) {
	vecStr := fmt.Sprintf("%v", float32SliceToVector(queryVec))
	rows, err := pool.Query(ctx,
		`SELECT `+skillColumns+`, (embedding <=> $1::vector) AS score
		 FROM skills
		 WHERE status = 'published'
		   AND scope_id = ANY($2)
		   AND ($3 = 'any' OR 'any' = ANY(agent_types) OR $3 = ANY(agent_types))
		 ORDER BY embedding <=> $1::vector
		 LIMIT $4`,
		vecStr, scopeIDs, agentType, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSkillScoreRows(rows)
}

// RecallSkillsByFTS retrieves published skills via full-text search.
func RecallSkillsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query, agentType string, limit int) ([]SkillScore, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+skillColumns+`, ts_rank_cd(to_tsvector('postbrain_fts', description || ' ' || body),
		         plainto_tsquery('postbrain_fts', $1)) AS score
		 FROM skills
		 WHERE status = 'published'
		   AND scope_id = ANY($2)
		   AND ($3 = 'any' OR 'any' = ANY(agent_types) OR $3 = ANY(agent_types))
		   AND to_tsvector('postbrain_fts', description || ' ' || body)
		       @@ plainto_tsquery('postbrain_fts', $1)
		 ORDER BY score DESC
		 LIMIT $4`,
		query, scopeIDs, agentType, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSkillScoreRows(rows)
}

// ListPublishedSkillsForAgent returns all published skills visible to the given agent type
// across the provided scope IDs.
func ListPublishedSkillsForAgent(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, agentType string) ([]*Skill, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+skillColumns+`
		 FROM skills
		 WHERE status = 'published'
		   AND scope_id = ANY($1)
		   AND ($2 = 'any' OR 'any' = ANY(agent_types) OR $2 = ANY(agent_types))
		 ORDER BY created_at DESC`,
		scopeIDs, agentType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []*Skill
	for rows.Next() {
		var embeddingText *string
		var s Skill
		if err := rows.Scan(
			&s.ID, &s.ScopeID, &s.AuthorID, &s.SourceArtifactID,
			&s.Slug, &s.Name, &s.Description, &s.AgentTypes, &s.Body, &s.Parameters,
			&s.Visibility, &s.Status, &s.PublishedAt, &s.DeprecatedAt, &s.ReviewRequired,
			&s.Version, &s.PreviousVersion, &embeddingText, &s.EmbeddingModelID,
			&s.InvocationCount, &s.LastInvokedAt, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		skills = append(skills, &s)
	}
	return skills, rows.Err()
}

// scanSkillScoreRows scans rows that include a trailing score column.
func scanSkillScoreRows(rows pgx.Rows) ([]SkillScore, error) {
	var results []SkillScore
	for rows.Next() {
		var s Skill
		var embeddingText *string
		var score float64
		if err := rows.Scan(
			&s.ID, &s.ScopeID, &s.AuthorID, &s.SourceArtifactID,
			&s.Slug, &s.Name, &s.Description, &s.AgentTypes, &s.Body, &s.Parameters,
			&s.Visibility, &s.Status, &s.PublishedAt, &s.DeprecatedAt, &s.ReviewRequired,
			&s.Version, &s.PreviousVersion, &embeddingText, &s.EmbeddingModelID,
			&s.InvocationCount, &s.LastInvokedAt, &s.CreatedAt, &s.UpdatedAt,
			&score,
		); err != nil {
			return nil, err
		}
		results = append(results, SkillScore{Skill: &s, Score: score})
	}
	return results, rows.Err()
}

// ── Memories ──────────────────────────────────────────────────────────────────

// memoryColumns is the canonical SELECT column list for memories.
const memoryColumns = `id, memory_type, scope_id, author_id,
	content, summary, embedding::text, embedding_model_id,
	embedding_code::text, embedding_code_model_id, content_kind, meta,
	version, is_active, confidence, importance, access_count, last_accessed,
	expires_at, promotion_status, promoted_to, source_ref, created_at, updated_at`

// scanMemory scans one memories row into a Memory struct.
// The embedding columns are returned as text and are not parsed back.
func scanMemory(row pgx.Row) (*Memory, error) {
	var m Memory
	var embText, embCodeText *string
	err := row.Scan(
		&m.ID, &m.MemoryType, &m.ScopeID, &m.AuthorID,
		&m.Content, &m.Summary, &embText, &m.EmbeddingModelID,
		&embCodeText, &m.EmbeddingCodeModelID, &m.ContentKind, &m.Meta,
		&m.Version, &m.IsActive, &m.Confidence, &m.Importance, &m.AccessCount, &m.LastAccessed,
		&m.ExpiresAt, &m.PromotionStatus, &m.PromotedTo, &m.SourceRef, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// scanMemoryRows scans multiple memories rows.
func scanMemoryRows(rows pgx.Rows) ([]*Memory, error) {
	var results []*Memory
	for rows.Next() {
		var m Memory
		var embText, embCodeText *string
		if err := rows.Scan(
			&m.ID, &m.MemoryType, &m.ScopeID, &m.AuthorID,
			&m.Content, &m.Summary, &embText, &m.EmbeddingModelID,
			&embCodeText, &m.EmbeddingCodeModelID, &m.ContentKind, &m.Meta,
			&m.Version, &m.IsActive, &m.Confidence, &m.Importance, &m.AccessCount, &m.LastAccessed,
			&m.ExpiresAt, &m.PromotionStatus, &m.PromotedTo, &m.SourceRef, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &m)
	}
	return results, rows.Err()
}

// ListMemoriesByScope returns active memories for a scope, ordered by created_at DESC.
func ListMemoriesByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, limit, offset int) ([]*Memory, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+memoryColumns+`
		 FROM memories WHERE scope_id=$1 AND is_active=true
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		scopeID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list memories by scope: %w", err)
	}
	defer rows.Close()
	return scanMemoryRows(rows)
}

// GetMemory retrieves a memory by ID. Returns nil, nil if not found.
func GetMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Memory, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+memoryColumns+` FROM memories WHERE id = $1`, id,
	)
	m, err := scanMemory(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get memory: %w", err)
	}
	return m, nil
}

// CreateMemory inserts a new memory record and returns the created record.
func CreateMemory(ctx context.Context, pool *pgxpool.Pool, m *Memory) (*Memory, error) {
	if m.Meta == nil {
		m.Meta = []byte("{}")
	}
	if m.ContentKind == "" {
		m.ContentKind = "text"
	}
	if m.PromotionStatus == "" {
		m.PromotionStatus = "none"
	}

	var embVal, embCodeVal interface{}
	if len(m.Embedding) > 0 {
		embVal = float32SliceToVector(m.Embedding)
	}
	if len(m.EmbeddingCode) > 0 {
		embCodeVal = float32SliceToVector(m.EmbeddingCode)
	}

	row := pool.QueryRow(ctx,
		`INSERT INTO memories
		 (memory_type, scope_id, author_id, content, summary,
		  embedding, embedding_model_id, embedding_code, embedding_code_model_id,
		  content_kind, meta, version, is_active, confidence, importance,
		  access_count, expires_at, promotion_status, promoted_to, source_ref)
		 VALUES ($1,$2,$3,$4,$5,$6::vector,$7,$8::vector,$9,$10,$11,
		         COALESCE($12,1),true,COALESCE($13,1.0),COALESCE($14,0.5),
		         0,$15,$16,$17,$18)
		 RETURNING `+memoryColumns,
		m.MemoryType, m.ScopeID, m.AuthorID, m.Content, m.Summary,
		embVal, m.EmbeddingModelID, embCodeVal, m.EmbeddingCodeModelID,
		m.ContentKind, m.Meta, m.Version, m.Confidence, m.Importance,
		m.ExpiresAt, m.PromotionStatus, m.PromotedTo, m.SourceRef,
	)
	created, err := scanMemory(row)
	if err != nil {
		return nil, fmt.Errorf("db: create memory: %w", err)
	}
	return created, nil
}

// UpdateMemoryContent updates content, embeddings, and bumps version.
func UpdateMemoryContent(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*Memory, error) {
	var embVal, embCodeVal interface{}
	if len(embedding) > 0 {
		embVal = float32SliceToVector(embedding)
	}
	if len(embeddingCode) > 0 {
		embCodeVal = float32SliceToVector(embeddingCode)
	}
	row := pool.QueryRow(ctx,
		`UPDATE memories
		 SET content=$2, embedding=$3::vector, embedding_model_id=$4,
		     embedding_code=$5::vector, embedding_code_model_id=$6,
		     content_kind=$7, version=version+1, updated_at=now()
		 WHERE id=$1
		 RETURNING `+memoryColumns,
		id, content, embVal, textModelID, embCodeVal, codeModelID, contentKind,
	)
	m, err := scanMemory(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: update memory content: %w", err)
	}
	return m, nil
}

// SoftDeleteMemory marks a memory as inactive.
func SoftDeleteMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE memories SET is_active=false, updated_at=now() WHERE id=$1`, id,
	)
	return err
}

// HardDeleteMemory permanently deletes a memory row.
func HardDeleteMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `DELETE FROM memories WHERE id=$1`, id)
	return err
}

// IncrementMemoryAccess increments access_count and sets last_accessed = now().
func IncrementMemoryAccess(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE memories SET access_count=access_count+1, last_accessed=now(), updated_at=now() WHERE id=$1`,
		id,
	)
	return err
}

// FindNearDuplicates finds active memories in the same scope with cosine distance <= threshold.
func FindNearDuplicates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*Memory, error) {
	vecStr := float32SliceToVector(embedding)
	rows, err := pool.Query(ctx,
		`SELECT `+memoryColumns+`
		 FROM memories
		 WHERE scope_id = $1
		   AND is_active = true
		   AND (embedding <=> $2::vector) <= $3
		   AND ($4::uuid IS NULL OR id != $4)
		 LIMIT 5`,
		scopeID, vecStr, threshold, excludeID,
	)
	if err != nil {
		return nil, fmt.Errorf("db: find near duplicates: %w", err)
	}
	defer rows.Close()
	return scanMemoryRows(rows)
}

// MemoryScore pairs a memory with its retrieval scores.
type MemoryScore struct {
	Memory    *Memory
	VecScore  float64
	BM25Score float64
}

// RecallMemoriesByVector performs ANN search with scope filter.
func RecallMemoriesByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]MemoryScore, error) {
	vecStr := float32SliceToVector(queryVec)
	rows, err := pool.Query(ctx,
		`SELECT `+memoryColumns+`, 1 - (embedding <=> $3::vector) AS vec_score
		 FROM memories
		 WHERE is_active = true AND scope_id = ANY($1)
		 ORDER BY embedding <=> $3::vector
		 LIMIT $2`,
		scopeIDs, limit, vecStr,
	)
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by vector: %w", err)
	}
	defer rows.Close()
	return scanMemoryScoreRows(rows, false)
}

// RecallMemoriesByCodeVector performs ANN on embedding_code.
func RecallMemoriesByCodeVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]MemoryScore, error) {
	vecStr := float32SliceToVector(queryVec)
	rows, err := pool.Query(ctx,
		`SELECT `+memoryColumns+`, 1 - (embedding_code <=> $3::vector) AS vec_score
		 FROM memories
		 WHERE is_active = true AND scope_id = ANY($1) AND embedding_code IS NOT NULL
		 ORDER BY embedding_code <=> $3::vector
		 LIMIT $2`,
		scopeIDs, limit, vecStr,
	)
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by code vector: %w", err)
	}
	defer rows.Close()
	return scanMemoryScoreRows(rows, false)
}

// RecallMemoriesByFTS performs BM25 full-text search via ts_rank_cd.
func RecallMemoriesByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int) ([]MemoryScore, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+memoryColumns+`,
		        ts_rank_cd(to_tsvector('postbrain_fts', content), plainto_tsquery('postbrain_fts', $3)) AS bm25_score
		 FROM memories
		 WHERE is_active = true AND scope_id = ANY($1)
		   AND to_tsvector('postbrain_fts', content) @@ plainto_tsquery('postbrain_fts', $3)
		 ORDER BY bm25_score DESC
		 LIMIT $2`,
		scopeIDs, limit, query,
	)
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by fts: %w", err)
	}
	defer rows.Close()
	return scanMemoryScoreRows(rows, true)
}

// scanMemoryScoreRows scans rows that include a trailing score column.
// isBM25 determines whether to populate VecScore or BM25Score.
func scanMemoryScoreRows(rows pgx.Rows, isBM25 bool) ([]MemoryScore, error) {
	var results []MemoryScore
	for rows.Next() {
		var m Memory
		var embText, embCodeText *string
		var score float64
		if err := rows.Scan(
			&m.ID, &m.MemoryType, &m.ScopeID, &m.AuthorID,
			&m.Content, &m.Summary, &embText, &m.EmbeddingModelID,
			&embCodeText, &m.EmbeddingCodeModelID, &m.ContentKind, &m.Meta,
			&m.Version, &m.IsActive, &m.Confidence, &m.Importance, &m.AccessCount, &m.LastAccessed,
			&m.ExpiresAt, &m.PromotionStatus, &m.PromotedTo, &m.SourceRef, &m.CreatedAt, &m.UpdatedAt,
			&score,
		); err != nil {
			return nil, err
		}
		ms := MemoryScore{Memory: &m}
		if isBM25 {
			ms.BM25Score = score
		} else {
			ms.VecScore = score
		}
		results = append(results, ms)
	}
	return results, rows.Err()
}

// CreateConsolidation inserts a consolidation record.
func CreateConsolidation(ctx context.Context, pool *pgxpool.Pool, c *Consolidation) (*Consolidation, error) {
	var result Consolidation
	err := pool.QueryRow(ctx,
		`INSERT INTO consolidations (scope_id, source_ids, result_id, strategy, reason)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, scope_id, source_ids, result_id, strategy, reason, created_at`,
		c.ScopeID, c.SourceIDs, c.ResultID, c.Strategy, c.Reason,
	).Scan(
		&result.ID, &result.ScopeID, &result.SourceIDs, &result.ResultID,
		&result.Strategy, &result.Reason, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("db: create consolidation: %w", err)
	}
	return &result, nil
}

// ListConsolidationCandidates returns low-importance, low-access memories for a scope.
func ListConsolidationCandidates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*Memory, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+memoryColumns+`
		 FROM memories
		 WHERE is_active = true AND scope_id = $1 AND importance < 0.7 AND access_count < 3`,
		scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list consolidation candidates: %w", err)
	}
	defer rows.Close()
	return scanMemoryRows(rows)
}

// ── Entities ──────────────────────────────────────────────────────────────────

// UpsertEntity inserts or updates an entity by (scope_id, entity_type, canonical).
func UpsertEntity(ctx context.Context, pool *pgxpool.Pool, e *Entity) (*Entity, error) {
	if e.Meta == nil {
		e.Meta = []byte("{}")
	}
	var embVal interface{}
	if len(e.Embedding) > 0 {
		embVal = float32SliceToVector(e.Embedding)
	}
	var result Entity
	var embText *string
	err := pool.QueryRow(ctx,
		`INSERT INTO entities (scope_id, entity_type, name, canonical, meta, embedding, embedding_model_id)
		 VALUES ($1,$2,$3,$4,$5,$6::vector,$7)
		 ON CONFLICT (scope_id, entity_type, canonical)
		 DO UPDATE SET name=EXCLUDED.name, meta=EXCLUDED.meta,
		               embedding=EXCLUDED.embedding, embedding_model_id=EXCLUDED.embedding_model_id,
		               updated_at=now()
		 RETURNING id, scope_id, entity_type, name, canonical, meta,
		           embedding::text, embedding_model_id, created_at, updated_at`,
		e.ScopeID, e.EntityType, e.Name, e.Canonical, e.Meta, embVal, e.EmbeddingModelID,
	).Scan(
		&result.ID, &result.ScopeID, &result.EntityType, &result.Name, &result.Canonical, &result.Meta,
		&embText, &result.EmbeddingModelID, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("db: upsert entity: %w", err)
	}
	return &result, nil
}

// GetEntityByCanonical looks up an entity by scope, type, and canonical name.
func GetEntityByCanonical(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, entityType, canonical string) (*Entity, error) {
	var result Entity
	var embText *string
	err := pool.QueryRow(ctx,
		`SELECT id, scope_id, entity_type, name, canonical, meta,
		        embedding::text, embedding_model_id, created_at, updated_at
		 FROM entities WHERE scope_id=$1 AND entity_type=$2 AND canonical=$3`,
		scopeID, entityType, canonical,
	).Scan(
		&result.ID, &result.ScopeID, &result.EntityType, &result.Name, &result.Canonical, &result.Meta,
		&embText, &result.EmbeddingModelID, &result.CreatedAt, &result.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get entity by canonical: %w", err)
	}
	return &result, nil
}

// LinkMemoryToEntity inserts a memory_entities row.
func LinkMemoryToEntity(ctx context.Context, pool *pgxpool.Pool, memoryID, entityID uuid.UUID, role string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO memory_entities (memory_id, entity_id, role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		memoryID, entityID, role,
	)
	if err != nil {
		return fmt.Errorf("db: link memory to entity: %w", err)
	}
	return nil
}

// ── Relations ─────────────────────────────────────────────────────────────────

// UpsertRelation inserts or updates a relation between two entities.
func UpsertRelation(ctx context.Context, pool *pgxpool.Pool, r *Relation) (*Relation, error) {
	var result Relation
	err := pool.QueryRow(ctx,
		`INSERT INTO relations (scope_id, subject_id, predicate, object_id, confidence, source_memory)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (scope_id, subject_id, predicate, object_id)
		 DO UPDATE SET confidence=EXCLUDED.confidence, source_memory=EXCLUDED.source_memory
		 RETURNING id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at`,
		r.ScopeID, r.SubjectID, r.Predicate, r.ObjectID, r.Confidence, r.SourceMemory,
	).Scan(
		&result.ID, &result.ScopeID, &result.SubjectID, &result.Predicate,
		&result.ObjectID, &result.Confidence, &result.SourceMemory, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("db: upsert relation: %w", err)
	}
	return &result, nil
}

// ListRelationsForEntity returns all relations where the entity is subject or object.
// If predicate is non-empty, it filters by that predicate.
func ListRelationsForEntity(ctx context.Context, pool *pgxpool.Pool, entityID uuid.UUID, predicate string) ([]*Relation, error) {
	var rows pgx.Rows
	var err error
	if predicate != "" {
		rows, err = pool.Query(ctx,
			`SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at
			 FROM relations WHERE (subject_id=$1 OR object_id=$1) AND predicate=$2
			 ORDER BY created_at`,
			entityID, predicate,
		)
	} else {
		rows, err = pool.Query(ctx,
			`SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at
			 FROM relations WHERE subject_id=$1 OR object_id=$1
			 ORDER BY created_at`,
			entityID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("db: list relations for entity: %w", err)
	}
	defer rows.Close()

	var results []*Relation
	for rows.Next() {
		var rel Relation
		if err := rows.Scan(
			&rel.ID, &rel.ScopeID, &rel.SubjectID, &rel.Predicate,
			&rel.ObjectID, &rel.Confidence, &rel.SourceMemory, &rel.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &rel)
	}
	return results, rows.Err()
}

// ── Knowledge artifacts ───────────────────────────────────────────────────────

// artifactColumns is the canonical SELECT column list for knowledge_artifacts.
const artifactColumns = `id, knowledge_type, owner_scope_id, author_id,
	visibility, status, published_at, deprecated_at, review_required,
	title, content, summary, embedding::text, embedding_model_id, meta,
	endorsement_count, access_count, last_accessed,
	version, previous_version, source_memory_id, source_ref,
	created_at, updated_at`

// scanArtifact scans one knowledge_artifacts row into a KnowledgeArtifact.
// The embedding column is returned as its text representation and is not parsed.
func scanArtifact(row pgx.Row) (*KnowledgeArtifact, error) {
	var a KnowledgeArtifact
	var embeddingText *string
	err := row.Scan(
		&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
		&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt, &a.ReviewRequired,
		&a.Title, &a.Content, &a.Summary, &embeddingText, &a.EmbeddingModelID, &a.Meta,
		&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
		&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// scanArtifactRows scans multiple knowledge_artifacts rows.
func scanArtifactRows(rows pgx.Rows) ([]*KnowledgeArtifact, error) {
	var results []*KnowledgeArtifact
	for rows.Next() {
		var a KnowledgeArtifact
		var embeddingText *string
		if err := rows.Scan(
			&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
			&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt, &a.ReviewRequired,
			&a.Title, &a.Content, &a.Summary, &embeddingText, &a.EmbeddingModelID, &a.Meta,
			&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
			&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &a)
	}
	return results, rows.Err()
}

// CreateArtifact inserts a new knowledge artifact and returns the created record.
func CreateArtifact(ctx context.Context, pool *pgxpool.Pool, a *KnowledgeArtifact) (*KnowledgeArtifact, error) {
	var embedding interface{}
	if len(a.Embedding) > 0 {
		embedding = float32SliceToVector(a.Embedding)
	}
	if a.Meta == nil {
		a.Meta = []byte("{}")
	}
	row := pool.QueryRow(ctx,
		`INSERT INTO knowledge_artifacts
		 (knowledge_type, owner_scope_id, author_id, visibility, status,
		  published_at, deprecated_at, review_required,
		  title, content, summary, embedding, embedding_model_id, meta,
		  version, previous_version, source_memory_id, source_ref)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::vector,$13,$14,$15,$16,$17,$18)
		 RETURNING `+artifactColumns,
		a.KnowledgeType, a.OwnerScopeID, a.AuthorID, a.Visibility, a.Status,
		a.PublishedAt, a.DeprecatedAt, a.ReviewRequired,
		a.Title, a.Content, a.Summary, embedding, a.EmbeddingModelID, a.Meta,
		a.Version, a.PreviousVersion, a.SourceMemoryID, a.SourceRef,
	)
	result, err := scanArtifact(row)
	if err != nil {
		return nil, fmt.Errorf("db: create artifact: %w", err)
	}
	return result, nil
}

// GetArtifact retrieves a knowledge artifact by ID. Returns nil, nil if not found.
func GetArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*KnowledgeArtifact, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+artifactColumns+` FROM knowledge_artifacts WHERE id = $1`, id,
	)
	a, err := scanArtifact(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get artifact: %w", err)
	}
	return a, nil
}

// UpdateArtifact updates title, content, summary, embedding, and bumps the version.
func UpdateArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, title, content string, summary *string, embedding []float32, modelID *uuid.UUID) (*KnowledgeArtifact, error) {
	var embeddingVal interface{}
	if len(embedding) > 0 {
		embeddingVal = float32SliceToVector(embedding)
	}
	row := pool.QueryRow(ctx,
		`UPDATE knowledge_artifacts
		 SET title=$2, content=$3, summary=$4, embedding=$5::vector,
		     embedding_model_id=$6, version=version+1, updated_at=now()
		 WHERE id=$1
		 RETURNING `+artifactColumns,
		id, title, content, summary, embeddingVal, modelID,
	)
	a, err := scanArtifact(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: update artifact: %w", err)
	}
	return a, nil
}

// UpdateArtifactStatus updates status, published_at, and deprecated_at for an artifact.
func UpdateArtifactStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, publishedAt, deprecatedAt *time.Time) error {
	_, err := pool.Exec(ctx,
		`UPDATE knowledge_artifacts
		 SET status=$2, published_at=$3, deprecated_at=$4, updated_at=now()
		 WHERE id=$1`,
		id, status, publishedAt, deprecatedAt,
	)
	return err
}

// IncrementArtifactEndorsementCount increments the denormalized endorsement_count by 1.
func IncrementArtifactEndorsementCount(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE knowledge_artifacts SET endorsement_count=endorsement_count+1, updated_at=now() WHERE id=$1`,
		id,
	)
	return err
}

// IncrementArtifactAccess increments access_count and sets last_accessed = now().
func IncrementArtifactAccess(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE knowledge_artifacts SET access_count=access_count+1, last_accessed=now(), updated_at=now() WHERE id=$1`,
		id,
	)
	return err
}

// SnapshotArtifactVersion inserts a knowledge_history row.
func SnapshotArtifactVersion(ctx context.Context, pool *pgxpool.Pool, h *KnowledgeHistory) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO knowledge_history (artifact_id, version, content, summary, changed_by, change_note)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		h.ArtifactID, h.Version, h.Content, h.Summary, h.ChangedBy, h.ChangeNote,
	)
	return err
}

// ── Endorsements ─────────────────────────────────────────────────────────────

// CreateEndorsement inserts a knowledge_endorsements row and returns the created record.
func CreateEndorsement(ctx context.Context, pool *pgxpool.Pool, artifactID, endorserID uuid.UUID, note *string) (*KnowledgeEndorsement, error) {
	var e KnowledgeEndorsement
	err := pool.QueryRow(ctx,
		`INSERT INTO knowledge_endorsements (artifact_id, endorser_id, note)
		 VALUES ($1,$2,$3)
		 RETURNING id, artifact_id, endorser_id, note, created_at`,
		artifactID, endorserID, note,
	).Scan(&e.ID, &e.ArtifactID, &e.EndorserID, &e.Note, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetEndorsementByEndorser finds an endorsement by (artifact, endorser) pair.
// Returns nil, nil if not found.
func GetEndorsementByEndorser(ctx context.Context, pool *pgxpool.Pool, artifactID, endorserID uuid.UUID) (*KnowledgeEndorsement, error) {
	var e KnowledgeEndorsement
	err := pool.QueryRow(ctx,
		`SELECT id, artifact_id, endorser_id, note, created_at
		 FROM knowledge_endorsements WHERE artifact_id=$1 AND endorser_id=$2`,
		artifactID, endorserID,
	).Scan(&e.ID, &e.ArtifactID, &e.EndorserID, &e.Note, &e.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ── Visibility / listing ──────────────────────────────────────────────────────

// ListVisibleArtifacts returns published artifacts visible to the given scope IDs.
// For MVP: filters by owner_scope_id = ANY(callerScopeIDs) OR visibility = 'company'.
// Full ltree visibility query goes in knowledge/visibility.go.
func ListVisibleArtifacts(ctx context.Context, pool *pgxpool.Pool, callerScopeIDs []uuid.UUID, limit, offset int) ([]*KnowledgeArtifact, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+artifactColumns+`
		 FROM knowledge_artifacts
		 WHERE status = 'published'
		   AND (owner_scope_id = ANY($1) OR visibility = 'company')
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		callerScopeIDs, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactRows(rows)
}

// ── Retrieval ─────────────────────────────────────────────────────────────────

// ArtifactScore pairs a knowledge artifact with its retrieval scores.
type ArtifactScore struct {
	Artifact  *KnowledgeArtifact
	VecScore  float64
	BM25Score float64
}

// RecallArtifactsByVector retrieves published artifacts ordered by vector similarity.
func RecallArtifactsByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]ArtifactScore, error) {
	vecStr := float32SliceToVector(queryVec)
	rows, err := pool.Query(ctx,
		`SELECT `+artifactColumns+`, 1 - (embedding <=> $3::vector) AS vec_score
		 FROM knowledge_artifacts ka
		 WHERE ka.status = 'published' AND ka.owner_scope_id = ANY($1)
		 ORDER BY ka.embedding <=> $3::vector
		 LIMIT $2`,
		scopeIDs, limit, vecStr,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactScoreRows(rows)
}

// RecallArtifactsByFTS retrieves published artifacts via full-text search.
func RecallArtifactsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int) ([]ArtifactScore, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+artifactColumns+`,
		        ts_rank_cd(to_tsvector('postbrain_fts', content),
		                   plainto_tsquery('postbrain_fts', $3)) AS bm25_score
		 FROM knowledge_artifacts
		 WHERE status = 'published'
		   AND owner_scope_id = ANY($1)
		   AND to_tsvector('postbrain_fts', content) @@ plainto_tsquery('postbrain_fts', $3)
		 ORDER BY bm25_score DESC
		 LIMIT $2`,
		scopeIDs, limit, query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactFTSRows(rows)
}

// scanArtifactScoreRows scans rows that include a trailing vec_score column.
func scanArtifactScoreRows(rows pgx.Rows) ([]ArtifactScore, error) {
	var results []ArtifactScore
	for rows.Next() {
		var a KnowledgeArtifact
		var embeddingText *string
		var vecScore float64
		if err := rows.Scan(
			&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
			&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt, &a.ReviewRequired,
			&a.Title, &a.Content, &a.Summary, &embeddingText, &a.EmbeddingModelID, &a.Meta,
			&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
			&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
			&a.CreatedAt, &a.UpdatedAt,
			&vecScore,
		); err != nil {
			return nil, err
		}
		results = append(results, ArtifactScore{Artifact: &a, VecScore: vecScore})
	}
	return results, rows.Err()
}

// scanArtifactFTSRows scans rows that include a trailing bm25_score column.
func scanArtifactFTSRows(rows pgx.Rows) ([]ArtifactScore, error) {
	var results []ArtifactScore
	for rows.Next() {
		var a KnowledgeArtifact
		var embeddingText *string
		var bm25Score float64
		if err := rows.Scan(
			&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
			&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt, &a.ReviewRequired,
			&a.Title, &a.Content, &a.Summary, &embeddingText, &a.EmbeddingModelID, &a.Meta,
			&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
			&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
			&a.CreatedAt, &a.UpdatedAt,
			&bm25Score,
		); err != nil {
			return nil, err
		}
		results = append(results, ArtifactScore{Artifact: &a, BM25Score: bm25Score})
	}
	return results, rows.Err()
}

// ── Collections ───────────────────────────────────────────────────────────────

const collectionColumns = `id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at`

func scanCollection(row pgx.Row) (*KnowledgeCollection, error) {
	var c KnowledgeCollection
	err := row.Scan(
		&c.ID, &c.ScopeID, &c.OwnerID, &c.Slug, &c.Name, &c.Description,
		&c.Visibility, &c.Meta, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateCollection inserts a new knowledge collection and returns the created record.
func CreateCollection(ctx context.Context, pool *pgxpool.Pool, c *KnowledgeCollection) (*KnowledgeCollection, error) {
	if c.Meta == nil {
		c.Meta = []byte("{}")
	}
	row := pool.QueryRow(ctx,
		`INSERT INTO knowledge_collections (scope_id, owner_id, slug, name, description, visibility, meta)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING `+collectionColumns,
		c.ScopeID, c.OwnerID, c.Slug, c.Name, c.Description, c.Visibility, c.Meta,
	)
	result, err := scanCollection(row)
	if err != nil {
		return nil, fmt.Errorf("db: create collection: %w", err)
	}
	return result, nil
}

// GetCollection retrieves a knowledge collection by ID. Returns nil, nil if not found.
func GetCollection(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*KnowledgeCollection, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+collectionColumns+` FROM knowledge_collections WHERE id=$1`, id,
	)
	c, err := scanCollection(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get collection: %w", err)
	}
	return c, nil
}

// GetCollectionBySlug retrieves a knowledge collection by scope + slug. Returns nil, nil if not found.
func GetCollectionBySlug(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, slug string) (*KnowledgeCollection, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+collectionColumns+` FROM knowledge_collections WHERE scope_id=$1 AND slug=$2`,
		scopeID, slug,
	)
	c, err := scanCollection(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get collection by slug: %w", err)
	}
	return c, nil
}

// ListCollections returns all collections for a given scope.
func ListCollections(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*KnowledgeCollection, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+collectionColumns+` FROM knowledge_collections WHERE scope_id=$1 ORDER BY name`,
		scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cs []*KnowledgeCollection
	for rows.Next() {
		var c KnowledgeCollection
		if err := rows.Scan(
			&c.ID, &c.ScopeID, &c.OwnerID, &c.Slug, &c.Name, &c.Description,
			&c.Visibility, &c.Meta, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		cs = append(cs, &c)
	}
	return cs, rows.Err()
}

// AddCollectionItem inserts a knowledge_collection_items row.
func AddCollectionItem(ctx context.Context, pool *pgxpool.Pool, collectionID, artifactID, addedBy uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO knowledge_collection_items (collection_id, artifact_id, added_by)
		 VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		collectionID, artifactID, addedBy,
	)
	return err
}

// RemoveCollectionItem deletes a knowledge_collection_items row.
func RemoveCollectionItem(ctx context.Context, pool *pgxpool.Pool, collectionID, artifactID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`DELETE FROM knowledge_collection_items WHERE collection_id=$1 AND artifact_id=$2`,
		collectionID, artifactID,
	)
	return err
}

// ListCollectionItems returns the artifacts in a collection, ordered by position.
func ListCollectionItems(ctx context.Context, pool *pgxpool.Pool, collectionID uuid.UUID) ([]*KnowledgeArtifact, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+artifactColumns+`
		 FROM knowledge_artifacts ka
		 JOIN knowledge_collection_items kci ON kci.artifact_id = ka.id
		 WHERE kci.collection_id = $1
		 ORDER BY kci.position, kci.added_at`,
		collectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactRows(rows)
}

// ── Staleness flags ───────────────────────────────────────────────────────────

// InsertStalenessFlag inserts a staleness_flags row and returns the created record.
func InsertStalenessFlag(ctx context.Context, pool *pgxpool.Pool, f *StalenessFlag) (*StalenessFlag, error) {
	if f.Evidence == nil {
		f.Evidence = []byte("{}")
	}
	var result StalenessFlag
	err := pool.QueryRow(ctx,
		`INSERT INTO staleness_flags (artifact_id, signal, confidence, evidence, status)
		 VALUES ($1,$2,$3,$4,COALESCE($5,'open'))
		 RETURNING id, artifact_id, signal, confidence, evidence, status, flagged_at,
		           reviewed_by, reviewed_at, review_note`,
		f.ArtifactID, f.Signal, f.Confidence, f.Evidence, f.Status,
	).Scan(
		&result.ID, &result.ArtifactID, &result.Signal, &result.Confidence,
		&result.Evidence, &result.Status, &result.FlaggedAt,
		&result.ReviewedBy, &result.ReviewedAt, &result.ReviewNote,
	)
	if err != nil {
		return nil, fmt.Errorf("db: insert staleness flag: %w", err)
	}
	return &result, nil
}

// HasOpenStalenessFlag reports whether an artifact has an open staleness flag for the given signal.
func HasOpenStalenessFlag(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID, signal string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(
		     SELECT 1 FROM staleness_flags
		     WHERE artifact_id=$1 AND signal=$2 AND status='open'
		 )`,
		artifactID, signal,
	).Scan(&exists)
	return exists, err
}

// UpdateStalenessFlag updates the status and review fields of a staleness flag.
func UpdateStalenessFlag(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, reviewedBy *uuid.UUID, note *string) error {
	_, err := pool.Exec(ctx,
		`UPDATE staleness_flags
		 SET status=$2, reviewed_by=$3, review_note=$4, reviewed_at=now()
		 WHERE id=$1`,
		id, status, reviewedBy, note,
	)
	return err
}

// ── Promotion requests ────────────────────────────────────────────────────────

const promotionColumns = `id, memory_id, requested_by, target_scope_id, target_visibility,
	proposed_title, proposed_collection_id, status, reviewer_id, review_note,
	reviewed_at, result_artifact_id, created_at`

func scanPromotion(row pgx.Row) (*PromotionRequest, error) {
	var p PromotionRequest
	err := row.Scan(
		&p.ID, &p.MemoryID, &p.RequestedBy, &p.TargetScopeID, &p.TargetVisibility,
		&p.ProposedTitle, &p.ProposedCollectionID, &p.Status, &p.ReviewerID, &p.ReviewNote,
		&p.ReviewedAt, &p.ResultArtifactID, &p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePromotionRequest inserts a new promotion_requests row and returns it.
func CreatePromotionRequest(ctx context.Context, pool *pgxpool.Pool, req *PromotionRequest) (*PromotionRequest, error) {
	row := pool.QueryRow(ctx,
		`INSERT INTO promotion_requests
		 (memory_id, requested_by, target_scope_id, target_visibility,
		  proposed_title, proposed_collection_id)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING `+promotionColumns,
		req.MemoryID, req.RequestedBy, req.TargetScopeID, req.TargetVisibility,
		req.ProposedTitle, req.ProposedCollectionID,
	)
	result, err := scanPromotion(row)
	if err != nil {
		return nil, fmt.Errorf("db: create promotion request: %w", err)
	}
	return result, nil
}

// GetPromotionRequest retrieves a promotion request by ID. Returns nil, nil if not found.
func GetPromotionRequest(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*PromotionRequest, error) {
	row := pool.QueryRow(ctx,
		`SELECT `+promotionColumns+` FROM promotion_requests WHERE id=$1`, id,
	)
	p, err := scanPromotion(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get promotion request: %w", err)
	}
	return p, nil
}

// ListPendingPromotions returns pending promotion requests for a target scope.
func ListPendingPromotions(ctx context.Context, pool *pgxpool.Pool, targetScopeID uuid.UUID) ([]*PromotionRequest, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+promotionColumns+`
		 FROM promotion_requests
		 WHERE status='pending' AND target_scope_id=$1
		 ORDER BY created_at`,
		targetScopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*PromotionRequest
	for rows.Next() {
		var p PromotionRequest
		if err := rows.Scan(
			&p.ID, &p.MemoryID, &p.RequestedBy, &p.TargetScopeID, &p.TargetVisibility,
			&p.ProposedTitle, &p.ProposedCollectionID, &p.Status, &p.ReviewerID, &p.ReviewNote,
			&p.ReviewedAt, &p.ResultArtifactID, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &p)
	}
	return results, rows.Err()
}

// ── Entity/Relation scope queries ─────────────────────────────────────────────

// ListEntitiesByScope returns entities in a scope, optionally filtered by type.
func ListEntitiesByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, entityType string, limit, offset int) ([]*Entity, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, scope_id, entity_type, name, canonical, meta,
		        embedding::text, embedding_model_id, created_at, updated_at
		 FROM entities
		 WHERE scope_id=$1 AND ($2='' OR entity_type=$2)
		 ORDER BY name
		 LIMIT $3 OFFSET $4`,
		scopeID, entityType, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list entities by scope: %w", err)
	}
	defer rows.Close()

	var results []*Entity
	for rows.Next() {
		var e Entity
		var embText *string
		if err := rows.Scan(
			&e.ID, &e.ScopeID, &e.EntityType, &e.Name, &e.Canonical, &e.Meta,
			&embText, &e.EmbeddingModelID, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &e)
	}
	return results, rows.Err()
}

// ListRelationsByScope returns all relations in a scope.
func ListRelationsByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, limit, offset int) ([]*Relation, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, scope_id, subject_id, predicate, object_id, confidence, source_memory, created_at
		 FROM relations
		 WHERE scope_id=$1
		 ORDER BY created_at
		 LIMIT $2 OFFSET $3`,
		scopeID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list relations by scope: %w", err)
	}
	defer rows.Close()

	var results []*Relation
	for rows.Next() {
		var r Relation
		if err := rows.Scan(
			&r.ID, &r.ScopeID, &r.SubjectID, &r.Predicate,
			&r.ObjectID, &r.Confidence, &r.SourceMemory, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &r)
	}
	return results, rows.Err()
}

// ListStalenessFlags returns staleness flags optionally filtered by status.
func ListStalenessFlags(ctx context.Context, pool *pgxpool.Pool, status string, limit, offset int) ([]*StalenessFlag, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, artifact_id, signal, confidence, evidence, status, flagged_at,
		        reviewed_by, reviewed_at, review_note
		 FROM staleness_flags
		 WHERE ($1='' OR status=$1)
		 ORDER BY flagged_at DESC
		 LIMIT $2 OFFSET $3`,
		status, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list staleness flags: %w", err)
	}
	defer rows.Close()

	var results []*StalenessFlag
	for rows.Next() {
		var f StalenessFlag
		if err := rows.Scan(
			&f.ID, &f.ArtifactID, &f.Signal, &f.Confidence, &f.Evidence,
			&f.Status, &f.FlaggedAt, &f.ReviewedBy, &f.ReviewedAt, &f.ReviewNote,
		); err != nil {
			return nil, err
		}
		results = append(results, &f)
	}
	return results, rows.Err()
}

// ListPrincipals returns principals ordered by creation time.
func ListPrincipals(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]*Principal, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, kind, slug, display_name, meta, created_at, updated_at
		 FROM principals
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list principals: %w", err)
	}
	defer rows.Close()

	var results []*Principal
	for rows.Next() {
		var p Principal
		if err := rows.Scan(
			&p.ID, &p.Kind, &p.Slug, &p.DisplayName, &p.Meta, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &p)
	}
	return results, rows.Err()
}

// ExportFloat32SliceToVector formats a []float32 as a pg_vector literal string.
// It is exported so other packages can format vector literals for raw SQL queries.
func ExportFloat32SliceToVector(v []float32) string { return float32SliceToVector(v) }

// float32SliceToVector formats a []float32 as a pg_vector literal string, e.g. "[1,2,3]".
func float32SliceToVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	b := make([]byte, 0, len(v)*8+2)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = fmt.Appendf(b, "%g", f)
	}
	b = append(b, ']')
	return string(b)
}

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

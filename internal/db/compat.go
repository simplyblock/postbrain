// Package db provides database access via sqlc-generated code.
// This file exposes the legacy free-function API that existing callers depend on.
// Each function creates a Queries value from the pool and delegates to the
// generated method. Callers should migrate to the Queries methods over time.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// SkillParameter is the in-memory representation of one parameter descriptor.
// Not stored in a DB column directly; serialised to/from JSONB.
type SkillParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // string | integer | boolean | enum
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"` // for enum type
}

// Membership is an alias for PrincipalMembership for backward compatibility.
type Membership = PrincipalMembership

// MembershipRow is a denormalised membership row with principal display names.
type MembershipRow struct {
	MemberID          uuid.UUID
	MemberSlug        string
	MemberDisplayName string
	ParentID          uuid.UUID
	ParentSlug        string
	ParentDisplayName string
	Role              string
	CreatedAt         time.Time
}

// MemoryScore pairs a memory with its retrieval scores.
type MemoryScore struct {
	Memory    *Memory
	VecScore  float64
	BM25Score float64
}

// ArtifactScore pairs a knowledge artifact with its retrieval scores.
type ArtifactScore struct {
	Artifact  *KnowledgeArtifact
	VecScore  float64
	BM25Score float64
}

// SkillScore pairs a skill with its retrieval score.
type SkillScore struct {
	Skill *Skill
	Score float64
}

// ExportFloat32SliceToVector formats a []float32 as a pg_vector literal string.
// Kept for backward compatibility with callers that build raw SQL.
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

// ── Principals ────────────────────────────────────────────────────────────────

// CreatePrincipal inserts a new principal row and returns the created record.
func CreatePrincipal(ctx context.Context, pool *pgxpool.Pool, kind, slug, displayName string, meta []byte) (*Principal, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := New(pool)
	p, err := q.CreatePrincipal(ctx, CreatePrincipalParams{
		Kind:        kind,
		Slug:        slug,
		DisplayName: displayName,
		Meta:        meta,
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetPrincipalByID looks up a principal by its UUID. Returns nil, nil if not found.
func GetPrincipalByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Principal, error) {
	q := New(pool)
	p, err := q.GetPrincipalByID(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetPrincipalBySlug looks up a principal by slug. Returns nil, nil if not found.
func GetPrincipalBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (*Principal, error) {
	q := New(pool)
	p, err := q.GetPrincipalBySlug(ctx, slug)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ── Memberships ───────────────────────────────────────────────────────────────

// CreateMembership inserts a membership record.
// grantedBy may be nil; the column is nullable in the schema.
func CreateMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID, role string, grantedBy *uuid.UUID) (*Membership, error) {
	const q = `
INSERT INTO principal_memberships (member_id, parent_id, role, granted_by)
VALUES ($1, $2, $3, NULLIF($4, '00000000-0000-0000-0000-000000000000'::uuid))
RETURNING member_id, parent_id, role, granted_by, created_at`
	var gb uuid.UUID
	if grantedBy != nil {
		gb = *grantedBy
	}
	row := pool.QueryRow(ctx, q, memberID, parentID, role, gb)
	var m Membership
	if err := row.Scan(&m.MemberID, &m.ParentID, &m.Role, &m.GrantedBy, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteMembership removes a direct membership.
func DeleteMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID) error {
	q := New(pool)
	return q.DeleteMembership(ctx, DeleteMembershipParams{
		MemberID: memberID,
		ParentID: parentID,
	})
}

// GetMemberships returns direct parent memberships for a principal.
func GetMemberships(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]*Membership, error) {
	q := New(pool)
	return q.GetMemberships(ctx, memberID)
}

// GetAllParentIDs returns all ancestor principal IDs via recursive CTE.
func GetAllParentIDs(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]uuid.UUID, error) {
	q := New(pool)
	return q.GetAllParentIDs(ctx, memberID)
}

// ListAllMemberships returns all memberships with member and parent display names.
func ListAllMemberships(ctx context.Context, pool *pgxpool.Pool) ([]*MembershipRow, error) {
	const query = `
SELECT pm.member_id, mp.slug, mp.display_name,
       pm.parent_id, pp.slug, pp.display_name,
       pm.role, pm.created_at
FROM principal_memberships pm
JOIN principals mp ON mp.id = pm.member_id
JOIN principals pp ON pp.id = pm.parent_id
ORDER BY pm.created_at DESC`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("db: list all memberships: %w", err)
	}
	defer rows.Close()
	var items []*MembershipRow
	for rows.Next() {
		var r MembershipRow
		if err := rows.Scan(&r.MemberID, &r.MemberSlug, &r.MemberDisplayName,
			&r.ParentID, &r.ParentSlug, &r.ParentDisplayName,
			&r.Role, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: list all memberships scan: %w", err)
		}
		items = append(items, &r)
	}
	return items, rows.Err()
}

// ── Scopes ────────────────────────────────────────────────────────────────────

// CreateScope inserts a new scope row.
func CreateScope(ctx context.Context, pool *pgxpool.Pool, kind, externalID, name string, parentID *uuid.UUID, principalID uuid.UUID, meta []byte) (*Scope, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := New(pool)
	row, err := q.CreateScope(ctx, CreateScopeParams{
		Kind:        kind,
		ExternalID:  externalID,
		Name:        name,
		Column4:     parentID,
		PrincipalID: principalID,
		Meta:        meta,
	})
	if err != nil {
		return nil, err
	}
	return &Scope{
		ID:          row.ID,
		Kind:        row.Kind,
		ExternalID:  row.ExternalID,
		Name:        row.Name,
		ParentID:    row.ParentID,
		PrincipalID: row.PrincipalID,
		Path:        row.Path,
		Meta:        row.Meta,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// GetScopeByID retrieves a scope by UUID. Returns nil, nil if not found.
func GetScopeByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Scope, error) {
	q := New(pool)
	row, err := q.GetScopeByID(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Scope{
		ID:          row.ID,
		Kind:        row.Kind,
		ExternalID:  row.ExternalID,
		Name:        row.Name,
		ParentID:    row.ParentID,
		PrincipalID: row.PrincipalID,
		Path:        row.Path,
		Meta:        row.Meta,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// GetScopeByExternalID retrieves a scope by kind and external_id. Returns nil, nil if not found.
func GetScopeByExternalID(ctx context.Context, pool *pgxpool.Pool, kind, externalID string) (*Scope, error) {
	q := New(pool)
	row, err := q.GetScopeByExternalID(ctx, GetScopeByExternalIDParams{
		Kind:       kind,
		ExternalID: externalID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Scope{
		ID:          row.ID,
		Kind:        row.Kind,
		ExternalID:  row.ExternalID,
		Name:        row.Name,
		ParentID:    row.ParentID,
		PrincipalID: row.PrincipalID,
		Path:        row.Path,
		Meta:        row.Meta,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// GetAncestorScopeIDs returns all ancestor scope IDs.
func GetAncestorScopeIDs(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]uuid.UUID, error) {
	q := New(pool)
	return q.GetAncestorScopeIDs(ctx, scopeID)
}

// ListScopes returns scopes ordered by creation time, with pagination.
func ListScopes(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]*Scope, error) {
	q := New(pool)
	rows, err := q.ListScopes(ctx, ListScopesParams{Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := make([]*Scope, len(rows))
	for i, row := range rows {
		out[i] = &Scope{
			ID:          row.ID,
			Kind:        row.Kind,
			ExternalID:  row.ExternalID,
			Name:        row.Name,
			ParentID:    row.ParentID,
			PrincipalID: row.PrincipalID,
			Path:        row.Path,
			Meta:        row.Meta,
			CreatedAt:   row.CreatedAt,
		}
	}
	return out, nil
}

// UpdateScope updates the name and meta of a scope.
func UpdateScope(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name string, meta []byte) (*Scope, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := New(pool)
	row, err := q.UpdateScope(ctx, UpdateScopeParams{ID: id, Name: name, Meta: meta})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Scope{
		ID:          row.ID,
		Kind:        row.Kind,
		ExternalID:  row.ExternalID,
		Name:        row.Name,
		ParentID:    row.ParentID,
		PrincipalID: row.PrincipalID,
		Path:        row.Path,
		Meta:        row.Meta,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// DeleteScope removes a scope by UUID.
func DeleteScope(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.DeleteScope(ctx, id)
}

// ── Tokens ────────────────────────────────────────────────────────────────────

// CreateToken inserts a new token record.
func CreateToken(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID, tokenHash, name string, scopeIDs []uuid.UUID, permissions []string, expiresAt *time.Time) (*Token, error) {
	if len(permissions) == 0 {
		permissions = []string{"read"}
	}
	q := New(pool)
	t, err := q.CreateToken(ctx, CreateTokenParams{
		PrincipalID: principalID,
		TokenHash:   tokenHash,
		Name:        name,
		ScopeIds:    scopeIDs,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create token: %w", err)
	}
	return t, nil
}

// LookupToken finds a token by hash. Returns nil, nil if not found.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (*Token, error) {
	q := New(pool)
	t, err := q.LookupToken(ctx, tokenHash)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: lookup token: %w", err)
	}
	return t, nil
}

// RevokeToken soft-revokes a token.
func RevokeToken(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	q := New(pool)
	return q.RevokeToken(ctx, tokenID)
}

// UpdateTokenLastUsed sets last_used_at = now().
func UpdateTokenLastUsed(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	q := New(pool)
	return q.UpdateTokenLastUsed(ctx, tokenID)
}

// ListTokens returns all tokens, optionally filtered to a single principal.
// Pass nil to list all tokens across all principals.
func ListTokens(ctx context.Context, pool *pgxpool.Pool, principalID *uuid.UUID) ([]*Token, error) {
	q := New(pool)
	if principalID == nil {
		return q.ListAllTokens(ctx)
	}
	return q.ListTokensByPrincipal(ctx, *principalID)
}

// ── Sessions ─────────────────────────────────────────────────────────────────

// CreateSession inserts a new session row.
func CreateSession(ctx context.Context, pool *pgxpool.Pool, scopeID, principalID uuid.UUID, meta []byte) (*Session, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := New(pool)
	return q.CreateSession(ctx, CreateSessionParams{
		ScopeID:     scopeID,
		PrincipalID: &principalID,
		Meta:        meta,
	})
}

// GetSession retrieves a session by ID. Returns nil, nil if not found.
func GetSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Session, error) {
	q := New(pool)
	s, err := q.GetSession(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// EndSession marks a session as ended, optionally merging meta.
func EndSession(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, meta []byte) (*Session, error) {
	q := New(pool)
	return q.EndSession(ctx, EndSessionParams{
		ID:      id,
		Column3: meta,
	})
}

// ── Memories ──────────────────────────────────────────────────────────────────

// GetMemory retrieves a memory by ID. Returns nil, nil if not found.
func GetMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Memory, error) {
	q := New(pool)
	m, err := q.GetMemory(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get memory: %w", err)
	}
	return m, nil
}

// ListMemoriesByScope returns active memories for a scope.
func ListMemoriesByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, limit, offset int) ([]*Memory, error) {
	q := New(pool)
	ms, err := q.ListMemoriesByScope(ctx, ListMemoriesByScopeParams{
		ScopeID: scopeID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list memories by scope: %w", err)
	}
	return ms, nil
}

// vecPtr returns a *pgvector.Vector when the slice is non-empty, or nil (SQL NULL)
// when the slice is empty. This is necessary because pgvector.NewVector(nil).Value()
// returns "[]" rather than NULL, which PostgreSQL rejects with "vector must have at
// least 1 dimension".
func vecPtr(vec []float32) *pgvector.Vector {
	if len(vec) == 0 {
		return nil
	}
	v := pgvector.NewVector(vec)
	return &v
}

// CreateMemory inserts a new memory record.
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

	var version interface{}
	if m.Version != 0 {
		version = m.Version
	}
	var confidence interface{}
	if m.Confidence != 0 {
		confidence = m.Confidence
	}
	var importance interface{}
	if m.Importance != 0 {
		importance = m.Importance
	}

	q := New(pool)
	created, err := q.CreateMemory(ctx, CreateMemoryParams{
		MemoryType:           m.MemoryType,
		ScopeID:              m.ScopeID,
		AuthorID:             m.AuthorID,
		Content:              m.Content,
		Summary:              m.Summary,
		Embedding:            m.Embedding,
		EmbeddingModelID:     m.EmbeddingModelID,
		EmbeddingCode:        m.EmbeddingCode,
		EmbeddingCodeModelID: m.EmbeddingCodeModelID,
		ContentKind:          m.ContentKind,
		Meta:                 m.Meta,
		Column12:             version,
		Column13:             confidence,
		Column14:             importance,
		ExpiresAt:            m.ExpiresAt,
		PromotionStatus:      m.PromotionStatus,
		PromotedTo:           m.PromotedTo,
		SourceRef:            m.SourceRef,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create memory: %w", err)
	}
	return created, nil
}

// UpdateMemoryContent updates content and embeddings.
func UpdateMemoryContent(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*Memory, error) {
	q := New(pool)
	m, err := q.UpdateMemoryContent(ctx, UpdateMemoryContentParams{
		ID:                   id,
		Content:              content,
		Embedding:            vecPtr(embedding),
		EmbeddingModelID:     textModelID,
		EmbeddingCode:        vecPtr(embeddingCode),
		EmbeddingCodeModelID: codeModelID,
		ContentKind:          contentKind,
	})
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
	q := New(pool)
	return q.SoftDeleteMemory(ctx, id)
}

// HardDeleteMemory permanently deletes a memory.
func HardDeleteMemory(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.HardDeleteMemory(ctx, id)
}

// IncrementMemoryAccess increments access_count and sets last_accessed.
func IncrementMemoryAccess(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.IncrementMemoryAccess(ctx, id)
}

// FindNearDuplicates finds active memories with cosine distance <= threshold.
func FindNearDuplicates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, embedding []float32, threshold float64, excludeID *uuid.UUID) ([]*Memory, error) {
	var excl uuid.UUID
	if excludeID != nil {
		excl = *excludeID
	}
	q := New(pool)
	ms, err := q.FindNearDuplicates(ctx, FindNearDuplicatesParams{
		ScopeID:   scopeID,
		Embedding: vecPtr(embedding),
		Column3:   threshold,
		Column4:   excl,
	})
	if err != nil {
		return nil, fmt.Errorf("db: find near duplicates: %w", err)
	}
	return ms, nil
}

// RecallMemoriesByVector performs ANN search.
func RecallMemoriesByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]MemoryScore, error) {
	q := New(pool)
	rows, err := q.RecallMemoriesByVector(ctx, RecallMemoriesByVectorParams{
		Column1:   scopeIDs,
		Limit:     int32(limit),
		Embedding: vecPtr(queryVec),
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by vector: %w", err)
	}
	results := make([]MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByVectorRow(r)
		results[i] = MemoryScore{
			Memory:   mem,
			VecScore: float64(r.VecScore),
		}
	}
	return results, nil
}

// RecallMemoriesByCodeVector performs ANN on embedding_code.
func RecallMemoriesByCodeVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]MemoryScore, error) {
	q := New(pool)
	rows, err := q.RecallMemoriesByCodeVector(ctx, RecallMemoriesByCodeVectorParams{
		Column1:       scopeIDs,
		Limit:         int32(limit),
		EmbeddingCode: vecPtr(queryVec),
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by code vector: %w", err)
	}
	results := make([]MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByCodeVectorRow(r)
		results[i] = MemoryScore{
			Memory:   mem,
			VecScore: float64(r.VecScore),
		}
	}
	return results, nil
}

// RecallMemoriesByFTS performs BM25 full-text search.
func RecallMemoriesByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int) ([]MemoryScore, error) {
	q := New(pool)
	rows, err := q.RecallMemoriesByFTS(ctx, RecallMemoriesByFTSParams{
		Column1:        scopeIDs,
		Limit:          int32(limit),
		PlaintoTsquery: query,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by fts: %w", err)
	}
	results := make([]MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByFTSRow(r)
		results[i] = MemoryScore{
			Memory:    mem,
			BM25Score: float64(r.Bm25Score),
		}
	}
	return results, nil
}

// ListConsolidationCandidates returns low-importance, low-access memories.
func ListConsolidationCandidates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*Memory, error) {
	q := New(pool)
	ms, err := q.ListConsolidationCandidates(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("db: list consolidation candidates: %w", err)
	}
	return ms, nil
}

// ── Consolidations ────────────────────────────────────────────────────────────

// CreateConsolidation inserts a consolidation record.
func CreateConsolidation(ctx context.Context, pool *pgxpool.Pool, c *Consolidation) (*Consolidation, error) {
	q := New(pool)
	result, err := q.CreateConsolidation(ctx, CreateConsolidationParams{
		ScopeID:   c.ScopeID,
		SourceIds: c.SourceIds,
		ResultID:  c.ResultID,
		Strategy:  c.Strategy,
		Reason:    c.Reason,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create consolidation: %w", err)
	}
	return result, nil
}

// ── Entities ──────────────────────────────────────────────────────────────────

// UpsertEntity inserts or updates an entity.
func UpsertEntity(ctx context.Context, pool *pgxpool.Pool, e *Entity) (*Entity, error) {
	if e.Meta == nil {
		e.Meta = []byte("{}")
	}
	q := New(pool)
	result, err := q.UpsertEntity(ctx, UpsertEntityParams{
		ScopeID:          e.ScopeID,
		EntityType:       e.EntityType,
		Name:             e.Name,
		Canonical:        e.Canonical,
		Meta:             e.Meta,
		Embedding:        e.Embedding,
		EmbeddingModelID: e.EmbeddingModelID,
	})
	if err != nil {
		return nil, fmt.Errorf("db: upsert entity: %w", err)
	}
	return result, nil
}

// GetEntityByCanonical looks up an entity by scope, type, and canonical name.
func GetEntityByCanonical(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, entityType, canonical string) (*Entity, error) {
	q := New(pool)
	e, err := q.GetEntityByCanonical(ctx, GetEntityByCanonicalParams{
		ScopeID:    scopeID,
		EntityType: entityType,
		Canonical:  canonical,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get entity by canonical: %w", err)
	}
	return e, nil
}

// LinkMemoryToEntity inserts a memory_entities row.
func LinkMemoryToEntity(ctx context.Context, pool *pgxpool.Pool, memoryID, entityID uuid.UUID, role string) error {
	q := New(pool)
	var rolePtr *string
	if role != "" {
		rolePtr = &role
	}
	err := q.LinkMemoryToEntity(ctx, LinkMemoryToEntityParams{
		MemoryID: memoryID,
		EntityID: entityID,
		Role:     rolePtr,
	})
	if err != nil {
		return fmt.Errorf("db: link memory to entity: %w", err)
	}
	return nil
}

// UpsertRelation inserts or updates a relation.
func UpsertRelation(ctx context.Context, pool *pgxpool.Pool, r *Relation) (*Relation, error) {
	q := New(pool)
	result, err := q.UpsertRelation(ctx, UpsertRelationParams{
		ScopeID:      r.ScopeID,
		SubjectID:    r.SubjectID,
		Predicate:    r.Predicate,
		ObjectID:     r.ObjectID,
		Confidence:   r.Confidence,
		SourceMemory: r.SourceMemory,
	})
	if err != nil {
		return nil, fmt.Errorf("db: upsert relation: %w", err)
	}
	return result, nil
}

// ListRelationsForEntity returns relations for an entity, optionally filtered by predicate.
func ListRelationsForEntity(ctx context.Context, pool *pgxpool.Pool, entityID uuid.UUID, predicate string) ([]*Relation, error) {
	q := New(pool)
	if predicate != "" {
		rows, err := q.ListRelationsForEntityByPredicate(ctx, ListRelationsForEntityByPredicateParams{
			SubjectID: entityID,
			Predicate: predicate,
		})
		if err != nil {
			return nil, fmt.Errorf("db: list relations for entity by predicate: %w", err)
		}
		return rows, nil
	}
	rows, err := q.ListRelationsForEntity(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("db: list relations for entity: %w", err)
	}
	return rows, nil
}

// ListEntitiesByScope returns entities in a scope.
func ListEntitiesByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, entityType string, limit, offset int) ([]*Entity, error) {
	q := New(pool)
	es, err := q.ListEntitiesByScope(ctx, ListEntitiesByScopeParams{
		ScopeID: scopeID,
		Column2: entityType,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list entities by scope: %w", err)
	}
	return es, nil
}

// ListRelationsByScope returns all relations in a scope.
func ListRelationsByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, limit, offset int) ([]*Relation, error) {
	q := New(pool)
	rs, err := q.ListRelationsByScope(ctx, ListRelationsByScopeParams{
		ScopeID: scopeID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list relations by scope: %w", err)
	}
	return rs, nil
}

// ── Knowledge artifacts ───────────────────────────────────────────────────────

// CreateArtifact inserts a new knowledge artifact.
func CreateArtifact(ctx context.Context, pool *pgxpool.Pool, a *KnowledgeArtifact) (*KnowledgeArtifact, error) {
	if a.Meta == nil {
		a.Meta = []byte("{}")
	}
	q := New(pool)
	result, err := q.CreateArtifact(ctx, CreateArtifactParams{
		KnowledgeType:    a.KnowledgeType,
		OwnerScopeID:     a.OwnerScopeID,
		AuthorID:         a.AuthorID,
		Visibility:       a.Visibility,
		Status:           a.Status,
		PublishedAt:      a.PublishedAt,
		DeprecatedAt:     a.DeprecatedAt,
		ReviewRequired:   a.ReviewRequired,
		Title:            a.Title,
		Content:          a.Content,
		Summary:          a.Summary,
		Embedding:        a.Embedding,
		EmbeddingModelID: a.EmbeddingModelID,
		Meta:             a.Meta,
		Version:          a.Version,
		Column16:         a.PreviousVersion,
		Column17:         a.SourceMemoryID,
		SourceRef:        a.SourceRef,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create artifact: %w", err)
	}
	return result, nil
}

// GetArtifact retrieves a knowledge artifact by ID. Returns nil, nil if not found.
func GetArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*KnowledgeArtifact, error) {
	q := New(pool)
	a, err := q.GetArtifact(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get artifact: %w", err)
	}
	return a, nil
}

// UpdateArtifact updates title, content, summary, embedding, and bumps version.
func UpdateArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, title, content string, summary *string, embedding []float32, modelID *uuid.UUID) (*KnowledgeArtifact, error) {
	q := New(pool)
	a, err := q.UpdateArtifact(ctx, UpdateArtifactParams{
		ID:               id,
		Title:            title,
		Content:          content,
		Summary:          summary,
		Embedding:        vecPtr(embedding),
		EmbeddingModelID: modelID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: update artifact: %w", err)
	}
	return a, nil
}

// UpdateArtifactStatus updates status, published_at, and deprecated_at.
func UpdateArtifactStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, publishedAt, deprecatedAt *time.Time) error {
	q := New(pool)
	return q.UpdateArtifactStatus(ctx, UpdateArtifactStatusParams{
		ID:           id,
		Status:       status,
		PublishedAt:  publishedAt,
		DeprecatedAt: deprecatedAt,
	})
}

// IncrementArtifactEndorsementCount increments endorsement_count.
func IncrementArtifactEndorsementCount(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.IncrementArtifactEndorsementCount(ctx, id)
}

// IncrementArtifactAccess increments access_count and sets last_accessed.
func IncrementArtifactAccess(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.IncrementArtifactAccess(ctx, id)
}

// SnapshotArtifactVersion inserts a knowledge_history row.
func SnapshotArtifactVersion(ctx context.Context, pool *pgxpool.Pool, h *KnowledgeHistory) error {
	q := New(pool)
	return q.SnapshotArtifactVersion(ctx, SnapshotArtifactVersionParams{
		ArtifactID: h.ArtifactID,
		Version:    h.Version,
		Content:    h.Content,
		Summary:    h.Summary,
		ChangedBy:  h.ChangedBy,
		ChangeNote: h.ChangeNote,
	})
}

// GetArtifactHistory returns the version history for a knowledge artifact.
func GetArtifactHistory(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID) ([]*KnowledgeHistory, error) {
	q := New(pool)
	return q.GetArtifactHistory(ctx, artifactID)
}

// CreateEndorsement inserts a knowledge_endorsements row.
func CreateEndorsement(ctx context.Context, pool *pgxpool.Pool, artifactID, endorserID uuid.UUID, note *string) (*KnowledgeEndorsement, error) {
	q := New(pool)
	e, err := q.CreateEndorsement(ctx, CreateEndorsementParams{
		ArtifactID: artifactID,
		EndorserID: endorserID,
		Note:       note,
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetEndorsementByEndorser finds an endorsement by (artifact, endorser). Returns nil, nil if not found.
func GetEndorsementByEndorser(ctx context.Context, pool *pgxpool.Pool, artifactID, endorserID uuid.UUID) (*KnowledgeEndorsement, error) {
	q := New(pool)
	e, err := q.GetEndorsementByEndorser(ctx, GetEndorsementByEndorserParams{
		ArtifactID: artifactID,
		EndorserID: endorserID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// ListAllArtifacts returns all artifacts regardless of status or scope (admin view).
func ListAllArtifacts(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]*KnowledgeArtifact, error) {
	q := New(pool)
	return q.ListAllArtifacts(ctx, ListAllArtifactsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
}

// ListVisibleArtifacts returns published artifacts visible to the given scope IDs.
func ListVisibleArtifacts(ctx context.Context, pool *pgxpool.Pool, callerScopeIDs []uuid.UUID, limit, offset int) ([]*KnowledgeArtifact, error) {
	q := New(pool)
	as, err := q.ListVisibleArtifacts(ctx, ListVisibleArtifactsParams{
		Column1: callerScopeIDs,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, err
	}
	return as, nil
}

// RecallArtifactsByVector retrieves published artifacts by vector similarity.
func RecallArtifactsByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, limit int) ([]ArtifactScore, error) {
	q := New(pool)
	rows, err := q.RecallArtifactsByVector(ctx, RecallArtifactsByVectorParams{
		Column1:   scopeIDs,
		Limit:     int32(limit),
		Embedding: vecPtr(queryVec),
	})
	if err != nil {
		return nil, err
	}
	results := make([]ArtifactScore, len(rows))
	for i, r := range rows {
		art := artifactFromRecallByVectorRow(r)
		results[i] = ArtifactScore{
			Artifact: art,
			VecScore: float64(r.VecScore),
		}
	}
	return results, nil
}

// RecallArtifactsByFTS retrieves published artifacts via full-text search.
func RecallArtifactsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int) ([]ArtifactScore, error) {
	q := New(pool)
	rows, err := q.RecallArtifactsByFTS(ctx, RecallArtifactsByFTSParams{
		Column1:        scopeIDs,
		Limit:          int32(limit),
		PlaintoTsquery: query,
	})
	if err != nil {
		return nil, err
	}
	results := make([]ArtifactScore, len(rows))
	for i, r := range rows {
		art := artifactFromRecallByFTSRow(r)
		results[i] = ArtifactScore{
			Artifact:  art,
			BM25Score: float64(r.Bm25Score),
		}
	}
	return results, nil
}

// ── Collections ───────────────────────────────────────────────────────────────

// CreateCollection inserts a new knowledge collection.
func CreateCollection(ctx context.Context, pool *pgxpool.Pool, c *KnowledgeCollection) (*KnowledgeCollection, error) {
	if c.Meta == nil {
		c.Meta = []byte("{}")
	}
	q := New(pool)
	result, err := q.CreateCollection(ctx, CreateCollectionParams{
		ScopeID:     c.ScopeID,
		OwnerID:     c.OwnerID,
		Slug:        c.Slug,
		Name:        c.Name,
		Description: c.Description,
		Visibility:  c.Visibility,
		Meta:        c.Meta,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create collection: %w", err)
	}
	return result, nil
}

// GetCollection retrieves a collection by ID. Returns nil, nil if not found.
func GetCollection(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*KnowledgeCollection, error) {
	q := New(pool)
	c, err := q.GetCollection(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get collection: %w", err)
	}
	return c, nil
}

// GetCollectionBySlug retrieves a collection by scope + slug. Returns nil, nil if not found.
func GetCollectionBySlug(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, slug string) (*KnowledgeCollection, error) {
	q := New(pool)
	c, err := q.GetCollectionBySlug(ctx, GetCollectionBySlugParams{
		ScopeID: scopeID,
		Slug:    slug,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get collection by slug: %w", err)
	}
	return c, nil
}

// ListCollections returns all collections for a scope.
func ListCollections(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*KnowledgeCollection, error) {
	q := New(pool)
	cs, err := q.ListCollections(ctx, scopeID)
	if err != nil {
		return nil, err
	}
	return cs, nil
}

// AddCollectionItem inserts a knowledge_collection_items row.
func AddCollectionItem(ctx context.Context, pool *pgxpool.Pool, collectionID, artifactID, addedBy uuid.UUID) error {
	q := New(pool)
	return q.AddCollectionItem(ctx, AddCollectionItemParams{
		CollectionID: collectionID,
		ArtifactID:   artifactID,
		AddedBy:      addedBy,
	})
}

// RemoveCollectionItem deletes a knowledge_collection_items row.
func RemoveCollectionItem(ctx context.Context, pool *pgxpool.Pool, collectionID, artifactID uuid.UUID) error {
	q := New(pool)
	return q.RemoveCollectionItem(ctx, RemoveCollectionItemParams{
		CollectionID: collectionID,
		ArtifactID:   artifactID,
	})
}

// ListCollectionItems returns the artifacts in a collection.
func ListCollectionItems(ctx context.Context, pool *pgxpool.Pool, collectionID uuid.UUID) ([]*KnowledgeArtifact, error) {
	q := New(pool)
	as, err := q.ListCollectionItems(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	return as, nil
}

// ── Staleness flags ───────────────────────────────────────────────────────────

// InsertStalenessFlag inserts a staleness_flags row.
func InsertStalenessFlag(ctx context.Context, pool *pgxpool.Pool, f *StalenessFlag) (*StalenessFlag, error) {
	if f.Evidence == nil {
		f.Evidence = []byte("{}")
	}
	q := New(pool)
	var statusPtr *string
	if f.Status != "" {
		statusPtr = &f.Status
	}
	result, err := q.InsertStalenessFlag(ctx, InsertStalenessFlagParams{
		ArtifactID: f.ArtifactID,
		Signal:     f.Signal,
		Confidence: f.Confidence,
		Evidence:   f.Evidence,
		Column5:    statusPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("db: insert staleness flag: %w", err)
	}
	return result, nil
}

// HasOpenStalenessFlag reports whether an artifact has an open staleness flag.
func HasOpenStalenessFlag(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID, signal string) (bool, error) {
	q := New(pool)
	exists, err := q.HasOpenStalenessFlag(ctx, HasOpenStalenessFlagParams{
		ArtifactID: artifactID,
		Signal:     signal,
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

// UpdateStalenessFlag updates a staleness flag.
func UpdateStalenessFlag(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, reviewedBy *uuid.UUID, note *string) error {
	q := New(pool)
	return q.UpdateStalenessFlag(ctx, UpdateStalenessFlagParams{
		ID:         id,
		Status:     status,
		ReviewedBy: reviewedBy,
		ReviewNote: note,
	})
}

// ListStalenessFlags returns staleness flags optionally filtered by status.
func ListStalenessFlags(ctx context.Context, pool *pgxpool.Pool, status string, limit, offset int) ([]*StalenessFlag, error) {
	q := New(pool)
	fs, err := q.ListStalenessFlags(ctx, ListStalenessFlagsParams{
		Column1: &status,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list staleness flags: %w", err)
	}
	return fs, nil
}

// ── Promotion requests ────────────────────────────────────────────────────────

// CreatePromotionRequest inserts a new promotion_requests row.
func CreatePromotionRequest(ctx context.Context, pool *pgxpool.Pool, req *PromotionRequest) (*PromotionRequest, error) {
	q := New(pool)
	result, err := q.CreatePromotionRequest(ctx, CreatePromotionRequestParams{
		MemoryID:             req.MemoryID,
		RequestedBy:          req.RequestedBy,
		TargetScopeID:        req.TargetScopeID,
		TargetVisibility:     req.TargetVisibility,
		ProposedTitle:        req.ProposedTitle,
		ProposedCollectionID: req.ProposedCollectionID,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create promotion request: %w", err)
	}
	return result, nil
}

// GetPromotionRequest retrieves a promotion request by ID. Returns nil, nil if not found.
func GetPromotionRequest(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*PromotionRequest, error) {
	q := New(pool)
	p, err := q.GetPromotionRequest(ctx, id)
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
	q := New(pool)
	ps, err := q.ListPendingPromotions(ctx, targetScopeID)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// ── Skills ────────────────────────────────────────────────────────────────────

// CreateSkill inserts a new skill row.
func CreateSkill(ctx context.Context, pool *pgxpool.Pool, s *Skill) (*Skill, error) {
	q := New(pool)
	result, err := q.CreateSkill(ctx, CreateSkillParams{
		ScopeID:          s.ScopeID,
		AuthorID:         s.AuthorID,
		Column3:          s.SourceArtifactID,
		Slug:             s.Slug,
		Name:             s.Name,
		Description:      s.Description,
		AgentTypes:       s.AgentTypes,
		Body:             s.Body,
		Parameters:       s.Parameters,
		Visibility:       s.Visibility,
		Status:           s.Status,
		PublishedAt:      s.PublishedAt,
		DeprecatedAt:     s.DeprecatedAt,
		ReviewRequired:   s.ReviewRequired,
		Version:          s.Version,
		Column16:         s.PreviousVersion,
		Embedding:        s.Embedding,
		EmbeddingModelID: s.EmbeddingModelID,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetSkill retrieves a skill by UUID. Returns nil, nil if not found.
func GetSkill(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Skill, error) {
	q := New(pool)
	s, err := q.GetSkill(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// GetSkillBySlug retrieves a skill by scope and slug. Returns nil, nil if not found.
func GetSkillBySlug(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, slug string) (*Skill, error) {
	q := New(pool)
	s, err := q.GetSkillBySlug(ctx, GetSkillBySlugParams{
		ScopeID: scopeID,
		Slug:    slug,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// UpdateSkillContent updates the body, parameters, embedding, and bumps version.
func UpdateSkillContent(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, body string, parameters []byte, embedding []float32, modelID *uuid.UUID) (*Skill, error) {
	q := New(pool)
	s, err := q.UpdateSkillContent(ctx, UpdateSkillContentParams{
		ID:               id,
		Body:             body,
		Parameters:       parameters,
		Embedding:        vecPtr(embedding),
		EmbeddingModelID: modelID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// UpdateSkillStatus updates status, published_at, and deprecated_at.
func UpdateSkillStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, publishedAt, deprecatedAt *time.Time) error {
	q := New(pool)
	return q.UpdateSkillStatus(ctx, UpdateSkillStatusParams{
		ID:           id,
		Status:       status,
		PublishedAt:  publishedAt,
		DeprecatedAt: deprecatedAt,
	})
}

// SnapshotSkillVersion inserts a skill_history row.
func SnapshotSkillVersion(ctx context.Context, pool *pgxpool.Pool, h *SkillHistory) error {
	q := New(pool)
	return q.SnapshotSkillVersion(ctx, SnapshotSkillVersionParams{
		SkillID:    h.SkillID,
		Version:    h.Version,
		Body:       h.Body,
		Parameters: h.Parameters,
		ChangedBy:  h.ChangedBy,
		ChangeNote: h.ChangeNote,
	})
}

// CreateSkillEndorsement inserts a skill_endorsements row.
func CreateSkillEndorsement(ctx context.Context, pool *pgxpool.Pool, skillID, endorserID uuid.UUID, note *string) (*SkillEndorsement, error) {
	q := New(pool)
	e, err := q.CreateSkillEndorsement(ctx, CreateSkillEndorsementParams{
		SkillID:    skillID,
		EndorserID: endorserID,
		Note:       note,
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetSkillEndorsementByEndorser finds an endorsement. Returns nil, nil if not found.
func GetSkillEndorsementByEndorser(ctx context.Context, pool *pgxpool.Pool, skillID, endorserID uuid.UUID) (*SkillEndorsement, error) {
	q := New(pool)
	e, err := q.GetSkillEndorsementByEndorser(ctx, GetSkillEndorsementByEndorserParams{
		SkillID:    skillID,
		EndorserID: endorserID,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// CountSkillEndorsements returns the number of endorsements for a skill.
func CountSkillEndorsements(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID) (int, error) {
	q := New(pool)
	count, err := q.CountSkillEndorsements(ctx, skillID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// RecallSkillsByVector retrieves published skills by vector similarity.
func RecallSkillsByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, agentType string, limit int) ([]SkillScore, error) {
	q := New(pool)
	rows, err := q.RecallSkillsByVector(ctx, RecallSkillsByVectorParams{
		Embedding: vecPtr(queryVec),
		Column2:   scopeIDs,
		Column3:   agentType,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	results := make([]SkillScore, len(rows))
	for i, r := range rows {
		skill := skillFromRecallByVectorRow(r)
		var score float64
		if f, ok := r.Score.(float64); ok {
			score = f
		} else if f32, ok := r.Score.(float32); ok {
			score = float64(f32)
		}
		results[i] = SkillScore{
			Skill: skill,
			Score: score,
		}
	}
	return results, nil
}

// RecallSkillsByFTS retrieves published skills via full-text search.
func RecallSkillsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query, agentType string, limit int) ([]SkillScore, error) {
	q := New(pool)
	rows, err := q.RecallSkillsByFTS(ctx, RecallSkillsByFTSParams{
		PlaintoTsquery: query,
		Column2:        scopeIDs,
		Column3:        agentType,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	results := make([]SkillScore, len(rows))
	for i, r := range rows {
		skill := skillFromRecallByFTSRow(r)
		results[i] = SkillScore{
			Skill: skill,
			Score: float64(r.Score),
		}
	}
	return results, nil
}

// ListPublishedSkillsForAgent returns all published skills for the given agent type.
func ListPublishedSkillsForAgent(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, agentType string) ([]*Skill, error) {
	q := New(pool)
	ss, err := q.ListPublishedSkillsForAgent(ctx, ListPublishedSkillsForAgentParams{
		Column1: scopeIDs,
		Column2: agentType,
	})
	if err != nil {
		return nil, err
	}
	return ss, nil
}

// ListPrincipals returns principals ordered by creation time.
func ListPrincipals(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]*Principal, error) {
	q := New(pool)
	ps, err := q.ListPrincipals(ctx, ListPrincipalsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list principals: %w", err)
	}
	return ps, nil
}

// ── Helper ────────────────────────────────────────────────────────────────────

// derefTime returns the zero time.Time if t is nil, else *t.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// memoryFromRecallByVectorRow converts a RecallMemoriesByVectorRow to a *Memory.
func memoryFromRecallByVectorRow(r *RecallMemoriesByVectorRow) *Memory {
	return &Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromRecallByCodeVectorRow converts a RecallMemoriesByCodeVectorRow to a *Memory.
func memoryFromRecallByCodeVectorRow(r *RecallMemoriesByCodeVectorRow) *Memory {
	return &Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromRecallByFTSRow converts a RecallMemoriesByFTSRow to a *Memory.
func memoryFromRecallByFTSRow(r *RecallMemoriesByFTSRow) *Memory {
	return &Memory{
		ID:                   r.ID,
		MemoryType:           r.MemoryType,
		ScopeID:              r.ScopeID,
		AuthorID:             r.AuthorID,
		Content:              r.Content,
		Summary:              r.Summary,
		Embedding:            r.Embedding,
		EmbeddingModelID:     r.EmbeddingModelID,
		EmbeddingCode:        r.EmbeddingCode,
		EmbeddingCodeModelID: r.EmbeddingCodeModelID,
		ContentKind:          r.ContentKind,
		Meta:                 r.Meta,
		Version:              r.Version,
		IsActive:             r.IsActive,
		Confidence:           r.Confidence,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		ExpiresAt:            r.ExpiresAt,
		PromotionStatus:      r.PromotionStatus,
		PromotedTo:           r.PromotedTo,
		SourceRef:            r.SourceRef,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// artifactFromRecallByVectorRow converts a RecallArtifactsByVectorRow to a *KnowledgeArtifact.
func artifactFromRecallByVectorRow(r *RecallArtifactsByVectorRow) *KnowledgeArtifact {
	return &KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// artifactFromRecallByFTSRow converts a RecallArtifactsByFTSRow to a *KnowledgeArtifact.
func artifactFromRecallByFTSRow(r *RecallArtifactsByFTSRow) *KnowledgeArtifact {
	return &KnowledgeArtifact{
		ID:               r.ID,
		KnowledgeType:    r.KnowledgeType,
		OwnerScopeID:     r.OwnerScopeID,
		AuthorID:         r.AuthorID,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Title:            r.Title,
		Content:          r.Content,
		Summary:          r.Summary,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		Meta:             r.Meta,
		EndorsementCount: r.EndorsementCount,
		AccessCount:      r.AccessCount,
		LastAccessed:     r.LastAccessed,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		SourceMemoryID:   r.SourceMemoryID,
		SourceRef:        r.SourceRef,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// skillFromRecallByVectorRow converts a RecallSkillsByVectorRow to a *Skill.
func skillFromRecallByVectorRow(r *RecallSkillsByVectorRow) *Skill {
	return &Skill{
		ID:               r.ID,
		ScopeID:          r.ScopeID,
		AuthorID:         r.AuthorID,
		SourceArtifactID: r.SourceArtifactID,
		Slug:             r.Slug,
		Name:             r.Name,
		Description:      r.Description,
		AgentTypes:       r.AgentTypes,
		Body:             r.Body,
		Parameters:       r.Parameters,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		InvocationCount:  r.InvocationCount,
		LastInvokedAt:    r.LastInvokedAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// skillFromRecallByFTSRow converts a RecallSkillsByFTSRow to a *Skill.
func skillFromRecallByFTSRow(r *RecallSkillsByFTSRow) *Skill {
	return &Skill{
		ID:               r.ID,
		ScopeID:          r.ScopeID,
		AuthorID:         r.AuthorID,
		SourceArtifactID: r.SourceArtifactID,
		Slug:             r.Slug,
		Name:             r.Name,
		Description:      r.Description,
		AgentTypes:       r.AgentTypes,
		Body:             r.Body,
		Parameters:       r.Parameters,
		Visibility:       r.Visibility,
		Status:           r.Status,
		PublishedAt:      r.PublishedAt,
		DeprecatedAt:     r.DeprecatedAt,
		ReviewRequired:   r.ReviewRequired,
		Version:          r.Version,
		PreviousVersion:  r.PreviousVersion,
		Embedding:        r.Embedding,
		EmbeddingModelID: r.EmbeddingModelID,
		InvocationCount:  r.InvocationCount,
		LastInvokedAt:    r.LastInvokedAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// GetScopesByIDs fetches all scopes whose IDs are in the provided slice.
// Returns an empty slice (not nil) when ids is empty.
func GetScopesByIDs(ctx context.Context, pool *pgxpool.Pool, ids []uuid.UUID) ([]*Scope, error) {
	if len(ids) == 0 {
		return []*Scope{}, nil
	}
	rows, err := pool.Query(ctx,
		`SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
		 FROM scopes WHERE id = ANY($1)
		 ORDER BY created_at DESC`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get scopes by ids: %w", err)
	}
	defer rows.Close()
	var scopes []*Scope
	for rows.Next() {
		var s Scope
		if err := rows.Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID,
			&s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: get scopes by ids scan: %w", err)
		}
		scopes = append(scopes, &s)
	}
	return scopes, rows.Err()
}

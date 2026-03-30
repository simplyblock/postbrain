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
	TrgmScore float64
}

// ArtifactScore pairs a knowledge artifact with its retrieval scores.
type ArtifactScore struct {
	Artifact  *KnowledgeArtifact
	VecScore  float64
	BM25Score float64
	TrgmScore float64
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
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
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
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
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
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
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
			ID:                row.ID,
			Kind:              row.Kind,
			ExternalID:        row.ExternalID,
			Name:              row.Name,
			ParentID:          row.ParentID,
			PrincipalID:       row.PrincipalID,
			Path:              row.Path,
			Meta:              row.Meta,
			RepoUrl:           row.RepoUrl,
			RepoDefaultBranch: row.RepoDefaultBranch,
			LastIndexedCommit: row.LastIndexedCommit,
			CreatedAt:         row.CreatedAt,
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
		ID:                row.ID,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// DeleteScope removes a scope by UUID.
func DeleteScope(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.DeleteScope(ctx, id)
}

// SetScopeRepo attaches a git repository URL and default branch to a project-kind scope.
func SetScopeRepo(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, repoURL, defaultBranch string) (*Scope, error) {
	q := New(pool)
	row, err := q.SetScopeRepo(ctx, SetScopeRepoParams{
		ID:                id,
		RepoUrl:           &repoURL,
		RepoDefaultBranch: defaultBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("db: set scope repo: %w", err)
	}
	return &Scope{
		ID:                id,
		Kind:              row.Kind,
		ExternalID:        row.ExternalID,
		Name:              row.Name,
		ParentID:          row.ParentID,
		PrincipalID:       row.PrincipalID,
		Path:              row.Path,
		Meta:              row.Meta,
		RepoUrl:           row.RepoUrl,
		RepoDefaultBranch: row.RepoDefaultBranch,
		LastIndexedCommit: row.LastIndexedCommit,
		CreatedAt:         row.CreatedAt,
	}, nil
}

// SetLastIndexedCommit records the last successfully indexed commit SHA for a scope.
func SetLastIndexedCommit(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, sha string) error {
	q := New(pool)
	return q.SetLastIndexedCommit(ctx, SetLastIndexedCommitParams{
		ID:                id,
		LastIndexedCommit: &sha,
	})
}

// DeleteRelationsBySourceFile removes all relations from a scope that were extracted from a given file.
func DeleteRelationsBySourceFile(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, sourceFile string) error {
	q := New(pool)
	return q.DeleteRelationsBySourceFile(ctx, DeleteRelationsBySourceFileParams{
		ScopeID:    scopeID,
		SourceFile: &sourceFile,
	})
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

// InsertEvent appends a typed event to the partitioned events table.
// sessionID may be uuid.Nil when no session context is available.
func InsertEvent(ctx context.Context, pool *pgxpool.Pool, sessionID, scopeID uuid.UUID, eventType string, payload []byte) error {
	if payload == nil {
		payload = []byte("{}")
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO events (session_id, scope_id, event_type, payload) VALUES ($1,$2,$3,$4)`,
		sessionID, scopeID, eventType, payload,
	)
	if err != nil {
		return fmt.Errorf("db: insert event: %w", err)
	}
	return nil
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
	return memoryFromGetMemoryRow(m), nil
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
	out := make([]*Memory, len(ms))
	for i, r := range ms {
		out[i] = memoryFromListMemoriesByScopeRow(r)
	}
	return out, nil
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
		ParentMemoryID:       m.ParentMemoryID,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create memory: %w", err)
	}
	return memoryFromCreateMemoryRow(created), nil
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
	return memoryFromUpdateMemoryContentRow(m), nil
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
	out := make([]*Memory, len(ms))
	for i, r := range ms {
		out[i] = memoryFromFindNearDuplicatesRow(r)
	}
	return out, nil
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

// RecallMemoriesByTrigram performs trigram similarity recall.
func RecallMemoriesByTrigram(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query string, limit int) ([]MemoryScore, error) {
	q := New(pool)
	rows, err := q.RecallMemoriesByTrigram(ctx, RecallMemoriesByTrigramParams{
		Column1:    scopeIDs,
		Limit:      int32(limit),
		Similarity: query,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall memories by trigram: %w", err)
	}
	results := make([]MemoryScore, len(rows))
	for i, r := range rows {
		mem := memoryFromRecallByTrigramRow(r)
		results[i] = MemoryScore{
			Memory:    mem,
			TrgmScore: float64(r.TrgmScore),
		}
	}
	return results, nil
}

// ListChunkMemories returns chunk memories (children) for a given parent memory.
func ListChunkMemories(ctx context.Context, pool *pgxpool.Pool, parentMemoryID uuid.UUID) ([]*Memory, error) {
	q := New(pool)
	rows, err := q.ListChunkMemories(ctx, &parentMemoryID)
	if err != nil {
		return nil, fmt.Errorf("db: list chunk memories: %w", err)
	}
	out := make([]*Memory, len(rows))
	for i, r := range rows {
		out[i] = memoryFromListChunkMemoriesRow(r)
	}
	return out, nil
}

// ListConsolidationCandidates returns low-importance, low-access memories.
func ListConsolidationCandidates(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*Memory, error) {
	q := New(pool)
	ms, err := q.ListConsolidationCandidates(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("db: list consolidation candidates: %w", err)
	}
	out := make([]*Memory, len(ms))
	for i, r := range ms {
		out[i] = memoryFromListConsolidationCandidatesRow(r)
	}
	return out, nil
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

// LinkArtifactToEntity inserts an artifact_entities row.
func LinkArtifactToEntity(ctx context.Context, pool *pgxpool.Pool, artifactID, entityID uuid.UUID, role string) error {
	q := New(pool)
	var rolePtr *string
	if role != "" {
		rolePtr = &role
	}
	err := q.LinkArtifactToEntity(ctx, LinkArtifactToEntityParams{
		ArtifactID: artifactID,
		EntityID:   entityID,
		Role:       rolePtr,
	})
	if err != nil {
		return fmt.Errorf("db: link artifact to entity: %w", err)
	}
	return nil
}

// DeleteArtifactEntityLinks removes all artifact_entities rows for the given artifact.
func DeleteArtifactEntityLinks(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID) error {
	q := New(pool)
	return q.DeleteArtifactEntityLinks(ctx, artifactID)
}

// DeleteArtifact permanently removes a knowledge artifact by ID.
// Callers must pre-null any NO ACTION FK references before calling this.
func DeleteArtifact(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.DeleteArtifact(ctx, id)
}

// NullPreviousVersionRefs clears self-referential previous_version FK pointing at id.
func NullPreviousVersionRefs(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.NullPreviousVersionRefs(ctx, &id)
}

// NullPromotionRequestArtifactRef clears result_artifact_id FK in promotion_requests.
func NullPromotionRequestArtifactRef(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.NullPromotionRequestArtifactRef(ctx, &id)
}

// ResetPromotedMemoryStatus clears promotion_status on memories whose promoted_to points at id.
func ResetPromotedMemoryStatus(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	q := New(pool)
	return q.ResetPromotedMemoryStatus(ctx, &id)
}

// UpsertRelation inserts or updates a relation.
func UpsertRelation(ctx context.Context, pool *pgxpool.Pool, r *Relation) (*Relation, error) {
	q := New(pool)
	result, err := q.UpsertRelation(ctx, UpsertRelationParams{
		ScopeID:        r.ScopeID,
		SubjectID:      r.SubjectID,
		Predicate:      r.Predicate,
		ObjectID:       r.ObjectID,
		Confidence:     r.Confidence,
		SourceMemory:   r.SourceMemory,
		SourceArtifact: r.SourceArtifact,
		SourceFile:     r.SourceFile,
	})
	if err != nil {
		return nil, fmt.Errorf("db: upsert relation: %w", err)
	}
	return &Relation{
		ID:             result.ID,
		ScopeID:        result.ScopeID,
		SubjectID:      result.SubjectID,
		Predicate:      result.Predicate,
		ObjectID:       result.ObjectID,
		Confidence:     result.Confidence,
		SourceMemory:   result.SourceMemory,
		SourceArtifact: result.SourceArtifact,
		SourceFile:     result.SourceFile,
		CreatedAt:      result.CreatedAt,
	}, nil
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
		out := make([]*Relation, len(rows))
		for i, r := range rows {
			out[i] = &Relation{
				ID:             r.ID,
				ScopeID:        r.ScopeID,
				SubjectID:      r.SubjectID,
				Predicate:      r.Predicate,
				ObjectID:       r.ObjectID,
				Confidence:     r.Confidence,
				SourceMemory:   r.SourceMemory,
				SourceArtifact: r.SourceArtifact,
				CreatedAt:      r.CreatedAt,
			}
		}
		return out, nil
	}
	rows, err := q.ListRelationsForEntity(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("db: list relations for entity: %w", err)
	}
	out := make([]*Relation, len(rows))
	for i, r := range rows {
		out[i] = &Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}

// ListEntitiesByCanonical returns all entities in a scope that share a canonical
// but have a different entity_type than excludeType. Used to find siblings for
// same_as relation creation.
func ListEntitiesByCanonical(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, canonical, excludeType string) ([]*Entity, error) {
	q := New(pool)
	es, err := q.ListEntitiesByCanonical(ctx, ListEntitiesByCanonicalParams{
		ScopeID:    scopeID,
		Canonical:  canonical,
		EntityType: excludeType,
	})
	if err != nil {
		return nil, fmt.Errorf("db: list entities by canonical: %w", err)
	}
	return es, nil
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

// FindEntitiesBySuffix returns entities in a scope whose canonical name equals
// suffix or ends with ".suffix", "::suffix", or "#suffix".
// Used for heuristic call-target resolution in code graph extraction.
func FindEntitiesBySuffix(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, suffix string) ([]*Entity, error) {
	q := New(pool)
	es, err := q.FindEntitiesBySuffix(ctx, FindEntitiesBySuffixParams{
		ScopeID:   scopeID,
		Canonical: suffix,
	})
	if err != nil {
		return nil, fmt.Errorf("db: find entities by suffix: %w", err)
	}
	return es, nil
}

// GetEntityByID retrieves an entity by its UUID. Returns nil, nil if not found.
func GetEntityByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Entity, error) {
	q := New(pool)
	e, err := q.GetEntityByID(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get entity by id: %w", err)
	}
	return e, nil
}

// ListOutgoingRelations returns relations where the entity is the subject,
// optionally filtered by predicate (empty string = all predicates).
func ListOutgoingRelations(ctx context.Context, pool *pgxpool.Pool, scopeID, entityID uuid.UUID, predicate string) ([]*Relation, error) {
	q := New(pool)
	rows, err := q.ListOutgoingRelations(ctx, ListOutgoingRelationsParams{
		ScopeID:   scopeID,
		SubjectID: entityID,
		Column3:   predicate,
	})
	if err != nil {
		return nil, fmt.Errorf("db: list outgoing relations: %w", err)
	}
	out := make([]*Relation, len(rows))
	for i, r := range rows {
		out[i] = &Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			SourceFile:     r.SourceFile,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}

// ListIncomingRelations returns relations where the entity is the object,
// optionally filtered by predicate (empty string = all predicates).
func ListIncomingRelations(ctx context.Context, pool *pgxpool.Pool, scopeID, entityID uuid.UUID, predicate string) ([]*Relation, error) {
	q := New(pool)
	rows, err := q.ListIncomingRelations(ctx, ListIncomingRelationsParams{
		ScopeID:  scopeID,
		ObjectID: entityID,
		Column3:  predicate,
	})
	if err != nil {
		return nil, fmt.Errorf("db: list incoming relations: %w", err)
	}
	out := make([]*Relation, len(rows))
	for i, r := range rows {
		out[i] = &Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			SourceFile:     r.SourceFile,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}

// ListMemoriesForEntity returns active memories linked to a given entity.
func ListMemoriesForEntity(ctx context.Context, pool *pgxpool.Pool, entityID uuid.UUID, limit int) ([]*Memory, error) {
	q := New(pool)
	rows, err := q.ListMemoriesForEntity(ctx, ListMemoriesForEntityParams{
		EntityID: entityID,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list memories for entity: %w", err)
	}
	out := make([]*Memory, len(rows))
	for i, r := range rows {
		out[i] = memoryFromListMemoriesForEntityRow(r)
	}
	return out, nil
}

// ListRelationsByScope returns all relations in a scope.
func ListRelationsByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*Relation, error) {
	q := New(pool)
	rs, err := q.ListRelationsByScope(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("db: list relations by scope: %w", err)
	}
	out := make([]*Relation, len(rs))
	for i, r := range rs {
		out[i] = &Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
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

// SearchArtifacts filters artifacts by a text query (ILIKE on title/content) and optional status.
func SearchArtifacts(ctx context.Context, pool *pgxpool.Pool, query, status string, limit, offset int) ([]*KnowledgeArtifact, error) {
	q := New(pool)
	return q.SearchArtifacts(ctx, SearchArtifactsParams{
		Title:  "%" + query + "%",
		Column2: status,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
}

// ListArtifactsByStatus returns artifacts filtered by a single status value.
func ListArtifactsByStatus(ctx context.Context, pool *pgxpool.Pool, status string, limit, offset int) ([]*KnowledgeArtifact, error) {
	q := New(pool)
	return q.ListArtifactsByStatus(ctx, ListArtifactsByStatusParams{
		Status: status,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
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

// RecallArtifactsByVector retrieves published artifacts by vector similarity,
// resolving visibility (project/team/department/company/grants) from scopeID.
func RecallArtifactsByVector(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, queryVec []float32, limit int) ([]ArtifactScore, error) {
	q := New(pool)
	rows, err := q.RecallArtifactsByVector(ctx, RecallArtifactsByVectorParams{
		OwnerScopeID: scopeID,
		Limit:        int32(limit),
		Embedding:    vecPtr(queryVec),
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

// RecallArtifactsByFTS retrieves published artifacts via full-text search,
// resolving visibility (project/team/department/company/grants) from scopeID.
func RecallArtifactsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, query string, limit int) ([]ArtifactScore, error) {
	q := New(pool)
	rows, err := q.RecallArtifactsByFTS(ctx, RecallArtifactsByFTSParams{
		OwnerScopeID:   scopeID,
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

// RecallArtifactsByTrigram retrieves published artifacts via trigram similarity,
// resolving visibility (project/team/department/company/grants) from scopeID.
func RecallArtifactsByTrigram(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, query string, limit int) ([]ArtifactScore, error) {
	q := New(pool)
	rows, err := q.RecallArtifactsByTrigram(ctx, RecallArtifactsByTrigramParams{
		OwnerScopeID: scopeID,
		Limit:        int32(limit),
		Similarity:   query,
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall artifacts by trigram: %w", err)
	}
	results := make([]ArtifactScore, len(rows))
	for i, r := range rows {
		art := artifactFromRecallByTrigramRow(r)
		results[i] = ArtifactScore{
			Artifact:  art,
			TrgmScore: float64(r.TrgmScore),
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

// ListAllCollections returns all collections across all scopes, ordered by name.
func ListAllCollections(ctx context.Context, pool *pgxpool.Pool) ([]*KnowledgeCollection, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at
		 FROM knowledge_collections ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list all collections: %w", err)
	}
	defer rows.Close()
	var cs []*KnowledgeCollection
	for rows.Next() {
		var c KnowledgeCollection
		if err := rows.Scan(&c.ID, &c.ScopeID, &c.OwnerID, &c.Slug, &c.Name, &c.Description,
			&c.Visibility, &c.Meta, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("db: list all collections scan: %w", err)
		}
		cs = append(cs, &c)
	}
	return cs, rows.Err()
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

// ListPendingPromotions returns pending promotion requests.
// When targetScopeID is the zero UUID, all pending promotions across all scopes are returned.
func ListPendingPromotions(ctx context.Context, pool *pgxpool.Pool, targetScopeID uuid.UUID) ([]*PromotionRequest, error) {
	if targetScopeID == (uuid.UUID{}) {
		rows, err := pool.Query(ctx,
			`SELECT id, memory_id, requested_by, target_scope_id, target_visibility,
			        proposed_title, proposed_collection_id, status, reviewer_id, review_note,
			        reviewed_at, result_artifact_id, created_at
			 FROM promotion_requests WHERE status='pending' ORDER BY created_at`,
		)
		if err != nil {
			return nil, fmt.Errorf("db: list all pending promotions: %w", err)
		}
		defer rows.Close()
		var ps []*PromotionRequest
		for rows.Next() {
			var p PromotionRequest
			if err := rows.Scan(
				&p.ID, &p.MemoryID, &p.RequestedBy, &p.TargetScopeID, &p.TargetVisibility,
				&p.ProposedTitle, &p.ProposedCollectionID, &p.Status, &p.ReviewerID, &p.ReviewNote,
				&p.ReviewedAt, &p.ResultArtifactID, &p.CreatedAt,
			); err != nil {
				return nil, fmt.Errorf("db: list all pending promotions scan: %w", err)
			}
			ps = append(ps, &p)
		}
		return ps, rows.Err()
	}
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

// GetSkillHistory returns the version history for a skill, ordered descending by version.
func GetSkillHistory(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID) ([]*SkillHistory, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, skill_id, version, body, parameters, changed_by, change_note, created_at
		 FROM skill_history WHERE skill_id=$1 ORDER BY version DESC`,
		skillID,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get skill history: %w", err)
	}
	defer rows.Close()
	var items []*SkillHistory
	for rows.Next() {
		var h SkillHistory
		if err := rows.Scan(&h.ID, &h.SkillID, &h.Version, &h.Body, &h.Parameters,
			&h.ChangedBy, &h.ChangeNote, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: get skill history scan: %w", err)
		}
		items = append(items, &h)
	}
	return items, rows.Err()
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

// RecallSkillsByTrigram retrieves published skills via trigram similarity.
func RecallSkillsByTrigram(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query, agentType string, limit int) ([]SkillScore, error) {
	q := New(pool)
	rows, err := q.RecallSkillsByTrigram(ctx, RecallSkillsByTrigramParams{
		Similarity: query,
		Column2:    scopeIDs,
		Column3:    agentType,
		Limit:      int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall skills by trigram: %w", err)
	}
	results := make([]SkillScore, len(rows))
	for i, r := range rows {
		skill := skillFromRecallByTrigramRow(r)
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

// memoryFromRow is a shared helper that copies all common fields from any memory-like struct.
// Used by the typed conversion helpers below.

// memoryFromGetMemoryRow converts a GetMemoryRow to a *Memory.
func memoryFromGetMemoryRow(r *GetMemoryRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromCreateMemoryRow converts a CreateMemoryRow to a *Memory.
func memoryFromCreateMemoryRow(r *CreateMemoryRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromUpdateMemoryContentRow converts an UpdateMemoryContentRow to a *Memory.
func memoryFromUpdateMemoryContentRow(r *UpdateMemoryContentRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromFindNearDuplicatesRow converts a FindNearDuplicatesRow to a *Memory.
func memoryFromFindNearDuplicatesRow(r *FindNearDuplicatesRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromListMemoriesByScopeRow converts a ListMemoriesByScopeRow to a *Memory.
func memoryFromListMemoriesByScopeRow(r *ListMemoriesByScopeRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromListConsolidationCandidatesRow converts a ListConsolidationCandidatesRow to a *Memory.
func memoryFromListConsolidationCandidatesRow(r *ListConsolidationCandidatesRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromListMemoriesForEntityRow converts a ListMemoriesForEntityRow to a *Memory.
func memoryFromListMemoriesForEntityRow(r *ListMemoriesForEntityRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// memoryFromListChunkMemoriesRow converts a ListChunkMemoriesRow to a *Memory.
func memoryFromListChunkMemoriesRow(r *ListChunkMemoriesRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
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
		ParentMemoryID:       r.ParentMemoryID,
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
		ParentMemoryID:       r.ParentMemoryID,
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
		ParentMemoryID:       r.ParentMemoryID,
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

// memoryFromRecallByTrigramRow converts a RecallMemoriesByTrigramRow to a *Memory.
func memoryFromRecallByTrigramRow(r *RecallMemoriesByTrigramRow) *Memory {
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
		ParentMemoryID:       r.ParentMemoryID,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// artifactFromRecallByTrigramRow converts a RecallArtifactsByTrigramRow to a *KnowledgeArtifact.
func artifactFromRecallByTrigramRow(r *RecallArtifactsByTrigramRow) *KnowledgeArtifact {
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

// skillFromRecallByTrigramRow converts a RecallSkillsByTrigramRow to a *Skill.
func skillFromRecallByTrigramRow(r *RecallSkillsByTrigramRow) *Skill {
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

// ─────────────────────────────────────────────────────────────────────────────
// Topic synthesis: digest sources + audit log
// ─────────────────────────────────────────────────────────────────────────────

// DigestLog is an audit record for a synthesis operation.
type DigestLog struct {
	ID            uuid.UUID
	ScopeID       uuid.UUID
	DigestID      uuid.UUID
	SourceIDs     []uuid.UUID
	Strategy      string
	SynthesisedBy *uuid.UUID
	CreatedAt     time.Time
}

// InsertDigestSources records which source artifacts a digest was synthesised from.
func InsertDigestSources(ctx context.Context, pool *pgxpool.Pool, digestID uuid.UUID, sourceIDs []uuid.UUID) error {
	if len(sourceIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, sid := range sourceIDs {
		batch.Queue(
			`INSERT INTO artifact_digest_sources (digest_id, source_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			digestID, sid,
		)
	}
	return pool.SendBatch(ctx, batch).Close()
}

// ListDigestSources returns the source artifacts for a given digest artifact.
func ListDigestSources(ctx context.Context, pool *pgxpool.Pool, digestID uuid.UUID) ([]*KnowledgeArtifact, error) {
	rows, err := pool.Query(ctx, `
		SELECT ka.id, ka.knowledge_type, ka.owner_scope_id, ka.author_id,
		       ka.visibility, ka.status, ka.published_at, ka.deprecated_at,
		       ka.review_required, ka.title, ka.content, ka.summary,
		       ka.embedding, ka.embedding_model_id, ka.meta,
		       ka.endorsement_count, ka.access_count, ka.last_accessed,
		       ka.version, ka.previous_version, ka.source_memory_id, ka.source_ref,
		       ka.created_at, ka.updated_at
		FROM knowledge_artifacts ka
		JOIN artifact_digest_sources ads ON ads.source_id = ka.id
		WHERE ads.digest_id = $1
		ORDER BY ads.added_at ASC`, digestID)
	if err != nil {
		return nil, fmt.Errorf("db: list digest sources: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeArtifactRows(rows)
}

// ListDigestsForSource returns published digests that cover a given source artifact.
func ListDigestsForSource(ctx context.Context, pool *pgxpool.Pool, sourceID uuid.UUID) ([]*KnowledgeArtifact, error) {
	rows, err := pool.Query(ctx, `
		SELECT ka.id, ka.knowledge_type, ka.owner_scope_id, ka.author_id,
		       ka.visibility, ka.status, ka.published_at, ka.deprecated_at,
		       ka.review_required, ka.title, ka.content, ka.summary,
		       ka.embedding, ka.embedding_model_id, ka.meta,
		       ka.endorsement_count, ka.access_count, ka.last_accessed,
		       ka.version, ka.previous_version, ka.source_memory_id, ka.source_ref,
		       ka.created_at, ka.updated_at
		FROM knowledge_artifacts ka
		JOIN artifact_digest_sources ads ON ads.digest_id = ka.id
		WHERE ads.source_id = $1
		  AND ka.status = 'published'
		ORDER BY ka.created_at DESC`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("db: list digests for source: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeArtifactRows(rows)
}

// GetSuppressedSourceIDs returns source artifact IDs that are covered by at
// least one published digest in digestIDs. Used for post-recall suppression.
func GetSuppressedSourceIDs(ctx context.Context, pool *pgxpool.Pool, digestIDs []uuid.UUID) (map[uuid.UUID]struct{}, error) {
	if len(digestIDs) == 0 {
		return nil, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT ads.source_id
		FROM artifact_digest_sources ads
		JOIN knowledge_artifacts ka ON ka.id = ads.digest_id
		WHERE ads.digest_id = ANY($1)
		  AND ka.status = 'published'`, digestIDs)
	if err != nil {
		return nil, fmt.Errorf("db: get suppressed source ids: %w", err)
	}
	defer rows.Close()
	out := make(map[uuid.UUID]struct{})
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("db: get suppressed source ids scan: %w", err)
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// ScopeInLineage reports whether scopeA and scopeB are in the same ltree
// lineage (one is an ancestor or descendant of the other, or they are equal).
func ScopeInLineage(ctx context.Context, pool *pgxpool.Pool, scopeA, scopeB uuid.UUID) (bool, error) {
	if scopeA == scopeB {
		return true, nil
	}
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM scopes a, scopes b
		    WHERE a.id = $1 AND b.id = $2
		      AND (a.path @> b.path OR b.path @> a.path)
		)`, scopeA, scopeB).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("db: scope lineage check: %w", err)
	}
	return ok, nil
}

// FlagDigestsStaleness inserts a staleness_flags row for every published digest
// that covers sourceID, skipping digests that already have an open flag with
// the same signal (ON CONFLICT DO NOTHING via the partial unique index).
func FlagDigestsStaleness(ctx context.Context, pool *pgxpool.Pool, sourceID uuid.UUID, signal string, confidence float64, evidence []byte) error {
	if evidence == nil {
		evidence = []byte("{}")
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO staleness_flags (artifact_id, signal, confidence, evidence)
		SELECT ads.digest_id, $2, $3, $4
		FROM artifact_digest_sources ads
		JOIN knowledge_artifacts ka ON ka.id = ads.digest_id
		WHERE ads.source_id = $1
		  AND ka.status = 'published'
		  AND NOT EXISTS (
		      SELECT 1 FROM staleness_flags sf
		      WHERE sf.artifact_id = ads.digest_id
		        AND sf.signal = $2
		        AND sf.status = 'open'
		  )`,
		sourceID, signal, confidence, evidence)
	if err != nil {
		return fmt.Errorf("db: flag digests staleness: %w", err)
	}
	return nil
}

// InsertDigestLog records a synthesis audit entry.
func InsertDigestLog(ctx context.Context, pool *pgxpool.Pool, l *DigestLog) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO knowledge_digest_log
		    (scope_id, digest_id, source_ids, strategy, synthesised_by)
		VALUES ($1, $2, $3, $4, $5)`,
		l.ScopeID, l.DigestID, l.SourceIDs, l.Strategy, l.SynthesisedBy)
	if err != nil {
		return fmt.Errorf("db: insert digest log: %w", err)
	}
	return nil
}

// scanKnowledgeArtifactRows is a shared scanner for knowledge artifact SELECT results.
func scanKnowledgeArtifactRows(rows pgx.Rows) ([]*KnowledgeArtifact, error) {
	var out []*KnowledgeArtifact
	for rows.Next() {
		var a KnowledgeArtifact
		if err := rows.Scan(
			&a.ID, &a.KnowledgeType, &a.OwnerScopeID, &a.AuthorID,
			&a.Visibility, &a.Status, &a.PublishedAt, &a.DeprecatedAt,
			&a.ReviewRequired, &a.Title, &a.Content, &a.Summary,
			&a.Embedding, &a.EmbeddingModelID, &a.Meta,
			&a.EndorsementCount, &a.AccessCount, &a.LastAccessed,
			&a.Version, &a.PreviousVersion, &a.SourceMemoryID, &a.SourceRef,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan knowledge artifact: %w", err)
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

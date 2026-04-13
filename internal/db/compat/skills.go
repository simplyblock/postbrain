package compat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreateSkill inserts a new skill row.
func CreateSkill(ctx context.Context, pool *pgxpool.Pool, s *db.Skill) (*db.Skill, error) {
	q := db.New(pool)
	result, err := q.CreateSkill(ctx, db.CreateSkillParams{
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
func GetSkill(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.Skill, error) {
	q := db.New(pool)
	s, err := q.GetSkill(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// GetSkillBySlug retrieves a skill by scope and slug. Returns nil, nil if not found.
func GetSkillBySlug(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, slug string) (*db.Skill, error) {
	q := db.New(pool)
	s, err := q.GetSkillBySlug(ctx, db.GetSkillBySlugParams{
		ScopeID: scopeID,
		Slug:    slug,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// UpdateSkillContent updates the body, parameters, embedding, and bumps version.
func UpdateSkillContent(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, body string, parameters []byte, embedding []float32, modelID *uuid.UUID) (*db.Skill, error) {
	q := db.New(pool)
	s, err := q.UpdateSkillContent(ctx, db.UpdateSkillContentParams{
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
	q := db.New(pool)
	return q.UpdateSkillStatus(ctx, db.UpdateSkillStatusParams{
		ID:           id,
		Status:       status,
		PublishedAt:  publishedAt,
		DeprecatedAt: deprecatedAt,
	})
}

// SnapshotSkillVersion inserts a skill_history row.
func SnapshotSkillVersion(ctx context.Context, pool *pgxpool.Pool, h *db.SkillHistory) error {
	q := db.New(pool)
	return q.SnapshotSkillVersion(ctx, db.SnapshotSkillVersionParams{
		SkillID:    h.SkillID,
		Version:    h.Version,
		Body:       h.Body,
		Parameters: h.Parameters,
		ChangedBy:  h.ChangedBy,
		ChangeNote: h.ChangeNote,
	})
}

// GetSkillHistory returns the version history for a skill, ordered descending by version.
func GetSkillHistory(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID) ([]*db.SkillHistory, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, skill_id, version, body, parameters, changed_by, change_note, created_at
		 FROM skill_history WHERE skill_id=$1 ORDER BY version DESC`,
		skillID,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get skill history: %w", err)
	}
	defer rows.Close()
	var items []*db.SkillHistory
	for rows.Next() {
		var h db.SkillHistory
		if err := rows.Scan(&h.ID, &h.SkillID, &h.Version, &h.Body, &h.Parameters,
			&h.ChangedBy, &h.ChangeNote, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: get skill history scan: %w", err)
		}
		items = append(items, &h)
	}
	return items, rows.Err()
}

// CreateSkillEndorsement inserts a skill_endorsements row.
func CreateSkillEndorsement(ctx context.Context, pool *pgxpool.Pool, skillID, endorserID uuid.UUID, note *string) (*db.SkillEndorsement, error) {
	q := db.New(pool)
	e, err := q.CreateSkillEndorsement(ctx, db.CreateSkillEndorsementParams{
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
func GetSkillEndorsementByEndorser(ctx context.Context, pool *pgxpool.Pool, skillID, endorserID uuid.UUID) (*db.SkillEndorsement, error) {
	q := db.New(pool)
	e, err := q.GetSkillEndorsementByEndorser(ctx, db.GetSkillEndorsementByEndorserParams{
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
	q := db.New(pool)
	count, err := q.CountSkillEndorsements(ctx, skillID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// RecallSkillsByVector retrieves published skills by vector similarity.
func RecallSkillsByVector(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, queryVec []float32, agentType string, limit int) ([]db.SkillScore, error) {
	q := db.New(pool)
	rows, err := q.RecallSkillsByVector(ctx, db.RecallSkillsByVectorParams{
		Embedding: vecPtr(queryVec),
		Column2:   scopeIDs,
		Column3:   agentType,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	results := make([]db.SkillScore, len(rows))
	for i, r := range rows {
		skill := skillFromRecallByVectorRow(r)
		var score float64
		if f, ok := r.Score.(float64); ok {
			score = f
		} else if f32, ok := r.Score.(float32); ok {
			score = float64(f32)
		}
		results[i] = db.SkillScore{
			Skill: skill,
			Score: score,
		}
	}
	return results, nil
}

// RecallSkillsByFTS retrieves published skills via full-text search.
func RecallSkillsByFTS(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query, agentType string, limit int) ([]db.SkillScore, error) {
	q := db.New(pool)
	rows, err := q.RecallSkillsByFTS(ctx, db.RecallSkillsByFTSParams{
		PlaintoTsquery: query,
		Column2:        scopeIDs,
		Column3:        agentType,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	results := make([]db.SkillScore, len(rows))
	for i, r := range rows {
		skill := skillFromRecallByFTSRow(r)
		results[i] = db.SkillScore{
			Skill: skill,
			Score: float64(r.Score),
		}
	}
	return results, nil
}

// RecallSkillsByTrigram retrieves published skills via trigram similarity.
func RecallSkillsByTrigram(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, query, agentType string, limit int) ([]db.SkillScore, error) {
	q := db.New(pool)
	rows, err := q.RecallSkillsByTrigram(ctx, db.RecallSkillsByTrigramParams{
		Similarity: query,
		Column2:    scopeIDs,
		Column3:    agentType,
		Limit:      int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("db: recall skills by trigram: %w", err)
	}
	results := make([]db.SkillScore, len(rows))
	for i, r := range rows {
		skill := skillFromRecallByTrigramRow(r)
		results[i] = db.SkillScore{
			Skill: skill,
			Score: float64(r.Score),
		}
	}
	return results, nil
}

// ListPublishedSkillsForAgent returns all published skills for the given agent type.
func ListPublishedSkillsForAgent(ctx context.Context, pool *pgxpool.Pool, scopeIDs []uuid.UUID, agentType string) ([]*db.Skill, error) {
	q := db.New(pool)
	ss, err := q.ListPublishedSkillsForAgent(ctx, db.ListPublishedSkillsForAgentParams{
		Column1: scopeIDs,
		Column2: agentType,
	})
	if err != nil {
		return nil, err
	}
	return ss, nil
}

// ── private row converters ────────────────────────────────────────────────────

func skillFromRecallByVectorRow(r *db.RecallSkillsByVectorRow) *db.Skill {
	return &db.Skill{
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

func skillFromRecallByFTSRow(r *db.RecallSkillsByFTSRow) *db.Skill {
	return &db.Skill{
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

func skillFromRecallByTrigramRow(r *db.RecallSkillsByTrigramRow) *db.Skill {
	return &db.Skill{
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

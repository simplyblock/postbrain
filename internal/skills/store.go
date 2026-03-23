// Package skills provides creation, lifecycle management, recall, installation,
// and invocation of versioned parameterised prompt templates (skills).
package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// skillCreator abstracts the db.CreateSkill call so the store can be unit-tested
// without a real database connection.
type skillCreator interface {
	createSkill(ctx context.Context, s *db.Skill) (*db.Skill, error)
}

// poolCreator wraps a real pgxpool.Pool to implement skillCreator.
type poolCreator struct {
	pool *pgxpool.Pool
}

func (p *poolCreator) createSkill(ctx context.Context, s *db.Skill) (*db.Skill, error) {
	return db.CreateSkill(ctx, p.pool, s)
}

// Store provides skill CRUD and related operations.
type Store struct {
	pool    *pgxpool.Pool
	svc     *embedding.EmbeddingService
	creator skillCreator // injectable for tests
}

// NewStore creates a new Store backed by the given pool and embedding service.
func NewStore(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Store {
	return &Store{
		pool:    pool,
		svc:     svc,
		creator: &poolCreator{pool: pool},
	}
}

// CreateInput holds the fields required to create a new skill.
type CreateInput struct {
	ScopeID          uuid.UUID
	AuthorID         uuid.UUID
	SourceArtifactID *uuid.UUID
	Slug             string
	Name             string
	Description      string
	AgentTypes       []string // default ["any"] if nil/empty
	Body             string
	Parameters       []db.SkillParameter
	Visibility       string
	ReviewRequired   int // default 1 if 0
}

// Create persists a new skill in draft status, embedding its description+body.
func (s *Store) Create(ctx context.Context, input CreateInput) (*db.Skill, error) {
	// Apply defaults.
	if len(input.AgentTypes) == 0 {
		input.AgentTypes = []string{"any"}
	}
	if input.ReviewRequired == 0 {
		input.ReviewRequired = 1
	}

	// Serialize parameters to JSON.
	paramsJSON, err := json.Marshal(input.Parameters)
	if err != nil {
		return nil, fmt.Errorf("skills: marshal parameters: %w", err)
	}

	// Embed description + body using text model.
	embeddingVec, err := s.embedText(ctx, input.Description+" "+input.Body)
	if err != nil {
		return nil, fmt.Errorf("skills: embed: %w", err)
	}

	embVec := pgvector.NewVector(embeddingVec)
	skill := &db.Skill{
		ScopeID:          input.ScopeID,
		AuthorID:         input.AuthorID,
		SourceArtifactID: input.SourceArtifactID,
		Slug:             input.Slug,
		Name:             input.Name,
		Description:      input.Description,
		AgentTypes:       input.AgentTypes,
		Body:             input.Body,
		Parameters:       paramsJSON,
		Visibility:       input.Visibility,
		Status:           "draft",
		ReviewRequired:   int32(input.ReviewRequired),
		Version:          1,
		Embedding:        &embVec,
	}

	created, err := s.creator.createSkill(ctx, skill)
	if err != nil {
		return nil, fmt.Errorf("skills: create: %w", err)
	}
	return created, nil
}

// Update re-embeds, snapshots the current version to history, then updates the skill.
func (s *Store) Update(ctx context.Context, id uuid.UUID, callerID uuid.UUID, body string, params []db.SkillParameter) (*db.Skill, error) {
	existing, err := db.GetSkill(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("skills: update get: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("skills: update: skill %s not found", id)
	}

	// Snapshot the current version.
	if err := db.SnapshotSkillVersion(ctx, s.pool, &db.SkillHistory{
		SkillID:    id,
		Version:    existing.Version,
		Body:       existing.Body,
		Parameters: existing.Parameters,
		ChangedBy:  callerID,
	}); err != nil {
		return nil, fmt.Errorf("skills: snapshot: %w", err)
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("skills: marshal parameters: %w", err)
	}

	embeddingVec, err := s.embedText(ctx, existing.Description+" "+body)
	if err != nil {
		return nil, fmt.Errorf("skills: embed: %w", err)
	}

	updated, err := db.UpdateSkillContent(ctx, s.pool, id, body, paramsJSON, embeddingVec, existing.EmbeddingModelID)
	if err != nil {
		return nil, fmt.Errorf("skills: update content: %w", err)
	}
	return updated, nil
}

// GetBySlug retrieves a skill by scope + slug. Returns nil, nil if not found.
func (s *Store) GetBySlug(ctx context.Context, scopeID uuid.UUID, slug string) (*db.Skill, error) {
	skill, err := db.GetSkillBySlug(ctx, s.pool, scopeID, slug)
	if err != nil {
		return nil, fmt.Errorf("skills: get by slug: %w", err)
	}
	return skill, nil
}

// GetByID retrieves a skill by ID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*db.Skill, error) {
	skill, err := db.GetSkill(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("skills: get by id: %w", err)
	}
	return skill, nil
}

// embedText embeds text, tolerating a nil service (unit test path).
func (s *Store) embedText(ctx context.Context, text string) ([]float32, error) {
	if s.svc == nil {
		return nil, nil
	}
	return s.svc.EmbedText(ctx, text)
}

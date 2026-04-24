// Package skills provides creation, lifecycle management, recall, installation,
// and invocation of versioned parameterised prompt templates (skills).
package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/providers"
)

var ErrEmptyEmbedding = errors.New("skills: empty embedding result")

// skillCreator abstracts the compat.CreateSkill call so the store can be unit-tested
// without a real database connection.
type skillCreator interface {
	createSkill(ctx context.Context, s *db.Skill) (*db.Skill, error)
}

// poolCreator wraps a real pgxpool.Pool to implement skillCreator.
type poolCreator struct {
	pool *pgxpool.Pool
}

func (p *poolCreator) createSkill(ctx context.Context, s *db.Skill) (*db.Skill, error) {
	return compat.CreateSkill(ctx, p.pool, s)
}

// Store provides skill CRUD and related operations.
type Store struct {
	pool    *pgxpool.Pool
	svc     *providers.EmbeddingService
	creator skillCreator // injectable for tests
	repo    *db.EmbeddingRepository
}

// NewStore creates a new Store backed by the given pool and embedding service.
func NewStore(pool *pgxpool.Pool, svc *providers.EmbeddingService) *Store {
	return &Store{
		pool:    pool,
		svc:     svc,
		creator: &poolCreator{pool: pool},
		repo:    db.NewEmbeddingRepository(pool),
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
	Status           string
	PublishedAt      *time.Time
	// Files are optional supplementary files to attach to the skill.
	// Each path must pass ValidateSkillFile; requires a non-nil pool.
	Files []db.SkillFileInput
}

// UpdateInput holds optional fields for UpdateWithFiles.
// Files semantics: nil = leave existing files untouched;
// non-nil (even empty slice) = replace all files with the provided set.
type UpdateInput struct {
	Body       string
	Parameters []db.SkillParameter
	Files      *[]db.SkillFileInput
}

// Create persists a new skill in draft status, embedding its description+body.
// When Files are provided, the skill row and all supplementary files are written
// in a single transaction so a mid-file failure cannot leave an orphaned skill.
func (s *Store) Create(ctx context.Context, input CreateInput) (*db.Skill, error) {
	// Apply defaults.
	if len(input.AgentTypes) == 0 {
		input.AgentTypes = []string{"any"}
	}
	if input.ReviewRequired == 0 {
		input.ReviewRequired = 1
	}
	if input.Status == "" {
		input.Status = "draft"
	}

	// Validate supplementary files before any expensive work or DB writes.
	for _, f := range input.Files {
		if err := ValidateSkillFile(f); err != nil {
			return nil, fmt.Errorf("skills: create: %w", err)
		}
	}
	// Fail fast: file persistence requires a pool.
	if len(input.Files) > 0 && s.pool == nil {
		return nil, fmt.Errorf("skills: create: supplementary files require a database pool")
	}

	// Serialize parameters to JSON.
	paramsJSON, err := json.Marshal(input.Parameters)
	if err != nil {
		return nil, fmt.Errorf("skills: marshal parameters: %w", err)
	}

	// Embed description + body using text model.
	embeddingVec, modelID, err := s.embedText(ctx, input.Description+" "+input.Body)
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
		Status:           input.Status,
		PublishedAt:      input.PublishedAt,
		ReviewRequired:   int32(input.ReviewRequired),
		Version:          1,
		Embedding:        &embVec,
		EmbeddingModelID: modelID,
	}

	var created *db.Skill
	if len(input.Files) > 0 {
		// Atomically insert the skill row and all supplementary files so that
		// a failure mid-way (e.g. constraint violation on file N) rolls back the
		// entire operation and does not leave an orphaned, file-less skill.
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("skills: create begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		q := db.New(tx)
		created, err = q.CreateSkill(ctx, db.CreateSkillParams{
			ScopeID:          skill.ScopeID,
			AuthorID:         skill.AuthorID,
			Column3:          skill.SourceArtifactID,
			Slug:             skill.Slug,
			Name:             skill.Name,
			Description:      skill.Description,
			AgentTypes:       skill.AgentTypes,
			Body:             skill.Body,
			Parameters:       skill.Parameters,
			Visibility:       skill.Visibility,
			Status:           skill.Status,
			PublishedAt:      skill.PublishedAt,
			DeprecatedAt:     skill.DeprecatedAt,
			ReviewRequired:   skill.ReviewRequired,
			Version:          skill.Version,
			Column16:         skill.PreviousVersion,
			Embedding:        skill.Embedding,
			EmbeddingModelID: skill.EmbeddingModelID,
		})
		if err != nil {
			return nil, fmt.Errorf("skills: create: %w", err)
		}
		for _, f := range input.Files {
			if _, err := q.UpsertSkillFile(ctx, db.UpsertSkillFileParams{
				SkillID:      created.ID,
				RelativePath: f.RelativePath,
				Content:      f.Content,
				IsExecutable: f.IsExecutable,
			}); err != nil {
				return nil, fmt.Errorf("skills: create upsert file %q: %w", f.RelativePath, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("skills: create commit: %w", err)
		}
	} else {
		created, err = s.creator.createSkill(ctx, skill)
		if err != nil {
			return nil, fmt.Errorf("skills: create: %w", err)
		}
	}

	if err := s.dualWriteSkillEmbedding(ctx, created.ID, created.ScopeID, embeddingVec, modelID); err != nil {
		return nil, fmt.Errorf("skills: dual-write create: %w", err)
	}

	return created, nil
}

// Update re-embeds, snapshots the current version to history, then updates the skill.
// Files are left untouched; use UpdateWithFiles to also replace supplementary files.
func (s *Store) Update(ctx context.Context, id uuid.UUID, callerID uuid.UUID, body string, params []db.SkillParameter) (*db.Skill, error) {
	return s.UpdateWithFiles(ctx, id, callerID, UpdateInput{Body: body, Parameters: params, Files: nil})
}

// UpdateWithFiles snapshots the current version (body, params, files), then updates
// the skill content. If input.Files is non-nil, all existing supplementary files are
// replaced with the provided set (empty slice deletes all files).
// The full sequence — snapshot, content update, and file replacement — executes in
// a single transaction so a partial failure cannot leave the skill in an inconsistent
// state or diverge the file-history snapshot from the final content.
func (s *Store) UpdateWithFiles(ctx context.Context, id uuid.UUID, callerID uuid.UUID, input UpdateInput) (*db.Skill, error) {
	// Validate supplementary files before any DB writes.
	if input.Files != nil {
		for _, f := range *input.Files {
			if err := ValidateSkillFile(f); err != nil {
				return nil, fmt.Errorf("skills: update: %w", err)
			}
		}
	}

	// Read the current skill outside the transaction; GetSkill is a plain SELECT.
	existing, err := compat.GetSkill(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("skills: update get: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("skills: update: skill %s not found", id)
	}

	// Serialize and embed outside the transaction — both are pure CPU / network
	// work that must not hold a DB connection open.
	paramsJSON, err := json.Marshal(input.Parameters)
	if err != nil {
		return nil, fmt.Errorf("skills: marshal parameters: %w", err)
	}
	embeddingVec, modelID, err := s.embedText(ctx, existing.Description+" "+input.Body)
	if err != nil {
		return nil, fmt.Errorf("skills: embed: %w", err)
	}
	embVec := pgvector.NewVector(embeddingVec)

	// Atomically: snapshot history, update content, replace files.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("skills: update begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)

	// Snapshot current body/params version.
	if err := q.SnapshotSkillVersion(ctx, db.SnapshotSkillVersionParams{
		SkillID:    id,
		Version:    existing.Version,
		Body:       existing.Body,
		Parameters: existing.Parameters,
		ChangedBy:  callerID,
	}); err != nil {
		return nil, fmt.Errorf("skills: snapshot: %w", err)
	}

	// Snapshot supplementary files for this version.
	if err := q.SnapshotSkillFiles(ctx, db.SnapshotSkillFilesParams{
		SkillID: id,
		Version: existing.Version,
	}); err != nil {
		return nil, fmt.Errorf("skills: snapshot files: %w", err)
	}

	updated, err := q.UpdateSkillContent(ctx, db.UpdateSkillContentParams{
		ID:               id,
		Body:             input.Body,
		Parameters:       paramsJSON,
		Embedding:        &embVec,
		EmbeddingModelID: modelID,
	})
	if err != nil {
		return nil, fmt.Errorf("skills: update content: %w", err)
	}

	// Replace supplementary files if explicitly provided.
	if input.Files != nil {
		if err := q.DeleteAllSkillFiles(ctx, id); err != nil {
			return nil, fmt.Errorf("skills: update delete files: %w", err)
		}
		for _, f := range *input.Files {
			if _, err := q.UpsertSkillFile(ctx, db.UpsertSkillFileParams{
				SkillID:      id,
				RelativePath: f.RelativePath,
				Content:      f.Content,
				IsExecutable: f.IsExecutable,
			}); err != nil {
				return nil, fmt.Errorf("skills: update upsert file %q: %w", f.RelativePath, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("skills: update commit: %w", err)
	}

	if err := s.dualWriteSkillEmbedding(ctx, updated.ID, updated.ScopeID, embeddingVec, modelID); err != nil {
		return nil, fmt.Errorf("skills: dual-write update: %w", err)
	}

	return updated, nil
}

// GetBySlug retrieves a skill by scope + slug. Returns nil, nil if not found.
func (s *Store) GetBySlug(ctx context.Context, scopeID uuid.UUID, slug string) (*db.Skill, error) {
	skill, err := compat.GetSkillBySlug(ctx, s.pool, scopeID, slug)
	if err != nil {
		return nil, fmt.Errorf("skills: get by slug: %w", err)
	}
	return skill, nil
}

// GetByID retrieves a skill by ID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*db.Skill, error) {
	skill, err := compat.GetSkill(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("skills: get by id: %w", err)
	}
	return skill, nil
}

// embedText embeds text, tolerating a nil service (unit test path).
func (s *Store) embedText(ctx context.Context, text string) ([]float32, *uuid.UUID, error) {
	if s.svc == nil {
		return nil, nil, fmt.Errorf("skills: embedding service is not configured")
	}
	res, err := s.svc.EmbedTextResult(ctx, text)
	if err != nil {
		return nil, nil, err
	}
	if res == nil {
		return nil, nil, ErrEmptyEmbedding
	}
	if len(res.Embedding) == 0 {
		return nil, nil, ErrEmptyEmbedding
	}
	if res.ModelID != uuid.Nil {
		modelID := res.ModelID
		return res.Embedding, &modelID, nil
	}
	return res.Embedding, nil, nil
}

func (s *Store) dualWriteSkillEmbedding(
	ctx context.Context,
	skillID, scopeID uuid.UUID,
	embeddingVec []float32,
	modelID *uuid.UUID,
) error {
	return db.UpsertEmbeddingIfPresent(ctx, s.repo, "skill", skillID, scopeID, embeddingVec, modelID)
}

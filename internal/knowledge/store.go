// Package knowledge provides creation, lifecycle management, recall, and
// collection management for curated knowledge artifacts.
package knowledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// Sentinel errors for the knowledge store.
var (
	ErrNotEditable = errors.New("knowledge: artifact cannot be edited in current status")
	ErrNotFound    = errors.New("knowledge: artifact not found")
)

// embeddingService is the subset of embedding.EmbeddingService used by this package.
type embeddingService interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
	TextEmbedder() embeddingIface
}

// embeddingIface is the subset of embedding.Embedder needed to read the model slug.
// embedding.Embedder satisfies this interface.
type embeddingIface interface {
	ModelSlug() string
}

// embeddingServiceAdapter wraps *embedding.EmbeddingService to satisfy embeddingService.
// This is needed because TextEmbedder() returns embedding.Embedder (a concrete interface)
// while our local embeddingService expects the narrower embeddingIface.
type embeddingServiceAdapter struct {
	svc *embedding.EmbeddingService
}

func (a *embeddingServiceAdapter) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return a.svc.EmbedText(ctx, text)
}

func (a *embeddingServiceAdapter) TextEmbedder() embeddingIface {
	return a.svc.TextEmbedder()
}

// artifactCreator abstracts db.CreateArtifact so the store can be unit-tested
// without a real database connection.
type artifactCreator interface {
	createArtifact(ctx context.Context, a *db.KnowledgeArtifact) (*db.KnowledgeArtifact, error)
}

// artifactGetter abstracts db.GetArtifact.
type artifactGetter interface {
	getArtifact(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error)
}

// poolArtifactCreator wraps a real pgxpool.Pool to implement artifactCreator.
type poolArtifactCreator struct {
	pool *pgxpool.Pool
}

func (p *poolArtifactCreator) createArtifact(ctx context.Context, a *db.KnowledgeArtifact) (*db.KnowledgeArtifact, error) {
	return db.CreateArtifact(ctx, p.pool, a)
}

// poolArtifactGetter wraps a real pgxpool.Pool to implement artifactGetter.
type poolArtifactGetter struct {
	pool *pgxpool.Pool
}

func (p *poolArtifactGetter) getArtifact(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error) {
	return db.GetArtifact(ctx, p.pool, id)
}

// Store provides knowledge artifact CRUD operations.
type Store struct {
	pool    *pgxpool.Pool
	svc     embeddingService
	creator artifactCreator
	getter  artifactGetter
}

// NewStore creates a new Store backed by the given pool and embedding service.
func NewStore(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Store {
	return &Store{
		pool:    pool,
		svc:     &embeddingServiceAdapter{svc: svc},
		creator: &poolArtifactCreator{pool: pool},
		getter:  &poolArtifactGetter{pool: pool},
	}
}

// CreateInput holds the fields required to create a new knowledge artifact.
type CreateInput struct {
	KnowledgeType  string
	OwnerScopeID   uuid.UUID
	AuthorID       uuid.UUID
	Visibility     string
	Title          string
	Content        string
	Summary        *string
	SourceMemoryID *uuid.UUID
	SourceRef      *string
	AutoReview     bool // if true: status = "in_review"; else "draft"
	ReviewRequired int  // default 1 if 0
}

// Create persists a new knowledge artifact, embedding its content.
// If input.Summary is nil and the content exceeds 150 words, an extractive
// summary is generated automatically.
func (s *Store) Create(ctx context.Context, input CreateInput) (*db.KnowledgeArtifact, error) {
	if input.ReviewRequired == 0 {
		input.ReviewRequired = 1
	}

	if input.Summary == nil {
		if sum := Summarize(input.Content, 150); sum != input.Content {
			input.Summary = &sum
		}
	}

	status := "draft"
	if input.AutoReview {
		status = "in_review"
	}

	embeddingVec, modelID, err := s.embedContent(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("knowledge: embed: %w", err)
	}

	var srcMemID uuid.UUID
	if input.SourceMemoryID != nil {
		srcMemID = *input.SourceMemoryID
	}
	var embModelID uuid.UUID
	if modelID != nil {
		embModelID = *modelID
	}
	artifact := &db.KnowledgeArtifact{
		KnowledgeType:    input.KnowledgeType,
		OwnerScopeID:     input.OwnerScopeID,
		AuthorID:         input.AuthorID,
		Visibility:       input.Visibility,
		Status:           status,
		ReviewRequired:   int32(input.ReviewRequired),
		Title:            input.Title,
		Content:          input.Content,
		Summary:          input.Summary,
		Embedding:        pgvector.NewVector(embeddingVec),
		EmbeddingModelID: embModelID,
		Version:          1,
		SourceMemoryID:   srcMemID,
		SourceRef:        input.SourceRef,
	}

	created, err := s.creator.createArtifact(ctx, artifact)
	if err != nil {
		return nil, fmt.Errorf("knowledge: create: %w", err)
	}
	return created, nil
}

// Update re-embeds and persists updated content for a draft or in_review artifact.
// Published artifacts must not be edited; use the knowledge lifecycle to manage transitions.
// callerID is recorded in the history snapshot when the artifact was previously published.
func (s *Store) Update(ctx context.Context, id, callerID uuid.UUID, title, content string, summary *string) (*db.KnowledgeArtifact, error) {
	existing, err := s.getter.getArtifact(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("knowledge: update get: %w", err)
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.Status != "draft" && existing.Status != "in_review" {
		return nil, ErrNotEditable
	}

	embeddingVec, modelID, err := s.embedContent(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("knowledge: embed: %w", err)
	}

	// Snapshot before update when in_review (informational; not strictly enforced here).
	if existing.Status == "in_review" && s.pool != nil {
		_ = db.SnapshotArtifactVersion(ctx, s.pool, &db.KnowledgeHistory{
			ArtifactID: id,
			Version:    existing.Version,
			Content:    existing.Content,
			Summary:    existing.Summary,
			ChangedBy:  callerID,
		})
	}

	updated, err := db.UpdateArtifact(ctx, s.pool, id, title, content, summary, embeddingVec, modelID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: update: %w", err)
	}
	return updated, nil
}

// GetByID retrieves a knowledge artifact by ID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*db.KnowledgeArtifact, error) {
	a, err := db.GetArtifact(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("knowledge: get by id: %w", err)
	}
	return a, nil
}

// embedContent embeds text and returns the vector plus the model ID (if any).
// Tolerates a nil service (unit test path).
func (s *Store) embedContent(ctx context.Context, text string) ([]float32, *uuid.UUID, error) {
	if s.svc == nil {
		return nil, nil, nil
	}
	vec, err := s.svc.EmbedText(ctx, text)
	if err != nil {
		return nil, nil, err
	}
	// Resolve the active model ID from the DB by the embedder's slug.
	if s.pool != nil {
		slug := s.svc.TextEmbedder().ModelSlug()
		q := db.New(s.pool)
		model, err := q.GetActiveTextModel(ctx)
		if err == nil && model != nil && model.Slug == slug {
			return vec, &model.ID, nil
		}
	}
	return vec, nil, nil
}

// Package knowledge provides creation, lifecycle management, recall, and
// collection management for curated knowledge artifacts.
package knowledge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/chunking"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/graph"
)

// Sentinel errors for the knowledge store.
var (
	ErrNotEditable = errors.New("knowledge: artifact cannot be edited in current status")
	ErrNotFound    = errors.New("knowledge: artifact not found")
)

// embeddingService is the subset of embedding.EmbeddingService used by this package.
type embeddingService interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
	Summarize(ctx context.Context, text string) (string, error)
	Analyze(ctx context.Context, text string) (*embedding.DocumentAnalysis, error)
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

func (a *embeddingServiceAdapter) Summarize(ctx context.Context, text string) (string, error) {
	return a.svc.Summarize(ctx, text)
}

func (a *embeddingServiceAdapter) Analyze(ctx context.Context, text string) (*embedding.DocumentAnalysis, error) {
	return a.svc.Analyze(ctx, text)
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
	AutoReview     bool // if true and !AutoPublish: status = "in_review"
	AutoPublish    bool // if true: status = "published", skips review
	ReviewRequired int  // default 1 if 0; ignored when AutoPublish is true
}

// Create persists a new knowledge artifact, embedding its content.
// If input.Summary is nil and the content exceeds 150 words, an extractive
// summary is generated automatically.
func (s *Store) Create(ctx context.Context, input CreateInput) (*db.KnowledgeArtifact, error) {
	if input.ReviewRequired == 0 {
		input.ReviewRequired = 1
	}

	summary, entities := s.analyzeContent(ctx, input.Content, input.SourceRef)
	if input.Summary == nil && summary != "" {
		input.Summary = &summary
	}

	status := "draft"
	if input.AutoPublish {
		status = "published"
	} else if input.AutoReview {
		status = "in_review"
	}

	var publishedAt *time.Time
	if input.AutoPublish {
		now := time.Now()
		publishedAt = &now
	}

	embeddingVec, modelID, err := s.embedContent(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("knowledge: embed: %w", err)
	}

	embVec := pgvector.NewVector(embeddingVec)
	artifact := &db.KnowledgeArtifact{
		KnowledgeType:    input.KnowledgeType,
		OwnerScopeID:     input.OwnerScopeID,
		AuthorID:         input.AuthorID,
		Visibility:       input.Visibility,
		Status:           status,
		PublishedAt:      publishedAt,
		ReviewRequired:   int32(input.ReviewRequired),
		Title:            input.Title,
		Content:          input.Content,
		Summary:          input.Summary,
		Embedding:        &embVec,
		EmbeddingModelID: modelID,
		Version:          1,
		SourceMemoryID:   input.SourceMemoryID,
		SourceRef:        input.SourceRef,
	}

	created, err := s.creator.createArtifact(ctx, artifact)
	if err != nil {
		return nil, fmt.Errorf("knowledge: create: %w", err)
	}

	// Best-effort: link extracted entities and their co-occurrence relations.
	s.linkExtractedEntities(ctx, created.ID, input.OwnerScopeID, entities)

	// Best-effort: create chunk memories so recall can surface specific
	// passages rather than the whole document's averaged embedding.
	if utf8.RuneCountInString(input.Content) > chunking.MinContentRunes {
		s.createChunks(ctx, created.ID, input.OwnerScopeID, input.AuthorID, input.Content)
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

	// Flag covering digests stale when a published source is updated — non-fatal.
	if existing.Status == "published" && s.pool != nil {
		evidence := []byte(`{"signal":"source_modified"}`)
		_ = db.FlagDigestsStaleness(ctx, s.pool, id, "source_modified", 0.8, evidence)
	}

	// Re-extract entities from updated content — best-effort, non-fatal.
	_, entities := s.analyzeContent(ctx, content, existing.SourceRef)
	s.linkExtractedEntities(ctx, id, existing.OwnerScopeID, entities)

	return updated, nil
}

// analyzeContent attempts a combined LLM summarize+extract call. On success it
// returns the generated summary and the LLM-extracted entities. When no LLM
// is configured, or when the call fails, it falls back to extractive
// summarization (knowledge.Summarize) and heuristic entity extraction
// (graph.ExtractEntitiesFromMemory). All errors from the LLM path are silently
// swallowed so a missing or misbehaving model never blocks a write.
func (s *Store) analyzeContent(ctx context.Context, content string, sourceRef *string) (string, []*db.Entity) {
	if s.svc != nil {
		analysis, err := s.svc.Analyze(ctx, content)
		if err != nil {
			slog.Warn("knowledge: analyze failed, falling back to heuristics", "error", err)
		} else if analysis != nil && analysis.Summary != "" {
			entities := make([]*db.Entity, 0, len(analysis.Entities))
			for _, e := range analysis.Entities {
				if e.Type == "" || e.Canonical == "" {
					continue
				}
				name := strings.ToLower(e.Name)
				if name == "" {
					name = strings.ToLower(e.Canonical)
				}
				entities = append(entities, &db.Entity{
					EntityType: e.Type,
					Name:       name,
					Canonical:  strings.ToLower(e.Canonical),
				})
			}
			return analysis.Summary, entities
		}
	}

	// Fallback: summarize and extract separately.
	var summary string
	if s.svc != nil {
		if sum, err := s.svc.Summarize(ctx, content); err == nil {
			summary = sum
		}
	}
	if summary == "" {
		if sum := Summarize(content, 150); sum != content {
			summary = sum
		}
	}
	return summary, graph.ExtractEntitiesFromMemory(content, sourceRef)
}

// linkExtractedEntities upserts each entity into the graph, links it to the
// artifact, and creates co_occurs_with relations between all co-extracted
// entities. All errors are silently dropped — graph population is best-effort
// and must never block a write.
func (s *Store) linkExtractedEntities(ctx context.Context, artifactID, scopeID uuid.UUID, entities []*db.Entity) {
	if len(entities) == 0 || s.pool == nil {
		return
	}

	linkedIDs := make([]uuid.UUID, 0, len(entities))
	for _, e := range entities {
		e.ScopeID = scopeID
		upserted, err := db.UpsertEntity(ctx, s.pool, e)
		if err != nil {
			continue
		}
		_ = db.LinkArtifactToEntity(ctx, s.pool, artifactID, upserted.ID, "related")
		linkedIDs = append(linkedIDs, upserted.ID)

		// Connect to sibling entities that share the same canonical but a
		// different type (e.g. concept:postgresql ↔ technology:postgresql).
		siblings, err := db.ListEntitiesByCanonical(ctx, s.pool, scopeID, e.Canonical, e.EntityType)
		if err == nil {
			for _, sib := range siblings {
				// Always store with the lesser ID as subject to avoid
				// creating both (A→B) and (B→A) for the same pair.
				subj, obj := upserted.ID, sib.ID
				if bytes.Compare(subj[:], obj[:]) > 0 {
					subj, obj = obj, subj
				}
				_, _ = db.UpsertRelation(ctx, s.pool, &db.Relation{
					ScopeID:    scopeID,
					SubjectID:  subj,
					Predicate:  "same_as",
					ObjectID:   obj,
					Confidence: 1.0,
				})
			}
		}
	}

	artID := artifactID
	for i := 0; i < len(linkedIDs); i++ {
		for j := i + 1; j < len(linkedIDs); j++ {
			_, _ = db.UpsertRelation(ctx, s.pool, &db.Relation{
				ScopeID:        scopeID,
				SubjectID:      linkedIDs[i],
				Predicate:      "co_occurs_with",
				ObjectID:       linkedIDs[j],
				Confidence:     1.0,
				SourceArtifact: &artID,
			})
		}
	}
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
	if len(vec) == 0 {
		return nil, nil, fmt.Errorf("embedding service returned empty vector (is the model available?)")
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

// createChunks splits a large artifact's content into overlapping segments and
// stores each one as a memory with source_ref "artifact:<id>:chunk:<n>".
// This lets the memory recall path surface specific passages via their own
// embeddings without replacing the artifact's own whole-document embedding.
// Errors are logged but not returned — the artifact has already been persisted.
func (s *Store) createChunks(ctx context.Context, artifactID, scopeID, authorID uuid.UUID, content string) {
	if s.pool == nil {
		return
	}
	chunks := chunking.Chunk(content, chunking.DefaultChunkRunes, chunking.DefaultOverlap)
	if len(chunks) <= 1 {
		return
	}
	for i, chunk := range chunks {
		vec, err := s.svc.EmbedText(ctx, chunk)
		if err != nil {
			slog.WarnContext(ctx, "knowledge: chunk embed failed", "artifact_id", artifactID, "chunk", i, "err", err)
			continue
		}
		ref := fmt.Sprintf("artifact:%s:chunk:%d", artifactID, i)
		v := pgvector.NewVector(vec)
		m := &db.Memory{
			MemoryType:      "semantic",
			ScopeID:         scopeID,
			AuthorID:        authorID,
			Content:         chunk,
			ContentKind:     "text",
			Embedding:       &v,
			SourceRef:       &ref,
			PromotionStatus: "none",
		}
		if _, err := db.CreateMemory(ctx, s.pool, m); err != nil {
			slog.WarnContext(ctx, "knowledge: chunk store failed", "artifact_id", artifactID, "chunk", i, "err", err)
		}
	}
	slog.InfoContext(ctx, "knowledge: created chunks", "artifact_id", artifactID, "count", len(chunks))
}

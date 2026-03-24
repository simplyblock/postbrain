package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// Sentinel errors for synthesis validation.
var (
	ErrDigestSource  = errors.New("knowledge: digest sources may not themselves be digests")
	ErrScopeLineage  = errors.New("knowledge: source scope is not in the lineage of the digest scope")
	ErrTooFewSources = errors.New("knowledge: synthesis requires at least 2 source artifacts")
	ErrSourceNotPublished = errors.New("knowledge: all source artifacts must be published")
)

// SynthesisInput holds parameters for creating a topic digest.
type SynthesisInput struct {
	ScopeID    uuid.UUID
	AuthorID   uuid.UUID
	SourceIDs  []uuid.UUID
	Title      string // if empty, inferred from sources
	Visibility string // defaults to "team"
	AutoReview bool
}

// Synthesiser creates topic digest artifacts from multiple source artifacts.
type Synthesiser struct {
	pool *pgxpool.Pool
	svc  embeddingService
}

// NewSynthesiser creates a Synthesiser backed by the given pool and embedding service.
func NewSynthesiser(pool *pgxpool.Pool, svc *embedding.EmbeddingService) *Synthesiser {
	return &Synthesiser{
		pool: pool,
		svc:  &embeddingServiceAdapter{svc: svc},
	}
}

// Create synthesises a topic digest from the given source artifact IDs.
// All sources must be published, non-digest, and within the lineage of ScopeID.
func (s *Synthesiser) Create(ctx context.Context, input SynthesisInput) (*db.KnowledgeArtifact, error) {
	if len(input.SourceIDs) < 2 {
		return nil, ErrTooFewSources
	}
	if input.Visibility == "" {
		input.Visibility = "team"
	}

	// Fetch and validate each source.
	sources := make([]*db.KnowledgeArtifact, 0, len(input.SourceIDs))
	for _, id := range input.SourceIDs {
		a, err := db.GetArtifact(ctx, s.pool, id)
		if err != nil {
			return nil, fmt.Errorf("knowledge: fetch source %v: %w", id, err)
		}
		if a == nil {
			return nil, fmt.Errorf("knowledge: source artifact %v not found", id)
		}
		if a.Status != "published" {
			return nil, ErrSourceNotPublished
		}
		if a.KnowledgeType == "digest" {
			return nil, ErrDigestSource
		}
		sources = append(sources, a)
	}

	// Validate scope lineage.
	if err := s.validateLineage(ctx, input.ScopeID, sources); err != nil {
		return nil, err
	}

	// Synthesise content from source summaries.
	content, err := s.synthesiseContent(ctx, sources)
	if err != nil {
		return nil, fmt.Errorf("knowledge: synthesise content: %w", err)
	}

	title := input.Title
	if title == "" {
		title = inferTitle(sources)
	}

	// Auto-generate summary.
	var summary *string
	if sum := s.summarizeContent(ctx, content); sum != "" {
		summary = &sum
	}

	status := "draft"
	if input.AutoReview {
		status = "in_review"
	}

	// Embed.
	vec, modelID, err := s.embedContent(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("knowledge: embed digest: %w", err)
	}

	var embVec *pgvector.Vector
	if vec != nil {
		v := pgvector.NewVector(vec)
		embVec = &v
	}

	artifact := &db.KnowledgeArtifact{
		KnowledgeType:    "digest",
		OwnerScopeID:     input.ScopeID,
		AuthorID:         input.AuthorID,
		Visibility:       input.Visibility,
		Status:           status,
		ReviewRequired:   1,
		Title:            title,
		Content:          content,
		Summary:          summary,
		Embedding:        embVec,
		EmbeddingModelID: modelID,
		Version:          1,
	}

	created, err := db.CreateArtifact(ctx, s.pool, artifact)
	if err != nil {
		return nil, fmt.Errorf("knowledge: create digest artifact: %w", err)
	}

	// Record source links.
	if err := db.InsertDigestSources(ctx, s.pool, created.ID, input.SourceIDs); err != nil {
		return nil, fmt.Errorf("knowledge: record digest sources: %w", err)
	}

	// Audit log.
	if err := db.InsertDigestLog(ctx, s.pool, &db.DigestLog{
		ScopeID:       input.ScopeID,
		DigestID:      created.ID,
		SourceIDs:     input.SourceIDs,
		Strategy:      "manual",
		SynthesisedBy: &input.AuthorID,
	}); err != nil {
		return nil, fmt.Errorf("knowledge: record digest log: %w", err)
	}

	return created, nil
}

// ListSources returns the source artifacts for a digest.
func (s *Synthesiser) ListSources(ctx context.Context, digestID uuid.UUID) ([]*db.KnowledgeArtifact, error) {
	return db.ListDigestSources(ctx, s.pool, digestID)
}

// ListDigests returns published digests that cover a given source artifact.
func (s *Synthesiser) ListDigests(ctx context.Context, sourceID uuid.UUID) ([]*db.KnowledgeArtifact, error) {
	return db.ListDigestsForSource(ctx, s.pool, sourceID)
}

// validateLineage checks every source scope is an ancestor or descendant of the digest scope.
func (s *Synthesiser) validateLineage(ctx context.Context, digestScopeID uuid.UUID, sources []*db.KnowledgeArtifact) error {
	for _, src := range sources {
		if src.OwnerScopeID == digestScopeID {
			continue
		}
		ok, err := db.ScopeInLineage(ctx, s.pool, digestScopeID, src.OwnerScopeID)
		if err != nil {
			return fmt.Errorf("knowledge: scope lineage check: %w", err)
		}
		if !ok {
			return ErrScopeLineage
		}
	}
	return nil
}

// synthesiseContent builds digest content from source summaries (or lead extracts),
// optionally using the LLM to produce a unified narrative.
func (s *Synthesiser) synthesiseContent(ctx context.Context, sources []*db.KnowledgeArtifact) (string, error) {
	var parts []string
	for i, src := range sources {
		text := src.Content
		if src.Summary != nil && *src.Summary != "" {
			text = *src.Summary
		}
		parts = append(parts, fmt.Sprintf("--- Source %d: %s ---\n%s", i+1, src.Title, text))
	}
	combined := strings.Join(parts, "\n\n")

	if s.svc != nil {
		prompt := "Synthesise the following knowledge documents into a single, unified, non-redundant summary. " +
			"Preserve all distinct facts. Do not add new information. Write in plain prose.\n\n" + combined
		if synthesised, err := s.svc.Summarize(ctx, prompt); err == nil && synthesised != "" {
			return synthesised, nil
		}
	}
	// Fallback: concatenated summaries.
	return combined, nil
}

// summarizeContent mirrors Store.summarizeContent.
func (s *Synthesiser) summarizeContent(ctx context.Context, content string) string {
	if s.svc != nil {
		if sum, err := s.svc.Summarize(ctx, content); err == nil && sum != "" {
			return sum
		}
	}
	sum := Summarize(content, 150)
	if sum == content {
		return ""
	}
	return sum
}

// embedContent mirrors Store.embedContent.
func (s *Synthesiser) embedContent(ctx context.Context, text string) ([]float32, *uuid.UUID, error) {
	if s.svc == nil {
		return nil, nil, nil
	}
	vec, err := s.svc.EmbedText(ctx, text)
	if err != nil {
		return nil, nil, err
	}
	if len(vec) == 0 {
		return nil, nil, fmt.Errorf("embedding service returned empty vector")
	}
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

// inferTitle produces a default digest title from source titles.
func inferTitle(sources []*db.KnowledgeArtifact) string {
	if len(sources) == 0 {
		return "Digest"
	}
	if len(sources) == 1 {
		return "Digest: " + sources[0].Title
	}
	return fmt.Sprintf("Digest: %s (+%d more)", sources[0].Title, len(sources)-1)
}

package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// ── embedding service helpers ─────────────────────────────────────────────────

// newFakeEmbeddingService returns an embeddingService backed by embedding.FakeEmbedder.
// Different input texts produce distinct, deterministic vectors.
func newFakeEmbeddingService() embeddingService {
	svc := embedding.NewServiceFromEmbedders(embedding.NewFakeEmbedder(4), nil)
	return &embeddingServiceAdapter{svc: svc}
}

// noopEmbeddingService satisfies embeddingService with silent no-ops.
// Embed individual tests override only the method under test.
type noopEmbeddingService struct{}

func (noopEmbeddingService) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}
func (noopEmbeddingService) Summarize(_ context.Context, _ string) (string, error) { return "", nil }
func (noopEmbeddingService) Analyze(_ context.Context, _ string) (*embedding.DocumentAnalysis, error) {
	return nil, nil
}
func (noopEmbeddingService) TextEmbedder() embeddingIface { return &fakeEmbedderIface{slug: "noop"} }

type fakeEmbedderIface struct{ slug string }

func (f *fakeEmbedderIface) ModelSlug() string { return f.slug }

// errorEmbeddingService wraps noopEmbeddingService but always fails EmbedText.
type errorEmbeddingService struct{ noopEmbeddingService }

func (errorEmbeddingService) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embed failed")
}

// ── artifact fakes ────────────────────────────────────────────────────────────

type fakeArtifactCreator struct {
	created *db.KnowledgeArtifact
	err     error
}

func (f *fakeArtifactCreator) createArtifact(_ context.Context, a *db.KnowledgeArtifact) (*db.KnowledgeArtifact, error) {
	if f.err != nil {
		return nil, f.err
	}
	a.ID = uuid.New()
	f.created = a
	return a, nil
}

type fakeArtifactGetter struct {
	artifact *db.KnowledgeArtifact
	err      error
}

func (f *fakeArtifactGetter) getArtifact(_ context.Context, _ uuid.UUID) (*db.KnowledgeArtifact, error) {
	return f.artifact, f.err
}

type fakeArtifactUpdater struct {
	updated *db.KnowledgeArtifact
	err     error
}

func (f *fakeArtifactUpdater) updateArtifact(_ context.Context, id uuid.UUID, title, content string, summary *string, _ []float32, _ *uuid.UUID) (*db.KnowledgeArtifact, error) {
	if f.err != nil {
		return nil, f.err
	}
	a := &db.KnowledgeArtifact{ID: id, Title: title, Content: content, Summary: summary, Status: "draft"}
	f.updated = a
	return a, nil
}

// ── store constructor ─────────────────────────────────────────────────────────

func newTestStore() (*Store, *fakeArtifactCreator) {
	creator := &fakeArtifactCreator{}
	s := &Store{
		svc:     newFakeEmbeddingService(),
		creator: creator,
		getter:  &fakeArtifactGetter{},
		updater: &fakeArtifactUpdater{},
	}
	return s, creator
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestCreate_DraftStatus(t *testing.T) {
	t.Parallel()
	s, creator := newTestStore()

	_, err := s.Create(context.Background(), CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  uuid.New(),
		AuthorID:      uuid.New(),
		Visibility:    "team",
		Title:         "Test",
		Content:       "some content",
		AutoReview:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.created.Status != "draft" {
		t.Errorf("expected status=draft, got %s", creator.created.Status)
	}
}

func TestCreate_AutoReviewStatus(t *testing.T) {
	t.Parallel()
	s, creator := newTestStore()

	_, err := s.Create(context.Background(), CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  uuid.New(),
		AuthorID:      uuid.New(),
		Visibility:    "team",
		Title:         "Test",
		Content:       "some content",
		AutoReview:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.created.Status != "in_review" {
		t.Errorf("expected status=in_review, got %s", creator.created.Status)
	}
}

func TestCreate_DefaultReviewRequired(t *testing.T) {
	t.Parallel()
	s, creator := newTestStore()

	_, err := s.Create(context.Background(), CreateInput{
		KnowledgeType:  "semantic",
		OwnerScopeID:   uuid.New(),
		AuthorID:       uuid.New(),
		Visibility:     "team",
		Title:          "Test",
		Content:        "some content",
		ReviewRequired: 0, // should default to 1
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.created.ReviewRequired != 1 {
		t.Errorf("expected ReviewRequired=1, got %d", creator.created.ReviewRequired)
	}
}

func TestCreate_AutoPublish_SetsPublishedStatusAndTimestamp(t *testing.T) {
	t.Parallel()
	s, creator := newTestStore()

	_, err := s.Create(context.Background(), CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  uuid.New(),
		AuthorID:      uuid.New(),
		Visibility:    "public",
		Title:         "Published Doc",
		Content:       "some content",
		AutoPublish:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creator.created.Status != "published" {
		t.Errorf("status = %q, want %q", creator.created.Status, "published")
	}
	if creator.created.PublishedAt == nil {
		t.Error("PublishedAt = nil, want non-nil timestamp")
	}
}

func TestCreate_EmbedErrorPropagated(t *testing.T) {
	t.Parallel()
	creator := &fakeArtifactCreator{}
	s := &Store{
		svc:     errorEmbeddingService{},
		creator: creator,
	}

	_, err := s.Create(context.Background(), CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  uuid.New(),
		AuthorID:      uuid.New(),
		Visibility:    "team",
		Title:         "Test",
		Content:       "some content",
	})
	if err == nil {
		t.Fatal("expected error from failing embedder, got nil")
	}
}

func TestCreate_CreatorErrorPropagated(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("db write failed")
	creator := &fakeArtifactCreator{err: wantErr}
	s := &Store{
		svc:     newFakeEmbeddingService(),
		creator: creator,
	}

	_, err := s.Create(context.Background(), CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  uuid.New(),
		AuthorID:      uuid.New(),
		Visibility:    "team",
		Title:         "Test",
		Content:       "some content",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped %v, got %v", wantErr, err)
	}
}

// ── Update tests ──────────────────────────────────────────────────────────────

func TestUpdate_PublishedArtifactReturnsErrNotEditable(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore()
	s.getter = &fakeArtifactGetter{
		artifact: &db.KnowledgeArtifact{
			ID:     uuid.New(),
			Status: "published",
		},
	}

	_, err := s.Update(context.Background(), uuid.New(), uuid.New(), "title", "content", nil)
	if !errors.Is(err, ErrNotEditable) {
		t.Errorf("expected ErrNotEditable, got %v", err)
	}
}

func TestUpdate_NilGetterResultReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore()
	s.getter = &fakeArtifactGetter{artifact: nil}

	_, err := s.Update(context.Background(), uuid.New(), uuid.New(), "title", "content", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdate_DraftArtifactSucceeds(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	updater := &fakeArtifactUpdater{}
	s := &Store{
		svc: newFakeEmbeddingService(),
		getter: &fakeArtifactGetter{
			artifact: &db.KnowledgeArtifact{ID: id, Status: "draft"},
		},
		updater: updater,
	}

	got, err := s.Update(context.Background(), id, uuid.New(), "New Title", "updated content", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Content != "updated content" {
		t.Errorf("Content = %q, want %q", got.Content, "updated content")
	}
	if got.Title != "New Title" {
		t.Errorf("Title = %q, want %q", got.Title, "New Title")
	}
}

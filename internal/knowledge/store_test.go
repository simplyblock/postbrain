package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakeEmbedder implements embeddingService for unit tests.
type fakeEmbedder struct {
	vec []float32
	err error
}

func (f *fakeEmbedder) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return f.vec, f.err
}

func (f *fakeEmbedder) TextEmbedder() embeddingIface {
	return &fakeEmbedderIface{slug: "test-model"}
}

type fakeEmbedderIface struct{ slug string }

func (f *fakeEmbedderIface) ModelSlug() string { return f.slug }

// fakeArtifactCreator implements artifactCreator for unit tests.
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

func newTestStore() (*Store, *fakeEmbedder, *fakeArtifactCreator) {
	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3}}
	creator := &fakeArtifactCreator{}
	s := &Store{
		svc:     emb,
		creator: creator,
	}
	return s, emb, creator
}

func TestCreate_DraftStatus(t *testing.T) {
	t.Parallel()
	s, _, creator := newTestStore()

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
	s, _, creator := newTestStore()

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
	s, _, creator := newTestStore()

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

func TestUpdate_PublishedArtifactReturnsErrNotEditable(t *testing.T) {
	t.Parallel()
	s, _, _ := newTestStore()

	// Inject a fake getter that returns a published artifact.
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

// fakeArtifactGetter implements artifactGetter for unit tests.
type fakeArtifactGetter struct {
	artifact *db.KnowledgeArtifact
	err      error
}

func (f *fakeArtifactGetter) getArtifact(_ context.Context, _ uuid.UUID) (*db.KnowledgeArtifact, error) {
	return f.artifact, f.err
}

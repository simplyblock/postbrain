package memory

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// ── Mock embedding service ───────────────────────────────────────────────────

type mockEmbedder struct {
	slug string
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}
func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}
func (m *mockEmbedder) ModelSlug() string { return m.slug }
func (m *mockEmbedder) Dimensions() int   { return 3 }

var _ embedding.Embedder = (*mockEmbedder)(nil)

func newMockEmbeddingService(withCode bool) *embedding.EmbeddingService {
	text := &mockEmbedder{slug: "text-model"}
	if withCode {
		code := &mockEmbedder{slug: "code-model"}
		return embedding.NewServiceFromEmbedders(text, code)
	}
	return embedding.NewServiceFromEmbedders(text, nil)
}

// ── Mock memory creator (captures CreateMemory args) ────────────────────────

type capturedMemory struct {
	m *db.Memory
}

type mockCreator struct {
	created *capturedMemory
	stored  []*db.Memory // all soft-deleted memories
}

func (mc *mockCreator) CreateMemory(_ context.Context, m *db.Memory) (*db.Memory, error) {
	m.ID = uuid.New()
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	mc.created = &capturedMemory{m: m}
	return m, nil
}

func (mc *mockCreator) FindNearDuplicates(_ context.Context, _ uuid.UUID, _ []float32, _ float64, _ *uuid.UUID) ([]*db.Memory, error) {
	return nil, nil
}

func (mc *mockCreator) UpdateMemoryContent(_ context.Context, id uuid.UUID, content string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string) (*db.Memory, error) {
	return &db.Memory{ID: id, Content: content, ContentKind: contentKind}, nil
}

func (mc *mockCreator) SoftDeleteMemory(_ context.Context, id uuid.UUID) error {
	mc.stored = append(mc.stored, &db.Memory{ID: id})
	return nil
}

func (mc *mockCreator) UpsertEntity(_ context.Context, e *db.Entity) (*db.Entity, error) {
	e.ID = uuid.New()
	return e, nil
}

func (mc *mockCreator) LinkMemoryToEntity(_ context.Context, _, _ uuid.UUID, _ string) error {
	return nil
}

// ── Tests ────────────────────────────────────────────────────────────────────

func newTestStore(withCode bool, creator memoryDB) *Store {
	svc := newMockEmbeddingService(withCode)
	return &Store{
		svc:     &embeddingServiceAdapter{svc: svc},
		creator: creator,
	}
}

func TestCreate_WorkingMemory_DefaultTTL(t *testing.T) {
	mc := &mockCreator{}
	s := newTestStore(false, mc)

	before := time.Now()
	input := CreateInput{
		Content:    "test working memory",
		MemoryType: "working",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		Importance: 0.5,
	}
	result, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != "created" {
		t.Fatalf("expected action 'created', got %q", result.Action)
	}

	created := mc.created.m
	if created.ExpiresAt.IsZero() {
		t.Fatal("expected ExpiresAt to be set for working memory")
	}
	// Should be ~3600s from now.
	expected := before.Add(3600 * time.Second)
	diff := created.ExpiresAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Fatalf("ExpiresAt %v not within 5s of expected %v (diff=%v)", created.ExpiresAt, expected, diff)
	}
}

func TestCreate_WorkingMemory_ExplicitTTL(t *testing.T) {
	mc := &mockCreator{}
	s := newTestStore(false, mc)

	ttl := 7200
	before := time.Now()
	input := CreateInput{
		Content:    "test working memory explicit ttl",
		MemoryType: "working",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		ExpiresIn:  &ttl,
	}
	_, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	created := mc.created.m
	if created.ExpiresAt.IsZero() {
		t.Fatal("expected ExpiresAt to be set")
	}
	expected := before.Add(7200 * time.Second)
	diff := created.ExpiresAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Fatalf("ExpiresAt %v not within 5s of expected %v (diff=%v)", created.ExpiresAt, expected, diff)
	}
}

func TestCreate_NonWorkingMemory_ExpiresAtNil(t *testing.T) {
	mc := &mockCreator{}
	s := newTestStore(false, mc)

	ttl := 3600
	input := CreateInput{
		Content:    "semantic memory with ignored TTL",
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		ExpiresIn:  &ttl, // should be ignored
	}
	_, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	created := mc.created.m
	if !created.ExpiresAt.IsZero() {
		t.Fatalf("expected ExpiresAt to be zero for semantic memory, got %v", created.ExpiresAt)
	}
}

func TestCreate_CodeContent_CodeEmbeddingCalled(t *testing.T) {
	mc := &mockCreator{}
	// Provide a code embedder so code embedding is attempted.
	s := newTestStore(true, mc)

	// Use a file: source ref pointing to a .go file so content is classified as code.
	ref := "file:src/auth.go:10"
	input := CreateInput{
		Content:    "func main() { }",
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		SourceRef:  &ref,
	}
	_, err := s.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	created := mc.created.m
	if created.ContentKind != "code" {
		t.Fatalf("expected content_kind=code, got %q", created.ContentKind)
	}
	if len(created.EmbeddingCode.Slice()) == 0 {
		t.Fatal("expected EmbeddingCode to be populated for code content")
	}
}

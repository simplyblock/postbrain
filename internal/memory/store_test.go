package memory

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/chunking"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

// ── embedding service helpers ─────────────────────────────────────────────────

func newFakeEmbeddingService(withCode bool) *embedding.EmbeddingService {
	text := embedding.NewFakeEmbedder(4)
	if withCode {
		code := embedding.NewFakeEmbedder(4)
		return embedding.NewServiceFromEmbedders(text, code)
	}
	return embedding.NewServiceFromEmbedders(text, nil)
}

// newMockEmbeddingService is an alias kept for compatibility with consolidate_test.go
// and recall_test.go. New tests should use newFakeEmbeddingService directly.
var newMockEmbeddingService = newFakeEmbeddingService

// noopMemoryEmbeddingService satisfies embeddingService with silent no-ops.
// Test fakes embed it and override only the method under test.
type noopMemoryEmbeddingService struct{}

func (noopMemoryEmbeddingService) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}
func (noopMemoryEmbeddingService) EmbedCode(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}
func (noopMemoryEmbeddingService) TextEmbedder() embeddingIface {
	return embedding.NewFakeEmbedder(4)
}
func (noopMemoryEmbeddingService) CodeEmbedder() embeddingIface { return nil }

// errorEmbeddingService fails EmbedText.
type errorEmbeddingService struct{ noopMemoryEmbeddingService }

func (errorEmbeddingService) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embed failed")
}

// ── memoryDB mock ─────────────────────────────────────────────────────────────

type mockCreator struct {
	// All memories passed to CreateMemory, in call order.
	createdAll []*db.Memory
	// near-duplicates to return from FindNearDuplicates (nil = no dupes).
	dupes []*db.Memory
	// updated tracks calls to UpdateMemoryContent.
	updated []*db.Memory
	// upsertRelErr, if set, is returned by UpsertRelation.
	upsertRelErr error
	// instrumentation for graph-link assertions.
	upsertEntityCalls int
	linkEntityCalls   int
	upsertRelCalls    int
}

func (mc *mockCreator) CreateMemory(_ context.Context, m *db.Memory) (*db.Memory, error) {
	m.ID = uuid.New()
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	mc.createdAll = append(mc.createdAll, m)
	return m, nil
}

func (mc *mockCreator) FindNearDuplicates(_ context.Context, _ uuid.UUID, _ []float32, _ float64, _ *uuid.UUID) ([]*db.Memory, error) {
	return mc.dupes, nil
}

func (mc *mockCreator) UpdateMemoryContent(_ context.Context, id uuid.UUID, content string, summary *string, embedding, embeddingCode []float32, textModelID, codeModelID *uuid.UUID, contentKind string, meta []byte) (*db.Memory, error) {
	m := &db.Memory{ID: id, Content: content, Summary: summary, ContentKind: contentKind, Meta: meta}
	mc.updated = append(mc.updated, m)
	return m, nil
}

func (mc *mockCreator) SoftDeleteMemory(_ context.Context, id uuid.UUID) error {
	mc.createdAll = append(mc.createdAll, &db.Memory{ID: id})
	return nil
}

func (mc *mockCreator) UpsertEntity(_ context.Context, e *db.Entity) (*db.Entity, error) {
	mc.upsertEntityCalls++
	e.ID = uuid.New()
	return e, nil
}

func (mc *mockCreator) LinkMemoryToEntity(_ context.Context, _, _ uuid.UUID, _ string) error {
	mc.linkEntityCalls++
	return nil
}

func (mc *mockCreator) UpsertRelation(_ context.Context, r *db.Relation) (*db.Relation, error) {
	mc.upsertRelCalls++
	if mc.upsertRelErr != nil {
		return nil, mc.upsertRelErr
	}
	r.ID = uuid.New()
	return r, nil
}

func (mc *mockCreator) FindEntitiesBySuffix(_ context.Context, _ uuid.UUID, _ string) ([]*db.Entity, error) {
	return nil, nil
}

// ── store constructor ─────────────────────────────────────────────────────────

func newTestStore(withCode bool, creator memoryDB) *Store {
	svc := newFakeEmbeddingService(withCode)
	return &Store{
		svc:     &embeddingServiceAdapter{svc: svc},
		creator: creator,
	}
}

// ── TTL tests ─────────────────────────────────────────────────────────────────

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

	created := mc.createdAll[0]
	if created.ExpiresAt.IsZero() {
		t.Fatal("expected ExpiresAt to be set for working memory")
	}
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

	created := mc.createdAll[0]
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

	created := mc.createdAll[0]
	if created.ExpiresAt != nil {
		t.Fatalf("expected ExpiresAt to be nil for semantic memory, got %v", created.ExpiresAt)
	}
}

// ── code embedding test ───────────────────────────────────────────────────────

func TestCreate_CodeContent_CodeEmbeddingCalled(t *testing.T) {
	mc := &mockCreator{}
	s := newTestStore(true, mc)

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

	created := mc.createdAll[0]
	if created.ContentKind != "code" {
		t.Fatalf("expected content_kind=code, got %q", created.ContentKind)
	}
	if len(created.EmbeddingCode.Slice()) == 0 {
		t.Fatal("expected EmbeddingCode to be populated for code content")
	}
}

// ── near-duplicate tests ──────────────────────────────────────────────────────

func TestCreate_NearDuplicateFound_ActionIsUpdated(t *testing.T) {
	t.Parallel()
	existingID := uuid.New()
	mc := &mockCreator{
		dupes: []*db.Memory{{ID: existingID, Content: "similar content"}},
	}
	s := newTestStore(false, mc)

	result, err := s.Create(context.Background(), CreateInput{
		Content:    "nearly identical content",
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != "updated" {
		t.Errorf("Action = %q, want %q", result.Action, "updated")
	}
	if result.MemoryID != existingID {
		t.Errorf("MemoryID = %v, want existing ID %v", result.MemoryID, existingID)
	}
	if len(mc.updated) != 1 {
		t.Errorf("UpdateMemoryContent called %d times, want 1", len(mc.updated))
	}
	// No new memory should have been created.
	if len(mc.createdAll) != 0 {
		t.Errorf("CreateMemory called %d times, want 0 (duplicate path skips insert)", len(mc.createdAll))
	}
}

func TestCreate_NearDuplicateFound_StillLinksEntitiesAndRelations(t *testing.T) {
	t.Parallel()
	existingID := uuid.New()
	mc := &mockCreator{
		dupes: []*db.Memory{{ID: existingID, Content: "similar content"}},
	}
	s := newTestStore(false, mc)

	result, err := s.Create(context.Background(), CreateInput{
		Content:    "near duplicate but with explicit entities",
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		Entities: []EntityInput{
			{Name: "alpha", Type: "concept"},
			{Name: "beta", Type: "concept"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != "updated" {
		t.Fatalf("Action = %q, want %q", result.Action, "updated")
	}
	if result.MemoryID != existingID {
		t.Fatalf("MemoryID = %v, want existing ID %v", result.MemoryID, existingID)
	}
	if mc.upsertEntityCalls < 2 {
		t.Fatalf("UpsertEntity called %d times, want at least 2 for explicit entities", mc.upsertEntityCalls)
	}
	if mc.linkEntityCalls < 2 {
		t.Fatalf("LinkMemoryToEntity called %d times, want at least 2", mc.linkEntityCalls)
	}
	if mc.upsertRelCalls < 1 {
		t.Fatalf("UpsertRelation called %d times, want at least 1 co_occurs_with relation", mc.upsertRelCalls)
	}
}

// ── embed error test ──────────────────────────────────────────────────────────

func TestCreate_EmbedErrorPropagated(t *testing.T) {
	t.Parallel()
	mc := &mockCreator{}
	s := &Store{
		svc:     errorEmbeddingService{},
		creator: mc,
	}

	_, err := s.Create(context.Background(), CreateInput{
		Content:    "some content",
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error from failing embedder, got nil")
	}
	if !strings.Contains(err.Error(), "embed text") {
		t.Errorf("error %q should mention embed text", err.Error())
	}
}

// ── chunk backfill test ───────────────────────────────────────────────────────

func TestCreate_LargeContent_ChunkChildrenHaveParentID(t *testing.T) {
	t.Parallel()
	mc := &mockCreator{}
	s := newTestStore(false, mc)

	// Build content that exceeds MinContentRunes so chunking kicks in.
	// Use multiple long sentences so the chunker produces at least 2 chunks.
	sentenceRunes := chunking.MinContentRunes/4 + 50
	sentence := strings.Repeat("word ", sentenceRunes/5) + ". "
	content := strings.Repeat(sentence, 5) // well above MinContentRunes

	_, err := s.Create(context.Background(), CreateInput{
		Content:    content,
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First call is the parent memory, subsequent calls are chunk children.
	if len(mc.createdAll) < 2 {
		t.Fatalf("expected at least 2 CreateMemory calls (parent + chunks), got %d", len(mc.createdAll))
	}
	parentID := mc.createdAll[0].ID
	for i, m := range mc.createdAll[1:] {
		if m.ParentMemoryID == nil {
			t.Errorf("chunk[%d]: ParentMemoryID = nil, want %v", i, parentID)
			continue
		}
		if *m.ParentMemoryID != parentID {
			t.Errorf("chunk[%d]: ParentMemoryID = %v, want %v", i, *m.ParentMemoryID, parentID)
		}
	}
}

// ── UpsertRelation error logging ──────────────────────────────────────────────

// TestCreate_UpsertRelationError_LogsWarning verifies that a failure in the
// best-effort co_occurs_with upsert step is logged as a warning rather than
// silently discarded. The parent Create must still succeed.
//
// Not parallel — temporarily replaces the global slog default.
func TestCreate_UpsertRelationError_LogsWarning(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	mc := &mockCreator{upsertRelErr: errors.New("db relation error")}
	s := newTestStore(false, mc)

	// Two explicit entities guarantee the co_occurs_with loop runs.
	_, err := s.Create(context.Background(), CreateInput{
		Content:    "foo and bar coexist",
		MemoryType: "semantic",
		ScopeID:    uuid.New(),
		AuthorID:   uuid.New(),
		Entities: []EntityInput{
			{Name: "foo", Type: "concept"},
			{Name: "bar", Type: "concept"},
		},
	})
	if err != nil {
		t.Fatalf("Create should succeed even when UpsertRelation fails: %v", err)
	}

	logged := buf.String()
	if !strings.Contains(logged, "co_occurs_with") {
		t.Errorf("expected warning mentioning co_occurs_with in log output, got: %q", logged)
	}
}

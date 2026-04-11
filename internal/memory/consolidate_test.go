package memory

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
)

// vec makes a *pgvector.Vector from a plain float slice for test fixtures.
func vec(vals ...float32) *pgvector.Vector {
	v := pgvector.NewVector(vals)
	return &v
}

// ── cosineDist tests ─────────────────────────────────────────────────────────

func TestCosineDist_IdenticalVectors(t *testing.T) {
	a := []float32{1, 2, 3}
	dist := cosineDist(a, a)
	if math.Abs(dist) > 1e-6 {
		t.Fatalf("expected 0 for identical vectors, got %v", dist)
	}
}

func TestCosineDist_OrthogonalVectors(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	dist := cosineDist(a, b)
	if math.Abs(dist-1.0) > 1e-6 {
		t.Fatalf("expected 1 for orthogonal vectors, got %v", dist)
	}
}

func TestCosineDist_EmptyVector(t *testing.T) {
	dist := cosineDist(nil, []float32{1, 2, 3})
	if dist != 2.0 {
		t.Fatalf("expected 2.0 for nil vector, got %v", dist)
	}
}

// ── FindClusters tests ───────────────────────────────────────────────────────

// mockConsolidatorDB implements the minimal DB interface needed by Consolidator.
type mockConsolidatorDB struct {
	candidates    []*db.Memory
	softDeleted   []uuid.UUID
	created       *db.Memory
	consolidation *db.Consolidation
}

func (m *mockConsolidatorDB) ListConsolidationCandidates(_ context.Context, _ uuid.UUID) ([]*db.Memory, error) {
	return m.candidates, nil
}

func (m *mockConsolidatorDB) CreateMemory(_ context.Context, mem *db.Memory) (*db.Memory, error) {
	mem.ID = uuid.New()
	mem.CreatedAt = time.Now()
	m.created = mem
	return mem, nil
}

func (m *mockConsolidatorDB) SoftDeleteMemory(_ context.Context, id uuid.UUID) error {
	m.softDeleted = append(m.softDeleted, id)
	return nil
}

func (m *mockConsolidatorDB) CreateConsolidation(_ context.Context, c *db.Consolidation) (*db.Consolidation, error) {
	c.ID = uuid.New()
	m.consolidation = c
	return c, nil
}

func TestFindClusters_EmptyInput(t *testing.T) {
	mdb := &mockConsolidatorDB{candidates: nil}
	c := &Consolidator{
		svc: &embeddingServiceAdapter{svc: newMockEmbeddingService(false)},
		cdb: mdb,
	}
	clusters, err := c.FindClusters(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 0 {
		t.Fatalf("expected empty clusters for empty input, got %d", len(clusters))
	}
}

// TestFindClusters_SingleItem_NoCluster verifies that a single candidate memory
// never forms a cluster (clusters require at least 2 members).
func TestFindClusters_SingleItem_NoCluster(t *testing.T) {
	singleMem := &db.Memory{
		ID:        uuid.New(),
		Content:   "lonely memory",
		Embedding: vec(1, 0, 0, 0),
	}
	mdb := &mockConsolidatorDB{candidates: []*db.Memory{singleMem}}
	c := &Consolidator{
		svc: &embeddingServiceAdapter{svc: newMockEmbeddingService(false)},
		cdb: mdb,
	}

	clusters, err := c.FindClusters(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters for single candidate, got %d", len(clusters))
	}
}

// TestFindClusters_TwoIdentical_ProducesOneCluster verifies that two memories
// with identical embeddings (cosine distance = 0) are grouped into one cluster,
// and that MergeCluster then produces a single merged output.
func TestFindClusters_TwoIdentical_ProducesOneCluster(t *testing.T) {
	identical := vec(1, 0, 0, 0) // both memories share this embedding
	m1 := &db.Memory{ID: uuid.New(), Content: "first duplicate", Embedding: identical}
	m2 := &db.Memory{ID: uuid.New(), Content: "second duplicate", Embedding: identical}

	mdb := &mockConsolidatorDB{candidates: []*db.Memory{m1, m2}}
	c := &Consolidator{
		svc: &embeddingServiceAdapter{svc: newMockEmbeddingService(false)},
		cdb: mdb,
	}

	clusters, err := c.FindClusters(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("FindClusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if len(clusters[0]) != 2 {
		t.Errorf("cluster size = %d, want 2", len(clusters[0]))
	}

	// MergeCluster should produce exactly one merged memory.
	merged, err := c.MergeCluster(context.Background(), clusters[0], func(_ context.Context, contents []string) (string, error) {
		return "merged: " + contents[0], nil
	})
	if err != nil {
		t.Fatalf("MergeCluster: %v", err)
	}
	if merged == nil {
		t.Fatal("expected non-nil merged memory")
	}
	if mdb.created == nil {
		t.Fatal("expected CreateMemory to have been called")
	}
	// Both source memories must be soft-deleted.
	if len(mdb.softDeleted) != 2 {
		t.Errorf("soft-deleted count = %d, want 2", len(mdb.softDeleted))
	}
}

// TestFindClusters_MaxClusters_LimitsOutput verifies that when MaxClusters is
// set, FindClusters returns at most that many clusters even if more are found.
func TestFindClusters_MaxClusters_LimitsOutput(t *testing.T) {
	// Build 3 isolated near-duplicate pairs (6 memories, 3 expected clusters).
	// Each pair shares a distinct embedding that is orthogonal to every other pair.
	pairs := [][2]*pgvector.Vector{
		{vec(1, 0, 0, 0), vec(1, 0, 0, 0)}, // pair A
		{vec(0, 1, 0, 0), vec(0, 1, 0, 0)}, // pair B
		{vec(0, 0, 1, 0), vec(0, 0, 1, 0)}, // pair C
	}
	var candidates []*db.Memory
	for _, pair := range pairs {
		for _, emb := range pair {
			embCopy := *emb
			candidates = append(candidates, &db.Memory{
				ID:        uuid.New(),
				Content:   "content",
				Embedding: &embCopy,
			})
		}
	}

	mdb := &mockConsolidatorDB{candidates: candidates}
	c := &Consolidator{
		svc:         &embeddingServiceAdapter{svc: newMockEmbeddingService(false)},
		cdb:         mdb,
		MaxClusters: 2, // cap at 2 even though 3 clusters exist
	}

	clusters, err := c.FindClusters(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("FindClusters: %v", err)
	}
	if len(clusters) > 2 {
		t.Errorf("expected at most 2 clusters (MaxClusters=2), got %d", len(clusters))
	}
	if len(clusters) == 0 {
		t.Error("expected at least 1 cluster; 3 pairs were present")
	}
}

func TestMergeCluster_CallsSummarizerAndSoftDeletes(t *testing.T) {
	mdb := &mockConsolidatorDB{}
	c := &Consolidator{
		svc: &embeddingServiceAdapter{svc: newMockEmbeddingService(false)},
		cdb: mdb,
	}

	m1 := &db.Memory{ID: uuid.New(), Content: "memory one", Importance: 0.4, MemoryType: "episodic"}
	m2 := &db.Memory{ID: uuid.New(), Content: "memory two", Importance: 0.6, MemoryType: "episodic"}
	cluster := []*db.Memory{m1, m2}

	var summarized []string
	summarizer := func(_ context.Context, contents []string) (string, error) {
		summarized = contents
		return "merged summary", nil
	}

	result, err := c.MergeCluster(context.Background(), cluster, summarizer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(summarized) != 2 {
		t.Fatalf("expected summarizer called with 2 contents, got %d", len(summarized))
	}
	if len(mdb.softDeleted) != 2 {
		t.Fatalf("expected 2 soft deletes, got %d", len(mdb.softDeleted))
	}
	if mdb.consolidation == nil {
		t.Fatal("expected consolidation record to be created")
	}
}

type summaryLengthGuardEmbeddingService struct {
	noopMemoryEmbeddingService
	maxBytes int
	calls    []string
}

func (s *summaryLengthGuardEmbeddingService) EmbedText(_ context.Context, text string) ([]float32, error) {
	s.calls = append(s.calls, text)
	if len(text) > s.maxBytes {
		return nil, fmt.Errorf("openai: input[0] too long (%d bytes, max ~%d)", len(text), s.maxBytes)
	}
	return []float32{float32(len(text))}, nil
}

func TestMergeCluster_ChunkEmbedsOversizedSummaryBeforeEmbedding(t *testing.T) {
	mdb := &mockConsolidatorDB{}
	svc := &summaryLengthGuardEmbeddingService{maxBytes: 32000}
	c := &Consolidator{
		svc: svc,
		cdb: mdb,
	}

	scopeID := uuid.New()
	authorID := uuid.New()
	cluster := []*db.Memory{
		{ID: uuid.New(), ScopeID: scopeID, AuthorID: authorID, Content: "memory one", Importance: 0.2, MemoryType: "episodic"},
		{ID: uuid.New(), ScopeID: scopeID, AuthorID: authorID, Content: "memory two", Importance: 0.7, MemoryType: "episodic"},
	}

	oversized := strings.Repeat("a", 33000)
	summarizer := func(_ context.Context, _ []string) (string, error) {
		return oversized, nil
	}

	merged, err := c.MergeCluster(context.Background(), cluster, summarizer)
	if err != nil {
		t.Fatalf("expected oversized summary to be truncated and embedded, got error: %v", err)
	}
	if merged == nil {
		t.Fatal("expected non-nil merged memory")
	}
	if len(svc.calls) <= 1 {
		t.Fatalf("expected oversized summary to be embedded in multiple chunks, got %d call(s)", len(svc.calls))
	}
	for i, call := range svc.calls {
		if len(call) > 32000 {
			t.Fatalf("chunk embed input[%d] bytes = %d, exceeds limit", i, len(call))
		}
	}
	if mdb.created.Content != oversized {
		t.Fatal("expected stored summary content to remain untruncated")
	}
	if mdb.created.Embedding == nil {
		t.Fatal("expected created embedding to be set")
	}
	sum := 0.0
	for _, call := range svc.calls {
		sum += float64(len(call))
	}
	want := float32(sum / float64(len(svc.calls)))
	got := mdb.created.Embedding.Slice()[0]
	if math.Abs(float64(got-want)) > 1e-6 {
		t.Fatalf("pooled embedding[0] = %v, want %v", got, want)
	}
}

func TestMeanPoolEmbeddings_DimensionMismatch(t *testing.T) {
	_, err := meanPoolEmbeddings([][]float32{
		{1, 2},
		{3},
	})
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

package memory

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

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

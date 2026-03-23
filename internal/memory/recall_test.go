package memory

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

// ── DecayLambda tests ────────────────────────────────────────────────────────

func TestDecayLambda_Episodic(t *testing.T) {
	if got := DecayLambda("episodic"); got != 0.010 {
		t.Fatalf("expected 0.010, got %v", got)
	}
}

func TestDecayLambda_Semantic(t *testing.T) {
	if got := DecayLambda("semantic"); got != 0.005 {
		t.Fatalf("expected 0.005, got %v", got)
	}
}

func TestDecayLambda_Working(t *testing.T) {
	if got := DecayLambda("working"); got != 0.015 {
		t.Fatalf("expected 0.015, got %v", got)
	}
}

func TestDecayLambda_Unknown(t *testing.T) {
	if got := DecayLambda("unknown"); got != 0.005 {
		t.Fatalf("expected 0.005 for unknown type, got %v", got)
	}
}

// ── Score formula test ───────────────────────────────────────────────────────

func TestCombinedScore_Formula(t *testing.T) {
	vecScore := 0.8
	bm25Score := 0.6
	importance := 0.7
	recencyDecay := 0.9

	expected := 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recencyDecay
	got := combinedScore(vecScore, bm25Score, importance, recencyDecay)

	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

// ── Mock recall DB ───────────────────────────────────────────────────────────

type mockRecallDB struct {
	vecResults  []db.MemoryScore
	ftsResults  []db.MemoryScore
	codeResults []db.MemoryScore
	vecCalled   int
	ftsCalled   int
	codeCalled  int
}

func (m *mockRecallDB) RecallMemoriesByVector(_ context.Context, _ []uuid.UUID, _ []float32, _ int) ([]db.MemoryScore, error) {
	m.vecCalled++
	return m.vecResults, nil
}

func (m *mockRecallDB) RecallMemoriesByFTS(_ context.Context, _ []uuid.UUID, _ string, _ int) ([]db.MemoryScore, error) {
	m.ftsCalled++
	return m.ftsResults, nil
}

func (m *mockRecallDB) RecallMemoriesByCodeVector(_ context.Context, _ []uuid.UUID, _ []float32, _ int) ([]db.MemoryScore, error) {
	m.codeCalled++
	return m.codeResults, nil
}

func (m *mockRecallDB) IncrementMemoryAccess(_ context.Context, _ uuid.UUID) error {
	return nil
}

// mockScopeDB returns predetermined scope IDs.
type mockScopeDB struct {
	ids []uuid.UUID
}

func (m *mockScopeDB) GetAncestorScopeIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return m.ids, nil
}

func (m *mockScopeDB) PersonalScopeIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

// newTestRecallStore creates a Store wired for recall testing.
func newTestRecallStore(rdb recallDB) *Store {
	svc := newMockEmbeddingService(true)
	return &Store{
		svc:      &embeddingServiceAdapter{svc: svc},
		recallDB: rdb,
		fanOut:   staticFanOut(nil),
	}
}

// staticFanOut returns a fanOutFunc that always returns the given IDs (or derives
// from the input scopeID when ids is nil).
func staticFanOut(ids []uuid.UUID) fanOutFunc {
	return func(_ context.Context, scopeID, _ uuid.UUID, _ int, strict bool) ([]uuid.UUID, error) {
		if strict {
			return []uuid.UUID{scopeID}, nil
		}
		if ids != nil {
			return ids, nil
		}
		return []uuid.UUID{scopeID}, nil
	}
}

// makeMemory creates a db.Memory with minimal fields set.
func makeMemory(id uuid.UUID, memType string, importance float64) *db.Memory {
	now := time.Now()
	return &db.Memory{
		ID:           id,
		MemoryType:   memType,
		Importance:   importance,
		CreatedAt:    now,
		LastAccessed: now,
	}
}

// ── Recall unit tests ────────────────────────────────────────────────────────

func TestRecall_StrictScope_PassesSingleScopeID(t *testing.T) {
	scopeID := uuid.New()
	var capturedScopeIDs []uuid.UUID

	rdb := &mockRecallDB{}
	// Override vecResults to capture the scope IDs passed.
	s := newTestRecallStore(rdb)
	s.fanOut = func(_ context.Context, sid, _ uuid.UUID, _ int, strict bool) ([]uuid.UUID, error) {
		capturedScopeIDs = []uuid.UUID{sid}
		return capturedScopeIDs, nil
	}

	input := RecallInput{
		Query:       "test",
		ScopeID:     scopeID,
		StrictScope: true,
		SearchMode:  "text",
		Limit:       10,
	}
	_, err := s.Recall(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedScopeIDs) != 1 || capturedScopeIDs[0] != scopeID {
		t.Fatalf("expected [scopeID], got %v", capturedScopeIDs)
	}
}

func TestRecall_MinScore_FiltersResults(t *testing.T) {
	rdb := &mockRecallDB{
		vecResults: []db.MemoryScore{
			{Memory: makeMemory(uuid.New(), "semantic", 0.1), VecScore: 0.2},
			{Memory: makeMemory(uuid.New(), "semantic", 0.9), VecScore: 0.9},
		},
	}
	s := newTestRecallStore(rdb)

	input := RecallInput{
		Query:      "test",
		ScopeID:    uuid.New(),
		SearchMode: "text",
		Limit:      10,
		MinScore:   0.7,
	}
	results, err := s.Recall(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.Score < 0.7 {
			t.Fatalf("result with score %v should have been filtered", r.Score)
		}
	}
}

func TestRecall_HybridMode_CallsBothVecAndFTS(t *testing.T) {
	rdb := &mockRecallDB{}
	s := newTestRecallStore(rdb)

	input := RecallInput{
		Query:      "test",
		ScopeID:    uuid.New(),
		SearchMode: "hybrid",
		Limit:      10,
	}
	_, err := s.Recall(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rdb.vecCalled == 0 {
		t.Fatal("expected vector recall to be called")
	}
	if rdb.ftsCalled == 0 {
		t.Fatal("expected FTS recall to be called")
	}
}

func TestRecall_MemoryTypesFilter(t *testing.T) {
	semanticID := uuid.New()
	episodicID := uuid.New()
	rdb := &mockRecallDB{
		vecResults: []db.MemoryScore{
			{Memory: makeMemory(semanticID, "semantic", 0.8), VecScore: 0.9},
			{Memory: makeMemory(episodicID, "episodic", 0.8), VecScore: 0.8},
		},
	}
	s := newTestRecallStore(rdb)

	input := RecallInput{
		Query:       "test",
		ScopeID:     uuid.New(),
		SearchMode:  "text",
		MemoryTypes: []string{"semantic"},
		Limit:       10,
	}
	results, err := s.Recall(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.Memory.MemoryType != "semantic" {
			t.Fatalf("expected only semantic memories, got %q", r.Memory.MemoryType)
		}
	}
}

func TestRecall_DefaultSearchMode_IsHybrid(t *testing.T) {
	rdb := &mockRecallDB{}
	s := newTestRecallStore(rdb)

	// SearchMode left as zero value — should default to "hybrid".
	input := RecallInput{
		Query:   "test",
		ScopeID: uuid.New(),
		Limit:   10,
	}
	_, err := s.Recall(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rdb.vecCalled == 0 || rdb.ftsCalled == 0 {
		t.Fatalf("expected hybrid mode: vec=%d fts=%d", rdb.vecCalled, rdb.ftsCalled)
	}
}

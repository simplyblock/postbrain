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
	trgmScore := 0.5
	importance := 0.7
	recencyDecay := 0.9

	expected := 0.50*vecScore + 0.10*bm25Score + 0.10*trgmScore + 0.20*importance + 0.10*recencyDecay
	got := combinedScore(vecScore, bm25Score, trgmScore, importance, recencyDecay)

	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

// ── Mock recall DB ───────────────────────────────────────────────────────────

type mockRecallDB struct {
	vecResults   []db.MemoryScore
	ftsResults   []db.MemoryScore
	trgmResults  []db.MemoryScore
	codeResults  []db.MemoryScore
	vecCalled    int
	ftsCalled    int
	trgmCalled   int
	codeCalled   int
	lastScopeIDs []uuid.UUID
	lastSince    *time.Time
	lastUntil    *time.Time
}

func (m *mockRecallDB) RecallMemoriesByVector(_ context.Context, scopeIDs []uuid.UUID, _ []float32, _ int, since, until *time.Time) ([]db.MemoryScore, error) {
	m.vecCalled++
	m.lastScopeIDs = append([]uuid.UUID(nil), scopeIDs...)
	m.lastSince = since
	m.lastUntil = until
	return m.vecResults, nil
}

func (m *mockRecallDB) RecallMemoriesByFTS(_ context.Context, scopeIDs []uuid.UUID, _ string, _ int, since, until *time.Time) ([]db.MemoryScore, error) {
	m.ftsCalled++
	m.lastScopeIDs = append([]uuid.UUID(nil), scopeIDs...)
	m.lastSince = since
	m.lastUntil = until
	return m.ftsResults, nil
}

func (m *mockRecallDB) RecallMemoriesByTrigram(_ context.Context, scopeIDs []uuid.UUID, _ string, _ int, since, until *time.Time) ([]db.MemoryScore, error) {
	m.trgmCalled++
	m.lastScopeIDs = append([]uuid.UUID(nil), scopeIDs...)
	m.lastSince = since
	m.lastUntil = until
	return m.trgmResults, nil
}

func (m *mockRecallDB) RecallMemoriesByCodeVector(_ context.Context, scopeIDs []uuid.UUID, _ []float32, _ int, since, until *time.Time) ([]db.MemoryScore, error) {
	m.codeCalled++
	m.lastScopeIDs = append([]uuid.UUID(nil), scopeIDs...)
	m.lastSince = since
	m.lastUntil = until
	return m.codeResults, nil
}

func (m *mockRecallDB) IncrementMemoryAccess(_ context.Context, _ uuid.UUID) error {
	return nil
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
		LastAccessed: &now,
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

func TestRecall_CodeMode_FallsBackToFTSWhenCodeVectorEmpty(t *testing.T) {
	memID := uuid.New()
	rdb := &mockRecallDB{
		codeResults: nil,
		ftsResults: []db.MemoryScore{
			{Memory: makeMemory(memID, "semantic", 0.8), VecScore: 0.9},
		},
	}
	s := newTestRecallStore(rdb)

	results, err := s.Recall(context.Background(), RecallInput{
		Query:      "auth",
		ScopeID:    uuid.New(),
		SearchMode: "code",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rdb.codeCalled == 0 {
		t.Fatal("expected code-vector recall to be called")
	}
	if rdb.ftsCalled == 0 {
		t.Fatal("expected FTS fallback recall to be called")
	}
	if len(results) == 0 {
		t.Fatal("expected fallback results, got none")
	}
}

func TestRecall_CodeMode_DoesNotFallbackToFTSWhenCodeVectorHasResults(t *testing.T) {
	memID := uuid.New()
	rdb := &mockRecallDB{
		codeResults: []db.MemoryScore{
			{Memory: makeMemory(memID, "semantic", 0.8), VecScore: 0.9},
		},
		ftsResults: []db.MemoryScore{{Memory: makeMemory(uuid.New(), "semantic", 0.8), BM25Score: 0.7}},
	}
	s := newTestRecallStore(rdb)

	results, err := s.Recall(context.Background(), RecallInput{
		Query:      "auth",
		ScopeID:    uuid.New(),
		SearchMode: "code",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rdb.codeCalled == 0 {
		t.Fatal("expected code-vector recall to be called")
	}
	if rdb.ftsCalled != 0 {
		t.Fatalf("expected no FTS fallback when code results exist, got ftsCalled=%d", rdb.ftsCalled)
	}
	if len(results) == 0 {
		t.Fatal("expected code-vector results, got none")
	}
}

func TestRecall_IntersectAuthorizedScopeIDs(t *testing.T) {
	teamScope := uuid.New()
	ancestorScope := uuid.New()
	unrelatedScope := uuid.New()

	rdb := &mockRecallDB{}
	s := newTestRecallStore(rdb)
	s.fanOut = staticFanOut([]uuid.UUID{teamScope, ancestorScope, unrelatedScope})

	_, err := s.Recall(context.Background(), RecallInput{
		Query:              "test",
		ScopeID:            teamScope,
		SearchMode:         "text",
		Limit:              10,
		AuthorizedScopeIDs: []uuid.UUID{teamScope, unrelatedScope},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rdb.lastScopeIDs) != 2 {
		t.Fatalf("scopeIDs len = %d, want 2", len(rdb.lastScopeIDs))
	}
	if rdb.lastScopeIDs[0] != teamScope || rdb.lastScopeIDs[1] != unrelatedScope {
		t.Fatalf("scopeIDs = %v, want [%s %s]", rdb.lastScopeIDs, teamScope, unrelatedScope)
	}
}

func TestRecall_EmptyIntersectionSkipsDBQueries(t *testing.T) {
	teamScope := uuid.New()
	ancestorScope := uuid.New()

	rdb := &mockRecallDB{}
	s := newTestRecallStore(rdb)
	s.fanOut = staticFanOut([]uuid.UUID{teamScope, ancestorScope})

	results, err := s.Recall(context.Background(), RecallInput{
		Query:              "test",
		ScopeID:            teamScope,
		SearchMode:         "text",
		Limit:              10,
		AuthorizedScopeIDs: []uuid.UUID{uuid.New()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results len = %d, want 0", len(results))
	}
	if rdb.vecCalled != 0 || rdb.ftsCalled != 0 || rdb.trgmCalled != 0 || rdb.codeCalled != 0 {
		t.Fatalf("expected no DB recall calls, got vec=%d fts=%d trgm=%d code=%d", rdb.vecCalled, rdb.ftsCalled, rdb.trgmCalled, rdb.codeCalled)
	}
}

func TestRecall_TimeWindowSinceFiltersOlderMemories(t *testing.T) {
	now := time.Now().UTC()
	oldID := uuid.New()
	newID := uuid.New()
	rdb := &mockRecallDB{
		vecResults: []db.MemoryScore{
			{Memory: &db.Memory{ID: oldID, MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-48 * time.Hour)}, VecScore: 0.9},
			{Memory: &db.Memory{ID: newID, MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-2 * time.Hour)}, VecScore: 0.9},
		},
	}
	s := newTestRecallStore(rdb)
	since := now.Add(-24 * time.Hour)

	results, err := s.Recall(context.Background(), RecallInput{
		Query:      "time window",
		ScopeID:    uuid.New(),
		SearchMode: "text",
		Limit:      10,
		Since:      &since,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results)=%d, want 1", len(results))
	}
	if results[0].Memory.ID != newID {
		t.Fatalf("got memory %s, want %s", results[0].Memory.ID, newID)
	}
}

func TestRecall_TimeWindowUntilFiltersNewerMemories(t *testing.T) {
	now := time.Now().UTC()
	earlyID := uuid.New()
	lateID := uuid.New()
	rdb := &mockRecallDB{
		vecResults: []db.MemoryScore{
			{Memory: &db.Memory{ID: earlyID, MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-48 * time.Hour)}, VecScore: 0.9},
			{Memory: &db.Memory{ID: lateID, MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-2 * time.Hour)}, VecScore: 0.9},
		},
	}
	s := newTestRecallStore(rdb)
	until := now.Add(-24 * time.Hour)

	results, err := s.Recall(context.Background(), RecallInput{
		Query:      "time window",
		ScopeID:    uuid.New(),
		SearchMode: "text",
		Limit:      10,
		Until:      &until,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results)=%d, want 1", len(results))
	}
	if results[0].Memory.ID != earlyID {
		t.Fatalf("got memory %s, want %s", results[0].Memory.ID, earlyID)
	}
}

func TestRecall_TimeWindowForwardsToDBQueries(t *testing.T) {
	now := time.Now().UTC()
	since := now.Add(-48 * time.Hour)
	until := now.Add(-1 * time.Hour)
	rdb := &mockRecallDB{
		vecResults: []db.MemoryScore{
			{Memory: &db.Memory{ID: uuid.New(), MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-2 * time.Hour)}, VecScore: 0.9},
		},
		ftsResults: []db.MemoryScore{
			{Memory: &db.Memory{ID: uuid.New(), MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-2 * time.Hour)}, BM25Score: 0.9},
		},
		trgmResults: []db.MemoryScore{
			{Memory: &db.Memory{ID: uuid.New(), MemoryType: "semantic", Importance: 0.8, CreatedAt: now.Add(-2 * time.Hour)}, TrgmScore: 0.9},
		},
	}
	s := newTestRecallStore(rdb)

	_, err := s.Recall(context.Background(), RecallInput{
		Query:      "time window forwarding",
		ScopeID:    uuid.New(),
		SearchMode: "hybrid",
		Limit:      10,
		Since:      &since,
		Until:      &until,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rdb.vecCalled == 0 || rdb.ftsCalled == 0 || rdb.trgmCalled == 0 {
		t.Fatalf("expected hybrid recall paths called, got vec=%d fts=%d trgm=%d", rdb.vecCalled, rdb.ftsCalled, rdb.trgmCalled)
	}
	if rdb.lastSince == nil || !rdb.lastSince.Equal(since) {
		t.Fatalf("lastSince=%v, want %v", rdb.lastSince, since)
	}
	if rdb.lastUntil == nil || !rdb.lastUntil.Equal(until) {
		t.Fatalf("lastUntil=%v, want %v", rdb.lastUntil, until)
	}
}

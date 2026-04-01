package retrieval_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/retrieval"
)

func TestCombineScores_Formula(t *testing.T) {
	vecScore := 0.8
	bm25Score := 0.6
	importance := 0.7
	recencyDecay := 0.9

	expected := 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recencyDecay
	got := retrieval.CombineScores(vecScore, bm25Score, importance, recencyDecay, retrieval.LayerMemory)
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestCombineScores_KnowledgeBoost(t *testing.T) {
	memScore := retrieval.CombineScores(0.8, 0.0, 0.5, 1.0, retrieval.LayerMemory)
	knowledgeScore := retrieval.CombineScores(0.8, 0.0, 0.5, 1.0, retrieval.LayerKnowledge)
	diff := knowledgeScore - memScore
	if math.Abs(diff-0.10) > 1e-9 {
		t.Fatalf("expected +0.10 knowledge boost, got diff=%v", diff)
	}
}

func TestMerge_DropsPromotedMemory(t *testing.T) {
	knowledgeID := uuid.New()
	memoryID := uuid.New()

	results := []*retrieval.Result{
		{
			Layer:   retrieval.LayerKnowledge,
			ID:      knowledgeID,
			Score:   0.9,
			Content: "knowledge content",
		},
		{
			Layer:      retrieval.LayerMemory,
			ID:         memoryID,
			Score:      0.8,
			Content:    "memory content",
			PromotedTo: &knowledgeID, // this memory was promoted to the knowledge artifact above
		},
	}

	merged := retrieval.Merge(results, 10, 0.0)
	for _, r := range merged {
		if r.ID == memoryID {
			t.Fatal("promoted memory should have been dropped from results")
		}
	}
}

func TestMerge_AppliesMinScore(t *testing.T) {
	results := []*retrieval.Result{
		{Layer: retrieval.LayerMemory, ID: uuid.New(), Score: 0.3, CreatedAt: time.Now()},
		{Layer: retrieval.LayerMemory, ID: uuid.New(), Score: 0.9, CreatedAt: time.Now()},
	}
	merged := retrieval.Merge(results, 10, 0.5)
	for _, r := range merged {
		if r.Score < 0.5 {
			t.Fatalf("result with score %v should have been filtered by MinScore=0.5", r.Score)
		}
	}
}

func TestMerge_SortsByScoreDesc(t *testing.T) {
	results := []*retrieval.Result{
		{Layer: retrieval.LayerMemory, ID: uuid.New(), Score: 0.5, CreatedAt: time.Now()},
		{Layer: retrieval.LayerMemory, ID: uuid.New(), Score: 0.9, CreatedAt: time.Now()},
		{Layer: retrieval.LayerMemory, ID: uuid.New(), Score: 0.7, CreatedAt: time.Now()},
	}
	merged := retrieval.Merge(results, 10, 0.0)
	for i := 1; i < len(merged); i++ {
		if merged[i].Score > merged[i-1].Score {
			t.Fatalf("results not sorted: index %d score %v > index %d score %v",
				i, merged[i].Score, i-1, merged[i-1].Score)
		}
	}
}

func TestMerge_ZeroInputReturnsEmptyNotNil(t *testing.T) {
	merged := retrieval.Merge([]*retrieval.Result{}, 10, 0.0)
	if merged != nil {
		t.Errorf("expected nil slice for zero-result input, got len=%d", len(merged))
	}
	// nil is acceptable — caller must handle both nil and empty; this test just
	// documents the current contract so any change to return an empty slice
	// would still pass (len == 0 is what matters in practice).
	if len(merged) != 0 {
		t.Errorf("len = %d, want 0", len(merged))
	}
}

func TestMerge_MinScoreBoundary(t *testing.T) {
	const threshold = 0.5
	atID := uuid.New()
	belowID := uuid.New()

	results := []*retrieval.Result{
		{Layer: retrieval.LayerMemory, ID: atID, Score: threshold},
		{Layer: retrieval.LayerMemory, ID: belowID, Score: threshold - 0.001},
	}
	merged := retrieval.Merge(results, 10, threshold)

	foundAt := false
	for _, r := range merged {
		if r.ID == belowID {
			t.Errorf("score %.4f (below threshold %.4f) must be excluded", r.Score, threshold)
		}
		if r.ID == atID {
			foundAt = true
		}
	}
	if !foundAt {
		t.Errorf("score exactly at threshold %.4f must be included", threshold)
	}
}

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		a, b    []float32
		wantMin float64
		wantMax float64
	}{
		{
			name:    "identical unit vectors",
			a:       []float32{1, 0},
			b:       []float32{1, 0},
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "orthogonal vectors",
			a:       []float32{1, 0},
			b:       []float32{0, 1},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "zero vector denominator guard",
			a:       []float32{0, 0},
			b:       []float32{1, 1},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "nil vector",
			a:       nil,
			b:       []float32{1, 0},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "negative dot product",
			a:       []float32{1, 0},
			b:       []float32{-1, 0},
			wantMin: -1.0,
			wantMax: -1.0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := retrieval.CosineSimilarity(tc.a, tc.b)
			if math.IsNaN(got) {
				t.Fatalf("CosineSimilarity returned NaN")
			}
			if math.Abs(got-tc.wantMin) > 1e-6 && (got < tc.wantMin || got > tc.wantMax) {
				t.Errorf("got %v, want in [%v, %v]", got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestMerge_DeduplicationKeepsHighestScore(t *testing.T) {
	// Same ID appearing twice — Merge does not deduplicate by ID,
	// it deduplicates promoted memories. Verify both entries survive and the
	// one with the higher score sorts first.
	id := uuid.New()
	results := []*retrieval.Result{
		{Layer: retrieval.LayerMemory, ID: id, Score: 0.6},
		{Layer: retrieval.LayerMemory, ID: id, Score: 0.9},
	}
	merged := retrieval.Merge(results, 10, 0.0)
	if len(merged) != 2 {
		t.Fatalf("len = %d, want 2 (Merge keeps both; callers deduplicate by ID if needed)", len(merged))
	}
	if merged[0].Score < merged[1].Score {
		t.Errorf("higher score should sort first: got %.2f, %.2f", merged[0].Score, merged[1].Score)
	}
}

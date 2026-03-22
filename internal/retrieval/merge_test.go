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

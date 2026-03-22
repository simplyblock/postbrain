package knowledge

import (
	"math"
	"testing"
)

func TestRecallScore_KnowledgeBoost(t *testing.T) {
	// With w_vec=0.50, w_bm25=0.20, w_imp=0.20, w_rec=0.10, plus +0.1 boost.
	vecScore := 0.8
	bm25Score := 0.0
	importance := 0.5 // 5 endorsements / 10
	recency := 1.0

	base := 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recency
	expected := base + 0.10

	got := knowledgeCombinedScore(vecScore, bm25Score, importance, recency)
	if math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestNormalizeEndorsements(t *testing.T) {
	tests := []struct {
		count    int
		expected float64
	}{
		{0, 0.0},
		{5, 0.5},
		{10, 1.0},
		{20, 1.0}, // capped
	}
	for _, tt := range tests {
		got := normalizeEndorsements(tt.count)
		if math.Abs(got-tt.expected) > 1e-9 {
			t.Fatalf("normalizeEndorsements(%d) = %v, want %v", tt.count, got, tt.expected)
		}
	}
}

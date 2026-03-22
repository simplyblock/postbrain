package skills

import (
	"math"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
)

func TestScoreFormula(t *testing.T) {
	t.Parallel()

	// Scoring: vec=0.50, bm25=0.20, importance=0.20, recency=0.10
	// importance = min(1.0, invocationCount / 100.0)
	// recency weight = 1.0 (fixed for skills)

	skill := &db.Skill{
		InvocationCount: 50, // importance = 0.5
	}
	vecScore := 0.8
	bm25Score := 0.6
	invocations := skill.InvocationCount

	importance := math.Min(1.0, float64(invocations)/100.0)
	recency := 1.0

	got := computeSkillScore(vecScore, bm25Score, importance, recency)
	want := 0.50*vecScore + 0.20*bm25Score + 0.20*importance + 0.10*recency
	// want = 0.40 + 0.12 + 0.10 + 0.10 = 0.72

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("score mismatch: got %f, want %f", got, want)
	}
}

func TestScoreFormula_MaxImportance(t *testing.T) {
	t.Parallel()
	// 200 invocations → importance capped at 1.0
	importance := math.Min(1.0, float64(200)/100.0)
	if importance != 1.0 {
		t.Errorf("expected importance=1.0, got %f", importance)
	}
}

func TestScoreFormula_ZeroInvocations(t *testing.T) {
	t.Parallel()
	importance := math.Min(1.0, float64(0)/100.0)
	score := computeSkillScore(0.5, 0.3, importance, 1.0)
	want := 0.50*0.5 + 0.20*0.3 + 0.20*0.0 + 0.10*1.0
	if math.Abs(score-want) > 1e-9 {
		t.Errorf("score mismatch: got %f, want %f", score, want)
	}
}

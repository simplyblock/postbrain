package db

import "testing"

func TestSimilarityDistanceExpr_Vector(t *testing.T) {
	got := similarityDistanceExpr(1536)
	want := "t.embedding <=> $1::vector"
	if got != want {
		t.Fatalf("similarityDistanceExpr(1536) = %q, want %q", got, want)
	}
}

func TestSimilarityDistanceExpr_HalfvecExpression(t *testing.T) {
	got := similarityDistanceExpr(2560)
	want := "t.embedding::halfvec(2560) <=> $1::halfvec"
	if got != want {
		t.Fatalf("similarityDistanceExpr(2560) = %q, want %q", got, want)
	}
}

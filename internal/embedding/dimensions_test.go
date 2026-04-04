package embedding

import "testing"

func TestFitDimensions_Trim(t *testing.T) {
	in := []float32{1, 2, 3, 4}
	got := FitDimensions(in, 2)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("got=%v, want [1 2]", got)
	}
}

func TestFitDimensions_Pad(t *testing.T) {
	in := []float32{1, 2}
	got := FitDimensions(in, 4)
	if len(got) != 4 {
		t.Fatalf("len(got)=%d, want 4", len(got))
	}
	if got[0] != 1 || got[1] != 2 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("got=%v, want [1 2 0 0]", got)
	}
}

func TestFitDimensions_Noop(t *testing.T) {
	in := []float32{1, 2}
	got := FitDimensions(in, 0)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("got=%v, want unchanged", got)
	}
}

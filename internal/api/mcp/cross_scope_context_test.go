package mcp

import "testing"

func TestOrderedRequestedCrossScopeLayers_StableCanonicalOrder(t *testing.T) {
	got := orderedRequestedCrossScopeLayers(map[string]bool{
		"knowledge": true,
		"memory":    true,
	})
	want := []string{"memory", "knowledge"}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseCrossScopeGraphDepth_DefaultIsZero(t *testing.T) {
	if got := parseGraphDepthWithDefault(map[string]any{}, defaultCrossScopeGraphDepth); got != 0 {
		t.Fatalf("parseGraphDepthWithDefault(cross-scope default) = %d, want 0", got)
	}
}

func TestParseCrossScopeGraphDepth_ExplicitOne(t *testing.T) {
	if got := parseGraphDepthWithDefault(map[string]any{"graph_depth": float64(1)}, defaultCrossScopeGraphDepth); got != 1 {
		t.Fatalf("parseGraphDepthWithDefault(1) = %d, want 1", got)
	}
}

func TestParseCrossScopeGraphDepth_CapsAtTwo(t *testing.T) {
	if got := parseGraphDepthWithDefault(map[string]any{"graph_depth": float64(5)}, defaultCrossScopeGraphDepth); got != 2 {
		t.Fatalf("parseGraphDepthWithDefault(5) = %d, want 2", got)
	}
}

func TestParseCrossScopeGraphDepth_NegativeDisablesGraphAugmentation(t *testing.T) {
	if got := parseGraphDepthWithDefault(map[string]any{"graph_depth": float64(-1)}, defaultCrossScopeGraphDepth); got != 0 {
		t.Fatalf("parseGraphDepthWithDefault(-1) = %d, want 0", got)
	}
}

package mcp

import "testing"

func TestParseGraphDepth_DefaultIsOne(t *testing.T) {
	if got := parseGraphDepth(map[string]any{}); got != 1 {
		t.Fatalf("parseGraphDepth(default) = %d, want 1", got)
	}
}

func TestParseGraphDepth_ExplicitZeroDisablesGraphAugmentation(t *testing.T) {
	if got := parseGraphDepth(map[string]any{"graph_depth": float64(0)}); got != 0 {
		t.Fatalf("parseGraphDepth(0) = %d, want 0", got)
	}
}

func TestParseGraphDepth_CapsAtTwo(t *testing.T) {
	if got := parseGraphDepth(map[string]any{"graph_depth": float64(5)}); got != 2 {
		t.Fatalf("parseGraphDepth(5) = %d, want 2", got)
	}
}

func TestParseGraphDepth_NegativeDisablesGraphAugmentation(t *testing.T) {
	if got := parseGraphDepth(map[string]any{"graph_depth": float64(-1)}); got != 0 {
		t.Fatalf("parseGraphDepth(-1) = %d, want 0", got)
	}
}

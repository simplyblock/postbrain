package jobs

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultSummarizer_JoinsContents(t *testing.T) {
	contents := []string{"alpha", "beta", "gamma"}
	result, err := defaultSummarizer(context.Background(), contents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Each content should appear in the result.
	for _, c := range contents {
		if !strings.Contains(result, c) {
			t.Errorf("expected result to contain %q, got %q", c, result)
		}
	}
	// The separator should appear between items.
	if !strings.Contains(result, "---") {
		t.Errorf("expected separator '---' in result, got %q", result)
	}
}

func TestDefaultSummarizer_Empty(t *testing.T) {
	result, err := defaultSummarizer(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for nil contents, got %q", result)
	}
}

func TestConsolidateJob_Signature(t *testing.T) {
	// Compile-time check that Run has the expected signature.
	var _ func(context.Context) error = (*ConsolidateJob)(nil).Run
}

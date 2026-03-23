package knowledge

import (
	"strings"
	"testing"
)

func TestSummarize_ShortTextUnchanged(t *testing.T) {
	t.Parallel()
	text := "This is a short text."
	got := Summarize(text, 150)
	if got != text {
		t.Errorf("expected short text unchanged, got %q", got)
	}
}

func TestSummarize_TruncatesAtSentenceBoundary(t *testing.T) {
	t.Parallel()
	// Build a text with many words spread across two sentences.
	first := strings.Repeat("word ", 10) + "first sentence."
	second := strings.Repeat("extra ", 200) + "second sentence."
	text := first + " " + second
	got := Summarize(text, 15)
	if !strings.HasSuffix(got, ".") {
		t.Errorf("expected summary to end at sentence boundary, got %q", got)
	}
	words := strings.Fields(got)
	if len(words) > 20 {
		t.Errorf("summary too long: %d words", len(words))
	}
}

func TestSummarize_EmptyText(t *testing.T) {
	t.Parallel()
	got := Summarize("", 150)
	if got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}
}

func TestSummarize_NoSentenceBoundaryFallsBackToWordLimit(t *testing.T) {
	t.Parallel()
	// A long run-on with no sentence terminator.
	text := strings.Repeat("word ", 300)
	got := Summarize(text, 50)
	words := strings.Fields(got)
	if len(words) > 55 {
		t.Errorf("expected ~50 words, got %d", len(words))
	}
}

func TestSummarize_ExactlyAtLimit(t *testing.T) {
	t.Parallel()
	// Exactly 10 words with a period — should be returned whole.
	text := "one two three four five six seven eight nine ten."
	got := Summarize(text, 10)
	if got != text {
		t.Errorf("expected full text at exact limit, got %q", got)
	}
}

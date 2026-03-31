package chunking

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// sentence returns a string of exactly n runes ending with a period, making it
// a clean sentence boundary for the splitter. The prefix pads to distinguish
// sentences from one another (e.g. "aaa...X.").
func sentence(n int, prefix rune) string {
	// (n-2) 'a' runes + prefix rune + '.'
	if n < 2 {
		n = 2
	}
	return strings.Repeat("a", n-2) + string(prefix) + "."
}

// sentences joins count unique sentences of runesEach runes with a single space.
// Each sentence uses a distinct ASCII letter prefix so their content differs.
func sentences(count, runesEach int) string {
	parts := make([]string, count)
	for i := range parts {
		// Cycle through uppercase letters A-Z for the prefix.
		parts[i] = sentence(runesEach, rune('A'+i%26))
	}
	return strings.Join(parts, " ")
}

// ── Chunk ────────────────────────────────────────────────────────────────────

func TestChunk_ShortTextReturnsSingleElement(t *testing.T) {
	t.Parallel()
	text := "Hello world."
	got := Chunk(text, 100, 0)
	if len(got) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(got))
	}
	if got[0] != text {
		t.Errorf("want %q, got %q", text, got[0])
	}
}

func TestChunk_ExactlyMaxRunesReturnsSingleElement(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("x", 100)
	got := Chunk(text, 100, 0)
	if len(got) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(got))
	}
}

func TestChunk_SplitsIntoMultipleChunks(t *testing.T) {
	t.Parallel()
	// 6 sentences × 20 runes each = 120 runes + spaces; maxRunes=40 → must produce >1 chunk.
	text := sentences(6, 20)
	got := Chunk(text, 40, 0)
	if len(got) <= 1 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
}

func TestChunk_EachChunkWithinMaxRunes(t *testing.T) {
	t.Parallel()
	text := sentences(10, 15)
	maxRunes := 50
	for i, chunk := range Chunk(text, maxRunes, 0) {
		n := utf8.RuneCountInString(chunk)
		if n > maxRunes {
			t.Errorf("chunk[%d] has %d runes, exceeds maxRunes=%d", i, n, maxRunes)
		}
	}
}

func TestChunk_OverlapCarriesSentencesIntoNextChunk(t *testing.T) {
	t.Parallel()
	// Build 8 sentences that are each 20 runes. maxRunes=45 fits ~2 sentences per chunk.
	// With overlap=1, the last sentence of chunk N should appear at the start of chunk N+1.
	text := sentences(8, 20)
	overlap := 1
	chunks := Chunk(text, 45, overlap)
	if len(chunks) < 3 {
		t.Skip("not enough chunks to verify overlap")
	}
	// The last sentence of chunk[0] should appear somewhere in chunk[1].
	last0 := lastSentence(chunks[0])
	if !strings.Contains(chunks[1], last0) {
		t.Errorf("overlap sentence %q not found in next chunk %q", last0, chunks[1])
	}
}

func TestChunk_ZeroOverlapNoSharedSentences(t *testing.T) {
	t.Parallel()
	text := sentences(8, 20)
	chunks := Chunk(text, 45, 0)
	if len(chunks) < 2 {
		t.Skip("not enough chunks")
	}
	// With no overlap, the last sentence of chunk[0] must not appear in chunk[1].
	last0 := lastSentence(chunks[0])
	if strings.Contains(chunks[1], last0) {
		t.Errorf("zero overlap: last sentence of chunk[0] leaked into chunk[1]")
	}
}

func TestChunk_SingleLongSentenceHardSplits(t *testing.T) {
	t.Parallel()
	// One unsplittable sentence much longer than maxRunes.
	text := strings.Repeat("w ", 200) // 400 runes, no sentence boundary
	maxRunes := 80
	chunks := Chunk(text, maxRunes, 0)
	if len(chunks) <= 1 {
		t.Fatalf("expected hard-split chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		n := utf8.RuneCountInString(c)
		if n > maxRunes {
			t.Errorf("hard-split chunk[%d] has %d runes, exceeds maxRunes=%d", i, n, maxRunes)
		}
	}
}

func TestChunk_AllContentPreserved(t *testing.T) {
	t.Parallel()
	// Every word in the original text must appear in at least one chunk.
	text := sentences(12, 10)
	words := strings.Fields(text)
	chunks := Chunk(text, 60, 1)
	combined := strings.Join(chunks, " ")
	for _, w := range words {
		if !strings.Contains(combined, w) {
			t.Errorf("word %q missing from all chunks", w)
		}
	}
}

// ── splitSentences ────────────────────────────────────────────────────────────

func TestSplitSentences_ParagraphBreakIsHardBoundary(t *testing.T) {
	t.Parallel()
	text := "First paragraph sentence one. Sentence two.\n\nSecond paragraph."
	got := splitSentences(text)
	// Expect at least 3 sentences (two from para 1, one from para 2).
	if len(got) < 3 {
		t.Fatalf("expected ≥3 sentences, got %d: %v", len(got), got)
	}
	// The last sentence must be from the second paragraph.
	if got[len(got)-1] != "Second paragraph." {
		t.Errorf("last sentence = %q, want %q", got[len(got)-1], "Second paragraph.")
	}
}

func TestSplitSentences_EmptyParagraphsSkipped(t *testing.T) {
	t.Parallel()
	text := "Hello.\n\n\n\nWorld."
	got := splitSentences(text)
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences, got %d: %v", len(got), got)
	}
}

func TestSplitSentences_WindowsLineEndingsNormalized(t *testing.T) {
	t.Parallel()
	text := "First.\r\n\r\nSecond."
	got := splitSentences(text)
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences after CRLF normalisation, got %d: %v", len(got), got)
	}
}

func TestSplitSentences_PunctuationVariants(t *testing.T) {
	t.Parallel()
	text := "Is it true? Yes! Indeed."
	got := splitSentences(text)
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %v", len(got), got)
	}
}

func TestSplitSentences_NoPunctuationReturnsSingleSentence(t *testing.T) {
	t.Parallel()
	text := "no punctuation here at all"
	got := splitSentences(text)
	if len(got) != 1 {
		t.Fatalf("expected 1 sentence, got %d: %v", len(got), got)
	}
	if got[0] != text {
		t.Errorf("got %q, want %q", got[0], text)
	}
}

// ── splitByRunes ──────────────────────────────────────────────────────────────

func TestSplitByRunes_ShortTextReturnsSingleChunk(t *testing.T) {
	t.Parallel()
	text := "hello world"
	got := splitByRunes(text, 100)
	if len(got) != 1 || got[0] != text {
		t.Errorf("got %v, want [%q]", got, text)
	}
}

func TestSplitByRunes_BreaksAtWhitespace(t *testing.T) {
	t.Parallel()
	// 5 distinct words × 10 chars each. maxRunes=25 forces splits between words.
	text := "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee"
	words := strings.Fields(text) // ["aaaaaaaaaa", "bbbbbbbbbb", ...]
	chunks := splitByRunes(text, 25)

	// Every original word must appear intact in exactly one chunk (not cut mid-word).
	combined := strings.Join(chunks, " ")
	for _, w := range words {
		if !strings.Contains(combined, w) {
			t.Errorf("word %q missing from chunks", w)
		}
	}
	// Must have produced more than one chunk.
	if len(chunks) <= 1 {
		t.Errorf("expected multiple chunks for text of %d runes with maxRunes=25", utf8.RuneCountInString(text))
	}
}

func TestSplitByRunes_HardLimitWhenNoSpace(t *testing.T) {
	t.Parallel()
	// A single word longer than maxRunes with no spaces → must cut at hard limit.
	text := strings.Repeat("x", 200)
	maxRunes := 50
	chunks := splitByRunes(text, maxRunes)
	for i, c := range chunks {
		n := utf8.RuneCountInString(c)
		if n > maxRunes {
			t.Errorf("chunk[%d] has %d runes, exceeds maxRunes=%d", i, n, maxRunes)
		}
	}
	// All runes must be accounted for.
	total := 0
	for _, c := range chunks {
		total += utf8.RuneCountInString(c)
	}
	if total != utf8.RuneCountInString(text) {
		t.Errorf("total runes in chunks = %d, want %d", total, utf8.RuneCountInString(text))
	}
}

func TestSplitByRunes_UnicodeHandledCorrectly(t *testing.T) {
	t.Parallel()
	// Japanese characters are multi-byte but single rune each.
	text := strings.Repeat("日", 100)
	maxRunes := 30
	for i, c := range splitByRunes(text, maxRunes) {
		n := utf8.RuneCountInString(c)
		if n > maxRunes {
			t.Errorf("chunk[%d] has %d runes, exceeds maxRunes=%d", i, n, maxRunes)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// lastSentence returns the last sentence-like token from a chunk (split on ". ").
func lastSentence(chunk string) string {
	parts := strings.Split(chunk, ". ")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" && len(parts) > 1 {
		last = strings.TrimSpace(parts[len(parts)-2])
	}
	return last
}

// Package chunking splits long text into overlapping segments suitable for
// embedding. It is intentionally dependency-free so it can be used by any
// layer without creating import cycles.
package chunking

import (
	"strings"
	"unicode/utf8"
)

const (
	// DefaultChunkRunes is the target chunk size in runes (~250 tokens for most models).
	DefaultChunkRunes = 1000
	// DefaultOverlap is the number of sentences carried over between consecutive chunks.
	DefaultOverlap = 2
	// MinContentRunes is the threshold below which content is not chunked.
	// Content must be at least 2× a chunk before splitting is worthwhile.
	MinContentRunes = DefaultChunkRunes * 2
)

// Chunk splits text into overlapping segments on sentence/paragraph boundaries.
//
// maxRunes controls the target size of each chunk (in Unicode code points).
// overlap is the number of sentences carried over as context into the next chunk.
//
// Returns a single-element slice when text already fits in one chunk.
func Chunk(text string, maxRunes, overlap int) []string {
	if utf8.RuneCountInString(text) <= maxRunes {
		return []string{text}
	}

	sentences := splitSentences(text)

	// Single sentence or unsplittable: hard-split by rune count.
	if len(sentences) <= 1 {
		return splitByRunes(text, maxRunes)
	}

	var chunks []string
	start := 0

	for start < len(sentences) {
		end := start
		runes := 0
		for end < len(sentences) {
			sr := utf8.RuneCountInString(sentences[end])
			// Accept at least one sentence even if it alone exceeds the limit.
			if runes+sr > maxRunes && end > start {
				break
			}
			runes += sr + 1 // +1 for the joining space
			end++
		}

		chunks = append(chunks, strings.Join(sentences[start:end], " "))

		if end >= len(sentences) {
			break
		}

		// Start the next chunk `overlap` sentences before the current end so
		// readers see context from both sides of the boundary.
		next := end - overlap
		if next <= start {
			next = start + 1 // always make forward progress
		}
		start = next
	}

	return chunks
}

// splitSentences breaks text into individual sentences, preserving punctuation.
// Paragraph breaks (blank lines) are treated as hard sentence boundaries.
// Markdown horizontal rules (---, ***, ___) are treated as paragraph separators
// and stripped from output. Markdown header prefixes (# ## etc.) are removed
// so headers flow as prose rather than becoming punctuation-free "sentences"
// that inflate the overlap region.
func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = normalizeMarkdown(text)
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		out = append(out, splitParagraph(para)...)
	}
	return out
}

// normalizeMarkdown rewrites markdown structural elements so they don't
// produce empty punctuation-free "sentences" that pollute chunk overlap:
//   - Horizontal rules (lines of only -, *, or _ with 3+ chars) become blank
//     lines, acting as paragraph separators.
//   - ATX header prefixes (leading # characters) are stripped so the heading
//     text flows as a regular prose sentence.
func normalizeMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isMarkdownHRule(trimmed) {
			lines[i] = ""
			continue
		}
		if stripped := stripMarkdownHeader(trimmed); stripped != trimmed {
			lines[i] = stripped
		}
	}
	return strings.Join(lines, "\n")
}

// isMarkdownHRule reports whether s is a markdown horizontal rule: three or
// more of the same character from the set {-, *, _} with optional spaces.
func isMarkdownHRule(s string) bool {
	if len(s) < 3 {
		return false
	}
	var marker rune
	count := 0
	for _, r := range s {
		if r == ' ' {
			continue
		}
		if r != '-' && r != '*' && r != '_' {
			return false
		}
		if marker == 0 {
			marker = r
		} else if r != marker {
			return false
		}
		count++
	}
	return count >= 3
}

// stripMarkdownHeader removes leading ATX header markers ("# ", "## ", etc.)
// and returns the plain heading text. Returns s unchanged if it is not a header.
func stripMarkdownHeader(s string) string {
	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	if i == 0 || i >= len(s) {
		return s
	}
	return strings.TrimLeft(s[i:], " ")
}

func splitParagraph(text string) []string {
	var result []string
	var buf strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		buf.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			next := i + 1
			if next >= len(runes) || runes[next] == ' ' || runes[next] == '\n' {
				if s := strings.TrimSpace(buf.String()); s != "" {
					result = append(result, s)
				}
				buf.Reset()
			}
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		result = append(result, s)
	}
	return result
}

// splitByRunes hard-splits text into chunks of at most maxRunes runes,
// preferring to break at whitespace.
func splitByRunes(text string, maxRunes int) []string {
	var chunks []string
	runes := []rune(text)
	for len(runes) > 0 {
		if len(runes) <= maxRunes {
			chunks = append(chunks, string(runes))
			break
		}
		end := maxRunes
		// Walk back to the nearest space so we don't cut mid-word.
		for end > maxRunes/2 && runes[end] != ' ' && runes[end] != '\n' {
			end--
		}
		if end <= maxRunes/2 {
			end = maxRunes // no space found; cut at hard limit
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}

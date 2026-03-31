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
func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
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

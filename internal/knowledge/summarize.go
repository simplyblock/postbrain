package knowledge

import "strings"

// Summarize produces an extractive summary of text by truncating to at most
// maxWords words, preferring to break at a sentence boundary (. ! ?).
// If no sentence boundary is found within the word limit, the raw word-limited
// slice is returned.  The original text is returned unchanged when it is
// already within the limit.
func Summarize(text string, maxWords int) string {
	if text == "" {
		return ""
	}
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return text
	}

	// Scan the first maxWords words for the last sentence terminator.
	limited := words[:maxWords]
	lastSentence := -1
	for i, w := range limited {
		last := w[len(w)-1]
		if last == '.' || last == '!' || last == '?' {
			lastSentence = i
		}
	}

	if lastSentence >= 0 {
		return strings.Join(limited[:lastSentence+1], " ")
	}
	// No sentence boundary found; return the word-limited slice.
	return strings.Join(limited, " ")
}

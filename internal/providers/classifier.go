package providers

import (
	"strings"
)

// codeExtensions is the set of file extensions that indicate source code.
var codeExtensions = map[string]bool{
	".go":    true,
	".py":    true,
	".js":    true,
	".ts":    true,
	".jsx":   true,
	".tsx":   true,
	".rs":    true,
	".java":  true,
	".c":     true,
	".cpp":   true,
	".h":     true,
	".rb":    true,
	".sh":    true,
	".cs":    true,
	".php":   true,
	".swift": true,
	".kt":    true,
	".scala": true,
	".lua":   true,
	".r":     true,
	".m":     true,
	".ex":    true,
	".exs":   true,
}

// ClassifyContent returns "code" or "text" based on the source_ref and
// content heuristics.
//
// If source_ref starts with "file:", the file path is extracted and its
// extension is checked against a known set of code extensions. If the
// extension matches, "code" is returned; if the extension is known but not
// code (e.g. .md), "text" is returned; otherwise the function falls through
// to the content heuristic.
//
// The content heuristic counts lines that match code patterns. If the ratio
// of matching lines to total non-empty lines exceeds 0.4, "code" is returned.
// Default is "text".
func ClassifyContent(content, sourceRef string) string {
	if strings.HasPrefix(sourceRef, "file:") {
		// Extract the file path: everything after "file:" up to (but not
		// including) the last colon that precedes a line number, if present.
		rest := sourceRef[len("file:"):]
		// The format is file:<path>:<line> — find the last colon to strip
		// the line number, but only if what follows looks like a number (i.e.
		// the path itself may contain colons on Windows, but we'll keep it
		// simple: strip from the last colon onward).
		if idx := strings.LastIndex(rest, ":"); idx != -1 {
			rest = rest[:idx]
		}
		// Determine extension.
		ext := fileExtension(rest)
		if ext != "" {
			if codeExtensions[ext] {
				return "code"
			}
			// Known non-code extension (e.g. .md, .txt) — return text
			// without consulting the content heuristic.
			return "text"
		}
		// Unknown extension — fall through to content heuristic.
	}

	return classifyByContent(content)
}

// fileExtension returns the lower-case extension of path (e.g. ".go"), or ""
// if there is no extension.
func fileExtension(path string) string {
	// Use only the base filename.
	base := path
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		base = path[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx != -1 && idx > 0 {
		return strings.ToLower(base[idx:])
	}
	return ""
}

// classifyByContent uses line-level heuristics to decide "code" vs "text".
func classifyByContent(content string) string {
	lines := strings.Split(content, "\n")
	total := 0
	matches := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		total++

		if isCodeLine(line, trimmed) {
			matches++
		}
	}

	if total == 0 {
		return "text"
	}

	if float64(matches)/float64(total) > 0.4 {
		return "code"
	}

	return "text"
}

// isCodeLine reports whether a single line looks like code.
func isCodeLine(raw, trimmed string) bool {
	// Lines containing braces
	if strings.ContainsAny(trimmed, "{}") {
		return true
	}

	// Lines starting with at least 4 spaces of indentation
	if len(raw) >= 4 && raw[0] == ' ' && raw[1] == ' ' && raw[2] == ' ' && raw[3] == ' ' {
		return true
	}

	// Lines containing common code keywords
	codePatterns := []string{
		"func ", "def ", "class ", "import ", "return ",
		"const ", "var ", "let ",
	}
	for _, p := range codePatterns {
		if strings.Contains(trimmed, p) {
			return true
		}
	}

	return false
}

package postbraincli

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveScopeFromBaseFiles returns the first configured POSTBRAIN_SCOPE found
// in postbrain-base files, in deterministic priority order.
func ResolveScopeFromBaseFiles(targetDir string) string {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	candidates := []string{
		filepath.Join(targetDir, ".codex", "postbrain-base.md"),
		filepath.Join(targetDir, ".claude", "postbrain-base.md"),
		filepath.Join(targetDir, ".agents", "postbrain-base.md"),
	}
	for _, path := range candidates {
		if scope := readScopeFromPostbrainBase(path); scope != "" {
			return scope
		}
	}
	return ""
}

// ResolveURLFromBaseFiles returns the first configured POSTBRAIN_URL found in
// postbrain-base files, in deterministic priority order.
func ResolveURLFromBaseFiles(targetDir string) string {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	candidates := []string{
		filepath.Join(targetDir, ".codex", "postbrain-base.md"),
		filepath.Join(targetDir, ".claude", "postbrain-base.md"),
		filepath.Join(targetDir, ".agents", "postbrain-base.md"),
	}
	for _, path := range candidates {
		if url := readURLFromPostbrainBase(path); url != "" {
			return url
		}
	}
	return ""
}

func readScopeFromPostbrainBase(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if scope := parseScopeLine(line); scope != "" {
			return scope
		}
	}
	return ""
}

func readURLFromPostbrainBase(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if url := parseURLLine(line); url != "" {
			return url
		}
	}
	return ""
}

func parseScopeLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}

	const envKey = "POSTBRAIN_SCOPE="
	if strings.HasPrefix(trimmed, envKey) {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, envKey))
	}

	// Support documented markdown/yaml-style key: postbrain_scope: kind:external_id
	// Accept ':' or '=' separators and case-insensitive key matching.
	keyValue := strings.SplitN(trimmed, ":", 2)
	if len(keyValue) != 2 {
		keyValue = strings.SplitN(trimmed, "=", 2)
	}
	if len(keyValue) != 2 {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(keyValue[0]))
	if key != "postbrain_scope" {
		return ""
	}
	return strings.TrimSpace(keyValue[1])
}

func parseURLLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}

	const envKey = "POSTBRAIN_URL="
	if strings.HasPrefix(trimmed, envKey) {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, envKey))
	}

	// Support documented markdown/yaml-style key: postbrain_url: http://...
	// Accept ':' or '=' separators and case-insensitive key matching.
	keyValue := strings.SplitN(trimmed, ":", 2)
	if len(keyValue) != 2 {
		keyValue = strings.SplitN(trimmed, "=", 2)
	}
	if len(keyValue) != 2 {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(keyValue[0]))
	if key != "postbrain_url" {
		return ""
	}
	return strings.TrimSpace(keyValue[1])
}

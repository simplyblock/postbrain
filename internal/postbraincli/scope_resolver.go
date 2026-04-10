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

func readScopeFromPostbrainBase(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const key = "POSTBRAIN_SCOPE="
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, key) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, key))
		}
	}
	return ""
}

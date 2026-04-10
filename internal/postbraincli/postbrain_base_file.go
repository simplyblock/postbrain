package postbraincli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensurePostbrainBaseFile(targetDir, agentDir, scope string) error {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	baseDir := filepath.Join(targetDir, agentDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", agentDir, err)
	}
	basePath := filepath.Join(baseDir, "postbrain-base.md")
	if _, err := os.Stat(basePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", basePath, err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("postbrain_enabled: true\n")
	if strings.TrimSpace(scope) != "" {
		b.WriteString("postbrain_scope: ")
		b.WriteString(strings.TrimSpace(scope))
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString("Postbrain local bootstrap settings.\n")
	if strings.TrimSpace(scope) == "" {
		b.WriteString("Set `postbrain_scope: kind:external_id` in frontmatter to pin scope.\n")
	}
	if err := os.WriteFile(basePath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", basePath, err)
	}
	return nil
}

package postbraincli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ensurePostbrainBaseFile(targetDir, agentDir, scope, backendURL string) error {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = "."
	}
	baseDir := filepath.Join(targetDir, agentDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", agentDir, err)
	}
	basePath := filepath.Join(baseDir, "postbrain-base.md")
	data, err := os.ReadFile(basePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", basePath, err)
	}
	if os.IsNotExist(err) {
		return writePostbrainBaseFile(basePath, scope, backendURL)
	}

	updated := ensureFrontmatterKey(string(data), "postbrain_enabled", "true")
	content := updated
	if strings.TrimSpace(scope) != "" {
		content = ensureFrontmatterKey(content, "postbrain_scope", strings.TrimSpace(scope))
	}
	if strings.TrimSpace(backendURL) != "" {
		content = ensureFrontmatterKey(content, "postbrain_url", strings.TrimSpace(backendURL))
	}
	content = ensureFrontmatterKey(content, "updated_at", time.Now().Format("2006-01-02"))
	if err := os.WriteFile(basePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", basePath, err)
	}
	return nil
}

func writePostbrainBaseFile(basePath, scope, backendURL string) error {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("postbrain_enabled: true\n")
	if strings.TrimSpace(scope) != "" {
		b.WriteString("postbrain_scope: ")
		b.WriteString(strings.TrimSpace(scope))
		b.WriteString("\n")
	}
	if strings.TrimSpace(backendURL) != "" {
		b.WriteString("postbrain_url: ")
		b.WriteString(strings.TrimSpace(backendURL))
		b.WriteString("\n")
	}
	b.WriteString("updated_at: ")
	b.WriteString(time.Now().Format("2006-01-02"))
	b.WriteString("\n")
	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString("Postbrain local bootstrap settings.\n")
	if strings.TrimSpace(scope) == "" {
		b.WriteString("Set `postbrain_scope: kind:external_id` in frontmatter to pin scope.\n")
	}
	if strings.TrimSpace(backendURL) == "" {
		b.WriteString("Set `postbrain_url: http://localhost:7433` in frontmatter to pin backend URL.\n")
	}
	if err := os.WriteFile(basePath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", basePath, err)
	}
	return nil
}

func ensureFrontmatterKey(content, key, value string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Sprintf("---\n%s: %s\n---\n", key, value)
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		var b strings.Builder
		b.WriteString("---\n")
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteString("\n---\n\n")
		b.WriteString(content)
		return b.String()
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		var b strings.Builder
		b.WriteString("---\n")
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteString("\n---\n\n")
		b.WriteString(content)
		return b.String()
	}

	prefix := key + ":"
	for i := 1; i < end; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			lines[i] = key + ": " + value
			return strings.Join(lines, "\n")
		}
	}
	front := append([]string{}, lines[:end]...)
	front = append(front, key+": "+value)
	front = append(front, lines[end:]...)
	return strings.Join(front, "\n")
}

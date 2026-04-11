package postbraincli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePostbrainBaseFile_UpdatesFrontmatterKeys(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	baseDir := filepath.Join(targetDir, ".codex")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	basePath := filepath.Join(baseDir, "postbrain-base.md")
	seed := strings.Join([]string{
		"---",
		"postbrain_enabled: true",
		"postbrain_scope: project:existing",
		"---",
		"",
		"notes",
		"",
	}, "\n")
	if err := os.WriteFile(basePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	if err := ensurePostbrainBaseFile(targetDir, ".codex", "project:new", "http://localhost:7433"); err != nil {
		t.Fatalf("ensurePostbrainBaseFile: %v", err)
	}

	data, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read base file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "postbrain_scope: project:new") {
		t.Fatalf("base file scope not updated: %q", content)
	}
	if !strings.Contains(content, "postbrain_url: http://localhost:7433") {
		t.Fatalf("base file missing postbrain_url: %q", content)
	}
	if !strings.Contains(content, "updated_at: ") {
		t.Fatalf("base file missing updated_at: %q", content)
	}
	if !strings.Contains(content, "notes") {
		t.Fatalf("base file should retain existing body content: %q", content)
	}
}

func TestEnsurePostbrainBaseFile_WritesCanonicalFrontmatter(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	if err := ensurePostbrainBaseFile(targetDir, ".claude", "project:frontmatter", "http://localhost:7433"); err != nil {
		t.Fatalf("ensurePostbrainBaseFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(targetDir, ".claude", "postbrain-base.md"))
	if err != nil {
		t.Fatalf("read base file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "postbrain_enabled: true") {
		t.Fatal("missing postbrain_enabled in frontmatter")
	}
	if !strings.Contains(content, "postbrain_scope: project:frontmatter") {
		t.Fatal("missing postbrain_scope in frontmatter")
	}
	if !strings.Contains(content, "postbrain_url: http://localhost:7433") {
		t.Fatal("missing postbrain_url in frontmatter")
	}
	if !strings.Contains(content, "updated_at: ") {
		t.Fatal("missing updated_at in frontmatter")
	}
}

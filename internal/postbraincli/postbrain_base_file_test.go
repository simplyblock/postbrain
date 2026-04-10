package postbraincli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePostbrainBaseFile_DoesNotOverwriteExistingFile(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	baseDir := filepath.Join(targetDir, ".codex")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	basePath := filepath.Join(baseDir, "postbrain-base.md")
	seed := "postbrain_scope: project:existing\n"
	if err := os.WriteFile(basePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	if err := ensurePostbrainBaseFile(targetDir, ".codex", "project:new"); err != nil {
		t.Fatalf("ensurePostbrainBaseFile: %v", err)
	}

	data, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read base file: %v", err)
	}
	if string(data) != seed {
		t.Fatalf("base file was overwritten: got %q, want %q", string(data), seed)
	}
}

func TestEnsurePostbrainBaseFile_WritesCanonicalFrontmatter(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	if err := ensurePostbrainBaseFile(targetDir, ".claude", "project:frontmatter"); err != nil {
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
}

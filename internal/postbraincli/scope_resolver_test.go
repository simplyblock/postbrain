package postbraincli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveScopeFromBaseFiles_Order(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	for _, dir := range []string{
		filepath.Join(targetDir, ".codex"),
		filepath.Join(targetDir, ".claude"),
		filepath.Join(targetDir, ".agents"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(targetDir, ".codex", "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-codex\n"), 0o644); err != nil {
		t.Fatalf("write codex file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, ".claude", "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-claude\n"), 0o644); err != nil {
		t.Fatalf("write claude file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, ".agents", "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-agents\n"), 0o644); err != nil {
		t.Fatalf("write agents file: %v", err)
	}

	if got := ResolveScopeFromBaseFiles(targetDir); got != "project:from-codex" {
		t.Fatalf("ResolveScopeFromBaseFiles() = %q, want codex scope", got)
	}
}

func TestResolveScopeFromBaseFiles_IgnoreComments(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	content := "# POSTBRAIN_SCOPE=project:commented\n\nPOSTBRAIN_SCOPE=project:active\n"
	if err := os.WriteFile(filepath.Join(targetDir, ".claude", "postbrain-base.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if got := ResolveScopeFromBaseFiles(targetDir); got != "project:active" {
		t.Fatalf("ResolveScopeFromBaseFiles() = %q, want project:active", got)
	}
}

func TestResolveScopeFromBaseFiles_SupportsDocumentedPostbrainScopeKey(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}
	content := strings.Join([]string{
		"postbrain_enabled: true",
		"  PostBrain_Scope   :   project:documented-format  ",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(targetDir, ".agents", "postbrain-base.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if got := ResolveScopeFromBaseFiles(targetDir); got != "project:documented-format" {
		t.Fatalf("ResolveScopeFromBaseFiles() = %q, want project:documented-format", got)
	}
}

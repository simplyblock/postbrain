package postbraincli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallClaudeSkill_WritesSkillFileAndUpdatesCLAUDE(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	claudePath := filepath.Join(targetDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	installedPath, updatedClaude, err := InstallClaudeSkill(targetDir, "skill-content", "http://localhost:7433", "project:acme/api")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}
	if !updatedClaude {
		t.Fatal("updatedClaude = false, want true")
	}
	wantPath := filepath.Join(targetDir, ".claude", "postbrain.md")
	if installedPath != wantPath {
		t.Fatalf("installedPath = %q, want %q", installedPath, wantPath)
	}

	b, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if string(b) != "skill-content" {
		t.Fatalf("installed skill content = %q, want %q", string(b), "skill-content")
	}

	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(claude)
	if !strings.Contains(content, "<!-- postbrain-config -->") {
		t.Fatal("CLAUDE.md missing postbrain marker")
	}
	if !strings.Contains(content, "@.claude/postbrain.md") {
		t.Fatal("CLAUDE.md missing @.claude/postbrain.md import")
	}
	if !strings.Contains(content, "POSTBRAIN_URL=http://localhost:7433") {
		t.Fatal("CLAUDE.md missing POSTBRAIN_URL")
	}
	if !strings.Contains(content, "POSTBRAIN_SCOPE=project:acme/api") {
		t.Fatal("CLAUDE.md missing POSTBRAIN_SCOPE")
	}
}

func TestInstallClaudeSkill_DoesNotDuplicateBlock(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	claudePath := filepath.Join(targetDir, "CLAUDE.md")
	seed := strings.Join([]string{
		"# Project",
		"<!-- postbrain-config -->",
		"existing",
	}, "\n")
	if err := os.WriteFile(claudePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	_, updatedClaude, err := InstallClaudeSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}
	if updatedClaude {
		t.Fatal("updatedClaude = true, want false")
	}

	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if got := strings.Count(string(claude), "<!-- postbrain-config -->"); got != 1 {
		t.Fatalf("marker count = %d, want 1", got)
	}
}

func TestInstallClaudeSkill_NoCLAUDEFileStillInstallsSkill(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	installedPath, updatedClaude, err := InstallClaudeSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}
	if updatedClaude {
		t.Fatal("updatedClaude = true, want false")
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("installed skill stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md exists unexpectedly or stat error: %v", err)
	}
}

func TestInstallClaudeSkill_NoScopeOmitsScopeLine(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	claudePath := filepath.Join(targetDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	_, _, err := InstallClaudeSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}

	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(claude)
	if strings.Contains(content, "\nPOSTBRAIN_SCOPE=") {
		t.Fatal("CLAUDE.md should not contain an uncommented POSTBRAIN_SCOPE= line when scope is empty")
	}
	if !strings.Contains(content, "# POSTBRAIN_SCOPE=project:your-org/your-repo") {
		t.Fatal("CLAUDE.md should contain the placeholder scope comment")
	}
}

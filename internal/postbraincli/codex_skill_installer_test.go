package postbraincli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexSkill_WritesSkillFileAndAppendsAgentsBlock(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	agentsPath := filepath.Join(targetDir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	installedPath, updatedAgents, err := InstallCodexSkill(targetDir, "skill-content", "http://localhost:7433", "project:acme/api")
	if err != nil {
		t.Fatalf("InstallCodexSkill: %v", err)
	}
	if !updatedAgents {
		t.Fatal("updatedAgents = false, want true")
	}
	wantPath := filepath.Join(targetDir, ".codex", "skills", "postbrain.md")
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

	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(agents)
	if !strings.Contains(content, "<!-- postbrain-config -->") {
		t.Fatal("AGENTS.md missing postbrain marker")
	}
	if !strings.Contains(content, "POSTBRAIN_URL=http://localhost:7433") {
		t.Fatal("AGENTS.md missing POSTBRAIN_URL")
	}
	if !strings.Contains(content, "POSTBRAIN_SCOPE=project:acme/api") {
		t.Fatal("AGENTS.md missing POSTBRAIN_SCOPE")
	}
}

func TestInstallCodexSkill_DoesNotDuplicateAgentsBlock(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	agentsPath := filepath.Join(targetDir, "AGENTS.md")
	seed := strings.Join([]string{
		"# Project",
		"<!-- postbrain-config -->",
		"existing",
	}, "\n")
	if err := os.WriteFile(agentsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	_, updatedAgents, err := InstallCodexSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallCodexSkill: %v", err)
	}
	if updatedAgents {
		t.Fatal("updatedAgents = true, want false")
	}

	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if got := strings.Count(string(agents), "<!-- postbrain-config -->"); got != 1 {
		t.Fatalf("marker count = %d, want 1", got)
	}
}

func TestInstallCodexSkill_NoAgentsFileStillInstallsSkill(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	installedPath, updatedAgents, err := InstallCodexSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallCodexSkill: %v", err)
	}
	if updatedAgents {
		t.Fatal("updatedAgents = true, want false")
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("installed skill stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md exists unexpectedly or stat error: %v", err)
	}
}

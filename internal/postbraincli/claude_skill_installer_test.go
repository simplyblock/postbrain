package postbraincli

import (
	"encoding/json"
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

// ── InstallClaudeHooks ────────────────────────────────────────────────────────

func TestInstallClaudeHooks_CreatesSettingsWithHooks(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	updated, err := InstallClaudeHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("settings.json missing hooks")
	}
	postToolUse, _ := hooks["PostToolUse"].([]any)
	if len(postToolUse) == 0 {
		t.Fatal("hooks.PostToolUse is empty")
	}
	stop, _ := hooks["Stop"].([]any)
	if len(stop) == 0 {
		t.Fatal("hooks.Stop is empty")
	}

	// Snapshot command must include the literal scope.
	entry, _ := postToolUse[0].(map[string]any)
	hooksList, _ := entry["hooks"].([]any)
	if len(hooksList) == 0 {
		t.Fatal("PostToolUse entry missing hooks array")
	}
	hook, _ := hooksList[0].(map[string]any)
	cmd, _ := hook["command"].(string)
	if !strings.Contains(cmd, "postbrain-cli snapshot") {
		t.Errorf("PostToolUse command %q missing 'postbrain-cli snapshot'", cmd)
	}
	if !strings.Contains(cmd, "project:acme/api") {
		t.Errorf("PostToolUse command %q missing scope", cmd)
	}
}

func TestInstallClaudeHooks_NoScope_UsesEnvVar(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := InstallClaudeHooks(targetDir, ""); err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	if !strings.Contains(string(data), "$POSTBRAIN_SCOPE") {
		t.Error("settings.json should use $POSTBRAIN_SCOPE when no scope provided")
	}
}

func TestInstallClaudeHooks_MergesIntoExistingSettings(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	clDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `{"theme":"dark","hooks":{"PreToolUse":[{"command":"echo pre"}]}}`
	if err := os.WriteFile(filepath.Join(clDir, "settings.local.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	updated, err := InstallClaudeHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, _ := os.ReadFile(filepath.Join(clDir, "settings.local.json"))
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse merged settings.json: %v", err)
	}
	// Original settings must be preserved.
	if s["theme"] != "dark" {
		t.Errorf("theme lost after merge, got %v", s["theme"])
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("hooks missing after merge")
	}
	// Original PreToolUse hook must survive.
	pre, _ := hooks["PreToolUse"].([]any)
	if len(pre) == 0 {
		t.Fatal("PreToolUse hook lost during merge")
	}
	// Postbrain hooks must be added.
	if _, ok := hooks["PostToolUse"]; !ok {
		t.Fatal("PostToolUse missing after merge")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Fatal("Stop missing after merge")
	}
}

func TestInstallClaudeHooks_Idempotent(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := InstallClaudeHooks(targetDir, "project:acme/api"); err != nil {
		t.Fatalf("first install: %v", err)
	}
	updated, err := InstallClaudeHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if updated {
		t.Fatal("updated = true on second install, want false (idempotent)")
	}

	// Hooks must not be duplicated.
	data, _ := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	if strings.Count(string(data), "postbrain-cli snapshot") != 1 {
		t.Error("postbrain snapshot hook duplicated in settings.json")
	}
}

func TestInstallClaudeHooks_AddsMissingStopWhenSnapshotExists(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	clDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|Bash",
        "hooks": [
          { "type": "command", "command": "postbrain-cli snapshot --scope project:acme/api" }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(clDir, "settings.local.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}

	updated, err := InstallClaudeHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, _ := os.ReadFile(filepath.Join(clDir, "settings.local.json"))
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings.local.json: %v", err)
	}
	hooks, _ := s["hooks"].(map[string]any)
	postToolUse, _ := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("PostToolUse entries = %d, want 1", len(postToolUse))
	}
	stop, _ := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop entries = %d, want 1", len(stop))
	}
}

func TestInstallClaudeHooks_AddsMissingSnapshotWhenStopExists(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	clDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "postbrain-cli summarize-session --scope project:acme/api" }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(clDir, "settings.local.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}

	updated, err := InstallClaudeHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, _ := os.ReadFile(filepath.Join(clDir, "settings.local.json"))
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings.local.json: %v", err)
	}
	hooks, _ := s["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop entries = %d, want 1", len(stop))
	}
	postToolUse, _ := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("PostToolUse entries = %d, want 1", len(postToolUse))
	}
}

package postbraincli

import (
	"encoding/json"
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
	hooksPath := filepath.Join(targetDir, ".codex", "hooks.json")
	if _, err := os.Stat(hooksPath); err != nil {
		t.Fatalf("hooks.json should be created: %v", err)
	}
	hooksData, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(hooksData, &root); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("hooks.json missing hooks map")
	}
	if _, ok := hooks["PostToolUse"]; !ok {
		t.Fatal("hooks.json missing PostToolUse")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Fatal("hooks.json missing Stop")
	}
}

func TestInstallCodexSkill_DoesNotDuplicateHooks(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	_, _, err := InstallCodexSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallCodexSkill first call: %v", err)
	}
	_, _, err = InstallCodexSkill(targetDir, "skill-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallCodexSkill second call: %v", err)
	}

	hooksData, err := os.ReadFile(filepath.Join(targetDir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(hooksData, &root); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("hooks.json missing hooks map")
	}
	postToolUse, _ := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("PostToolUse entries = %d, want 1", len(postToolUse))
	}
	stop, _ := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop entries = %d, want 1", len(stop))
	}
}

func TestInstallCodexSkillWithOptions_DisablesHookInstall(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	installedPath, updatedAgents, err := InstallCodexSkillWithOptions(
		targetDir,
		"skill-content",
		"http://localhost:7433",
		"",
		CodexSkillInstallOptions{InstallHooks: false},
	)
	if err != nil {
		t.Fatalf("InstallCodexSkillWithOptions: %v", err)
	}
	if installedPath == "" {
		t.Fatal("installedPath is empty")
	}
	if updatedAgents {
		t.Fatal("updatedAgents = true, want false without AGENTS.md")
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("hooks.json exists unexpectedly or stat error: %v", err)
	}
}

func TestInstallCodexHooks_AddsMissingStopWhenSnapshotExists(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	configDir := filepath.Join(targetDir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	existing := `{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": "postbrain-cli snapshot --scope project:acme/api" }
        ]
      }
    ]
  }
}`
	hooksPath := filepath.Join(configDir, "hooks.json")
	if err := os.WriteFile(hooksPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write hooks.json: %v", err)
	}

	updated, err := InstallCodexHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("InstallCodexHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop entries = %d, want 1", len(stop))
	}
	postToolUse, _ := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("PostToolUse entries = %d, want 1", len(postToolUse))
	}
}

func TestInstallCodexHooks_AddsMissingSnapshotWhenStopExists(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	configDir := filepath.Join(targetDir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
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
	hooksPath := filepath.Join(configDir, "hooks.json")
	if err := os.WriteFile(hooksPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write hooks.json: %v", err)
	}

	updated, err := InstallCodexHooks(targetDir, "project:acme/api")
	if err != nil {
		t.Fatalf("InstallCodexHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	postToolUse, _ := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("PostToolUse entries = %d, want 1", len(postToolUse))
	}
	stop, _ := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("Stop entries = %d, want 1", len(stop))
	}
}

func TestInstallCodexHooks_QuotesExplicitScopeInCommands(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	scope := "project:acme/api; echo pwned"

	updated, err := InstallCodexHooks(targetDir, scope)
	if err != nil {
		t.Fatalf("InstallCodexHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "--scope 'project:acme/api; echo pwned'") {
		t.Fatalf("hooks.json missing quoted scope: %s", content)
	}
	if strings.Contains(content, "--scope project:acme/api; echo pwned") {
		t.Fatalf("hooks.json contains unquoted scope: %s", content)
	}
}

func TestEnableCodexHooks_CreatesConfigWhenMissing(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	updated, err := EnableCodexHooks(targetDir)
	if err != nil {
		t.Fatalf("EnableCodexHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	configPath := filepath.Join(targetDir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[features]") {
		t.Fatal("config.toml missing [features] section")
	}
	if !strings.Contains(content, "codex_hooks = true") {
		t.Fatal("config.toml missing codex_hooks = true")
	}
}

func TestEnableCodexHooks_MergesWithExistingConfigAndIsIdempotent(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	configDir := filepath.Join(targetDir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	existing := strings.Join([]string{
		"model = \"gpt-5.4\"",
		"",
		"[features]",
		"other_flag = true",
	}, "\n")
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	updated, err := EnableCodexHooks(targetDir)
	if err != nil {
		t.Fatalf("EnableCodexHooks first call: %v", err)
	}
	if !updated {
		t.Fatal("updated = false on first call, want true")
	}

	updated, err = EnableCodexHooks(targetDir)
	if err != nil {
		t.Fatalf("EnableCodexHooks second call: %v", err)
	}
	if updated {
		t.Fatal("updated = true on second call, want false")
	}

	data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "other_flag = true") {
		t.Fatal("existing [features] keys should be preserved")
	}
	if got := strings.Count(content, "codex_hooks = true"); got != 1 {
		t.Fatalf("codex_hooks line count = %d, want 1", got)
	}
}

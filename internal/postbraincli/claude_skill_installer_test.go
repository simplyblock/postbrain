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

	installedPath, updatedClaude, err := InstallClaudeSkill(targetDir, "skill-content", "inner-claude-content", "http://localhost:7433", "project:acme/api")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}
	if !updatedClaude {
		t.Fatal("updatedClaude = false, want true")
	}
	wantPath := filepath.Join(targetDir, ".claude", "skills", "postbrain", "SKILL.md")
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
	if !strings.Contains(content, "@.claude/skills/postbrain/SKILL.md") {
		t.Fatal("CLAUDE.md missing @.claude/skills/postbrain/SKILL.md import")
	}
	if !strings.Contains(content, "POSTBRAIN_URL=http://localhost:7433") {
		t.Fatal("CLAUDE.md missing POSTBRAIN_URL")
	}
	if !strings.Contains(content, "POSTBRAIN_SCOPE=project:acme/api") {
		t.Fatal("CLAUDE.md missing POSTBRAIN_SCOPE")
	}

	basePath := filepath.Join(targetDir, ".claude", "postbrain-base.md")
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read postbrain-base.md: %v", err)
	}
	base := string(baseData)
	if !strings.Contains(base, "---") {
		t.Fatal("postbrain-base.md missing frontmatter markers")
	}
	if !strings.Contains(base, "postbrain_enabled: true") {
		t.Fatal("postbrain-base.md missing postbrain_enabled")
	}
	if !strings.Contains(base, "postbrain_scope: project:acme/api") {
		t.Fatal("postbrain-base.md missing postbrain_scope")
	}
	if !strings.Contains(base, "postbrain_url: http://localhost:7433") {
		t.Fatal("postbrain-base.md missing postbrain_url")
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

	_, updatedClaude, err := InstallClaudeSkill(targetDir, "skill-content", "", "http://localhost:7433", "")
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

func TestInstallClaudeSkill_NoCLAUDEFileCreatesAndUpdatesCLAUDE(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	installedPath, updatedClaude, err := InstallClaudeSkill(targetDir, "skill-content", "", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}
	if !updatedClaude {
		t.Fatal("updatedClaude = false, want true")
	}
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("installed skill stat: %v", err)
	}
	claudeData, err := os.ReadFile(filepath.Join(targetDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(claudeData)
	if !strings.Contains(content, "<!-- postbrain-config -->") {
		t.Fatal("CLAUDE.md missing postbrain marker")
	}
	if !strings.Contains(content, "@.claude/skills/postbrain/SKILL.md") {
		t.Fatal("CLAUDE.md missing @.claude/skills/postbrain/SKILL.md import")
	}
	basePath := filepath.Join(targetDir, ".claude", "postbrain-base.md")
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read postbrain-base.md: %v", err)
	}
	if !strings.Contains(string(baseData), "postbrain_enabled: true") {
		t.Fatal("postbrain-base.md missing postbrain_enabled")
	}
	if !strings.Contains(string(baseData), "postbrain_url: http://localhost:7433") {
		t.Fatal("postbrain-base.md missing postbrain_url")
	}
}

func TestInstallClaudeSkill_RemovesLegacyRootSkillFile(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	legacyPath := filepath.Join(targetDir, ".claude", "postbrain.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy postbrain.md: %v", err)
	}

	_, _, err := InstallClaudeSkill(targetDir, "skill-content", "", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy .claude/postbrain.md should be removed, got stat err=%v", err)
	}
}

func TestInstallClaudeSkill_NoScopeOmitsScopeLine(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	claudePath := filepath.Join(targetDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	_, _, err := InstallClaudeSkill(targetDir, "skill-content", "", "http://localhost:7433", "")
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

// ── .claude/CLAUDE.md installation ───────────────────────────────────────────

func TestInstallClaudeSkill_CreatesDotClaudeCLAUDE_WhenAbsent(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	_, _, err := InstallClaudeSkill(targetDir, "skill-content", "inner-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}

	innerPath := filepath.Join(targetDir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(innerPath)
	if err != nil {
		t.Fatalf("read .claude/CLAUDE.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "inner-content") {
		t.Fatal(".claude/CLAUDE.md missing inner content")
	}
	if !strings.Contains(content, "<!-- postbrain-config -->") {
		t.Fatal(".claude/CLAUDE.md missing postbrain marker")
	}
}

func TestInstallClaudeSkill_MergesDotClaudeCLAUDE_WhenExists(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	claudeDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	innerPath := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(innerPath, []byte("## Existing Instructions\n"), 0o644); err != nil {
		t.Fatalf("write .claude/CLAUDE.md: %v", err)
	}

	_, _, err := InstallClaudeSkill(targetDir, "skill-content", "inner-content", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}

	data, err := os.ReadFile(innerPath)
	if err != nil {
		t.Fatalf("read .claude/CLAUDE.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## Existing Instructions") {
		t.Fatal(".claude/CLAUDE.md lost existing content")
	}
	if !strings.Contains(content, "inner-content") {
		t.Fatal(".claude/CLAUDE.md missing inner content")
	}
}

func TestInstallClaudeSkill_DotClaudeCLAUDE_Idempotent(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	if _, _, err := InstallClaudeSkill(targetDir, "skill-content", "inner-content", "http://localhost:7433", ""); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, _, err := InstallClaudeSkill(targetDir, "skill-content", "inner-content", "http://localhost:7433", ""); err != nil {
		t.Fatalf("second install: %v", err)
	}

	innerPath := filepath.Join(targetDir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(innerPath)
	if err != nil {
		t.Fatalf("read .claude/CLAUDE.md: %v", err)
	}
	if strings.Count(string(data), "<!-- postbrain-config -->") != 1 {
		t.Error(".claude/CLAUDE.md marker duplicated on second install")
	}
	if strings.Count(string(data), "inner-content") != 1 {
		t.Error(".claude/CLAUDE.md inner content duplicated on second install")
	}
}

func TestInstallClaudeSkill_DotClaudeCLAUDE_EmptyContentSkipped(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	_, _, err := InstallClaudeSkill(targetDir, "skill-content", "", "http://localhost:7433", "")
	if err != nil {
		t.Fatalf("InstallClaudeSkill: %v", err)
	}

	innerPath := filepath.Join(targetDir, ".claude", "CLAUDE.md")
	if _, err := os.Stat(innerPath); !os.IsNotExist(err) {
		t.Fatal(".claude/CLAUDE.md should not be created when inner content is empty")
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

	// Snapshot command should use runtime scope resolution (no fixed scope).
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
	if strings.Contains(cmd, "--scope") {
		t.Errorf("PostToolUse command %q should not include fixed scope", cmd)
	}
}

func TestInstallClaudeHooks_NoScope_UsesRuntimeResolutionCommands(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := InstallClaudeHooks(targetDir, ""); err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	content := string(data)
	if !strings.Contains(content, `"command": "postbrain-cli snapshot"`) {
		t.Error("settings.json should use postbrain-cli snapshot when no scope provided")
	}
	if !strings.Contains(content, `"command": "postbrain-cli summarize-session"`) {
		t.Error("settings.json should use postbrain-cli summarize-session when no scope provided")
	}
	if strings.Contains(content, "$POSTBRAIN_SCOPE") {
		t.Error("settings.json should not reference $POSTBRAIN_SCOPE")
	}
}

func TestInstallClaudeHooks_DoesNotInlineScopeFromPostbrainBaseWhenProvidedScopeEmpty(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetDir, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, ".claude", "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-claude\n"), 0o644); err != nil {
		t.Fatalf("write postbrain-base.md: %v", err)
	}

	if _, err := InstallClaudeHooks(targetDir, ""); err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	content := string(data)
	if strings.Contains(content, "--scope") {
		t.Fatalf("settings.local.json should not include fixed scope flags: %s", content)
	}
}

func TestInstallClaudeHooks_RewritesLegacyCommands(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	claudeDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	legacy := `{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|Bash",
        "hooks": [
          { "type": "command", "command": "[ -n \"$POSTBRAIN_SCOPE\" ] && ./postbrain-cli snapshot --scope \"$POSTBRAIN_SCOPE\" || true" }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "[ -n \"$POSTBRAIN_SCOPE\" ] && ./postbrain-cli summarize-session --scope \"$POSTBRAIN_SCOPE\" || true" }
        ]
      }
    ]
  }
}`
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}

	updated, err := InstallClaudeHooks(targetDir, "")
	if err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"command": "postbrain-cli snapshot"`) {
		t.Fatalf("settings.local.json missing rewritten snapshot command: %s", content)
	}
	if !strings.Contains(content, `"command": "postbrain-cli summarize-session"`) {
		t.Fatalf("settings.local.json missing rewritten summarize command: %s", content)
	}
	if strings.Contains(content, "./postbrain-cli") || strings.Contains(content, "$POSTBRAIN_SCOPE") {
		t.Fatalf("settings.local.json still contains legacy command fragments: %s", content)
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

func TestInstallClaudeHooks_QuotesExplicitScopeInCommands(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	scope := "project:acme/api; echo pwned"

	updated, err := InstallClaudeHooks(targetDir, scope)
	if err != nil {
		t.Fatalf("InstallClaudeHooks: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "--scope") {
		t.Fatalf("settings.local.json should not include fixed scope flags: %s", content)
	}
}

func TestInstallClaudeMCPConfig_CreatesProjectMCPJSON(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	updated, err := InstallClaudeMCPConfig(targetDir, "http://localhost:7433")
	if err != nil {
		t.Fatalf("InstallClaudeMCPConfig: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatal(".mcp.json missing mcpServers")
	}
	postbrain, _ := servers["postbrain"].(map[string]any)
	if postbrain == nil {
		t.Fatal(".mcp.json missing postbrain server")
	}
	if postbrain["type"] != "http" {
		t.Fatalf("postbrain.type = %v, want http", postbrain["type"])
	}
	if postbrain["url"] != "http://localhost:7433/mcp" {
		t.Fatalf("postbrain.url = %v, want http://localhost:7433/mcp", postbrain["url"])
	}
	headers, _ := postbrain["headers"].(map[string]any)
	if headers == nil {
		t.Fatal("postbrain.headers missing")
	}
	if headers["Authorization"] != "Bearer ${POSTBRAIN_TOKEN}" {
		t.Fatalf("Authorization header = %v, want Bearer ${POSTBRAIN_TOKEN}", headers["Authorization"])
	}
}

func TestInstallClaudeMCPConfig_IsIdempotent(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	if _, err := InstallClaudeMCPConfig(targetDir, "http://localhost:7433"); err != nil {
		t.Fatalf("first InstallClaudeMCPConfig: %v", err)
	}
	updated, err := InstallClaudeMCPConfig(targetDir, "http://localhost:7433")
	if err != nil {
		t.Fatalf("second InstallClaudeMCPConfig: %v", err)
	}
	if updated {
		t.Fatal("updated = true on second call, want false")
	}
}

// ── InstallClaudePermissions ──────────────────────────────────────────────────

func TestInstallClaudePermissions_CreatesSettingsWithPermissions(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	updated, err := InstallClaudePermissions(targetDir)
	if err != nil {
		t.Fatalf("InstallClaudePermissions: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read settings.local.json: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings.local.json: %v", err)
	}
	perms, _ := s["permissions"].(map[string]any)
	if perms == nil {
		t.Fatal("settings.local.json missing permissions")
	}
	allow, _ := perms["allow"].([]any)
	if len(allow) == 0 {
		t.Fatal("permissions.allow is empty")
	}
	found := false
	for _, v := range allow {
		if v == "mcp__postbrain__*" {
			found = true
		}
	}
	if !found {
		t.Fatalf("permissions.allow does not contain mcp__postbrain__*, got %v", allow)
	}
}

func TestInstallClaudePermissions_IsIdempotent(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	if _, err := InstallClaudePermissions(targetDir); err != nil {
		t.Fatalf("first InstallClaudePermissions: %v", err)
	}
	updated, err := InstallClaudePermissions(targetDir)
	if err != nil {
		t.Fatalf("second InstallClaudePermissions: %v", err)
	}
	if updated {
		t.Fatal("updated = true on second call, want false (idempotent)")
	}

	data, _ := os.ReadFile(filepath.Join(targetDir, ".claude", "settings.local.json"))
	if strings.Count(string(data), "mcp__postbrain__*") != 1 {
		t.Error("mcp__postbrain__* duplicated in settings.local.json")
	}
}

func TestInstallClaudePermissions_MergesIntoExistingSettings(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	clDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `{"theme":"dark","permissions":{"allow":["Bash(git:*)"],"defaultMode":"default"}}`
	if err := os.WriteFile(filepath.Join(clDir, "settings.local.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}

	updated, err := InstallClaudePermissions(targetDir)
	if err != nil {
		t.Fatalf("InstallClaudePermissions: %v", err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}

	data, _ := os.ReadFile(filepath.Join(clDir, "settings.local.json"))
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings.local.json: %v", err)
	}
	if s["theme"] != "dark" {
		t.Errorf("theme lost after merge, got %v", s["theme"])
	}
	perms, _ := s["permissions"].(map[string]any)
	if perms == nil {
		t.Fatal("permissions missing after merge")
	}
	if perms["defaultMode"] != "default" {
		t.Errorf("permissions.defaultMode lost, got %v", perms["defaultMode"])
	}
	allow, _ := perms["allow"].([]any)
	hasGit, hasPostbrain := false, false
	for _, v := range allow {
		switch v {
		case "Bash(git:*)":
			hasGit = true
		case "mcp__postbrain__*":
			hasPostbrain = true
		}
	}
	if !hasGit {
		t.Error("existing Bash(git:*) allow rule lost after merge")
	}
	if !hasPostbrain {
		t.Error("mcp__postbrain__* not added to allow list")
	}
}

func TestInstallClaudePermissions_ErrorOnNonObjectPermissions(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	clDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clDir, "settings.local.json"), []byte(`{"permissions":"bad"}`), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}

	_, err := InstallClaudePermissions(targetDir)
	if err == nil {
		t.Fatal("expected error when permissions is not an object, got nil")
	}
}

func TestInstallClaudePermissions_ErrorOnNonArrayAllow(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()
	clDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(clDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clDir, "settings.local.json"), []byte(`{"permissions":{"allow":"bad"}}`), 0o644); err != nil {
		t.Fatalf("write settings.local.json: %v", err)
	}

	_, err := InstallClaudePermissions(targetDir)
	if err == nil {
		t.Fatal("expected error when permissions.allow is not an array, got nil")
	}
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func TestParseSkillID_ValidUUID_ReturnsID(t *testing.T) {
	t.Parallel()
	want := uuid.New()
	got, err := parseSkillID(want.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseSkillID_InvalidUUID_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSkillID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestParseSkillID_EmptyString_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseSkillID("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

func TestRootVersionCommand_PrintsBuildVersion(t *testing.T) {
	old := buildVersion
	oldRef := buildGitRef
	oldTime := buildTimestamp
	buildVersion = "9.8.7-test"
	buildGitRef = "def5678"
	buildTimestamp = "2026-04-03T14:31:00Z"
	t.Cleanup(func() { buildVersion = old })
	t.Cleanup(func() { buildGitRef = oldRef })
	t.Cleanup(func() { buildTimestamp = oldTime })

	root := newRootCmd()
	root.SetArgs([]string{"version"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := "version=9.8.7-test git=def5678 built=2026-04-03T14:31:00Z"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestCodexVersionMeetsMinimum(t *testing.T) {
	t.Parallel()

	ok, err := codexVersionMeetsMinimum("codex-cli 0.114.0", minimumCodexHooksVersion)
	if err != nil {
		t.Fatalf("codexVersionMeetsMinimum: %v", err)
	}
	if !ok {
		t.Fatal("expected 0.114.0 to satisfy minimum")
	}
}

func TestCodexVersionMeetsMinimum_TooLow(t *testing.T) {
	t.Parallel()

	ok, err := codexVersionMeetsMinimum("codex-cli 0.113.9", minimumCodexHooksVersion)
	if err != nil {
		t.Fatalf("codexVersionMeetsMinimum: %v", err)
	}
	if ok {
		t.Fatal("expected 0.113.9 to be below minimum")
	}
}

func TestCodexVersionMeetsMinimum_InvalidVersion(t *testing.T) {
	t.Parallel()

	_, err := codexVersionMeetsMinimum("codex-cli unknown", minimumCodexHooksVersion)
	if err == nil {
		t.Fatal("expected parse error for invalid version output")
	}
}

func TestCodexSkillContent_WindowsUsesFullProfile(t *testing.T) {
	t.Parallel()
	if got := codexSkillContent("windows"); got != embeddedCodexSkillFull {
		t.Fatal("windows should use full Codex skill profile")
	}
}

func TestCodexSkillContent_NonWindowsUsesLightProfile(t *testing.T) {
	t.Parallel()
	if got := codexSkillContent("linux"); got != embeddedCodexSkillLight {
		t.Fatal("non-windows should use light Codex skill profile")
	}
}

func TestShouldEnforceCodexVersion_WindowsFalse(t *testing.T) {
	t.Parallel()
	if shouldEnforceCodexVersion("windows") {
		t.Fatal("windows should skip codex version enforcement")
	}
}

func TestShouldEnforceCodexVersion_NonWindowsTrue(t *testing.T) {
	t.Parallel()
	if !shouldEnforceCodexVersion("linux") {
		t.Fatal("non-windows should enforce codex version")
	}
}

func TestInstallSkillCommands_ShortDescriptionsUseCurrentPaths(t *testing.T) {
	t.Parallel()

	codexCmd := installCodexSkillCmd()
	if !strings.Contains(codexCmd.Short, ".agents/skills/postbrain/SKILL.md") {
		t.Fatalf("codex installer short description = %q, want updated SKILL.md path", codexCmd.Short)
	}

	claudeCmd := installClaudeSkillCmd()
	if !strings.Contains(claudeCmd.Short, ".claude/skills/postbrain/SKILL.md") {
		t.Fatalf("claude installer short description = %q, want .claude/skills/postbrain/SKILL.md path", claudeCmd.Short)
	}
}

func TestCodexSkillContent_ContainsRequiredFrontmatterFields(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		goos    string
		profile string
	}{
		{goos: "linux", profile: "lite"},
		{goos: "windows", profile: "full"},
	} {
		content := codexSkillContent(tc.goos)
		if !strings.HasPrefix(content, "---\n") {
			t.Fatalf("%s profile missing frontmatter start", tc.profile)
		}
		if !strings.Contains(content, "\nname: ") {
			t.Fatalf("%s profile missing frontmatter field name", tc.profile)
		}
		if !strings.Contains(content, "\ndescription: ") {
			t.Fatalf("%s profile missing frontmatter field description", tc.profile)
		}
		if !strings.Contains(content, "\nversion: ") {
			t.Fatalf("%s profile missing frontmatter field version", tc.profile)
		}
	}
}

func TestEmbeddedClaudeSkill_ContainsRequiredFrontmatterFields(t *testing.T) {
	t.Parallel()

	content := embeddedClaudeSkill
	if !strings.HasPrefix(content, "---\n") {
		t.Fatal("embedded Claude skill missing frontmatter start")
	}
	if !strings.Contains(content, "\nname: ") {
		t.Fatal("embedded Claude skill missing frontmatter field name")
	}
	if !strings.Contains(content, "\ndescription: ") {
		t.Fatal("embedded Claude skill missing frontmatter field description")
	}
	if !strings.Contains(content, "\nversion: ") {
		t.Fatal("embedded Claude skill missing frontmatter field version")
	}
}

func TestSkillAssets_UseAgentsBasePath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "codex-lite", content: embeddedCodexSkillLight},
		{name: "codex-full", content: embeddedCodexSkillFull},
		{name: "claude", content: embeddedClaudeSkill},
	} {
		if strings.Contains(tc.content, ".agent/postbrain-base.md") {
			t.Fatalf("%s skill uses deprecated .agent base path", tc.name)
		}
		if !strings.Contains(tc.content, ".agents/postbrain-base.md") {
			t.Fatalf("%s skill missing .agents base path guidance", tc.name)
		}
	}
}

func TestSkillAssets_ContainExecutionPatterns(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "codex-lite", content: embeddedCodexSkillLight},
		{name: "codex-full", content: embeddedCodexSkillFull},
		{name: "claude", content: embeddedClaudeSkill},
	} {
		if !strings.Contains(tc.content, "## Gotchas") {
			t.Fatalf("%s skill missing Gotchas section", tc.name)
		}
		if !strings.Contains(tc.content, "## Workflow Checklist") {
			t.Fatalf("%s skill missing Workflow Checklist section", tc.name)
		}
		if !strings.Contains(tc.content, "## Validation Loop") {
			t.Fatalf("%s skill missing Validation Loop section", tc.name)
		}
	}
}

func TestSkillAssets_DocumentPostbrainBaseFileFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "codex-lite", content: embeddedCodexSkillLight},
		{name: "codex-full", content: embeddedCodexSkillFull},
		{name: "claude", content: embeddedClaudeSkill},
	} {
		if !strings.Contains(tc.content, "postbrain_enabled: true") {
			t.Fatalf("%s skill missing postbrain_enabled format guidance", tc.name)
		}
		if !strings.Contains(tc.content, "postbrain_scope: project:acme/api") {
			t.Fatalf("%s skill missing postbrain_scope format guidance", tc.name)
		}
		if !strings.Contains(tc.content, "updated_at: YYYY-MM-DD") {
			t.Fatalf("%s skill missing updated_at format guidance", tc.name)
		}
	}
}

func TestSkillAssets_DescriptionsAreTriggerOriented(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "codex-lite", content: embeddedCodexSkillLight},
		{name: "codex-full", content: embeddedCodexSkillFull},
		{name: "claude", content: embeddedClaudeSkill},
	} {
		if !strings.Contains(tc.content, "\ndescription: Use this skill when ") {
			t.Fatalf("%s skill description should use trigger-oriented imperative phrasing", tc.name)
		}
	}
}

func TestCheckUpdateCommand_UpdateAvailable(t *testing.T) {
	oldBuild := buildVersion
	oldFetch := fetchLatestPostbrainVersionFn
	buildVersion = "0.0.1"
	fetchLatestPostbrainVersionFn = func(ctx context.Context) (string, error) {
		return "0.0.3", nil
	}
	t.Cleanup(func() { buildVersion = oldBuild })
	t.Cleanup(func() { fetchLatestPostbrainVersionFn = oldFetch })

	root := newRootCmd()
	root.SetArgs([]string{"check-update"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute check-update command: %v", err)
	}
	if !strings.Contains(out.String(), "update available") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestCheckUpdateCommand_UpToDate(t *testing.T) {
	oldBuild := buildVersion
	oldFetch := fetchLatestPostbrainVersionFn
	buildVersion = "0.0.3"
	fetchLatestPostbrainVersionFn = func(ctx context.Context) (string, error) {
		return "0.0.3", nil
	}
	t.Cleanup(func() { buildVersion = oldBuild })
	t.Cleanup(func() { fetchLatestPostbrainVersionFn = oldFetch })

	root := newRootCmd()
	root.SetArgs([]string{"check-update"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute check-update command: %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestCheckUpdateCommand_DevBuild(t *testing.T) {
	oldBuild := buildVersion
	oldFetch := fetchLatestPostbrainVersionFn
	buildVersion = "dev"
	fetchLatestPostbrainVersionFn = func(ctx context.Context) (string, error) {
		return "0.0.3", nil
	}
	t.Cleanup(func() { buildVersion = oldBuild })
	t.Cleanup(func() { fetchLatestPostbrainVersionFn = oldFetch })

	root := newRootCmd()
	root.SetArgs([]string{"check-update"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute check-update command: %v", err)
	}
	if !strings.Contains(out.String(), "unable to compare dev build") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestBuildSnapshotDescription_WriteIncludesContentSize(t *testing.T) {
	t.Parallel()

	desc := buildSnapshotDescription("Write", map[string]any{
		"file_path": "/tmp/main.go",
		"content":   "package main\n\nfunc main() {}\n",
	}, "file:/tmp/main.go")

	if !strings.Contains(desc, "Tool Write called on file:/tmp/main.go") {
		t.Fatalf("description missing source ref: %q", desc)
	}
	if !strings.Contains(desc, "content_bytes=") {
		t.Fatalf("description missing content size hint: %q", desc)
	}
}

func TestBuildSnapshotDescription_BashIncludesCommandAndTruncates(t *testing.T) {
	t.Parallel()

	cmdText := strings.Repeat("echo very-long-command ", 20)
	desc := buildSnapshotDescription("Bash", map[string]any{
		"command": cmdText,
	}, "")

	if !strings.Contains(desc, "Tool Bash called") {
		t.Fatalf("description missing tool call prefix: %q", desc)
	}
	if !strings.Contains(desc, "command=") {
		t.Fatalf("description missing command summary: %q", desc)
	}
	if strings.Contains(desc, cmdText) {
		t.Fatalf("description should truncate very long command: %q", desc)
	}
}

func TestBuildSnapshotDescription_UsesKnownInputFields(t *testing.T) {
	t.Parallel()

	desc := buildSnapshotDescription("Read", map[string]any{
		"path":    "/tmp/readme.md",
		"pattern": "TODO",
		"limit":   25,
	}, "")

	wantParts := []string{
		"Tool Read called",
		"path=/tmp/readme.md",
		"pattern=TODO",
		"limit=25",
	}
	for _, part := range wantParts {
		if !strings.Contains(desc, part) {
			t.Fatalf("description missing %q: %q", part, desc)
		}
	}
}

func TestBuildSnapshotDescription_LargeInputFallsBackToKeyCount(t *testing.T) {
	t.Parallel()

	input := map[string]any{}
	for i := 0; i < 40; i++ {
		input[fmt.Sprintf("k%d", i)] = i
	}

	desc := buildSnapshotDescription("Edit", input, "")
	if !strings.Contains(desc, "input_keys=40") {
		t.Fatalf("description missing key-count fallback: %q", desc)
	}
}

func TestNormalizePathToProjectRoot_AbsoluteUnderCWD_ReturnsRelative(t *testing.T) {
	origGetwd := getwdFn
	getwdFn = func() (string, error) { return "/Volumes/git/postbrain", nil }
	t.Cleanup(func() { getwdFn = origGetwd })

	got := normalizePathToProjectRoot("/Volumes/git/postbrain/internal/codegraph/lsp/pyright.go")
	want := "internal/codegraph/lsp/pyright.go"
	if got != want {
		t.Fatalf("normalizePathToProjectRoot() = %q, want %q", got, want)
	}
}

func TestNormalizePathToProjectRoot_PathOutsideCWD_StaysAbsolute(t *testing.T) {
	origGetwd := getwdFn
	getwdFn = func() (string, error) { return "/Volumes/git/postbrain", nil }
	t.Cleanup(func() { getwdFn = origGetwd })

	got := normalizePathToProjectRoot("/tmp/other/file.go")
	want := "/tmp/other/file.go"
	if got != want {
		t.Fatalf("normalizePathToProjectRoot() = %q, want %q", got, want)
	}
}

func TestNormalizeSnapshotToolInputPaths_FilePathAndPath(t *testing.T) {
	origGetwd := getwdFn
	getwdFn = func() (string, error) { return "/Volumes/git/postbrain", nil }
	t.Cleanup(func() { getwdFn = origGetwd })

	in := map[string]any{
		"file_path": "/Volumes/git/postbrain/cmd/postbrain-cli/main.go",
		"path":      "/Volumes/git/postbrain/internal/codegraph/lsp/pyright.go",
		"query":     "snapshot",
	}
	out := normalizeSnapshotToolInputPaths(in)

	if out["file_path"] != "cmd/postbrain-cli/main.go" {
		t.Fatalf("file_path = %v, want relative path", out["file_path"])
	}
	if out["path"] != "internal/codegraph/lsp/pyright.go" {
		t.Fatalf("path = %v, want relative path", out["path"])
	}
	if out["query"] != "snapshot" {
		t.Fatalf("query = %v, want unchanged", out["query"])
	}
}

func TestResolveScopeForInstall_PrefersEnvVar(t *testing.T) {
	t.Setenv("POSTBRAIN_SCOPE", "project:env/scope")

	targetDir := t.TempDir()
	codexDir := filepath.Join(targetDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:file/scope\n"), 0o644); err != nil {
		t.Fatalf("write codex postbrain-base.md: %v", err)
	}

	got := resolveScopeForInstall(targetDir)
	if got != "project:env/scope" {
		t.Fatalf("resolveScopeForInstall() = %q, want env scope", got)
	}
}

func TestResolveScopeForInstall_PriorityOrder(t *testing.T) {
	t.Setenv("POSTBRAIN_SCOPE", "")
	targetDir := t.TempDir()

	codexDir := filepath.Join(targetDir, ".codex")
	claudeDir := filepath.Join(targetDir, ".claude")
	agentsDir := filepath.Join(targetDir, ".agents")
	for _, dir := range []string{codexDir, claudeDir, agentsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(codexDir, "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-codex\n"), 0o644); err != nil {
		t.Fatalf("write codex postbrain-base.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-claude\n"), 0o644); err != nil {
		t.Fatalf("write claude postbrain-base.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-agents\n"), 0o644); err != nil {
		t.Fatalf("write agents postbrain-base.md: %v", err)
	}

	got := resolveScopeForInstall(targetDir)
	if got != "project:from-codex" {
		t.Fatalf("resolveScopeForInstall() = %q, want codex scope", got)
	}
}

func TestResolveScopeForInstall_FallsBackToClaudeThenAgents(t *testing.T) {
	t.Setenv("POSTBRAIN_SCOPE", "")
	targetDir := t.TempDir()

	claudeDir := filepath.Join(targetDir, ".claude")
	agentsDir := filepath.Join(targetDir, ".agents")
	for _, dir := range []string{claudeDir, agentsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-claude\n"), 0o644); err != nil {
		t.Fatalf("write claude postbrain-base.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-agents\n"), 0o644); err != nil {
		t.Fatalf("write agents postbrain-base.md: %v", err)
	}

	got := resolveScopeForInstall(targetDir)
	if got != "project:from-claude" {
		t.Fatalf("resolveScopeForInstall() = %q, want claude scope", got)
	}

	if err := os.Remove(filepath.Join(claudeDir, "postbrain-base.md")); err != nil {
		t.Fatalf("remove claude postbrain-base.md: %v", err)
	}
	got = resolveScopeForInstall(targetDir)
	if got != "project:from-agents" {
		t.Fatalf("resolveScopeForInstall() = %q, want agents scope", got)
	}
}

func TestResolveURLForInstall_PrefersEnvVar(t *testing.T) {
	t.Setenv("POSTBRAIN_URL", "http://env-url:7433")
	targetDir := t.TempDir()
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&bytes.Buffer{})

	got, err := resolveURLForInstall(cmd, targetDir)
	if err != nil {
		t.Fatalf("resolveURLForInstall: %v", err)
	}
	if got != "http://env-url:7433" {
		t.Fatalf("resolveURLForInstall() = %q, want env URL", got)
	}
}

func TestResolveURLForInstall_FallsBackToBaseFile(t *testing.T) {
	t.Setenv("POSTBRAIN_URL", "")
	targetDir := t.TempDir()
	agentsDir := filepath.Join(targetDir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "postbrain-base.md"), []byte("postbrain_url: http://from-base:7433\n"), 0o644); err != nil {
		t.Fatalf("write postbrain-base.md: %v", err)
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&bytes.Buffer{})

	got, err := resolveURLForInstall(cmd, targetDir)
	if err != nil {
		t.Fatalf("resolveURLForInstall: %v", err)
	}
	if got != "http://from-base:7433" {
		t.Fatalf("resolveURLForInstall() = %q, want base URL", got)
	}
}

func TestResolveURLForInstall_PromptsAndUsesDefaultOnEmptyInput(t *testing.T) {
	t.Setenv("POSTBRAIN_URL", "")
	targetDir := t.TempDir()
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("\n"))
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	got, err := resolveURLForInstall(cmd, targetDir)
	if err != nil {
		t.Fatalf("resolveURLForInstall: %v", err)
	}
	if got != "http://localhost:7433" {
		t.Fatalf("resolveURLForInstall() = %q, want default URL", got)
	}
	if !strings.Contains(errOut.String(), "Postbrain backend URL") {
		t.Fatalf("expected prompt output, got %q", errOut.String())
	}
}

func TestResolveURLForInstall_PromptsAndUsesUserInput(t *testing.T) {
	t.Setenv("POSTBRAIN_URL", "")
	targetDir := t.TempDir()
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("http://custom-host:8123/\n"))
	cmd.SetErr(&bytes.Buffer{})

	got, err := resolveURLForInstall(cmd, targetDir)
	if err != nil {
		t.Fatalf("resolveURLForInstall: %v", err)
	}
	if got != "http://custom-host:8123" {
		t.Fatalf("resolveURLForInstall() = %q, want trimmed custom URL", got)
	}
}

func TestResolveScopeForRuntime_PrefersScopeFlag(t *testing.T) {
	prev := scopeFlag
	scopeFlag = "project:from-flag"
	t.Cleanup(func() { scopeFlag = prev })
	t.Setenv("POSTBRAIN_SCOPE", "project:from-env")

	if got := resolveScopeForRuntime(); got != "project:from-flag" {
		t.Fatalf("resolveScopeForRuntime() = %q, want project:from-flag", got)
	}
}

func TestResolveScopeForRuntime_FallsBackToCwdPostbrainBase(t *testing.T) {
	prevFlag := scopeFlag
	scopeFlag = ""
	t.Cleanup(func() { scopeFlag = prevFlag })
	t.Setenv("POSTBRAIN_SCOPE", "")

	targetDir := t.TempDir()
	prevGetwd := getwdFn
	getwdFn = func() (string, error) { return targetDir, nil }
	t.Cleanup(func() { getwdFn = prevGetwd })

	if err := os.MkdirAll(filepath.Join(targetDir, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, ".codex", "postbrain-base.md"), []byte("POSTBRAIN_SCOPE=project:from-cwd\n"), 0o644); err != nil {
		t.Fatalf("write postbrain-base.md: %v", err)
	}

	if got := resolveScopeForRuntime(); got != "project:from-cwd" {
		t.Fatalf("resolveScopeForRuntime() = %q, want project:from-cwd", got)
	}
}

func TestResolveURLForRuntime_PrefersEnvVar(t *testing.T) {
	prevGetwd := getwdFn
	getwdFn = func() (string, error) { return "", nil }
	t.Cleanup(func() { getwdFn = prevGetwd })
	t.Setenv("POSTBRAIN_URL", "http://env-url:7433/")

	if got := resolveURLForRuntime(); got != "http://env-url:7433" {
		t.Fatalf("resolveURLForRuntime() = %q, want env URL", got)
	}
}

func TestResolveURLForRuntime_FallsBackToCwdPostbrainBase(t *testing.T) {
	t.Setenv("POSTBRAIN_URL", "")
	targetDir := t.TempDir()

	prevGetwd := getwdFn
	getwdFn = func() (string, error) { return targetDir, nil }
	t.Cleanup(func() { getwdFn = prevGetwd })

	if err := os.MkdirAll(filepath.Join(targetDir, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, ".agents", "postbrain-base.md"), []byte("postbrain_url: http://from-cwd:7433\n"), 0o644); err != nil {
		t.Fatalf("write postbrain-base.md: %v", err)
	}

	if got := resolveURLForRuntime(); got != "http://from-cwd:7433" {
		t.Fatalf("resolveURLForRuntime() = %q, want http://from-cwd:7433", got)
	}
}

func TestResolveURLForRuntime_DefaultWhenUnset(t *testing.T) {
	prevGetwd := getwdFn
	getwdFn = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { getwdFn = prevGetwd })
	t.Setenv("POSTBRAIN_URL", "")

	if got := resolveURLForRuntime(); got != "http://localhost:7433" {
		t.Fatalf("resolveURLForRuntime() = %q, want default URL", got)
	}
}

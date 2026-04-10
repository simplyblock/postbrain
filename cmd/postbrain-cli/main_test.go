package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
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

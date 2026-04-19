package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

// ── ValidateSlug ──────────────────────────────────────────────────────────────

func TestValidateSlug_ValidSlugs(t *testing.T) {
	t.Parallel()
	valid := []string{
		"my-skill",
		"deploy",
		"a",
		"a1",
		"hello-world",
		"skill123",
		"abc_def",
		"0starts-with-digit",
		strings.Repeat("a", 64), // max length
	}
	for _, s := range valid {
		s := s
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSlug(s); err != nil {
				t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
			}
		})
	}
}

func TestValidateSlug_InvalidSlugs(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"",                        // empty
		"../../../etc/passwd",     // path traversal
		"../../tmp/pwned",         // path traversal
		"/absolute/path",          // absolute path
		"has space",               // space
		"has.dot",                 // dot separator
		"UPPERCASE",               // uppercase
		"has/slash",               // forward slash
		"has\\backslash",          // backslash
		strings.Repeat("a", 65),   // too long
		"-starts-with-dash",       // leading dash
		"_starts-with-underscore", // leading underscore
	}
	for _, s := range invalid {
		s := s
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSlug(s); err == nil {
				t.Errorf("ValidateSlug(%q) = nil, want error", s)
			}
		})
	}
}

// ── TargetPath ────────────────────────────────────────────────────────────────

func TestTargetPath_ClaudeCode(t *testing.T) {
	t.Parallel()
	got := TargetPath("my-skill", "claude-code", "/work")
	want := filepath.Join("/work", ".claude", "skills", "my-skill", "SKILL.md")
	if got != want {
		t.Errorf("TargetPath = %q, want %q", got, want)
	}
}

func TestTargetPath_Codex(t *testing.T) {
	t.Parallel()
	got := TargetPath("deploy", "codex", "/work")
	want := filepath.Join("/work", ".agents", "skills", "deploy", "SKILL.md")
	if got != want {
		t.Errorf("TargetPath = %q, want %q", got, want)
	}
}

// ── Install path-traversal regression tests ───────────────────────────────────

func TestInstall_TraversalSlug_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("../../etc/passwd")
	_, err := Install(skill, nil, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with traversal slug must return an error, got nil")
	}
}

func TestInstall_AbsoluteSlug_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("/etc/cron.d/evil")
	_, err := Install(skill, nil, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with absolute-path slug must return an error, got nil")
	}
}

func TestInstall_SlugWithDot_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("bad.slug")
	_, err := Install(skill, nil, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with dot-containing slug must return an error, got nil")
	}
}

// ── Install path tests ────────────────────────────────────────────────────────

func TestInstall_ClaudeCodePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("my-skill")
	path, err := Install(skill, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	// Install resolves symlinks (e.g. /var → /private/var on macOS); resolve dir too.
	realDir, _ := filepath.EvalSymlinks(dir)
	expected := filepath.Join(realDir, ".claude", "skills", "my-skill", "SKILL.md")
	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not found at expected path: %v", err)
	}
}

func TestInstall_CodexPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("deploy")
	path, err := Install(skill, nil, "codex", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	realDir, _ := filepath.EvalSymlinks(dir)
	expected := filepath.Join(realDir, ".agents", "skills", "deploy", "SKILL.md")
	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not found at expected path: %v", err)
	}
}

// ── IsInstalled ───────────────────────────────────────────────────────────────

func TestIsInstalled_TrueAfterInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("check-skill")
	if _, err := Install(skill, nil, "claude-code", dir); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if !IsInstalled("check-skill", "claude-code", dir) {
		t.Error("expected IsInstalled=true after install")
	}
}

func TestIsInstalled_FalseForUnknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if IsInstalled("no-such-skill", "claude-code", dir) {
		t.Error("expected IsInstalled=false for unknown slug")
	}
}

// ── Frontmatter ───────────────────────────────────────────────────────────────

func TestInstall_FrontmatterPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("frontmatter-skill")
	path, err := Install(skill, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	s := string(content)
	if !strings.HasPrefix(s, "---\n") {
		t.Error("expected file to start with ---")
	}
	if !strings.Contains(s, "name: Test Skill") {
		t.Error("expected frontmatter to contain name")
	}
	if !strings.Contains(s, "description: Does something useful.") {
		t.Error("expected frontmatter to contain description")
	}
}

func TestInstall_OverwritesExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("overwrite-skill")
	if _, err := Install(skill, nil, "claude-code", dir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	skill.Body = "Updated body content."
	path, err := Install(skill, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(content), "Updated body content.") {
		t.Error("expected updated body in file after overwrite")
	}
}

// ── Symlink escape ─────────────────────────────────────────────────────────────

// TestInstall_SymlinkEscape_ReturnsError verifies that a symlink under workdir
// pointing outside the base directory is detected and rejected.
func TestInstall_SymlinkEscape_ReturnsError(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	base := filepath.Join(outer, "workdir")
	outside := filepath.Join(outer, "outside")

	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}

	// Create workdir/.claude as a symlink pointing to outer/outside.
	dotClaude := filepath.Join(base, ".claude")
	if err := os.Symlink(outside, dotClaude); err != nil {
		t.Skipf("symlink not supported on this OS/FS: %v", err)
	}

	skill := makeTestSkill("my-skill")
	_, err := Install(skill, nil, "claude-code", base)
	if err == nil {
		t.Fatal("Install through symlink-escaped directory must return an error, got nil")
	}
}

// ── Multi-file Install ────────────────────────────────────────────────────────

func TestInstall_WithScriptFile_GoesInScriptsDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("multi-skill")
	files := []*db.SkillFile{
		makeTestSkillFile("scripts/run.sh", "#!/bin/sh\necho hi", true),
	}
	path, err := Install(skill, files, "claude-code", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
	scriptPath := filepath.Join(dir, ".claude", "skills", "multi-skill", "scripts", "run.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("script file not found at %s: %v", scriptPath, err)
	}
	content, _ := os.ReadFile(scriptPath)
	if string(content) != "#!/bin/sh\necho hi" {
		t.Errorf("script content = %q, want %q", content, "#!/bin/sh\necho hi")
	}
}

func TestInstall_WithReferenceFile_GoesInReferencesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("ref-skill")
	files := []*db.SkillFile{
		makeTestSkillFile("references/guide.md", "# Guide", false),
	}
	_, err := Install(skill, files, "claude-code", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	refPath := filepath.Join(dir, ".claude", "skills", "ref-skill", "references", "guide.md")
	if _, err := os.Stat(refPath); err != nil {
		t.Errorf("reference file not found at %s: %v", refPath, err)
	}
}

func TestInstall_WithFiles_ExecutableBit_Set(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("chmod bits not enforced for root")
	}
	dir := t.TempDir()
	skill := makeTestSkill("exec-skill")
	files := []*db.SkillFile{
		makeTestSkillFile("scripts/tool.sh", "#!/bin/sh", true),
	}
	if _, err := Install(skill, files, "claude-code", dir); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	scriptPath := filepath.Join(dir, ".claude", "skills", "exec-skill", "scripts", "tool.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("script mode = %v, want executable bit set", info.Mode())
	}
}

func TestInstall_WithFiles_TraversalRelativePath_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("good-skill")
	// ValidateSkillFile will reject this before filesystem ops.
	files := []*db.SkillFile{{
		ID:           uuid.New(),
		SkillID:      uuid.New(),
		RelativePath: "scripts/../../../etc/passwd",
		Content:      "bad",
		IsExecutable: true,
	}}
	_, err := Install(skill, files, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with traversal relative_path must return an error, got nil")
	}
}

func TestInstall_NoFiles_BehaviorUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("simple-skill")
	path, err := Install(skill, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	realDir, _ := filepath.EvalSymlinks(dir)
	expected := filepath.Join(realDir, ".claude", "skills", "simple-skill", "SKILL.md")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
	// No scripts/ or references/ directories should exist.
	skillDir := filepath.Join(dir, ".claude", "skills", "simple-skill")
	entries, _ := os.ReadDir(skillDir)
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" {
		t.Errorf("expected only SKILL.md in skill dir, got %v", entries)
	}
}

// ── CLAUDE.md injection (claude-code only) ────────────────────────────────────

func TestInstall_ClaudeCode_InjectsClaudeMDReference(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("inject-skill")
	if _, err := Install(skill, nil, "claude-code", dir); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	claudeMD := filepath.Join(dir, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatalf(".claude/CLAUDE.md not created: %v", err)
	}
	wantRef := "@skills/inject-skill/SKILL.md"
	if !strings.Contains(string(content), wantRef) {
		t.Errorf(".claude/CLAUDE.md does not contain %q:\n%s", wantRef, content)
	}
}

func TestInstall_ClaudeCode_IdempotentClaudeMDInjection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("idem-skill")
	// Install twice.
	for i := 0; i < 2; i++ {
		if _, err := Install(skill, nil, "claude-code", dir); err != nil {
			t.Fatalf("Install #%d error: %v", i+1, err)
		}
	}
	claudeMD := filepath.Join(dir, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatalf(".claude/CLAUDE.md not found: %v", err)
	}
	ref := "@skills/idem-skill/SKILL.md"
	count := strings.Count(string(content), ref)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of %q in .claude/CLAUDE.md, got %d", ref, count)
	}
}

func TestInstall_Codex_DoesNotInjectClaudeMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("codex-skill")
	if _, err := Install(skill, nil, "codex", dir); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	claudeMD := filepath.Join(dir, ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err == nil {
		t.Error("Install for codex must not create .claude/CLAUDE.md")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeTestSkill(slug string) *db.Skill {
	return &db.Skill{
		Slug:        slug,
		Name:        "Test Skill",
		Description: "Does something useful.",
		AgentTypes:  []string{"any"},
		Body:        "Run the test for $TARGET.",
		Parameters:  []byte(`[{"name":"target","type":"string","required":true,"description":"The target"}]`),
		Version:     1,
	}
}

func makeTestSkillFile(relativePath, content string, isExecutable bool) *db.SkillFile {
	return &db.SkillFile{
		ID:           uuid.New(),
		SkillID:      uuid.New(),
		RelativePath: relativePath,
		Content:      content,
		IsExecutable: isExecutable,
	}
}

package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// ── Install path-traversal regression tests ───────────────────────────────────

// TestInstall_TraversalSlug_ReturnsError is the regression test for
// path-traversal via a malicious slug.  Before the fix, a slug like
// "../../etc/passwd" would cause Install to write outside the expected
// base directory.
func TestInstall_TraversalSlug_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("../../etc/passwd")
	_, err := Install(skill, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with traversal slug must return an error, got nil")
	}
}

func TestInstall_AbsoluteSlug_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("/etc/cron.d/evil")
	_, err := Install(skill, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with absolute-path slug must return an error, got nil")
	}
}

func TestInstall_SlugWithDot_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("bad.slug")
	_, err := Install(skill, "claude-code", dir)
	if err == nil {
		t.Fatal("Install with dot-containing slug must return an error, got nil")
	}
}

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

func TestInstall_ClaudeCodePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("my-skill")
	path, err := Install(skill, "claude-code", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	expected := filepath.Join(dir, ".claude", "commands", "my-skill.md")
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
	path, err := Install(skill, "codex", dir)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	expected := filepath.Join(dir, ".codex", "skills", "deploy.md")
	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not found at expected path: %v", err)
	}
}

func TestIsInstalled_TrueAfterInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("check-skill")
	if _, err := Install(skill, "claude-code", dir); err != nil {
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

func TestInstall_FrontmatterPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("frontmatter-skill")
	path, err := Install(skill, "claude-code", dir)
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
	if _, err := Install(skill, "claude-code", dir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	skill.Body = "Updated body content."
	path, err := Install(skill, "claude-code", dir)
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

// TestInstall_SymlinkEscape_ReturnsError verifies that a symlink under workdir
// pointing outside the base directory is detected and rejected.
// Before the EvalSymlinks fix, filepath.Abs alone would not follow symlinks,
// so a symlink like workdir/.claude → /tmp/outside could bypass the check.
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
	_, err := Install(skill, "claude-code", base)
	if err == nil {
		t.Fatal("Install through symlink-escaped directory must return an error, got nil")
	}
}

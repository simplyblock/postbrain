package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
)

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

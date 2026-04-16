package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakeSyncDB implements syncDB for unit tests.
type fakeSyncDB struct {
	skills []*db.Skill
}

func (f *fakeSyncDB) listPublishedSkillsForAgent(_ context.Context, _ []uuid.UUID, _ string) ([]*db.Skill, error) {
	return f.skills, nil
}

func newTestSync(skills []*db.Skill) (*fakeSyncDB, syncDB) {
	fdb := &fakeSyncDB{skills: skills}
	return fdb, fdb
}

func TestSync_EmptyRegistry_OrphanedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a local file that is not in the registry.
	cmdDir := filepath.Join(dir, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "old-skill.md"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	_, fdb := newTestSync(nil)
	result, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Installed) != 0 {
		t.Errorf("expected no installs, got %v", result.Installed)
	}
	if len(result.Orphaned) != 1 || result.Orphaned[0] != "old-skill" {
		t.Errorf("expected orphaned=[old-skill], got %v", result.Orphaned)
	}
}

func TestSync_SkillNotInstalled_AppearsInInstalled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	skill := &db.Skill{
		ID:         uuid.New(),
		Slug:       "new-skill",
		Name:       "New Skill",
		Body:       "Do the thing.",
		AgentTypes: []string{"any"},
		Parameters: []byte("[]"),
		Version:    1,
	}
	_, fdb := newTestSync([]*db.Skill{skill})
	result, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Installed) != 1 || result.Installed[0] != "new-skill" {
		t.Errorf("expected installed=[new-skill], got %v", result.Installed)
	}
}

func TestSync_OldVersionInstalled_AppearsInUpdated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	skill := &db.Skill{
		ID:          uuid.New(),
		Slug:        "versioned-skill",
		Name:        "Versioned Skill",
		Body:        "v2 body.",
		AgentTypes:  []string{"any"},
		Parameters:  []byte("[]"),
		Version:     2,
		Description: "A skill",
	}

	// Install version 1 first.
	v1 := *skill
	v1.Version = 1
	if _, err := Install(&v1, "claude-code", dir); err != nil {
		t.Fatalf("install v1: %v", err)
	}

	_, fdb := newTestSync([]*db.Skill{skill})
	result, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Updated) != 1 || result.Updated[0] != "versioned-skill" {
		t.Errorf("expected updated=[versioned-skill], got %v", result.Updated)
	}
}

func TestSync_DeprecatedSkill_AppearsInOrphaned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	skill := &db.Skill{
		ID:         uuid.New(),
		Slug:       "deprecated-skill",
		Name:       "Deprecated Skill",
		Body:       "old body.",
		AgentTypes: []string{"any"},
		Parameters: []byte("[]"),
		Version:    1,
	}
	// Install first.
	if _, err := Install(skill, "claude-code", dir); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Registry has no published skills (deprecated skill not returned by ListPublishedSkillsForAgent).
	_, fdb := newTestSync(nil)
	result, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Orphaned) != 1 || result.Orphaned[0] != "deprecated-skill" {
		t.Errorf("expected orphaned=[deprecated-skill], got %v", result.Orphaned)
	}
}

// ── readInstalledVersion path-traversal regression tests ──────────────────────

// TestReadInstalledVersion_TraversalSlug_ReturnsZero verifies that a slug
// containing path-traversal sequences is rejected and readInstalledVersion
// returns 0 (treated as not installed / outdated) without opening any file.
func TestReadInstalledVersion_TraversalSlug_ReturnsZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if v := readInstalledVersion("../../etc/passwd", "claude-code", dir); v != 0 {
		t.Errorf("readInstalledVersion with traversal slug = %d, want 0", v)
	}
}

func TestReadInstalledVersion_AbsoluteSlug_ReturnsZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if v := readInstalledVersion("/etc/passwd", "claude-code", dir); v != 0 {
		t.Errorf("readInstalledVersion with absolute slug = %d, want 0", v)
	}
}

func TestReadInstalledVersion_DotSlug_ReturnsZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if v := readInstalledVersion("bad.slug", "claude-code", dir); v != 0 {
		t.Errorf("readInstalledVersion with dot slug = %d, want 0", v)
	}
}

// TestReadInstalledVersion_ValidSlug_ReadsVersion verifies that a valid slug
// still reads the version field correctly (no regression in the happy path).
func TestReadInstalledVersion_ValidSlug_ReadsVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skill := makeTestSkill("my-skill")
	skill.Version = 7
	if _, err := Install(skill, "claude-code", dir); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if v := readInstalledVersion("my-skill", "claude-code", dir); v != 7 {
		t.Errorf("readInstalledVersion = %d, want 7", v)
	}
}

// TestSyncInternal_TraversalSlugInDB_ReturnsError verifies that syncInternal
// returns an error rather than silently opening or writing outside the base
// directory when the DB contains a skill with a traversal slug.
func TestSyncInternal_TraversalSlugInDB_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, fdb := newTestSync([]*db.Skill{makeTestSkill("../../etc/passwd")})
	_, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err == nil {
		t.Fatal("syncInternal with traversal slug in DB must return an error, got nil")
	}
}

// TestReadInstalledVersion_TraversalEscapesBase_ReturnsZero is the definitive
// regression test: it creates a real file at the path that a traversal slug
// would resolve to outside the intended base, then verifies readInstalledVersion
// returns 0 rather than opening that file.
func TestReadInstalledVersion_TraversalEscapesBase_ReturnsZero(t *testing.T) {
	t.Parallel()

	// dir/base is the "workdir". The traversal slug target lands in dir/escape/.
	outer := t.TempDir()
	base := filepath.Join(outer, "workdir")
	if err := os.MkdirAll(filepath.Join(base, ".claude", "commands"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create the file that the traversal would reach: outer/escape.md
	escapePath := filepath.Join(outer, "escape.md")
	content := "---\nversion: 99\n---\n"
	if err := os.WriteFile(escapePath, []byte(content), 0644); err != nil {
		t.Fatalf("write escape file: %v", err)
	}

	// The traversal slug would resolve to outer/escape.md:
	//   filepath.Join(base, ".claude", "commands", "../../../escape.md")
	//   = outer/workdir/.claude/commands/../../../escape.md = outer/escape.md
	// readInstalledVersion must return 0, NOT 99.
	if v := readInstalledVersion("../../../escape", "claude-code", base); v != 0 {
		t.Errorf("readInstalledVersion returned %d from an escaped path — traversal not blocked", v)
	}
}

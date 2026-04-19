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
	files  map[uuid.UUID][]*db.SkillFile // skillID → files
}

func (f *fakeSyncDB) listPublishedSkillsForAgent(_ context.Context, _ []uuid.UUID, _ string) ([]*db.Skill, error) {
	return f.skills, nil
}

func (f *fakeSyncDB) listSkillFiles(_ context.Context, skillID uuid.UUID) ([]*db.SkillFile, error) {
	return f.files[skillID], nil
}

func newTestSync(skills []*db.Skill) (*fakeSyncDB, syncDB) {
	fdb := &fakeSyncDB{skills: skills, files: make(map[uuid.UUID][]*db.SkillFile)}
	return fdb, fdb
}

// writeInstalledSkill creates a minimal SKILL.md for a slug in the new path layout.
func writeInstalledSkill(t *testing.T, dir, agentType, slug string, version int) {
	t.Helper()
	target := TargetPath(slug, agentType, dir)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", slug, err)
	}
	content := "---\nversion: " + itoa(version) + "\n---\n\nbody\n"
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md for %s: %v", slug, err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// ── Sync core behaviour ───────────────────────────────────────────────────────

func TestSync_EmptyRegistry_OrphanedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a local skill directory that is not in the registry.
	writeInstalledSkill(t, dir, "claude-code", "old-skill", 1)

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

	// Install version 1 first via the helper (bypasses DB).
	writeInstalledSkill(t, dir, "claude-code", "versioned-skill", 1)

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

	writeInstalledSkill(t, dir, "claude-code", "deprecated-skill", 1)

	// Registry has no published skills.
	_, fdb := newTestSync(nil)
	result, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Orphaned) != 1 || result.Orphaned[0] != "deprecated-skill" {
		t.Errorf("expected orphaned=[deprecated-skill], got %v", result.Orphaned)
	}
}

// ── Multi-file sync ───────────────────────────────────────────────────────────

func TestSync_SkillWithFiles_InstallsSupplementaryFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	skill := &db.Skill{
		ID:         uuid.New(),
		Slug:       "multi-skill",
		Name:       "Multi Skill",
		Body:       "body",
		AgentTypes: []string{"any"},
		Parameters: []byte("[]"),
		Version:    1,
	}
	fdb, sdb := newTestSync([]*db.Skill{skill})
	fdb.files[skill.ID] = []*db.SkillFile{
		{ID: uuid.New(), SkillID: skill.ID, RelativePath: "scripts/run.sh", Content: "#!/bin/sh\necho hi", IsExecutable: true},
	}

	if _, err := syncInternal(context.Background(), sdb, nil, "claude-code", dir); err != nil {
		t.Fatalf("syncInternal: %v", err)
	}

	scriptPath := filepath.Join(dir, ".claude", "skills", "multi-skill", "scripts", "run.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("supplementary file not installed at %s: %v", scriptPath, err)
	}
}

func TestSync_SkillWithFiles_UpdatesSupplementaryFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	skill := &db.Skill{
		ID:          uuid.New(),
		Slug:        "update-multi",
		Name:        "Update Multi",
		Body:        "v2",
		AgentTypes:  []string{"any"},
		Parameters:  []byte("[]"),
		Version:     2,
		Description: "d",
	}

	// Pre-install version 1.
	writeInstalledSkill(t, dir, "claude-code", "update-multi", 1)

	fdb, sdb := newTestSync([]*db.Skill{skill})
	fdb.files[skill.ID] = []*db.SkillFile{
		{ID: uuid.New(), SkillID: skill.ID, RelativePath: "scripts/new.sh", Content: "#!/bin/sh\necho new", IsExecutable: true},
	}

	result, err := syncInternal(context.Background(), sdb, nil, "claude-code", dir)
	if err != nil {
		t.Fatalf("syncInternal: %v", err)
	}
	if len(result.Updated) != 1 || result.Updated[0] != "update-multi" {
		t.Errorf("expected updated=[update-multi], got %v", result.Updated)
	}
	scriptPath := filepath.Join(dir, ".claude", "skills", "update-multi", "scripts", "new.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("updated supplementary file not at %s: %v", scriptPath, err)
	}
}

func TestSync_SkillWithFiles_TraversalRelativePath_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	skill := &db.Skill{
		ID:         uuid.New(),
		Slug:       "bad-skill",
		Name:       "Bad",
		Body:       "body",
		AgentTypes: []string{"any"},
		Parameters: []byte("[]"),
		Version:    1,
	}
	fdb, sdb := newTestSync([]*db.Skill{skill})
	fdb.files[skill.ID] = []*db.SkillFile{
		{ID: uuid.New(), SkillID: skill.ID, RelativePath: "scripts/../../../etc/passwd", Content: "evil", IsExecutable: true},
	}

	_, err := syncInternal(context.Background(), sdb, nil, "claude-code", dir)
	if err == nil {
		t.Fatal("syncInternal with traversal relative_path must return an error, got nil")
	}
}

// ── readInstalledVersion path-traversal regression tests ─────────────────────

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

func TestReadInstalledVersion_ValidSlug_ReadsVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeInstalledSkill(t, dir, "claude-code", "my-skill", 7)
	if v := readInstalledVersion("my-skill", "claude-code", dir); v != 7 {
		t.Errorf("readInstalledVersion = %d, want 7", v)
	}
}

func TestSyncInternal_TraversalSlugInDB_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, fdb := newTestSync([]*db.Skill{makeTestSkill("../../etc/passwd")})
	_, err := syncInternal(context.Background(), fdb, nil, "claude-code", dir)
	if err == nil {
		t.Fatal("syncInternal with traversal slug in DB must return an error, got nil")
	}
}

func TestReadInstalledVersion_TraversalEscapesBase_ReturnsZero(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	base := filepath.Join(outer, "workdir")
	if err := os.MkdirAll(filepath.Join(base, ".claude", "skills"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create the file that a traversal would reach: outer/escape/SKILL.md
	escapePath := filepath.Join(outer, "escape", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(escapePath), 0755); err != nil {
		t.Fatalf("mkdir escape: %v", err)
	}
	content := "---\nversion: 99\n---\n"
	if err := os.WriteFile(escapePath, []byte(content), 0644); err != nil {
		t.Fatalf("write escape file: %v", err)
	}

	if v := readInstalledVersion("../../../escape", "claude-code", base); v != 0 {
		t.Errorf("readInstalledVersion returned %d from an escaped path — traversal not blocked", v)
	}
}

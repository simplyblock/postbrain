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

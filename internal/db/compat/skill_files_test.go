//go:build integration

package compat_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestSkillFiles_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)

	// We need a scope, principal, and skill to satisfy FK constraints.
	var scopeID, authorID, skillID uuid.UUID

	// Insert a principal first (scopes.principal_id is NOT NULL).
	err := pool.QueryRow(ctx,
		`INSERT INTO principals (kind, slug, display_name) VALUES ('user','tester','Tester') RETURNING id`,
	).Scan(&authorID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	// Insert a scope (path is computed by trigger; principal_id is required).
	err = pool.QueryRow(ctx,
		`INSERT INTO scopes (kind, external_id, name, principal_id) VALUES ('project','test-skill-files','Test',$1) RETURNING id`,
		authorID,
	).Scan(&scopeID)
	if err != nil {
		t.Fatalf("insert scope: %v", err)
	}

	// Insert a skill.
	err = pool.QueryRow(ctx,
		`INSERT INTO skills (scope_id, author_id, slug, name, description, body, visibility, status)
		 VALUES ($1, $2, 'test-skill', 'Test Skill', 'desc', 'body', 'team', 'draft')
		 RETURNING id`,
		scopeID, authorID,
	).Scan(&skillID)
	if err != nil {
		t.Fatalf("insert skill: %v", err)
	}

	t.Run("UpsertAndList", func(t *testing.T) {
		f, err := compat.UpsertSkillFile(ctx, pool, skillID, "scripts/run.sh", "#!/bin/sh\necho hi", true)
		if err != nil {
			t.Fatalf("UpsertSkillFile: %v", err)
		}
		if f.RelativePath != "scripts/run.sh" {
			t.Errorf("RelativePath = %q, want %q", f.RelativePath, "scripts/run.sh")
		}
		if !f.IsExecutable {
			t.Error("IsExecutable = false, want true")
		}

		files, err := compat.ListSkillFiles(ctx, pool, skillID)
		if err != nil {
			t.Fatalf("ListSkillFiles: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("len(files) = %d, want 1", len(files))
		}
		if files[0].Content != "#!/bin/sh\necho hi" {
			t.Errorf("Content = %q, want %q", files[0].Content, "#!/bin/sh\necho hi")
		}
	})

	t.Run("UpsertUpdatesExisting", func(t *testing.T) {
		_, err := compat.UpsertSkillFile(ctx, pool, skillID, "references/guide.md", "# v1", false)
		if err != nil {
			t.Fatalf("UpsertSkillFile v1: %v", err)
		}
		updated, err := compat.UpsertSkillFile(ctx, pool, skillID, "references/guide.md", "# v2", false)
		if err != nil {
			t.Fatalf("UpsertSkillFile v2: %v", err)
		}
		if updated.Content != "# v2" {
			t.Errorf("Content = %q, want %q", updated.Content, "# v2")
		}
	})

	t.Run("DeleteSkillFile", func(t *testing.T) {
		_, err := compat.UpsertSkillFile(ctx, pool, skillID, "scripts/tmp.sh", "echo tmp", true)
		if err != nil {
			t.Fatalf("UpsertSkillFile: %v", err)
		}
		if err := compat.DeleteSkillFile(ctx, pool, skillID, "scripts/tmp.sh"); err != nil {
			t.Fatalf("DeleteSkillFile: %v", err)
		}
		files, err := compat.ListSkillFiles(ctx, pool, skillID)
		if err != nil {
			t.Fatalf("ListSkillFiles: %v", err)
		}
		for _, f := range files {
			if f.RelativePath == "scripts/tmp.sh" {
				t.Error("deleted file still present")
			}
		}
	})

	t.Run("SnapshotAndListHistory", func(t *testing.T) {
		// Ensure at least one file exists.
		_, err := compat.UpsertSkillFile(ctx, pool, skillID, "scripts/snap.sh", "echo snap", true)
		if err != nil {
			t.Fatalf("UpsertSkillFile: %v", err)
		}

		if err := compat.SnapshotSkillFiles(ctx, pool, skillID, 1); err != nil {
			t.Fatalf("SnapshotSkillFiles: %v", err)
		}

		history, err := compat.ListSkillHistoryFiles(ctx, pool, skillID, 1)
		if err != nil {
			t.Fatalf("ListSkillHistoryFiles: %v", err)
		}
		found := false
		for _, h := range history {
			if h.RelativePath == "scripts/snap.sh" {
				found = true
				if h.Content != "echo snap" {
					t.Errorf("snapshot Content = %q, want %q", h.Content, "echo snap")
				}
			}
		}
		if !found {
			t.Error("snapshot not found for scripts/snap.sh")
		}
	})

	t.Run("DeleteAllSkillFiles", func(t *testing.T) {
		// Create two files.
		_, _ = compat.UpsertSkillFile(ctx, pool, skillID, "scripts/a.sh", "a", true)
		_, _ = compat.UpsertSkillFile(ctx, pool, skillID, "scripts/b.sh", "b", true)

		if err := compat.DeleteAllSkillFiles(ctx, pool, skillID); err != nil {
			t.Fatalf("DeleteAllSkillFiles: %v", err)
		}
		files, err := compat.ListSkillFiles(ctx, pool, skillID)
		if err != nil {
			t.Fatalf("ListSkillFiles after delete: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("expected 0 files after DeleteAllSkillFiles, got %d", len(files))
		}
	})
}

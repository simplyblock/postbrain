//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestStalenessFlag_SourceModified(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "stale-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_stale", nil, author.ID)

	// Insert a published artifact directly (bypassing lifecycle for speed).
	var artifactID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO knowledge_artifacts
		    (knowledge_type, owner_scope_id, author_id, visibility, status,
		     published_at, review_required, title, content, version)
		VALUES ('semantic', $1, $2, 'team', 'published', now(), 1, 'Stale Test', 'content', 1)
		RETURNING id
	`, scope.ID, author.ID).Scan(&artifactID)
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Insert a source_modified staleness flag.
	flag := &db.StalenessFlag{
		ArtifactID: artifactID,
		Signal:     "source_modified",
		Confidence: 0.9,
		Evidence:   []byte(`{"files": ["src/auth.go"]}`),
		Status:     "open",
	}
	inserted, err := db.InsertStalenessFlag(ctx, pool, flag)
	if err != nil {
		t.Fatalf("InsertStalenessFlag: %v", err)
	}
	if inserted.Status != "open" {
		t.Errorf("expected open, got %s", inserted.Status)
	}

	// The HasOpenStalenessFlag check should find our flag.
	hasFlag, err := db.HasOpenStalenessFlag(ctx, pool, artifactID, "source_modified")
	if err != nil {
		t.Fatalf("HasOpenStalenessFlag: %v", err)
	}
	if !hasFlag {
		t.Error("expected open flag to be detected")
	}

	// A different signal should not be detected.
	hasOther, err := db.HasOpenStalenessFlag(ctx, pool, artifactID, "contradiction_detected")
	if err != nil {
		t.Fatalf("HasOpenStalenessFlag other signal: %v", err)
	}
	if hasOther {
		t.Error("should not find a flag for a different signal")
	}
}

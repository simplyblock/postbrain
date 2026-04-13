//go:build integration

package jobs

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestContradictionJob_Run_EmptyDB(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// No artifacts in DB → fetchArtifactBatch returns empty → Run returns nil immediately.
	j := NewContradictionJob(pool, svc, nil)
	if err := j.Run(ctx); err != nil {
		t.Fatalf("Run on empty DB: %v", err)
	}
}

func TestContradictionJob_Run_ContradictionFlagInserted(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "contra-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "contra-scope", nil, author.ID)
	artifact := testhelper.CreateTestArtifact(t, pool, scope.ID, author.ID, "Contradiction Artifact")

	// Compute the vector that the job will use as the negation query.
	// By storing it as both the artifact embedding and the memory embedding,
	// both the topic-similarity filter (> 0.6) and the negation filter (> 0.5) yield 1.0.
	negationQuery := artifact.Title + " is false, wrong, or outdated"
	negVec, err := svc.EmbedText(ctx, negationQuery)
	if err != nil {
		t.Fatalf("embed negation query: %v", err)
	}
	vecStr := db.ExportFloat32SliceToVector(negVec)

	// Set the artifact's embedding to negVec.
	if _, err := pool.Exec(ctx,
		`UPDATE knowledge_artifacts SET embedding = $1::vector WHERE id = $2`,
		vecStr, artifact.ID,
	); err != nil {
		t.Fatalf("set artifact embedding: %v", err)
	}

	// Create a recent memory and set its embedding to negVec.
	mem := testhelper.CreateTestMemory(t, pool, scope.ID, author.ID, "contradicting observation")
	if _, err := pool.Exec(ctx,
		`UPDATE memories SET embedding = $1::vector WHERE id = $2`,
		vecStr, mem.ID,
	); err != nil {
		t.Fatalf("set memory embedding: %v", err)
	}

	// Inject a classifier that always returns CONTRADICTS.
	fakeClassifier := func(_ context.Context, _, _ string) (string, string, error) {
		return "CONTRADICTS", "forced contradiction for test", nil
	}

	j := NewContradictionJob(pool, svc, fakeClassifier)
	if err := j.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	hasFlag, err := compat.HasOpenStalenessFlag(ctx, pool, artifact.ID, "contradiction_detected")
	if err != nil {
		t.Fatalf("HasOpenStalenessFlag: %v", err)
	}
	if !hasFlag {
		t.Error("expected a contradiction_detected staleness flag to be inserted")
	}
}

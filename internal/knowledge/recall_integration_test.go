//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// recallText is a content string that is unique enough to survive FTS stopword
// filtering and produce non-trivial trigram overlap with itself.
const recallText = "knowledge recall integration artifact"

// makePublished creates a knowledge artifact in the given scope with
// status=published using AutoPublish=true.
func makePublished(t *testing.T, ctx context.Context, store *knowledge.Store, scopeID, authorID uuid.UUID, knowledgeType, content string) *db.KnowledgeArtifact {
	t.Helper()
	a, err := store.Create(ctx, knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		OwnerScopeID:  scopeID,
		AuthorID:      authorID,
		Visibility:    "project",
		Title:         "inttest-" + knowledgeType,
		Content:       content,
		AutoPublish:   true,
	})
	if err != nil {
		t.Fatalf("create %s artifact: %v", knowledgeType, err)
	}
	return a
}

// TestRecall_EmptyQuery_NoResults verifies that calling Recall with an empty
// query and a non-nil scope completes without error and returns no results when
// the scope contains no published artifacts.
func TestRecall_EmptyQuery_NoResults(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "recall-empty-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-empty-"+uuid.New().String(), nil, principal.ID)
	store := knowledge.NewStore(pool, svc)

	results, err := store.Recall(ctx, pool, knowledge.RecallInput{
		Query:   "",
		ScopeID: scope.ID,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0 (empty scope + empty query)", len(results))
	}
}

// TestRecall_LimitZeroClampedToDefault verifies that a Limit of 0 is clamped
// to 10 before the DB query is issued. If Limit=0 were forwarded verbatim,
// LIMIT 0 would return zero rows even when matching artifacts exist.
func TestRecall_LimitZeroClampedToDefault(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "recall-limit-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-limit-"+uuid.New().String(), nil, principal.ID)
	store := knowledge.NewStore(pool, svc)

	makePublished(t, ctx, store, scope.ID, principal.ID, "semantic", recallText)

	results, err := store.Recall(ctx, pool, knowledge.RecallInput{
		Query:   recallText,
		ScopeID: scope.ID,
		Limit:   0, // must be clamped to 10
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result; Limit=0 should be clamped to 10, not passed as LIMIT 0 to the DB")
	}
}

// TestRecall_ScoreMerging verifies that when the same artifact is found by
// multiple recall layers (vector, FTS, trigram), it appears exactly once in the
// results with a combined score that incorporates all contributing layers.
func TestRecall_ScoreMerging(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "recall-merge-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-merge-"+uuid.New().String(), nil, principal.ID)
	store := knowledge.NewStore(pool, svc)

	// Content equals the query so FakeEmbedder produces identical vectors
	// (cosine distance = 0) AND the text matches for FTS/trigram.
	artifact := makePublished(t, ctx, store, scope.ID, principal.ID, "semantic", recallText)

	results, err := store.Recall(ctx, pool, knowledge.RecallInput{
		Query:   recallText,
		ScopeID: scope.ID,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result, got 0")
	}

	// Artifact must appear exactly once (merge deduplication worked).
	var count int
	var found *knowledge.ArtifactResult
	for _, r := range results {
		if r.Artifact.ID == artifact.ID {
			count++
			found = r
		}
	}
	if count != 1 {
		t.Fatalf("artifact appears %d times in results, want exactly 1", count)
	}

	// Combined score must be positive.
	if found.Score <= 0 {
		t.Errorf("Score = %v, want > 0", found.Score)
	}

	// At least one text-match layer (FTS or trigram) must have contributed.
	if found.BM25Score == 0 && found.TrgmScore == 0 {
		t.Error("BM25Score and TrgmScore are both 0; expected the artifact to " +
			"appear in at least one text-match recall layer")
	}
}

// TestRecall_DigestSuppression verifies that a source artifact is excluded from
// recall results when a published digest that covers it also appears in those
// results.
func TestRecall_DigestSuppression(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "recall-digest-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "recall-digest-"+uuid.New().String(), nil, principal.ID)
	store := knowledge.NewStore(pool, svc)

	// Both artifacts share content so both appear in recall results.
	source := makePublished(t, ctx, store, scope.ID, principal.ID, "semantic", recallText)
	digest := makePublished(t, ctx, store, scope.ID, principal.ID, "digest", recallText)

	// Record the digest → source relationship.
	if err := db.InsertDigestSources(ctx, pool, digest.ID, []uuid.UUID{source.ID}); err != nil {
		t.Fatalf("InsertDigestSources: %v", err)
	}

	results, err := store.Recall(ctx, pool, knowledge.RecallInput{
		Query:   recallText,
		ScopeID: scope.ID,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result (the digest)")
	}

	var foundDigest, foundSource bool
	for _, r := range results {
		switch r.Artifact.ID {
		case digest.ID:
			foundDigest = true
		case source.ID:
			foundSource = true
		}
	}
	if !foundDigest {
		t.Error("digest artifact should be present in recall results")
	}
	if foundSource {
		t.Error("source artifact should be suppressed from results when its published digest is present")
	}
}

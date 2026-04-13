//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestEndorsement_AutoPublish(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "endo-author")
	endorser := testhelper.CreateTestPrincipal(t, pool, "user", "endo-endorser")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_endo", nil, author.ID)

	knwStore := knowledge.NewStore(pool, svc)
	artifact, err := knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType:  "semantic",
		OwnerScopeID:   scope.ID,
		AuthorID:       author.ID,
		Visibility:     "team",
		Title:          "Test Artifact",
		Content:        "Content to endorse",
		AutoReview:     true, // start in_review immediately
		ReviewRequired: 1,
	})
	if err != nil {
		t.Fatalf("Create artifact: %v", err)
	}
	if artifact.Status != "in_review" {
		t.Fatalf("expected in_review, got %s", artifact.Status)
	}

	// nil membership: endorser is never the author, so admin check is not needed.
	lifecycle := knowledge.NewLifecycle(pool, nil)
	result, err := lifecycle.Endorse(ctx, artifact.ID, endorser.ID, nil)
	if err != nil {
		t.Fatalf("Endorse: %v", err)
	}
	if !result.AutoPublished {
		t.Error("expected auto_published=true")
	}
	if result.Status != "published" {
		t.Errorf("expected published, got %s", result.Status)
	}
}

func TestEndorsement_SelfEndorsementRejected(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "self-endo-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_selfendo", nil, author.ID)

	knwStore := knowledge.NewStore(pool, svc)
	artifact, err := knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType:  "semantic",
		OwnerScopeID:   scope.ID,
		AuthorID:       author.ID,
		Visibility:     "team",
		Title:          "Self Endo Test",
		Content:        "Content",
		AutoReview:     true,
		ReviewRequired: 1,
	})
	if err != nil {
		t.Fatalf("Create artifact: %v", err)
	}

	lifecycle := knowledge.NewLifecycle(pool, nil)
	_, err = lifecycle.Endorse(ctx, artifact.ID, author.ID, nil)
	if err == nil {
		t.Error("expected ErrSelfEndorsement")
	}
}

// ── SubmitForReview / RetractToDraft ──────────────────────────────────────────

func TestLifecycle_SubmitForReview_RetractToDraft(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "retract-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "retract-scope", nil, author.ID)

	knwStore := knowledge.NewStore(pool, svc)
	artifact, err := knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  scope.ID,
		AuthorID:      author.ID,
		Visibility:    "project",
		Title:         "Retract Test",
		Content:       "content",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if artifact.Status != "draft" {
		t.Fatalf("expected draft, got %s", artifact.Status)
	}

	lc := knowledge.NewLifecycle(pool, nil)

	if err := lc.SubmitForReview(ctx, artifact.ID, author.ID); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	got, err := compat.GetArtifact(ctx, pool, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact after SubmitForReview: %v", err)
	}
	if got.Status != "in_review" {
		t.Errorf("expected in_review, got %s", got.Status)
	}

	if err := lc.RetractToDraft(ctx, artifact.ID, author.ID); err != nil {
		t.Fatalf("RetractToDraft: %v", err)
	}
	got, err = compat.GetArtifact(ctx, pool, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact after RetractToDraft: %v", err)
	}
	if got.Status != "draft" {
		t.Errorf("expected draft, got %s", got.Status)
	}
}

// ── Republish ─────────────────────────────────────────────────────────────────

func TestLifecycle_Republish(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "republish-author")
	endorser := testhelper.CreateTestPrincipal(t, pool, "user", "republish-endorser")
	scope := testhelper.CreateTestScope(t, pool, "project", "republish-scope", nil, author.ID)

	ms := principals.NewMembershipStore(pool)
	lc := knowledge.NewLifecycle(pool, ms)

	knwStore := knowledge.NewStore(pool, svc)
	artifact, err := knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType:  "semantic",
		OwnerScopeID:   scope.ID,
		AuthorID:       author.ID,
		Visibility:     "project",
		Title:          "Republish Test",
		Content:        "content",
		AutoReview:     true,
		ReviewRequired: 1,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Publish via endorsement.
	if _, err := lc.Endorse(ctx, artifact.ID, endorser.ID, nil); err != nil {
		t.Fatalf("Endorse: %v", err)
	}

	// Deprecate (author owns the scope → admin).
	if err := lc.Deprecate(ctx, artifact.ID, author.ID); err != nil {
		t.Fatalf("Deprecate: %v", err)
	}

	// Republish.
	if err := lc.Republish(ctx, artifact.ID, author.ID); err != nil {
		t.Fatalf("Republish: %v", err)
	}

	got, err := compat.GetArtifact(ctx, pool, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact after Republish: %v", err)
	}
	if got.Status != "published" {
		t.Errorf("expected published, got %s", got.Status)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestLifecycle_Delete(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "delete-lc-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "delete-lc-scope", nil, author.ID)

	ms := principals.NewMembershipStore(pool)
	lc := knowledge.NewLifecycle(pool, ms)

	knwStore := knowledge.NewStore(pool, svc)
	artifact, err := knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  scope.ID,
		AuthorID:      author.ID,
		Visibility:    "project",
		Title:         "Delete Test",
		Content:       "content",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := lc.Delete(ctx, artifact.ID, author.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := compat.GetArtifact(ctx, pool, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact after Delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after Delete, got record with status %s", got.Status)
	}
}

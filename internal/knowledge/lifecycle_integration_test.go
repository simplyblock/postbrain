//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/knowledge"
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

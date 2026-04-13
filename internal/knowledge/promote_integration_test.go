//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestPromotionWorkflow(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewDeterministicEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "promo-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_promo", nil, author.ID)

	// Create a memory to promote.
	memStore := memory.NewStore(pool, svc)
	r, err := memStore.Create(ctx, memory.CreateInput{
		Content:    "Important architectural decision about caching",
		MemoryType: "semantic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	})
	if err != nil {
		t.Fatalf("Create memory: %v", err)
	}

	// Nominate for promotion.
	promoter := knowledge.NewPromoter(pool, svc)
	title := "Caching Architecture Decision"
	req, err := promoter.CreateRequest(ctx, knowledge.PromoteInput{
		MemoryID:         r.MemoryID,
		RequestedBy:      author.ID,
		TargetScopeID:    scope.ID,
		TargetVisibility: "team",
		ProposedTitle:    &title,
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if req.Status != "pending" {
		t.Errorf("expected pending, got %s", req.Status)
	}

	// Verify memory is marked nominated.
	mem, err := compat.GetMemory(ctx, pool, r.MemoryID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem.PromotionStatus != "nominated" {
		t.Errorf("expected nominated, got %s", mem.PromotionStatus)
	}

	// Approve promotion.
	artifact, err := promoter.Approve(ctx, req.ID, author.ID, author.ID)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if artifact.Status != "draft" {
		t.Errorf("expected draft, got %s", artifact.Status)
	}

	// Verify memory is marked promoted.
	mem, err = compat.GetMemory(ctx, pool, r.MemoryID)
	if err != nil {
		t.Fatalf("GetMemory after approve: %v", err)
	}
	if mem.PromotionStatus != "promoted" {
		t.Errorf("expected promoted, got %s", mem.PromotionStatus)
	}
	if mem.PromotedTo == nil || *mem.PromotedTo != artifact.ID {
		t.Errorf("expected promoted_to=%v, got %v", artifact.ID, mem.PromotedTo)
	}
}

func TestPromotionWorkflow_RenominationRejected(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewDeterministicEmbeddingService()
	ctx := context.Background()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "renominate-author")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme_renominate", nil, author.ID)

	memStore := memory.NewStore(pool, svc)
	r, err := memStore.Create(ctx, memory.CreateInput{
		Content:    "Already nominated memory",
		MemoryType: "semantic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	})
	if err != nil {
		t.Fatalf("Create memory: %v", err)
	}

	promoter := knowledge.NewPromoter(pool, svc)
	title := "First nomination"
	_, err = promoter.CreateRequest(ctx, knowledge.PromoteInput{
		MemoryID:         r.MemoryID,
		RequestedBy:      author.ID,
		TargetScopeID:    scope.ID,
		TargetVisibility: "team",
		ProposedTitle:    &title,
	})
	if err != nil {
		t.Fatalf("first CreateRequest: %v", err)
	}

	// Second nomination should fail.
	_, err = promoter.CreateRequest(ctx, knowledge.PromoteInput{
		MemoryID:         r.MemoryID,
		RequestedBy:      author.ID,
		TargetScopeID:    scope.ID,
		TargetVisibility: "team",
		ProposedTitle:    &title,
	})
	if err == nil {
		t.Error("expected ErrAlreadyPromoted, got nil")
	}
}

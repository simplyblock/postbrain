//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestDeleteScope_WithPromotionRequests_Succeeds(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	suffix := uuid.NewString()
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "scope-delete-promo-user-"+suffix)
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-delete-promo-scope-"+suffix, nil, principal.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "scope delete promotion memory")

	promo, err := compat.CreatePromotionRequest(ctx, pool, &db.PromotionRequest{
		MemoryID:         memory.ID,
		RequestedBy:      principal.ID,
		TargetScopeID:    scope.ID,
		TargetVisibility: "project",
	})
	if err != nil {
		t.Fatalf("CreatePromotionRequest: %v", err)
	}

	q := db.New(pool)
	if err := q.UpdatePromotionRequest(ctx, db.UpdatePromotionRequestParams{
		ID:         promo.ID,
		Status:     "approved",
		ReviewerID: &principal.ID,
	}); err != nil {
		t.Fatalf("UpdatePromotionRequest approved: %v", err)
	}

	if err := compat.DeleteScope(ctx, pool, scope.ID); err != nil {
		t.Fatalf("DeleteScope: %v", err)
	}
}

//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestResolveVisibleScopeIDs_NoParentNoPersonal verifies that a root scope
// with no parent and no personal scope returns exactly that scope ID.
func TestResolveVisibleScopeIDs_NoParentNoPersonal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "vis-root-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "vis-root-"+uuid.New().String(), nil, principal.ID)

	ids, err := knowledge.ResolveVisibleScopeIDs(ctx, pool, principal.ID, scope.ID)
	if err != nil {
		t.Fatalf("ResolveVisibleScopeIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 scope ID, got %d: %v", len(ids), ids)
	}
	if ids[0] != scope.ID {
		t.Errorf("expected scope ID %v, got %v", scope.ID, ids[0])
	}
}

// TestResolveVisibleScopeIDs_WithParent verifies that a child scope returns
// both its own ID and its parent's ID in the result.
func TestResolveVisibleScopeIDs_WithParent(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "vis-parent-"+uuid.New().String())
	parent := testhelper.CreateTestScope(t, pool, "project", "vis-parent-"+uuid.New().String(), nil, principal.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "vis-child-"+uuid.New().String(), &parent.ID, principal.ID)

	ids, err := knowledge.ResolveVisibleScopeIDs(ctx, pool, principal.ID, child.ID)
	if err != nil {
		t.Fatalf("ResolveVisibleScopeIDs: %v", err)
	}

	idSet := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	if _, ok := idSet[child.ID]; !ok {
		t.Errorf("child scope ID %v missing from result %v", child.ID, ids)
	}
	if _, ok := idSet[parent.ID]; !ok {
		t.Errorf("parent scope ID %v missing from result %v", parent.ID, ids)
	}
}

// TestResolveVisibleScopeIDs_NoPersonalScope verifies that when the principal
// has no personal scope, ResolveVisibleScopeIDs still returns the ancestor chain
// without error. This exercises the getPersonalScope nil-return path.
func TestResolveVisibleScopeIDs_NoPersonalScope(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	// Use a random principal ID that has no personal scope registered.
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "vis-nopersonal-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "vis-nopersonal-"+uuid.New().String(), nil, principal.ID)

	ids, err := knowledge.ResolveVisibleScopeIDs(ctx, pool, principal.ID, scope.ID)
	if err != nil {
		t.Fatalf("ResolveVisibleScopeIDs: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == scope.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("requested scope ID %v not in result %v", scope.ID, ids)
	}
}

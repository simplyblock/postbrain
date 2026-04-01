//go:build integration

package principals_test

import (
	"context"
	"errors"
	"testing"

	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMembershipStore_AddMembership_ValidRole(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	member := testhelper.CreateTestPrincipal(t, pool, "user", "mem-add-member")
	parent := testhelper.CreateTestPrincipal(t, pool, "team", "mem-add-parent")

	if err := ms.AddMembership(ctx, member.ID, parent.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
}

func TestMembershipStore_AddMembership_InvalidRole(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	member := testhelper.CreateTestPrincipal(t, pool, "user", "mem-badrole-member")
	parent := testhelper.CreateTestPrincipal(t, pool, "team", "mem-badrole-parent")

	err := ms.AddMembership(ctx, member.ID, parent.ID, "superadmin", nil)
	if !errors.Is(err, principals.ErrInvalidRole) {
		t.Errorf("expected ErrInvalidRole, got %v", err)
	}
}

func TestMembershipStore_AddMembership_CycleDetection(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	// Chain: C → B → A. Adding A → C would create a cycle.
	a := testhelper.CreateTestPrincipal(t, pool, "user", "mem-cycle-a")
	b := testhelper.CreateTestPrincipal(t, pool, "team", "mem-cycle-b")
	c := testhelper.CreateTestPrincipal(t, pool, "department", "mem-cycle-c")

	if err := ms.AddMembership(ctx, b.ID, a.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership B→A: %v", err)
	}
	if err := ms.AddMembership(ctx, c.ID, b.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership C→B: %v", err)
	}

	// A→C would form a cycle: A is an ancestor of C.
	err := ms.AddMembership(ctx, a.ID, c.ID, "member", nil)
	if !errors.Is(err, principals.ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestMembershipStore_RemoveMembership(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	member := testhelper.CreateTestPrincipal(t, pool, "user", "mem-remove-member")
	parent := testhelper.CreateTestPrincipal(t, pool, "team", "mem-remove-parent")

	if err := ms.AddMembership(ctx, member.ID, parent.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	if err := ms.RemoveMembership(ctx, member.ID, parent.ID); err != nil {
		t.Fatalf("RemoveMembership: %v", err)
	}
}

func TestMembershipStore_EffectiveScopeIDs(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	// Set up: child principal is a member of parent principal.
	// Each has its own scope. EffectiveScopeIDs(child) should include both.
	parent := testhelper.CreateTestPrincipal(t, pool, "team", "mem-eff-parent")
	child := testhelper.CreateTestPrincipal(t, pool, "user", "mem-eff-child")

	scopeParent := testhelper.CreateTestScope(t, pool, "project", "mem-eff-scope-parent", nil, parent.ID)
	scopeChild := testhelper.CreateTestScope(t, pool, "project", "mem-eff-scope-child", nil, child.ID)

	if err := ms.AddMembership(ctx, child.ID, parent.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	ids, err := ms.EffectiveScopeIDs(ctx, child.ID)
	if err != nil {
		t.Fatalf("EffectiveScopeIDs: %v", err)
	}

	found := make(map[string]bool)
	for _, id := range ids {
		found[id.String()] = true
	}
	if !found[scopeParent.ID.String()] {
		t.Errorf("parent scope %v not in effective scope IDs", scopeParent.ID)
	}
	if !found[scopeChild.ID.String()] {
		t.Errorf("child scope %v not in effective scope IDs", scopeChild.ID)
	}
}

func TestMembershipStore_IsScopeAdmin(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "mem-admin-owner")
	adminPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "mem-admin-explicit")
	memberPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "mem-admin-member")

	scope := testhelper.CreateTestScope(t, pool, "project", "mem-admin-scope", nil, owner.ID)

	if err := ms.AddMembership(ctx, adminPrincipal.ID, owner.ID, "admin", nil); err != nil {
		t.Fatalf("AddMembership admin: %v", err)
	}
	if err := ms.AddMembership(ctx, memberPrincipal.ID, owner.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership member: %v", err)
	}

	// Scope owner is admin.
	ok, err := ms.IsScopeAdmin(ctx, owner.ID, scope.ID)
	if err != nil {
		t.Fatalf("IsScopeAdmin(owner): %v", err)
	}
	if !ok {
		t.Error("scope owner should be admin")
	}

	// Explicit admin role is admin.
	ok, err = ms.IsScopeAdmin(ctx, adminPrincipal.ID, scope.ID)
	if err != nil {
		t.Fatalf("IsScopeAdmin(explicit admin): %v", err)
	}
	if !ok {
		t.Error("explicit admin should be admin")
	}

	// Member role is not admin.
	ok, err = ms.IsScopeAdmin(ctx, memberPrincipal.ID, scope.ID)
	if err != nil {
		t.Fatalf("IsScopeAdmin(member): %v", err)
	}
	if ok {
		t.Error("member role should not be admin")
	}
}

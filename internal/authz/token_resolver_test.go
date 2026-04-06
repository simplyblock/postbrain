//go:build integration

package authz_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// TestTokenResolver_ReadToken verifies that a token declaring only "read"
// against a member principal yields all :read permissions the member holds.
func TestTokenResolver_ReadToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "tr-read-team-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "tr-read-s-"+uuid.New().String(), nil, teamPrincipal.ID)
	member := testhelper.CreateTestPrincipal(t, pool, "user", "tr-read-u-"+uuid.New().String())

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, member.ID, teamPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	// Token declares only "read"
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: member.ID,
		Permissions: []string{"read"},
		ScopeIds:    []uuid.UUID{},
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	effective, err := tr.EffectiveTokenPermissions(ctx, tok, scope.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions: %v", err)
	}

	// Should have :read permissions that the member principal has
	assertHasPerm(t, "token read", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	assertHasPerm(t, "token read", effective, authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead))
	// Should NOT have write (token didn't declare it)
	assertLacksPerm(t, "token read no write", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
}

// TestTokenResolver_ResourceSpecific verifies a token with memories:read on an
// owner principal yields only memories:read.
func TestTokenResolver_ResourceSpecific(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "tr-specific-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "tr-specific-s-"+uuid.New().String(), nil, owner.ID)

	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: owner.ID,
		Permissions: []string{"memories:read"},
		ScopeIds:    []uuid.UUID{},
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	effective, err := tr.EffectiveTokenPermissions(ctx, tok, scope.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions: %v", err)
	}

	assertHasPerm(t, "specific", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	assertLacksPerm(t, "specific no knowledge", effective, authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead))
	assertLacksPerm(t, "specific no write", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
}

// TestTokenResolver_CannotEscalate verifies the intersection invariant: a token
// can never surface permissions the principal does not hold.
func TestTokenResolver_CannotEscalate(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "tr-esc-team-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "tr-esc-s-"+uuid.New().String(), nil, teamPrincipal.ID)
	member := testhelper.CreateTestPrincipal(t, pool, "user", "tr-esc-u-"+uuid.New().String())

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, member.ID, teamPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	// Token declares full permissions — but principal is only "member"
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: member.ID,
		Permissions: []string{"read", "write", "edit", "delete"},
		ScopeIds:    []uuid.UUID{},
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	effective, err := tr.EffectiveTokenPermissions(ctx, tok, scope.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions: %v", err)
	}

	// Principal (member) does not have delete rights on memories
	assertLacksPerm(t, "no escalation", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationDelete))
	// But principal does have write
	assertHasPerm(t, "member write", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
}

// TestTokenResolver_ScopeRestriction verifies that a token with scope_ids set
// is denied access to scopes not in that list.
func TestTokenResolver_ScopeRestriction(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "tr-scpres-owner-"+uuid.New().String())
	allowed := testhelper.CreateTestScope(t, pool, "project", "tr-scpres-allowed-"+uuid.New().String(), nil, owner.ID)
	denied := testhelper.CreateTestScope(t, pool, "project", "tr-scpres-denied-"+uuid.New().String(), nil, owner.ID)

	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: owner.ID,
		Permissions: []string{"read"},
		ScopeIds:    []uuid.UUID{allowed.ID}, // restricted to allowed only
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	// Allowed scope: should have permissions
	effectiveAllowed, err := tr.EffectiveTokenPermissions(ctx, tok, allowed.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions (allowed): %v", err)
	}
	assertHasPerm(t, "allowed scope", effectiveAllowed, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))

	// Denied scope: should have no permissions
	effectiveDenied, err := tr.EffectiveTokenPermissions(ctx, tok, denied.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions (denied): %v", err)
	}
	if !effectiveDenied.IsEmpty() {
		t.Errorf("denied scope should yield empty permissions, got %v", effectiveDenied.ToSlice())
	}
}

// TestTokenResolver_ScopeRestriction_IncludesDescendants verifies that a token
// restricted to a scope also allows access to that scope's descendants.
func TestTokenResolver_ScopeRestriction_IncludesDescendants(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "tr-desc-owner-"+uuid.New().String())
	parent := testhelper.CreateTestScope(t, pool, "project", "tr-desc-parent-"+uuid.New().String(), nil, owner.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "tr-desc-child-"+uuid.New().String(), &parent.ID, owner.ID)

	// Token restricted to parent only — should still allow child (descendant)
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: owner.ID,
		Permissions: []string{"read"},
		ScopeIds:    []uuid.UUID{parent.ID},
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	effective, err := tr.EffectiveTokenPermissions(ctx, tok, child.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions (descendant): %v", err)
	}
	assertHasPerm(t, "descendant of allowed scope", effective, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
}

// TestTokenResolver_RevokedToken verifies that a revoked token yields no permissions.
func TestTokenResolver_RevokedToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "tr-revoked-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "tr-revoked-s-"+uuid.New().String(), nil, owner.ID)

	now := time.Now()
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: owner.ID,
		Permissions: []string{"read", "write"},
		ScopeIds:    []uuid.UUID{},
		RevokedAt:   &now,
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	effective, err := tr.EffectiveTokenPermissions(ctx, tok, scope.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions (revoked): %v", err)
	}
	if !effective.IsEmpty() {
		t.Errorf("revoked token should yield empty permissions, got %v", effective.ToSlice())
	}
}

// TestTokenResolver_ExpiredToken verifies that an expired token yields no permissions.
func TestTokenResolver_ExpiredToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "tr-expired-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "tr-expired-s-"+uuid.New().String(), nil, owner.ID)

	past := time.Now().Add(-time.Hour)
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: owner.ID,
		Permissions: []string{"read"},
		ScopeIds:    []uuid.UUID{},
		ExpiresAt:   &past,
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	effective, err := tr.EffectiveTokenPermissions(ctx, tok, scope.ID)
	if err != nil {
		t.Fatalf("EffectiveTokenPermissions (expired): %v", err)
	}
	if !effective.IsEmpty() {
		t.Errorf("expired token should yield empty permissions, got %v", effective.ToSlice())
	}
}

// TestTokenResolver_HasTokenPermission verifies HasTokenPermission delegates correctly.
func TestTokenResolver_HasTokenPermission(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "tr-has-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "tr-has-s-"+uuid.New().String(), nil, owner.ID)

	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: owner.ID,
		Permissions: []string{"memories:read"},
		ScopeIds:    []uuid.UUID{},
	}

	r := authz.NewDBResolver(pool)
	tr := authz.NewTokenResolver(r)

	has, err := tr.HasTokenPermission(ctx, tok, scope.ID, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	if err != nil {
		t.Fatalf("HasTokenPermission: %v", err)
	}
	if !has {
		t.Error("expected true for memories:read on owner's scope with read token")
	}

	has, err = tr.HasTokenPermission(ctx, tok, scope.ID, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
	if err != nil {
		t.Fatalf("HasTokenPermission (write): %v", err)
	}
	if has {
		t.Error("expected false for memories:write when token only declares read")
	}
}

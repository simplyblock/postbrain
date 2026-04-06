//go:build integration

package authz_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

// ── helpers ────────────────────────────────────────────────────────────────

func newResolver(pool *pgxpool.Pool) authz.Resolver {
	return authz.NewDBResolver(pool)
}

func assertHasPerm(t *testing.T, label string, ps authz.PermissionSet, perm authz.Permission) {
	t.Helper()
	if !ps.Contains(perm) {
		t.Errorf("%s: missing expected permission %q", label, perm)
	}
}

func assertLacksPerm(t *testing.T, label string, ps authz.PermissionSet, perm authz.Permission) {
	t.Helper()
	if ps.Contains(perm) {
		t.Errorf("%s: should not have permission %q", label, perm)
	}
}

func makeGrant(t *testing.T, pool *pgxpool.Pool, principalID, scopeID uuid.UUID, perms []string, expiresAt *time.Time) {
	t.Helper()
	q := db.New(pool)
	if _, err := q.CreateScopeGrant(context.Background(), db.CreateScopeGrantParams{
		PrincipalID: principalID,
		ScopeID:     scopeID,
		Permissions: perms,
		ExpiresAt:   expiresAt,
	}); err != nil {
		t.Fatalf("CreateScopeGrant: %v", err)
	}
}

func setSystemAdmin(t *testing.T, pool *pgxpool.Pool, principalID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE principals SET is_system_admin = true WHERE id = $1`, principalID,
	); err != nil {
		t.Fatalf("set is_system_admin: %v", err)
	}
}

// ── tests ──────────────────────────────────────────────────────────────────

// TestDBResolver_SystemAdmin verifies that a systemadmin principal receives
// all permissions on any scope without any explicit grant.
func TestDBResolver_SystemAdmin(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	admin := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-sysadmin-"+uuid.New().String())
	setSystemAdmin(t, pool, admin.ID)
	other := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-sysadmin-other-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-sysadmin-s-"+uuid.New().String(), nil, other.ID)

	perms, err := r.EffectivePermissions(ctx, admin.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	// systemadmin must have all permissions
	for _, p := range authz.AllPermissions() {
		assertHasPerm(t, "systemadmin", perms, p)
	}
}

// TestDBResolver_DirectOwnership verifies that the scope owner receives RoleOwner permissions.
func TestDBResolver_DirectOwnership(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-owner-s-"+uuid.New().String(), nil, owner.ID)

	perms, err := r.EffectivePermissions(ctx, owner.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	for _, p := range authz.RolePermissions(authz.RoleOwner).Permissions() {
		assertHasPerm(t, "direct owner", perms, p)
	}
}

// TestDBResolver_MemberRole verifies that a member receives RoleMember permissions.
func TestDBResolver_MemberRole(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-team-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-member-s-"+uuid.New().String(), nil, teamPrincipal.ID)
	member := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-member-u-"+uuid.New().String())

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, member.ID, teamPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	perms, err := r.EffectivePermissions(ctx, member.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	for _, p := range authz.RolePermissions(authz.RoleMember).Permissions() {
		assertHasPerm(t, "member role", perms, p)
	}
	// Member must NOT have delete permissions
	assertLacksPerm(t, "member role", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationDelete))
}

// TestDBResolver_AdminRole verifies that an admin member receives RoleAdmin permissions.
func TestDBResolver_AdminRole(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-admin-team-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-admin-s-"+uuid.New().String(), nil, teamPrincipal.ID)
	admin := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-admin-u-"+uuid.New().String())

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, admin.ID, teamPrincipal.ID, "admin", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	perms, err := r.EffectivePermissions(ctx, admin.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	for _, p := range authz.RolePermissions(authz.RoleAdmin).Permissions() {
		assertHasPerm(t, "admin role", perms, p)
	}
}

// TestDBResolver_OwnerRole verifies that an owner-role member receives RoleOwner permissions.
func TestDBResolver_OwnerRole(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-ownrole-team-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-ownrole-s-"+uuid.New().String(), nil, teamPrincipal.ID)
	ownerMember := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-ownrole-u-"+uuid.New().String())

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, ownerMember.ID, teamPrincipal.ID, "owner", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	perms, err := r.EffectivePermissions(ctx, ownerMember.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	for _, p := range authz.RolePermissions(authz.RoleOwner).Permissions() {
		assertHasPerm(t, "owner-role member", perms, p)
	}
}

// TestDBResolver_DirectScopeGrant verifies that a direct scope grant is applied.
func TestDBResolver_DirectScopeGrant(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	scopeOwner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-grant-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-grant-s-"+uuid.New().String(), nil, scopeOwner.ID)
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-grant-u-"+uuid.New().String())

	makeGrant(t, pool, grantee.ID, scope.ID, []string{"memories:read", "knowledge:read"}, nil)

	perms, err := r.EffectivePermissions(ctx, grantee.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertHasPerm(t, "direct grant", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	assertHasPerm(t, "direct grant", perms, authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead))
	assertLacksPerm(t, "direct grant", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
}

// TestDBResolver_DirectScopeGrant_Expired verifies that an expired scope grant is not applied.
func TestDBResolver_DirectScopeGrant_Expired(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	scopeOwner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-expired-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-expired-s-"+uuid.New().String(), nil, scopeOwner.ID)
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-expired-u-"+uuid.New().String())

	past := time.Now().Add(-time.Hour)
	makeGrant(t, pool, grantee.ID, scope.ID, []string{"memories:read"}, &past)

	perms, err := r.EffectivePermissions(ctx, grantee.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertLacksPerm(t, "expired grant", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
}

// TestDBResolver_MembershipAndGrant_Union verifies that membership permissions and
// direct grants are combined via union.
func TestDBResolver_MembershipAndGrant_Union(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	teamPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-union-team-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-union-s-"+uuid.New().String(), nil, teamPrincipal.ID)
	member := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-union-u-"+uuid.New().String())

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, member.ID, teamPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	// Direct grant adds memories:delete (not in RoleMember)
	makeGrant(t, pool, member.ID, scope.ID, []string{"memories:delete"}, nil)

	perms, err := r.EffectivePermissions(ctx, member.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	// From membership
	assertHasPerm(t, "union", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	// From direct grant
	assertHasPerm(t, "union", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationDelete))
}

// TestDBResolver_DownwardInheritance verifies that a grant on a parent scope grants
// the same permissions on a child scope.
func TestDBResolver_DownwardInheritance(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-down-owner-"+uuid.New().String())
	parent := testhelper.CreateTestScope(t, pool, "project", "resolver-ddown-parent-"+uuid.New().String(), nil, owner.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "resolver-ddown-child-"+uuid.New().String(), &parent.ID, owner.ID)
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-ddown-u-"+uuid.New().String())

	// Grant on the PARENT scope
	makeGrant(t, pool, grantee.ID, parent.ID, []string{"memories:read", "knowledge:read"}, nil)

	// Grantee should have those permissions on the CHILD scope
	perms, err := r.EffectivePermissions(ctx, grantee.ID, child.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertHasPerm(t, "downward inheritance", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	assertHasPerm(t, "downward inheritance", perms, authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead))
}

// TestDBResolver_UpwardRead verifies that a grant on a child scope propagates
// matching :read permissions to the ancestor scope.
func TestDBResolver_UpwardRead(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-up-owner-"+uuid.New().String())
	parent := testhelper.CreateTestScope(t, pool, "project", "resolver-up-parent-"+uuid.New().String(), nil, owner.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "resolver-up-child-"+uuid.New().String(), &parent.ID, owner.ID)
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-up-u-"+uuid.New().String())

	// Grant on the CHILD scope
	makeGrant(t, pool, grantee.ID, child.ID, []string{"memories:read", "memories:write"}, nil)

	// Grantee should have :read on the PARENT scope (upward read)
	perms, err := r.EffectivePermissions(ctx, grantee.ID, parent.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertHasPerm(t, "upward read", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	// write must NOT propagate upward
	assertLacksPerm(t, "upward read (no write)", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
}

// TestDBResolver_UpwardRead_NoWriteEditDelete verifies the upward read rule strictly:
// only :read propagates, not :write, :edit, or :delete.
func TestDBResolver_UpwardRead_NoWriteEditDelete(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-upstrict-owner-"+uuid.New().String())
	parent := testhelper.CreateTestScope(t, pool, "project", "resolver-upstrict-parent-"+uuid.New().String(), nil, owner.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "resolver-upstrict-child-"+uuid.New().String(), &parent.ID, owner.ID)
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-upstrict-u-"+uuid.New().String())

	makeGrant(t, pool, grantee.ID, child.ID, []string{"memories:read", "memories:write", "memories:delete", "scopes:edit"}, nil)

	perms, err := r.EffectivePermissions(ctx, grantee.ID, parent.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertHasPerm(t, "upward strict", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	assertLacksPerm(t, "upward strict", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))
	assertLacksPerm(t, "upward strict", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationDelete))
	assertLacksPerm(t, "upward strict", perms, authz.NewPermission(authz.ResourceScopes, authz.OperationEdit))
}

// TestDBResolver_HasPermission verifies HasPermission delegates to EffectivePermissions.
func TestDBResolver_HasPermission(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-hasperm-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-hasperm-s-"+uuid.New().String(), nil, owner.ID)

	has, err := r.HasPermission(ctx, owner.ID, scope.ID, authz.NewPermission(authz.ResourceMemories, authz.OperationDelete))
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !has {
		t.Error("scope owner should have memories:delete")
	}

	other := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-hasperm-other-"+uuid.New().String())
	has, err = r.HasPermission(ctx, other.ID, scope.ID, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	if err != nil {
		t.Fatalf("HasPermission for unrelated principal: %v", err)
	}
	if has {
		t.Error("unrelated principal should not have any permissions")
	}
}

// TestDBResolver_UnrelatedPrincipal verifies an unrelated principal has no permissions.
func TestDBResolver_UnrelatedPrincipal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-unrel-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-unrel-s-"+uuid.New().String(), nil, owner.ID)
	stranger := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-unrel-u-"+uuid.New().String())

	perms, err := r.EffectivePermissions(ctx, stranger.ID, scope.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}
	if !perms.IsEmpty() {
		t.Errorf("unrelated principal should have empty permissions, got %v", perms.ToSlice())
	}
}

// TestDBResolver_OwnershipOnAncestorScope_GrantsDescendantPermissions verifies
// design rule 2: owning an ancestor scope grants full permissions on descendants.
func TestDBResolver_OwnershipOnAncestorScope_GrantsDescendantPermissions(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	ancestorOwner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-anc-owner-"+uuid.New().String())
	parent := testhelper.CreateTestScope(t, pool, "project", "resolver-anc-parent-"+uuid.New().String(), nil, ancestorOwner.ID)

	// Child deliberately owned by a different principal to ensure access comes from
	// ancestor ownership, not direct scope ownership.
	childOwner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-anc-child-owner-"+uuid.New().String())
	child := testhelper.CreateTestScope(t, pool, "project", "resolver-anc-child-"+uuid.New().String(), &parent.ID, childOwner.ID)

	perms, err := r.EffectivePermissions(ctx, ancestorOwner.ID, child.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertHasPerm(t, "ancestor ownership", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationDelete))
	assertHasPerm(t, "ancestor ownership", perms, authz.NewPermission(authz.ResourceScopes, authz.OperationDelete))
}

// TestDBResolver_UpwardRead_FromMembershipDerivedDescendantAccess verifies that
// upward-read also applies when descendant read is derived from membership (not only direct grants).
func TestDBResolver_UpwardRead_FromMembershipDerivedDescendantAccess(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	parentOwner := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-upmem-parent-owner-"+uuid.New().String())
	childOwner := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-upmem-child-owner-"+uuid.New().String())

	parent := testhelper.CreateTestScope(t, pool, "project", "resolver-upmem-parent-"+uuid.New().String(), nil, parentOwner.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "resolver-upmem-child-"+uuid.New().String(), &parent.ID, childOwner.ID)

	member := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-upmem-member-"+uuid.New().String())
	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, member.ID, childOwner.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	perms, err := r.EffectivePermissions(ctx, member.ID, parent.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}

	assertHasPerm(t, "upward read from membership-derived child access", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
	assertLacksPerm(t, "upward read from membership-derived child access", perms, authz.NewPermission(authz.ResourceMemories, authz.OperationWrite))

	// ensure hierarchy was constructed as expected and child access is indeed present
	childPerms, err := r.EffectivePermissions(ctx, member.ID, child.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions(child): %v", err)
	}
	assertHasPerm(t, "membership-derived child access", childPerms, authz.NewPermission(authz.ResourceMemories, authz.OperationRead))
}

// TestDBResolver_InvalidMembershipRole_ReturnsError verifies malformed membership
// role values from DB are surfaced as resolver errors, not silently ignored.
func TestDBResolver_InvalidMembershipRole_ReturnsError(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	parentPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "resolver-badrole-parent-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-badrole-scope-"+uuid.New().String(), nil, parentPrincipal.ID)
	member := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-badrole-member-"+uuid.New().String())

	if _, err := pool.Exec(ctx, `
		INSERT INTO principal_memberships (member_id, parent_id, role)
		VALUES ($1, $2, 'bogus-role')
	`, member.ID, parentPrincipal.ID); err != nil {
		t.Fatalf("insert malformed membership role: %v", err)
	}

	_, err := r.EffectivePermissions(ctx, member.ID, scope.ID)
	if err == nil {
		t.Fatal("expected error for malformed membership role, got nil")
	}
}

// TestDBResolver_InvalidScopeGrantPermissions_ReturnsError verifies malformed
// scope grant permission entries are surfaced as resolver errors.
func TestDBResolver_InvalidScopeGrantPermissions_ReturnsError(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	r := newResolver(pool)

	scopeOwner := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-badgrant-owner-"+uuid.New().String())
	scope := testhelper.CreateTestScope(t, pool, "project", "resolver-badgrant-scope-"+uuid.New().String(), nil, scopeOwner.ID)
	grantee := testhelper.CreateTestPrincipal(t, pool, "user", "resolver-badgrant-grantee-"+uuid.New().String())

	if _, err := pool.Exec(ctx, `
		INSERT INTO scope_grants (principal_id, scope_id, permissions, granted_by)
		VALUES ($1, $2, $3, $4)
	`, grantee.ID, scope.ID, []string{"totally-invalid-permission"}, scopeOwner.ID); err != nil {
		t.Fatalf("insert malformed scope grant: %v", err)
	}

	_, err := r.EffectivePermissions(ctx, grantee.ID, scope.ID)
	if err == nil {
		t.Fatal("expected error for malformed scope grant permissions, got nil")
	}
}

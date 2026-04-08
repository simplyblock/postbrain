//go:build integration

package principals_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

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

func TestMembershipStore_EffectiveScopeIDs_ChainMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		kinds []string
		role  string
	}{
		{name: "user", kinds: []string{"user"}, role: "member"},
		{name: "team", kinds: []string{"team"}, role: "member"},
		{name: "department", kinds: []string{"department"}, role: "member"},
		{name: "company", kinds: []string{"company"}, role: "member"},
		{name: "user_team", kinds: []string{"user", "team"}, role: "member"},
		{name: "user_department", kinds: []string{"user", "department"}, role: "member"},
		{name: "user_company", kinds: []string{"user", "company"}, role: "member"},
		{name: "team_department", kinds: []string{"team", "department"}, role: "member"},
		{name: "team_company", kinds: []string{"team", "company"}, role: "member"},
		{name: "department_company", kinds: []string{"department", "company"}, role: "member"},
		{name: "user_team_department", kinds: []string{"user", "team", "department"}, role: "member"},
		{name: "user_team_company", kinds: []string{"user", "team", "company"}, role: "member"},
		{name: "user_department_company", kinds: []string{"user", "department", "company"}, role: "member"},
		{name: "team_department_company", kinds: []string{"team", "department", "company"}, role: "member"},
		{name: "user_team_department_company", kinds: []string{"user", "team", "department", "company"}, role: "member"},
		{name: "user_team_company_owner_role", kinds: []string{"user", "team", "company"}, role: "owner"},
		{name: "user_team_company_admin_role", kinds: []string{"user", "team", "company"}, role: "admin"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testhelper.NewTestPool(t)
			ms := principals.NewMembershipStore(pool)
			ctx := context.Background()

			outsiderPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", fmt.Sprintf("mem-outsider-%s", tc.name))
			outsiderScope := testhelper.CreateTestScope(t, pool, "project", fmt.Sprintf("mem-outsider-scope-%s", tc.name), nil, outsiderPrincipal.ID)

			type node struct {
				principalID uuid.UUID
				scopeID     uuid.UUID
			}
			nodes := make([]node, 0, len(tc.kinds))

			for i, kind := range tc.kinds {
				slug := fmt.Sprintf("mem-%s-%d", tc.name, i)
				pr := testhelper.CreateTestPrincipal(t, pool, kind, slug)
				sc := testhelper.CreateTestScope(t, pool, "project", fmt.Sprintf("%s-scope", slug), nil, pr.ID)
				nodes = append(nodes, node{principalID: pr.ID, scopeID: sc.ID})
			}

			// chain[0] -> chain[1] -> ... -> chain[n-1]
			for i := 0; i+1 < len(nodes); i++ {
				if err := ms.AddMembership(ctx, nodes[i].principalID, nodes[i+1].principalID, tc.role, nil); err != nil {
					t.Fatalf("AddMembership %d->%d: %v", i, i+1, err)
				}
			}

			// For each node i, effective scopes must include i..n-1 and exclude 0..i-1.
			for i := range nodes {
				got, err := ms.EffectiveScopeIDs(ctx, nodes[i].principalID)
				if err != nil {
					t.Fatalf("EffectiveScopeIDs(%d): %v", i, err)
				}
				gotSet := toIDSet(got)
				if gotSet[outsiderScope.ID] {
					t.Fatalf("principal %d unexpectedly sees outsider scope %s", i, outsiderScope.ID)
				}

				for j := i; j < len(nodes); j++ {
					if !gotSet[nodes[j].scopeID] {
						t.Fatalf("principal %d missing expected scope %d (%s)", i, j, nodes[j].scopeID)
					}
				}
				for j := 0; j < i; j++ {
					if gotSet[nodes[j].scopeID] {
						t.Fatalf("principal %d unexpectedly sees descendant scope %d (%s)", i, j, nodes[j].scopeID)
					}
				}
			}
		})
	}
}

func toIDSet(ids []uuid.UUID) map[uuid.UUID]bool {
	set := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
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

func TestMembershipStore_IsSystemAdmin(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	regular := testhelper.CreateTestPrincipal(t, pool, "user", "sys-admin-regular-"+uuid.New().String())
	sysAdmin := testhelper.CreateTestPrincipal(t, pool, "user", "sys-admin-flag-"+uuid.New().String())

	if _, err := pool.Exec(ctx,
		`UPDATE principals SET is_system_admin = true WHERE id = $1`, sysAdmin.ID,
	); err != nil {
		t.Fatalf("set is_system_admin: %v", err)
	}

	ok, err := ms.IsSystemAdmin(ctx, regular.ID)
	if err != nil {
		t.Fatalf("IsSystemAdmin(regular): %v", err)
	}
	if ok {
		t.Error("regular principal should not be system admin")
	}

	ok, err = ms.IsSystemAdmin(ctx, sysAdmin.ID)
	if err != nil {
		t.Fatalf("IsSystemAdmin(sysAdmin): %v", err)
	}
	if !ok {
		t.Error("principal with is_system_admin=true should be system admin")
	}

	// Non-existent principal returns false without error.
	ok, err = ms.IsSystemAdmin(ctx, uuid.New())
	if err != nil {
		t.Fatalf("IsSystemAdmin(unknown): %v", err)
	}
	if ok {
		t.Error("unknown principal should not be system admin")
	}
}

func TestMembershipStore_SystemAdminBypassesAdminChecks(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	ms := principals.NewMembershipStore(pool)
	ctx := context.Background()

	sysAdmin := testhelper.CreateTestPrincipal(t, pool, "user", "sys-admin-bypass-"+uuid.NewString())
	target := testhelper.CreateTestPrincipal(t, pool, "team", "sys-admin-bypass-target-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "sys-admin-bypass-scope-"+uuid.NewString(), nil, target.ID)

	if _, err := pool.Exec(ctx, `UPDATE principals SET is_system_admin = true WHERE id = $1`, sysAdmin.ID); err != nil {
		t.Fatalf("set is_system_admin: %v", err)
	}

	ok, err := ms.IsPrincipalAdmin(ctx, sysAdmin.ID, target.ID)
	if err != nil {
		t.Fatalf("IsPrincipalAdmin(system admin): %v", err)
	}
	if !ok {
		t.Fatal("system admin should have principal admin access")
	}

	ok, err = ms.IsScopeAdmin(ctx, sysAdmin.ID, scope.ID)
	if err != nil {
		t.Fatalf("IsScopeAdmin(system admin): %v", err)
	}
	if !ok {
		t.Fatal("system admin should have scope admin access")
	}

	ok, err = ms.HasAnyAdminRole(ctx, sysAdmin.ID)
	if err != nil {
		t.Fatalf("HasAnyAdminRole(system admin): %v", err)
	}
	if !ok {
		t.Fatal("system admin should satisfy any-admin-role checks")
	}
}

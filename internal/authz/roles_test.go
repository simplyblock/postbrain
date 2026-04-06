package authz_test

import (
	"testing"

	"github.com/simplyblock/postbrain/internal/authz"
)

// TestRoleConstants verifies all Role constants are defined and parseable.
func TestRoleConstants(t *testing.T) {
	cases := []struct {
		raw  string
		want authz.Role
	}{
		{"member", authz.RoleMember},
		{"admin", authz.RoleAdmin},
		{"owner", authz.RoleOwner},
	}
	for _, tc := range cases {
		got, err := authz.ParseRole(tc.raw)
		if err != nil {
			t.Errorf("ParseRole(%q): unexpected error: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseRole(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestParseRole_Unknown verifies unknown role strings are rejected.
func TestParseRole_Unknown(t *testing.T) {
	for _, raw := range []string{"", "superuser", "guest", "read"} {
		_, err := authz.ParseRole(raw)
		if err == nil {
			t.Errorf("ParseRole(%q): expected error, got nil", raw)
		}
	}
}

// TestRolePermissions_Member verifies the exact permission set for RoleMember.
func TestRolePermissions_Member(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleMember)

	mustHave := []authz.Permission{
		authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
		authz.NewPermission(authz.ResourceMemories, authz.OperationWrite),
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead),
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite),
		authz.NewPermission(authz.ResourceCollections, authz.OperationRead),
		authz.NewPermission(authz.ResourceCollections, authz.OperationWrite),
		authz.NewPermission(authz.ResourceSkills, authz.OperationRead),
		authz.NewPermission(authz.ResourceSkills, authz.OperationWrite),
		authz.NewPermission(authz.ResourceSessions, authz.OperationWrite),
		authz.NewPermission(authz.ResourceGraph, authz.OperationRead),
		authz.NewPermission(authz.ResourceScopes, authz.OperationRead),
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationRead),
		authz.NewPermission(authz.ResourceTokens, authz.OperationRead),
		authz.NewPermission(authz.ResourceSharing, authz.OperationRead),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationRead),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationWrite),
	}
	mustNotHave := []authz.Permission{
		authz.NewPermission(authz.ResourceMemories, authz.OperationEdit),
		authz.NewPermission(authz.ResourceMemories, authz.OperationDelete),
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationEdit),
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationDelete),
		authz.NewPermission(authz.ResourceScopes, authz.OperationEdit),
		authz.NewPermission(authz.ResourceScopes, authz.OperationWrite),
		authz.NewPermission(authz.ResourceScopes, authz.OperationDelete),
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationEdit),
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationDelete),
		authz.NewPermission(authz.ResourceSharing, authz.OperationWrite),
		authz.NewPermission(authz.ResourceSharing, authz.OperationDelete),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationEdit),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationDelete),
		authz.NewPermission(authz.ResourceTokens, authz.OperationEdit),
		authz.NewPermission(authz.ResourceTokens, authz.OperationDelete),
	}

	assertHas(t, "RoleMember", perms, mustHave)
	assertLacks(t, "RoleMember", perms, mustNotHave)
}

// TestRolePermissions_Admin verifies the exact permission set for RoleAdmin.
func TestRolePermissions_Admin(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleAdmin)

	// admin has everything member has
	for _, p := range authz.RolePermissions(authz.RoleMember).Permissions() {
		if !perms.Contains(p) {
			t.Errorf("RoleAdmin missing permission from RoleMember: %q", p)
		}
	}

	mustHave := []authz.Permission{
		authz.NewPermission(authz.ResourceMemories, authz.OperationEdit),
		authz.NewPermission(authz.ResourceMemories, authz.OperationDelete),
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationEdit),
		authz.NewPermission(authz.ResourceCollections, authz.OperationEdit),
		authz.NewPermission(authz.ResourceSkills, authz.OperationEdit),
		authz.NewPermission(authz.ResourceScopes, authz.OperationEdit),
		authz.NewPermission(authz.ResourceScopes, authz.OperationWrite),
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationEdit),
		authz.NewPermission(authz.ResourceSharing, authz.OperationWrite),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationEdit),
		authz.NewPermission(authz.ResourceTokens, authz.OperationRead),
		authz.NewPermission(authz.ResourceTokens, authz.OperationEdit),
	}
	mustNotHave := []authz.Permission{
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationDelete),
		authz.NewPermission(authz.ResourceCollections, authz.OperationDelete),
		authz.NewPermission(authz.ResourceSkills, authz.OperationDelete),
		authz.NewPermission(authz.ResourceScopes, authz.OperationDelete),
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationDelete),
		authz.NewPermission(authz.ResourceSharing, authz.OperationDelete),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationDelete),
		authz.NewPermission(authz.ResourceTokens, authz.OperationDelete),
	}

	assertHas(t, "RoleAdmin", perms, mustHave)
	assertLacks(t, "RoleAdmin", perms, mustNotHave)
}

// TestRolePermissions_Owner verifies the exact permission set for RoleOwner.
func TestRolePermissions_Owner(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleOwner)

	// owner has everything admin has
	for _, p := range authz.RolePermissions(authz.RoleAdmin).Permissions() {
		if !perms.Contains(p) {
			t.Errorf("RoleOwner missing permission from RoleAdmin: %q", p)
		}
	}

	mustHave := []authz.Permission{
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationDelete),
		authz.NewPermission(authz.ResourceCollections, authz.OperationDelete),
		authz.NewPermission(authz.ResourceSkills, authz.OperationDelete),
		authz.NewPermission(authz.ResourceScopes, authz.OperationDelete),
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationDelete),
		authz.NewPermission(authz.ResourceSharing, authz.OperationDelete),
		authz.NewPermission(authz.ResourcePromotions, authz.OperationDelete),
		authz.NewPermission(authz.ResourceTokens, authz.OperationDelete),
	}

	assertHas(t, "RoleOwner", perms, mustHave)
}

// TestRoleHierarchy verifies admin ⊃ member and owner ⊃ admin (strict supersets).
func TestRoleHierarchy(t *testing.T) {
	member := authz.RolePermissions(authz.RoleMember)
	admin := authz.RolePermissions(authz.RoleAdmin)
	owner := authz.RolePermissions(authz.RoleOwner)

	for _, p := range member.Permissions() {
		if !admin.Contains(p) {
			t.Errorf("RoleAdmin is not a superset of RoleMember: missing %q", p)
		}
	}
	for _, p := range admin.Permissions() {
		if !owner.Contains(p) {
			t.Errorf("RoleOwner is not a superset of RoleAdmin: missing %q", p)
		}
	}

	// strict: admin must have something member does not
	adminExtra := false
	for _, p := range admin.Permissions() {
		if !member.Contains(p) {
			adminExtra = true
			break
		}
	}
	if !adminExtra {
		t.Error("RoleAdmin is not a strict superset of RoleMember")
	}

	ownerExtra := false
	for _, p := range owner.Permissions() {
		if !admin.Contains(p) {
			ownerExtra = true
			break
		}
	}
	if !ownerExtra {
		t.Error("RoleOwner is not a strict superset of RoleAdmin")
	}
}

// TestRolePermissions_UnknownRole verifies zero value is returned for unrecognised roles.
func TestRolePermissions_UnknownRole(t *testing.T) {
	perms := authz.RolePermissions(authz.Role("unknown"))
	if !perms.IsEmpty() {
		t.Errorf("expected empty PermissionSet for unknown role, got %v", perms.ToSlice())
	}
}

// TestRolePermissions_Member_CompleteAbsences verifies that the full set of
// permissions a member must NOT hold covers all missing operations.
func TestRolePermissions_Member_CompleteAbsences(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleMember)
	mustNotHave := []authz.Permission{
		// tokens: member has read-only visibility but no token creation rights.
		authz.NewPermission(authz.ResourceTokens, authz.OperationWrite),
		// sessions: member can create sessions (write) but not query history (read)
		authz.NewPermission(authz.ResourceSessions, authz.OperationRead),
		// principals: member can read but not create, edit, or delete
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationWrite),
		// collections: member can read+write content but not edit/delete the collection entity itself
		authz.NewPermission(authz.ResourceCollections, authz.OperationEdit),
		authz.NewPermission(authz.ResourceCollections, authz.OperationDelete),
		// skills: member can read+write skills but not edit (status/visibility) or delete
		authz.NewPermission(authz.ResourceSkills, authz.OperationEdit),
		authz.NewPermission(authz.ResourceSkills, authz.OperationDelete),
	}
	assertLacks(t, "RoleMember (complete absences)", perms, mustNotHave)
}

// TestRolePermissions_Admin_CompleteAbsences verifies that admin does NOT hold
// permissions that are reserved for owner or systemadmin.
func TestRolePermissions_Admin_CompleteAbsences(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleAdmin)
	mustNotHave := []authz.Permission{
		// principal creation is reserved for systemadmin
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationWrite),
		// token creation for others is reserved for systemadmin
		authz.NewPermission(authz.ResourceTokens, authz.OperationWrite),
		// session history reading is not granted by any role
		authz.NewPermission(authz.ResourceSessions, authz.OperationRead),
	}
	assertLacks(t, "RoleAdmin (complete absences)", perms, mustNotHave)
}

// TestRolePermissions_Owner_NeverGrantedByAnyRole verifies permissions that no
// membership role grants, regardless of owner/admin/member.
func TestRolePermissions_Owner_NeverGrantedByAnyRole(t *testing.T) {
	neverGranted := []authz.Permission{
		// principal creation: only systemadmin
		authz.NewPermission(authz.ResourcePrincipals, authz.OperationWrite),
		// token creation for others: only systemadmin or self-service
		authz.NewPermission(authz.ResourceTokens, authz.OperationWrite),
		// session history: not exposed via any membership role
		authz.NewPermission(authz.ResourceSessions, authz.OperationRead),
	}
	for _, role := range []authz.Role{authz.RoleMember, authz.RoleAdmin, authz.RoleOwner} {
		perms := authz.RolePermissions(role)
		assertLacks(t, string(role)+" (never-granted)", perms, neverGranted)
	}
}

// TestRolePermissions_NoDuplicates verifies each role's permission set contains no duplicate entries.
func TestRolePermissions_NoDuplicates(t *testing.T) {
	for _, role := range []authz.Role{authz.RoleMember, authz.RoleAdmin, authz.RoleOwner} {
		perms := authz.RolePermissions(role)
		all := perms.Permissions()
		seen := make(map[authz.Permission]bool)
		for _, p := range all {
			if seen[p] {
				t.Errorf("role %q: duplicate permission %q", role, p)
			}
			seen[p] = true
		}
	}
}

// TestParseRole_CaseSensitive verifies that role parsing is case-sensitive.
func TestParseRole_CaseSensitive(t *testing.T) {
	cases := []string{"Member", "MEMBER", "Admin", "ADMIN", "Owner", "OWNER"}
	for _, raw := range cases {
		_, err := authz.ParseRole(raw)
		if err == nil {
			t.Errorf("ParseRole(%q): expected error for non-lowercase role, got nil", raw)
		}
	}
}

// helpers

func assertHas(t *testing.T, label string, ps authz.PermissionSet, want []authz.Permission) {
	t.Helper()
	for _, p := range want {
		if !ps.Contains(p) {
			t.Errorf("%s: missing expected permission %q", label, p)
		}
	}
}

func assertLacks(t *testing.T, label string, ps authz.PermissionSet, must []authz.Permission) {
	t.Helper()
	for _, p := range must {
		if ps.Contains(p) {
			t.Errorf("%s: should not have permission %q", label, p)
		}
	}
}

package authz_test

import (
	"testing"

	"github.com/simplyblock/postbrain/internal/authz"
)

// TestParseTokenPermissions_Valid verifies that valid token permission strings are accepted.
func TestParseTokenPermissions_Valid(t *testing.T) {
	cases := []struct {
		raw      []string
		mustHave []authz.Permission
	}{
		{
			raw: []string{"read"},
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
				authz.NewPermission(authz.ResourceGraph, authz.OperationRead),
			},
		},
		{
			raw: []string{"write"},
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationWrite),
				authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite),
			},
		},
		{
			raw: []string{"edit"},
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceScopes, authz.OperationEdit),
				authz.NewPermission(authz.ResourcePrincipals, authz.OperationEdit),
			},
		},
		{
			raw: []string{"delete"},
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationDelete),
				authz.NewPermission(authz.ResourceScopes, authz.OperationDelete),
			},
		},
		{
			raw: []string{"memories:read", "knowledge:write"},
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
				authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite),
			},
		},
		{
			raw: []string{"read", "write", "edit", "delete"},
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
				authz.NewPermission(authz.ResourceMemories, authz.OperationWrite),
				authz.NewPermission(authz.ResourceMemories, authz.OperationEdit),
				authz.NewPermission(authz.ResourceMemories, authz.OperationDelete),
			},
		},
	}

	for _, tc := range cases {
		ps, err := authz.ParseTokenPermissions(tc.raw)
		if err != nil {
			t.Errorf("ParseTokenPermissions(%v): unexpected error: %v", tc.raw, err)
			continue
		}
		for _, want := range tc.mustHave {
			if !ps.Contains(want) {
				t.Errorf("ParseTokenPermissions(%v): missing %q", tc.raw, want)
			}
		}
	}
}

// TestParseTokenPermissions_RejectsAdmin verifies that the legacy "admin" value is rejected.
func TestParseTokenPermissions_RejectsAdmin(t *testing.T) {
	cases := [][]string{
		{"admin"},
		{"read", "admin"},
		{"admin", "write"},
	}
	for _, raw := range cases {
		_, err := authz.ParseTokenPermissions(raw)
		if err == nil {
			t.Errorf("ParseTokenPermissions(%v): expected error for 'admin', got nil", raw)
		}
	}
}

// TestParseTokenPermissions_RejectsEmpty verifies that an empty slice is rejected.
func TestParseTokenPermissions_RejectsEmpty(t *testing.T) {
	_, err := authz.ParseTokenPermissions([]string{})
	if err == nil {
		t.Error("ParseTokenPermissions([]): expected error, got nil")
	}
	_, err = authz.ParseTokenPermissions(nil)
	if err == nil {
		t.Error("ParseTokenPermissions(nil): expected error, got nil")
	}
}

// TestParseTokenPermissions_RejectsInvalid verifies invalid resource:operation pairs are rejected.
func TestParseTokenPermissions_RejectsInvalid(t *testing.T) {
	cases := [][]string{
		{"graph:write"},
		{"sessions:delete"},
		{"unknown:read"},
		{"sharing:edit"},
		{""},
		{"memories:"},
		{":read"},
	}
	for _, raw := range cases {
		_, err := authz.ParseTokenPermissions(raw)
		if err == nil {
			t.Errorf("ParseTokenPermissions(%v): expected error, got nil", raw)
		}
	}
}

// TestEffectiveTokenPermissions_Intersection verifies the token is bounded by principal permissions.
func TestEffectiveTokenPermissions_Intersection(t *testing.T) {
	// principal has read+write on memories and knowledge
	principalPerms, _ := authz.NewPermissionSet([]string{"memories:read", "memories:write", "knowledge:read", "knowledge:write"})
	// token only declares read
	tokenPerms, _ := authz.ParseTokenPermissions([]string{"read"})

	effective := authz.EffectiveTokenPermissions(principalPerms, tokenPerms)

	// only read permissions that principal also holds
	if !effective.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("effective: missing memories:read")
	}
	if !effective.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead)) {
		t.Error("effective: missing knowledge:read")
	}

	// write was in principal but not in token's declared permissions
	if effective.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("effective: should not contain memories:write (token declared read only)")
	}

	// skills:read was not in principal's permissions
	if effective.Contains(authz.NewPermission(authz.ResourceSkills, authz.OperationRead)) {
		t.Error("effective: should not contain skills:read (principal does not have it)")
	}
}

// TestEffectiveTokenPermissions_CannotEscalate verifies a token never exceeds principal permissions.
func TestEffectiveTokenPermissions_CannotEscalate(t *testing.T) {
	// principal has only read
	principalPerms, _ := authz.NewPermissionSet([]string{"memories:read"})
	// token declares full write — but principal doesn't have it
	tokenPerms, _ := authz.ParseTokenPermissions([]string{"read", "write", "edit", "delete"})

	effective := authz.EffectiveTokenPermissions(principalPerms, tokenPerms)

	if effective.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("effective: token must not escalate beyond principal permissions")
	}
	if effective.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationDelete)) {
		t.Error("effective: token must not escalate beyond principal permissions")
	}
	if !effective.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("effective: should still have memories:read from principal")
	}
}

// TestEffectiveTokenPermissions_FullPrincipal verifies a broad token passes principal perms through.
func TestEffectiveTokenPermissions_FullPrincipal(t *testing.T) {
	principalPerms := authz.RolePermissions(authz.RoleMember)
	tokenPerms, _ := authz.ParseTokenPermissions([]string{"read", "write", "edit", "delete"})

	effective := authz.EffectiveTokenPermissions(principalPerms, tokenPerms)

	// effective should match principal perms exactly (token is broader than principal)
	for _, p := range principalPerms.Permissions() {
		if !effective.Contains(p) {
			t.Errorf("effective: missing principal permission %q", p)
		}
	}
}

// TestEffectiveTokenPermissions_EmptyPrincipal verifies empty principal yields empty effective.
func TestEffectiveTokenPermissions_EmptyPrincipal(t *testing.T) {
	principalPerms := authz.EmptyPermissionSet()
	tokenPerms, _ := authz.ParseTokenPermissions([]string{"read", "write"})

	effective := authz.EffectiveTokenPermissions(principalPerms, tokenPerms)
	if !effective.IsEmpty() {
		t.Errorf("empty principal should yield empty effective permissions, got %v", effective.ToSlice())
	}
}

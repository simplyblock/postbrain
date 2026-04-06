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

// TestParseTokenPermissions_MixedShorthandAndResourceOp verifies that a mix of shorthands
// and specific resource:operation strings is accepted and expanded correctly.
func TestParseTokenPermissions_MixedShorthandAndResourceOp(t *testing.T) {
	// "read" expands to all :read permissions; "memories:write" adds exactly one more
	ps, err := authz.ParseTokenPermissions([]string{"read", "memories:write"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ps.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("missing memories:read (from shorthand read)")
	}
	if !ps.Contains(authz.NewPermission(authz.ResourceGraph, authz.OperationRead)) {
		t.Error("missing graph:read (from shorthand read)")
	}
	if !ps.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("missing memories:write (explicit)")
	}
	// write shorthand was NOT given, so knowledge:write should not be present
	if ps.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite)) {
		t.Error("knowledge:write should not be present (write shorthand not given)")
	}
}

// TestEffectiveTokenPermissions_ResourceSpecificRestriction verifies that a token
// declaring only a specific resource:operation is bounded to just that permission.
func TestEffectiveTokenPermissions_ResourceSpecificRestriction(t *testing.T) {
	// principal (owner) has full permissions
	principalPerms := authz.RolePermissions(authz.RoleOwner)
	// token narrowed to memories:read only
	tokenPerms, err := authz.ParseTokenPermissions([]string{"memories:read"})
	if err != nil {
		t.Fatalf("ParseTokenPermissions: %v", err)
	}

	effective := authz.EffectiveTokenPermissions(principalPerms, tokenPerms)

	if !effective.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("effective should contain memories:read")
	}
	// All other permissions must be absent
	for _, p := range principalPerms.Permissions() {
		if p == authz.NewPermission(authz.ResourceMemories, authz.OperationRead) {
			continue
		}
		if effective.Contains(p) {
			t.Errorf("effective should not contain %q (token only declared memories:read)", p)
		}
	}
}

// TestEffectiveTokenPermissions_BothEmpty verifies that empty principal and empty token
// (constructed via EmptyPermissionSet) yields an empty effective set.
func TestEffectiveTokenPermissions_BothEmpty(t *testing.T) {
	effective := authz.EffectiveTokenPermissions(authz.EmptyPermissionSet(), authz.EmptyPermissionSet())
	if !effective.IsEmpty() {
		t.Errorf("both empty should yield empty effective, got %v", effective.ToSlice())
	}
}

// TestEffectiveTokenPermissions_ExactOutputSize verifies the output size equals the
// intersection size — no permissions are dropped or duplicated.
func TestEffectiveTokenPermissions_ExactOutputSize(t *testing.T) {
	// principal has memories read+write, knowledge read+write
	principalPerms, _ := authz.NewPermissionSet([]string{
		"memories:read", "memories:write", "knowledge:read", "knowledge:write",
	})
	// token declares only read — intersection should be exactly memories:read + knowledge:read
	tokenPerms, _ := authz.ParseTokenPermissions([]string{"memories:read", "knowledge:read"})

	effective := authz.EffectiveTokenPermissions(principalPerms, tokenPerms)
	if effective.Len() != 2 {
		t.Errorf("expected 2 effective permissions (memories:read + knowledge:read), got %d: %v",
			effective.Len(), effective.ToSlice())
	}
}

// TestParseTokenPermissions_AllResourceOperationPairs verifies every valid
// resource:operation pair is accepted individually.
func TestParseTokenPermissions_AllResourceOperationPairs(t *testing.T) {
	for _, r := range authz.AllResources() {
		for _, op := range authz.ValidOperations(r) {
			raw := string(r) + ":" + string(op)
			_, err := authz.ParseTokenPermissions([]string{raw})
			if err != nil {
				t.Errorf("ParseTokenPermissions([%q]): unexpected error: %v", raw, err)
			}
		}
	}
}

// TestParseTokenPermissions_RejectsDuplicateAdmin verifies "admin" embedded in a
// longer list is still rejected even when surrounded by valid entries.
func TestParseTokenPermissions_RejectsDuplicateAdmin(t *testing.T) {
	cases := [][]string{
		{"memories:read", "admin", "knowledge:read"},
		{"write", "edit", "admin"},
	}
	for _, raw := range cases {
		_, err := authz.ParseTokenPermissions(raw)
		if err == nil {
			t.Errorf("ParseTokenPermissions(%v): expected error for 'admin', got nil", raw)
		}
	}
}

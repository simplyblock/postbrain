package authz_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/authz"
)

func scopeID(t *testing.T) authz.ScopeID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	return authz.ScopeID(id)
}

// TestApplyUpwardRead_PropagatesToAncestors verifies that read grants on a scope
// are extended to all ancestor scopes.
func TestApplyUpwardRead_PropagatesToAncestors(t *testing.T) {
	child := scopeID(t)
	parent := scopeID(t)
	grandparent := scopeID(t)

	// child has memories:read and knowledge:read
	childPerms, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:read"})
	grants := map[authz.ScopeID]authz.PermissionSet{
		child: childPerms,
	}
	ancestors := []authz.ScopeID{parent, grandparent}

	result := authz.ApplyUpwardRead(grants, child, ancestors)

	// parent and grandparent should gain :read permissions matching child's reads
	for _, ancestor := range ancestors {
		ps, ok := result[ancestor]
		if !ok {
			t.Errorf("ancestor %v not present in result", ancestor)
			continue
		}
		if !ps.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
			t.Errorf("ancestor %v missing memories:read", ancestor)
		}
		if !ps.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead)) {
			t.Errorf("ancestor %v missing knowledge:read", ancestor)
		}
	}

	// original child grant is preserved
	if !result[child].Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("child grant should be preserved in result")
	}
}

// TestApplyUpwardRead_DoesNotPropagateWrite verifies write is never propagated upward.
func TestApplyUpwardRead_DoesNotPropagateWrite(t *testing.T) {
	child := scopeID(t)
	parent := scopeID(t)

	childPerms, _ := authz.NewPermissionSet([]string{"memories:read", "memories:write"})
	grants := map[authz.ScopeID]authz.PermissionSet{child: childPerms}

	result := authz.ApplyUpwardRead(grants, child, []authz.ScopeID{parent})

	parentPerms, ok := result[parent]
	if !ok {
		t.Fatal("parent not in result")
	}
	if parentPerms.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("write must not propagate upward to parent")
	}
	if !parentPerms.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("read should propagate upward to parent")
	}
}

// TestApplyUpwardRead_DoesNotPropagateEdit verifies edit is never propagated upward.
func TestApplyUpwardRead_DoesNotPropagateEdit(t *testing.T) {
	child := scopeID(t)
	parent := scopeID(t)

	childPerms, _ := authz.NewPermissionSet([]string{"scopes:read", "scopes:edit"})
	grants := map[authz.ScopeID]authz.PermissionSet{child: childPerms}

	result := authz.ApplyUpwardRead(grants, child, []authz.ScopeID{parent})

	parentPerms := result[parent]
	if parentPerms.Contains(authz.NewPermission(authz.ResourceScopes, authz.OperationEdit)) {
		t.Error("edit must not propagate upward to parent")
	}
}

// TestApplyUpwardRead_DoesNotPropagateDelete verifies delete is never propagated upward.
func TestApplyUpwardRead_DoesNotPropagateDelete(t *testing.T) {
	child := scopeID(t)
	parent := scopeID(t)

	childPerms, _ := authz.NewPermissionSet([]string{"memories:read", "memories:delete"})
	grants := map[authz.ScopeID]authz.PermissionSet{child: childPerms}

	result := authz.ApplyUpwardRead(grants, child, []authz.ScopeID{parent})

	parentPerms := result[parent]
	if parentPerms.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationDelete)) {
		t.Error("delete must not propagate upward to parent")
	}
}

// TestApplyUpwardRead_NoReadGrant verifies that if the scope has no read grants,
// nothing is added to ancestors.
func TestApplyUpwardRead_NoReadGrant(t *testing.T) {
	child := scopeID(t)
	parent := scopeID(t)

	childPerms, _ := authz.NewPermissionSet([]string{"memories:write"})
	grants := map[authz.ScopeID]authz.PermissionSet{child: childPerms}

	result := authz.ApplyUpwardRead(grants, child, []authz.ScopeID{parent})

	if _, ok := result[parent]; ok {
		if !result[parent].IsEmpty() {
			t.Error("parent should have no grants when child has no read permissions")
		}
	}
}

// TestApplyUpwardRead_EmptyAncestors verifies the function is a no-op for empty ancestor list.
func TestApplyUpwardRead_EmptyAncestors(t *testing.T) {
	child := scopeID(t)
	childPerms, _ := authz.NewPermissionSet([]string{"memories:read"})
	grants := map[authz.ScopeID]authz.PermissionSet{child: childPerms}

	result := authz.ApplyUpwardRead(grants, child, nil)
	if len(result) != 1 {
		t.Errorf("result should only contain child entry, got %d entries", len(result))
	}
}

// TestMergeGrants_UnionPerScope verifies that grants from multiple sources
// are combined as a union per scope.
func TestMergeGrants_UnionPerScope(t *testing.T) {
	s1 := scopeID(t)
	s2 := scopeID(t)

	a := map[authz.ScopeID]authz.PermissionSet{
		s1: mustPS(t, "memories:read"),
		s2: mustPS(t, "knowledge:read"),
	}
	b := map[authz.ScopeID]authz.PermissionSet{
		s1: mustPS(t, "memories:write"),
		s2: mustPS(t, "knowledge:write"),
	}

	merged := authz.MergeGrants(a, b)

	if !merged[s1].Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("merged s1 missing memories:read")
	}
	if !merged[s1].Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("merged s1 missing memories:write")
	}
	if !merged[s2].Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead)) {
		t.Error("merged s2 missing knowledge:read")
	}
	if !merged[s2].Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite)) {
		t.Error("merged s2 missing knowledge:write")
	}
}

// TestMergeGrants_NewScope verifies scopes present in only one source are included.
func TestMergeGrants_NewScope(t *testing.T) {
	s1 := scopeID(t)
	s2 := scopeID(t)

	a := map[authz.ScopeID]authz.PermissionSet{
		s1: mustPS(t, "memories:read"),
	}
	b := map[authz.ScopeID]authz.PermissionSet{
		s2: mustPS(t, "knowledge:read"),
	}

	merged := authz.MergeGrants(a, b)

	if _, ok := merged[s1]; !ok {
		t.Error("merged missing s1")
	}
	if _, ok := merged[s2]; !ok {
		t.Error("merged missing s2")
	}
}

// TestMergeGrants_Empty verifies merging empty maps returns an empty result.
func TestMergeGrants_Empty(t *testing.T) {
	merged := authz.MergeGrants(
		map[authz.ScopeID]authz.PermissionSet{},
		map[authz.ScopeID]authz.PermissionSet{},
	)
	if len(merged) != 0 {
		t.Errorf("expected empty merge, got %d entries", len(merged))
	}
}

// mustPS is a test helper that creates a PermissionSet from a single raw string.
func mustPS(t *testing.T, raw string) authz.PermissionSet {
	t.Helper()
	ps, err := authz.NewPermissionSet([]string{raw})
	if err != nil {
		t.Fatalf("NewPermissionSet(%q): %v", raw, err)
	}
	return ps
}

package authz_test

import (
	"slices"
	"testing"

	"github.com/simplyblock/postbrain/internal/authz"
)

// TestNewPermissionSet_Valid verifies valid raw strings are accepted and shorthand is expanded.
func TestNewPermissionSet_Valid(t *testing.T) {
	cases := []struct {
		raw      []string
		wantLen  int
		mustHave []authz.Permission
	}{
		{
			raw:     []string{"memories:read"},
			wantLen: 1,
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
			},
		},
		{
			raw:     []string{"memories:read", "knowledge:write"},
			wantLen: 2,
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
				authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite),
			},
		},
		{
			// bare "read" expands to :read on all resources
			raw:     []string{"read"},
			wantLen: len(authz.AllResources()),
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
				authz.NewPermission(authz.ResourceGraph, authz.OperationRead),
				authz.NewPermission(authz.ResourceScopes, authz.OperationRead),
			},
		},
		{
			// duplicates are deduplicated
			raw:     []string{"memories:read", "memories:read"},
			wantLen: 1,
			mustHave: []authz.Permission{
				authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
			},
		},
	}

	for _, tc := range cases {
		ps, err := authz.NewPermissionSet(tc.raw)
		if err != nil {
			t.Errorf("NewPermissionSet(%v): unexpected error: %v", tc.raw, err)
			continue
		}
		if ps.Len() != tc.wantLen {
			t.Errorf("NewPermissionSet(%v): Len() = %d, want %d", tc.raw, ps.Len(), tc.wantLen)
		}
		for _, want := range tc.mustHave {
			if !ps.Contains(want) {
				t.Errorf("NewPermissionSet(%v): missing %q", tc.raw, want)
			}
		}
	}
}

// TestNewPermissionSet_Invalid verifies invalid inputs are rejected.
func TestNewPermissionSet_Invalid(t *testing.T) {
	cases := [][]string{
		{"unknown:read"},
		{"graph:write"},
		{"sessions:delete"},
		{"admin"},
		{""},
	}
	for _, raw := range cases {
		_, err := authz.NewPermissionSet(raw)
		if err == nil {
			t.Errorf("NewPermissionSet(%v): expected error, got nil", raw)
		}
	}
}

// TestPermissionSet_Satisfies verifies Satisfies checks for a specific permission.
func TestPermissionSet_Satisfies(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})

	if !ps.Satisfies(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("Satisfies(memories:read) should be true")
	}
	if !ps.Satisfies(authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite)) {
		t.Error("Satisfies(knowledge:write) should be true")
	}
	if ps.Satisfies(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("Satisfies(memories:write) should be false")
	}
	if ps.Satisfies(authz.NewPermission(authz.ResourceSkills, authz.OperationRead)) {
		t.Error("Satisfies(skills:read) should be false")
	}
}

// TestPermissionSet_Satisfies_Shorthand verifies a shorthand-expanded set satisfies specific perms.
func TestPermissionSet_Satisfies_Shorthand(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"read"})

	for _, r := range authz.AllResources() {
		p := authz.NewPermission(r, authz.OperationRead)
		if !ps.Satisfies(p) {
			t.Errorf("shorthand read set: Satisfies(%q) should be true", p)
		}
	}
	if ps.Satisfies(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("shorthand read set: Satisfies(memories:write) should be false")
	}
}

// TestPermissionSet_Union verifies Union produces the additive combination.
func TestPermissionSet_Union(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read"})
	b, _ := authz.NewPermissionSet([]string{"knowledge:write"})

	u := authz.Union(a, b)

	if !u.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("Union missing memories:read from first set")
	}
	if !u.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite)) {
		t.Error("Union missing knowledge:write from second set")
	}
	if u.Len() != 2 {
		t.Errorf("Union Len() = %d, want 2", u.Len())
	}
}

// TestPermissionSet_Union_Idempotent verifies Union of overlapping sets deduplicates.
func TestPermissionSet_Union_Idempotent(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})
	b, _ := authz.NewPermissionSet([]string{"memories:read"})

	u := authz.Union(a, b)
	if u.Len() != 2 {
		t.Errorf("Union(a,b) Len() = %d, want 2 (no duplicates)", u.Len())
	}
}

// TestPermissionSet_Intersect verifies Intersect returns only shared permissions.
func TestPermissionSet_Intersect(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write", "skills:read"})
	b, _ := authz.NewPermissionSet([]string{"memories:read", "skills:read", "scopes:edit"})

	i := authz.Intersect(a, b)

	if !i.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("Intersect missing memories:read")
	}
	if !i.Contains(authz.NewPermission(authz.ResourceSkills, authz.OperationRead)) {
		t.Error("Intersect missing skills:read")
	}
	if i.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite)) {
		t.Error("Intersect should not contain knowledge:write (only in a)")
	}
	if i.Contains(authz.NewPermission(authz.ResourceScopes, authz.OperationEdit)) {
		t.Error("Intersect should not contain scopes:edit (only in b)")
	}
	if i.Len() != 2 {
		t.Errorf("Intersect Len() = %d, want 2", i.Len())
	}
}

// TestPermissionSet_Intersect_Disjoint verifies disjoint sets produce empty intersection.
func TestPermissionSet_Intersect_Disjoint(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read"})
	b, _ := authz.NewPermissionSet([]string{"knowledge:write"})

	i := authz.Intersect(a, b)
	if !i.IsEmpty() {
		t.Errorf("Intersect of disjoint sets should be empty, got %v", i.ToSlice())
	}
}

// TestPermissionSet_IsEmpty verifies IsEmpty on empty and non-empty sets.
func TestPermissionSet_IsEmpty(t *testing.T) {
	empty := authz.EmptyPermissionSet()
	if !empty.IsEmpty() {
		t.Error("EmptyPermissionSet() should be empty")
	}

	ps, _ := authz.NewPermissionSet([]string{"memories:read"})
	if ps.IsEmpty() {
		t.Error("non-empty PermissionSet should not be empty")
	}
}

// TestPermissionSet_ToSlice verifies ToSlice returns sorted, canonical strings.
func TestPermissionSet_ToSlice(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"skills:read", "memories:read", "knowledge:write"})
	got := ps.ToSlice()

	if len(got) != 3 {
		t.Errorf("ToSlice() len = %d, want 3", len(got))
	}
	if !slices.IsSorted(got) {
		t.Errorf("ToSlice() is not sorted: %v", got)
	}
}

// TestPermissionSet_Contains verifies Contains is equivalent to Satisfies.
func TestPermissionSet_Contains(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"memories:read"})
	p := authz.NewPermission(authz.ResourceMemories, authz.OperationRead)

	if !ps.Contains(p) {
		t.Error("Contains should return true for a held permission")
	}
	if ps.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead)) {
		t.Error("Contains should return false for an absent permission")
	}
}

// TestPermissionSet_Len verifies Len returns the number of distinct permissions.
func TestPermissionSet_Len(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})
	if ps.Len() != 2 {
		t.Errorf("Len() = %d, want 2", ps.Len())
	}

	empty := authz.EmptyPermissionSet()
	if empty.Len() != 0 {
		t.Errorf("empty Len() = %d, want 0", empty.Len())
	}
}

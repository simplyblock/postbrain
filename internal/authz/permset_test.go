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

// TestNewPermissionSet_NilInput verifies nil input returns an empty set without error.
func TestNewPermissionSet_NilInput(t *testing.T) {
	ps, err := authz.NewPermissionSet(nil)
	if err != nil {
		t.Fatalf("NewPermissionSet(nil): unexpected error: %v", err)
	}
	if !ps.IsEmpty() {
		t.Errorf("NewPermissionSet(nil): expected empty set, got %v", ps.ToSlice())
	}
}

// TestNewPermissionSet_MixedShorthandAndResourceOp verifies a mix of shorthand and
// resource:operation strings is accepted and both are correctly included.
func TestNewPermissionSet_MixedShorthandAndResourceOp(t *testing.T) {
	ps, err := authz.NewPermissionSet([]string{"read", "memories:write"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// shorthand read expanded
	if !ps.Contains(authz.NewPermission(authz.ResourceKnowledge, authz.OperationRead)) {
		t.Error("missing knowledge:read from shorthand expansion")
	}
	// explicit resource:op present
	if !ps.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationWrite)) {
		t.Error("missing memories:write")
	}
	// memories:read was in shorthand expansion too; no duplicate
	if ps.Len() != len(authz.AllResources())+1 {
		// "read" expands to 11 perms (one per resource), "memories:write" adds 1 more
		t.Errorf("Len() = %d, want %d", ps.Len(), len(authz.AllResources())+1)
	}
}

// TestNewPermissionSet_MixedValidInvalid verifies that a mix of valid and invalid
// strings causes the whole call to fail.
func TestNewPermissionSet_MixedValidInvalid(t *testing.T) {
	_, err := authz.NewPermissionSet([]string{"memories:read", "graph:write"})
	if err == nil {
		t.Error("NewPermissionSet([valid, invalid]): expected error, got nil")
	}
}

// TestPermissionSet_Permissions verifies the Permissions method returns all held permissions.
func TestPermissionSet_Permissions(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write", "skills:delete"})
	got := ps.Permissions()
	if len(got) != 3 {
		t.Fatalf("Permissions() len = %d, want 3", len(got))
	}
	want := []authz.Permission{
		authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
		authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite),
		authz.NewPermission(authz.ResourceSkills, authz.OperationDelete),
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("Permissions() missing %q", w)
		}
	}
}

// TestPermissionSet_Permissions_Empty verifies Permissions on an empty set returns empty slice.
func TestPermissionSet_Permissions_Empty(t *testing.T) {
	got := authz.EmptyPermissionSet().Permissions()
	if len(got) != 0 {
		t.Errorf("Permissions() on empty set returned %v", got)
	}
}

// TestPermissionSet_ToSlice_ExactContent verifies ToSlice returns the correct alphabetically-sorted values.
func TestPermissionSet_ToSlice_ExactContent(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"skills:read", "memories:read", "knowledge:write"})
	got := ps.ToSlice()
	want := []string{"knowledge:write", "memories:read", "skills:read"}
	if len(got) != len(want) {
		t.Fatalf("ToSlice() len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("ToSlice()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestPermissionSet_ToSlice_Empty verifies ToSlice on an empty set returns an empty (non-nil) slice.
func TestPermissionSet_ToSlice_Empty(t *testing.T) {
	got := authz.EmptyPermissionSet().ToSlice()
	if got == nil {
		t.Error("ToSlice() on empty set should return non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("ToSlice() on empty set: got %v", got)
	}
}

// TestPermissionSet_Union_ZeroArgs verifies Union() with no arguments returns an empty set.
func TestPermissionSet_Union_ZeroArgs(t *testing.T) {
	u := authz.Union()
	if !u.IsEmpty() {
		t.Errorf("Union() with no args should be empty, got %v", u.ToSlice())
	}
}

// TestPermissionSet_Union_SingleArg verifies Union of a single set returns an equivalent set.
func TestPermissionSet_Union_SingleArg(t *testing.T) {
	ps, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})
	u := authz.Union(ps)
	if u.Len() != ps.Len() {
		t.Errorf("Union(a).Len() = %d, want %d", u.Len(), ps.Len())
	}
	for _, p := range ps.Permissions() {
		if !u.Contains(p) {
			t.Errorf("Union(a) missing %q", p)
		}
	}
}

// TestPermissionSet_Union_WithEmpty verifies Union(a, empty) equals a.
func TestPermissionSet_Union_WithEmpty(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read"})
	u := authz.Union(a, authz.EmptyPermissionSet())
	if u.Len() != a.Len() {
		t.Errorf("Union(a, empty).Len() = %d, want %d", u.Len(), a.Len())
	}
	if !u.Contains(authz.NewPermission(authz.ResourceMemories, authz.OperationRead)) {
		t.Error("Union(a, empty) missing memories:read")
	}
}

// TestPermissionSet_Intersect_WithEmpty verifies Intersect(a, empty) returns empty.
func TestPermissionSet_Intersect_WithEmpty(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})
	i := authz.Intersect(a, authz.EmptyPermissionSet())
	if !i.IsEmpty() {
		t.Errorf("Intersect(a, empty) should be empty, got %v", i.ToSlice())
	}
	i2 := authz.Intersect(authz.EmptyPermissionSet(), a)
	if !i2.IsEmpty() {
		t.Errorf("Intersect(empty, a) should be empty, got %v", i2.ToSlice())
	}
}

// TestPermissionSet_Intersect_Commutative verifies Intersect(a,b) equals Intersect(b,a).
func TestPermissionSet_Intersect_Commutative(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write", "skills:read"})
	b, _ := authz.NewPermissionSet([]string{"memories:read", "scopes:edit", "skills:read"})

	ab := authz.Intersect(a, b)
	ba := authz.Intersect(b, a)

	if ab.Len() != ba.Len() {
		t.Errorf("Intersect not commutative: Intersect(a,b).Len()=%d, Intersect(b,a).Len()=%d", ab.Len(), ba.Len())
	}
	for _, p := range ab.Permissions() {
		if !ba.Contains(p) {
			t.Errorf("Intersect(a,b) has %q but Intersect(b,a) does not", p)
		}
	}
}

// TestPermissionSet_Intersect_Idempotent verifies Intersect(a,a) equals a.
func TestPermissionSet_Intersect_Idempotent(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})
	i := authz.Intersect(a, a)
	if i.Len() != a.Len() {
		t.Errorf("Intersect(a,a).Len() = %d, want %d", i.Len(), a.Len())
	}
	for _, p := range a.Permissions() {
		if !i.Contains(p) {
			t.Errorf("Intersect(a,a) missing %q", p)
		}
	}
}

// TestPermissionSet_Immutability_Union verifies Union does not mutate its input sets.
func TestPermissionSet_Immutability_Union(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read"})
	b, _ := authz.NewPermissionSet([]string{"knowledge:write"})
	aLenBefore := a.Len()
	bLenBefore := b.Len()

	_ = authz.Union(a, b)

	if a.Len() != aLenBefore {
		t.Errorf("Union mutated first argument: Len changed from %d to %d", aLenBefore, a.Len())
	}
	if b.Len() != bLenBefore {
		t.Errorf("Union mutated second argument: Len changed from %d to %d", bLenBefore, b.Len())
	}
}

// TestPermissionSet_Immutability_Intersect verifies Intersect does not mutate its input sets.
func TestPermissionSet_Immutability_Intersect(t *testing.T) {
	a, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:write"})
	b, _ := authz.NewPermissionSet([]string{"memories:read"})
	aLenBefore := a.Len()
	bLenBefore := b.Len()

	_ = authz.Intersect(a, b)

	if a.Len() != aLenBefore {
		t.Errorf("Intersect mutated first argument: Len changed from %d to %d", aLenBefore, a.Len())
	}
	if b.Len() != bLenBefore {
		t.Errorf("Intersect mutated second argument: Len changed from %d to %d", bLenBefore, b.Len())
	}
}

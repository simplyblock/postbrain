package authz

import "sort"

// PermissionSet is an immutable set of Permission values.
type PermissionSet struct {
	perms map[Permission]struct{}
}

// EmptyPermissionSet returns an empty PermissionSet.
func EmptyPermissionSet() PermissionSet {
	return PermissionSet{perms: make(map[Permission]struct{})}
}

// NewPermissionSet builds a PermissionSet from raw permission strings.
// Each string is validated and expanded via Expand. Duplicates are silently
// deduplicated. Returns an error if any string is invalid.
func NewPermissionSet(raw []string) (PermissionSet, error) {
	m := make(map[Permission]struct{})
	for _, r := range raw {
		expanded, err := Expand(r)
		if err != nil {
			return PermissionSet{}, err
		}
		for _, p := range expanded {
			m[p] = struct{}{}
		}
	}
	return PermissionSet{perms: m}, nil
}

// Contains reports whether the set holds the given permission.
func (ps PermissionSet) Contains(p Permission) bool {
	_, ok := ps.perms[p]
	return ok
}

// Satisfies is an alias for Contains — reports whether the set satisfies
// the given permission requirement.
func (ps PermissionSet) Satisfies(required Permission) bool {
	return ps.Contains(required)
}

// IsEmpty reports whether the set contains no permissions.
func (ps PermissionSet) IsEmpty() bool {
	return len(ps.perms) == 0
}

// Len returns the number of distinct permissions in the set.
func (ps PermissionSet) Len() int {
	return len(ps.perms)
}

// Permissions returns all permissions in the set as a slice of Permission values.
// Order is not guaranteed; use ToSlice for a sorted string representation.
func (ps PermissionSet) Permissions() []Permission {
	out := make([]Permission, 0, len(ps.perms))
	for p := range ps.perms {
		out = append(out, p)
	}
	return out
}

// ToSlice returns the permissions as a sorted slice of strings.
func (ps PermissionSet) ToSlice() []string {
	out := make([]string, 0, len(ps.perms))
	for p := range ps.perms {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}

// Union returns a new PermissionSet containing all permissions from all input sets.
func Union(sets ...PermissionSet) PermissionSet {
	m := make(map[Permission]struct{})
	for _, s := range sets {
		for p := range s.perms {
			m[p] = struct{}{}
		}
	}
	return PermissionSet{perms: m}
}

// Intersect returns a new PermissionSet containing only permissions present in both a and b.
func Intersect(a, b PermissionSet) PermissionSet {
	m := make(map[Permission]struct{})
	for p := range a.perms {
		if _, ok := b.perms[p]; ok {
			m[p] = struct{}{}
		}
	}
	return PermissionSet{perms: m}
}

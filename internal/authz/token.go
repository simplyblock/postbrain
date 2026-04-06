package authz

import "fmt"

// ParseTokenPermissions validates and parses a slice of raw permission strings
// into a PermissionSet suitable for storing on a token.
//
// Rules:
//   - Must not be empty.
//   - The legacy "admin" value is rejected; use "read", "write", "edit", "delete" instead.
//   - Each entry is validated via Expand — unknown resources, invalid operations,
//     and malformed strings are rejected.
func ParseTokenPermissions(raw []string) (PermissionSet, error) {
	if len(raw) == 0 {
		return PermissionSet{}, fmt.Errorf("authz: token permissions must not be empty")
	}
	for _, r := range raw {
		if r == "admin" {
			return PermissionSet{}, fmt.Errorf("authz: 'admin' is not a valid token permission; use 'read', 'write', 'edit', 'delete' or resource-scoped equivalents")
		}
	}
	return NewPermissionSet(raw)
}

// EffectiveTokenPermissions returns the permissions a token may actually exercise,
// computed as the intersection of the principal's effective permissions and the
// token's declared permissions. A token can never exceed the principal's permissions.
func EffectiveTokenPermissions(principalPerms, tokenPerms PermissionSet) PermissionSet {
	return Intersect(principalPerms, tokenPerms)
}

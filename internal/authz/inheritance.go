package authz

import "github.com/google/uuid"

// ScopeID is a type alias for uuid.UUID identifying a scope.
type ScopeID = uuid.UUID

// ApplyUpwardRead takes the existing grants map, a source scope, and its ordered
// ancestor scopes (nearest first), and returns a new map with read permissions
// from the source scope propagated up to every ancestor.
//
// Only :read operations are propagated upward. Write, edit, and delete permissions
// are never inherited by ancestor scopes.
//
// Downward inheritance (parent permissions applying to child scopes) is handled at
// query time via SQL ancestor lookups and is not modelled here.
func ApplyUpwardRead(
	grants map[ScopeID]PermissionSet,
	source ScopeID,
	ancestors []ScopeID,
) map[ScopeID]PermissionSet {
	result := make(map[ScopeID]PermissionSet, len(grants)+len(ancestors))
	for k, v := range grants {
		result[k] = v
	}

	if len(ancestors) == 0 {
		return result
	}

	sourcePerms, ok := grants[source]
	if !ok {
		return result
	}

	// Collect only the :read permissions from the source scope.
	var readPerms []Permission
	for _, p := range sourcePerms.Permissions() {
		_, op, err := p.Parse()
		if err != nil {
			continue
		}
		if op == OperationRead {
			readPerms = append(readPerms, p)
		}
	}
	if len(readPerms) == 0 {
		return result
	}

	readOnly := PermissionSet{perms: make(map[Permission]struct{}, len(readPerms))}
	for _, p := range readPerms {
		readOnly.perms[p] = struct{}{}
	}

	for _, ancestor := range ancestors {
		if existing, ok := result[ancestor]; ok {
			result[ancestor] = Union(existing, readOnly)
		} else {
			result[ancestor] = readOnly
		}
	}
	return result
}

// MergeGrants combines multiple scope→PermissionSet maps into a single map by
// taking the union of permissions for each scope that appears in more than one source.
func MergeGrants(sources ...map[ScopeID]PermissionSet) map[ScopeID]PermissionSet {
	result := make(map[ScopeID]PermissionSet)
	for _, src := range sources {
		for scopeID, ps := range src {
			if existing, ok := result[scopeID]; ok {
				result[scopeID] = Union(existing, ps)
			} else {
				result[scopeID] = ps
			}
		}
	}
	return result
}

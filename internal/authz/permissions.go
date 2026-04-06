package authz

import (
	"fmt"
	"strings"
)

// Resource identifies a content domain within postbrain.
type Resource string

const (
	ResourceMemories    Resource = "memories"
	ResourceKnowledge   Resource = "knowledge"
	ResourceCollections Resource = "collections"
	ResourceSkills      Resource = "skills"
	ResourceSessions    Resource = "sessions"
	ResourceGraph       Resource = "graph"
	ResourceScopes      Resource = "scopes"
	ResourcePrincipals  Resource = "principals"
	ResourceTokens      Resource = "tokens"
	ResourceSharing     Resource = "sharing"
	ResourcePromotions  Resource = "promotions"
)

// Operation is a discrete action that may be performed on a resource.
type Operation string

const (
	OperationRead   Operation = "read"
	OperationWrite  Operation = "write"
	OperationEdit   Operation = "edit"
	OperationDelete Operation = "delete"
)

// validOps maps each resource to the operations it supports.
var validOps = map[Resource][]Operation{
	ResourceMemories:    {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourceKnowledge:   {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourceCollections: {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourceSkills:      {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourceSessions:    {OperationRead, OperationWrite},
	ResourceGraph:       {OperationRead},
	ResourceScopes:      {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourcePrincipals:  {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourceTokens:      {OperationRead, OperationWrite, OperationEdit, OperationDelete},
	ResourceSharing:     {OperationRead, OperationWrite, OperationDelete},
	ResourcePromotions:  {OperationRead, OperationWrite, OperationEdit, OperationDelete},
}

// allResources is the canonical ordered list of all resources.
var allResources = []Resource{
	ResourceMemories,
	ResourceKnowledge,
	ResourceCollections,
	ResourceSkills,
	ResourceSessions,
	ResourceGraph,
	ResourceScopes,
	ResourcePrincipals,
	ResourceTokens,
	ResourceSharing,
	ResourcePromotions,
}

// shorthandOps maps bare shorthand strings to the operation they represent.
var shorthandOps = map[string]Operation{
	"read":   OperationRead,
	"write":  OperationWrite,
	"edit":   OperationEdit,
	"delete": OperationDelete,
}

// Permission is a "{resource}:{operation}" string granting a specific capability.
type Permission string

// NewPermission constructs a Permission from a resource and operation.
func NewPermission(r Resource, op Operation) Permission {
	return Permission(string(r) + ":" + string(op))
}

// Parse splits the permission into its resource and operation components.
func (p Permission) Parse() (Resource, Operation, error) {
	s := string(p)
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return "", "", fmt.Errorf("authz: malformed permission %q: missing ':'", p)
	}
	r := Resource(s[:idx])
	op := Operation(s[idx+1:])
	ops, ok := validOps[r]
	if !ok {
		return "", "", fmt.Errorf("authz: unknown resource %q in permission %q", r, p)
	}
	found := false
	for _, valid := range ops {
		if valid == op {
			found = true
			break
		}
	}
	if !found {
		return "", "", fmt.Errorf("authz: operation %q is not valid for resource %q", op, r)
	}
	return r, op, nil
}

// AllResources returns the canonical list of all resources.
func AllResources() []Resource {
	out := make([]Resource, len(allResources))
	copy(out, allResources)
	return out
}

// ValidOperations returns the operations supported by the given resource.
// Returns nil if the resource is unknown.
func ValidOperations(r Resource) []Operation {
	ops, ok := validOps[r]
	if !ok {
		return nil
	}
	out := make([]Operation, len(ops))
	copy(out, ops)
	return out
}

// AllPermissions returns every valid Permission across all resources and their supported operations.
func AllPermissions() []Permission {
	var out []Permission
	for _, r := range allResources {
		for _, op := range validOps[r] {
			out = append(out, NewPermission(r, op))
		}
	}
	return out
}

// Expand converts a raw permission string into the concrete Permission values it represents.
//
// Accepted forms:
//   - Bare shorthand: "read", "write", "edit", "delete" — expands to all valid
//     permissions for that operation across every resource.
//   - Resource-scoped: "{resource}:{operation}" — expands to the single
//     Permission it names, provided the operation is valid for that resource.
//
// Any other input returns an error.
func Expand(raw string) ([]Permission, error) {
	if raw == "" {
		return nil, fmt.Errorf("authz: empty permission string")
	}

	// Check for bare shorthand first.
	if op, ok := shorthandOps[raw]; ok {
		var out []Permission
		for _, r := range allResources {
			for _, validOp := range validOps[r] {
				if validOp == op {
					out = append(out, NewPermission(r, op))
					break
				}
			}
		}
		return out, nil
	}

	// Must be "resource:operation" form.
	idx := strings.IndexByte(raw, ':')
	if idx < 0 {
		return nil, fmt.Errorf("authz: unknown permission %q: not a shorthand and contains no ':'", raw)
	}
	r := Resource(raw[:idx])
	op := Operation(raw[idx+1:])

	ops, ok := validOps[r]
	if !ok {
		return nil, fmt.Errorf("authz: unknown resource %q in permission %q", r, raw)
	}
	for _, valid := range ops {
		if valid == op {
			return []Permission{NewPermission(r, op)}, nil
		}
	}
	return nil, fmt.Errorf("authz: operation %q is not valid for resource %q", op, r)
}

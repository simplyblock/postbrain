package authz

import "fmt"

// Role is a membership role that determines permissions on owned scopes.
type Role string

const (
	RoleMember Role = "member"
	RoleAdmin  Role = "admin"
	RoleOwner  Role = "owner"
)

// ParseRole parses a role string, returning an error for unknown values.
func ParseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleMember, RoleAdmin, RoleOwner:
		return Role(s), nil
	default:
		return "", fmt.Errorf("authz: unknown role %q", s)
	}
}

// rolePerms defines the canonical permission set for each membership role.
// Each role is a strict superset of the previous: owner ⊃ admin ⊃ member.
var rolePerms = map[Role][]Permission{
	RoleMember: {
		NewPermission(ResourceMemories, OperationRead),
		NewPermission(ResourceMemories, OperationWrite),
		NewPermission(ResourceKnowledge, OperationRead),
		NewPermission(ResourceKnowledge, OperationWrite),
		NewPermission(ResourceCollections, OperationRead),
		NewPermission(ResourceCollections, OperationWrite),
		NewPermission(ResourceSkills, OperationRead),
		NewPermission(ResourceSkills, OperationWrite),
		NewPermission(ResourceSessions, OperationWrite),
		NewPermission(ResourceGraph, OperationRead),
		NewPermission(ResourceScopes, OperationRead),
		NewPermission(ResourcePrincipals, OperationRead),
		NewPermission(ResourceTokens, OperationRead),
		NewPermission(ResourceSharing, OperationRead),
		NewPermission(ResourcePromotions, OperationRead),
		NewPermission(ResourcePromotions, OperationWrite),
	},
}

func init() {
	// admin = member + structural control
	adminExtra := []Permission{
		NewPermission(ResourceMemories, OperationEdit),
		NewPermission(ResourceMemories, OperationDelete),
		NewPermission(ResourceKnowledge, OperationEdit),
		NewPermission(ResourceCollections, OperationEdit),
		NewPermission(ResourceSkills, OperationEdit),
		NewPermission(ResourceScopes, OperationEdit),
		NewPermission(ResourceScopes, OperationWrite),
		NewPermission(ResourcePrincipals, OperationEdit),
		NewPermission(ResourceSharing, OperationWrite),
		NewPermission(ResourcePromotions, OperationEdit),
		NewPermission(ResourceTokens, OperationRead),
		NewPermission(ResourceTokens, OperationEdit),
	}
	rolePerms[RoleAdmin] = append(append([]Permission{}, rolePerms[RoleMember]...), adminExtra...)

	// owner = admin + deletion rights
	ownerExtra := []Permission{
		NewPermission(ResourceKnowledge, OperationDelete),
		NewPermission(ResourceCollections, OperationDelete),
		NewPermission(ResourceSkills, OperationDelete),
		NewPermission(ResourceScopes, OperationDelete),
		NewPermission(ResourcePrincipals, OperationDelete),
		NewPermission(ResourceSharing, OperationDelete),
		NewPermission(ResourcePromotions, OperationDelete),
		NewPermission(ResourceTokens, OperationDelete),
	}
	rolePerms[RoleOwner] = append(append([]Permission{}, rolePerms[RoleAdmin]...), ownerExtra...)
}

// RolePermissions returns the PermissionSet for the given membership role.
// Returns an empty set for unknown roles.
func RolePermissions(r Role) PermissionSet {
	perms, ok := rolePerms[r]
	if !ok {
		return EmptyPermissionSet()
	}
	m := make(map[Permission]struct{}, len(perms))
	for _, p := range perms {
		m[p] = struct{}{}
	}
	return PermissionSet{perms: m}
}

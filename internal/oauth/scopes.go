package oauth

import (
	"fmt"
	"strings"

	"github.com/simplyblock/postbrain/internal/authz"
)

// OAuth scope constants — one per valid {resource}:{operation} pair.
// These map directly to authz.Permission values.
const (
	ScopeMemoriesRead   = "memories:read"
	ScopeMemoriesWrite  = "memories:write"
	ScopeMemoriesEdit   = "memories:edit"
	ScopeMemoriesDelete = "memories:delete"

	ScopeKnowledgeRead   = "knowledge:read"
	ScopeKnowledgeWrite  = "knowledge:write"
	ScopeKnowledgeEdit   = "knowledge:edit"
	ScopeKnowledgeDelete = "knowledge:delete"

	ScopeCollectionsRead   = "collections:read"
	ScopeCollectionsWrite  = "collections:write"
	ScopeCollectionsEdit   = "collections:edit"
	ScopeCollectionsDelete = "collections:delete"

	ScopeSkillsRead   = "skills:read"
	ScopeSkillsWrite  = "skills:write"
	ScopeSkillsEdit   = "skills:edit"
	ScopeSkillsDelete = "skills:delete"

	ScopeSessionsRead  = "sessions:read"
	ScopeSessionsWrite = "sessions:write"

	ScopeGraphRead = "graph:read"

	ScopeScopesRead   = "scopes:read"
	ScopeScopesWrite  = "scopes:write"
	ScopeScopesEdit   = "scopes:edit"
	ScopeScopesDelete = "scopes:delete"

	ScopePrincipalsRead   = "principals:read"
	ScopePrincipalsWrite  = "principals:write"
	ScopePrincipalsEdit   = "principals:edit"
	ScopePrincipalsDelete = "principals:delete"

	ScopeTokensRead   = "tokens:read"
	ScopeTokensWrite  = "tokens:write"
	ScopeTokensEdit   = "tokens:edit"
	ScopeTokensDelete = "tokens:delete"

	ScopeSharingRead   = "sharing:read"
	ScopeSharingWrite  = "sharing:write"
	ScopeSharingDelete = "sharing:delete"

	ScopePromotionsRead   = "promotions:read"
	ScopePromotionsWrite  = "promotions:write"
	ScopePromotionsEdit   = "promotions:edit"
	ScopePromotionsDelete = "promotions:delete"
)

// knownScopes is derived from the authz package's canonical permission set.
// It contains exactly the set of valid {resource}:{operation} OAuth scopes.
var knownScopes map[string]struct{}

func init() {
	perms := authz.AllPermissions()
	knownScopes = make(map[string]struct{}, len(perms))
	for _, p := range perms {
		knownScopes[string(p)] = struct{}{}
	}
}

// ParseScopes parses a space-separated scope string and validates every scope.
func ParseScopes(raw string) ([]string, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("scope is required")
	}
	out := make([]string, 0, len(parts))
	for _, scope := range parts {
		if _, ok := knownScopes[scope]; !ok {
			return nil, fmt.Errorf("unknown scope %q", scope)
		}
		out = append(out, scope)
	}
	return out, nil
}

// ScopeToPermissions maps OAuth scopes to token permissions.
// Since OAuth scopes are already in {resource}:{operation} format,
// they map directly to authz.Permission values.
func ScopeToPermissions(scopes []string) []string {
	out := make([]string, len(scopes))
	copy(out, scopes)
	return out
}

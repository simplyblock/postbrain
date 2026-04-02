package oauth

import (
	"fmt"
	"strings"
)

const (
	ScopeMemoriesRead   = "memories:read"
	ScopeMemoriesWrite  = "memories:write"
	ScopeKnowledgeRead  = "knowledge:read"
	ScopeKnowledgeWrite = "knowledge:write"
	ScopeSkillsRead     = "skills:read"
	ScopeSkillsWrite    = "skills:write"
	ScopeAdmin          = "admin"
)

var knownScopes = map[string]struct{}{
	ScopeMemoriesRead:   {},
	ScopeMemoriesWrite:  {},
	ScopeKnowledgeRead:  {},
	ScopeKnowledgeWrite: {},
	ScopeSkillsRead:     {},
	ScopeSkillsWrite:    {},
	ScopeAdmin:          {},
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
func ScopeToPermissions(scopes []string) []string {
	out := make([]string, len(scopes))
	copy(out, scopes)
	return out
}

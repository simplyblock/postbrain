package oauth

import "strings"

// ServerMetadata builds RFC 8414 OAuth authorization server metadata.
func ServerMetadata(baseURL string) map[string]any {
	baseURL = strings.TrimSuffix(baseURL, "/")
	return map[string]any{
		"issuer":                           baseURL,
		"authorization_endpoint":           baseURL + "/oauth/authorize",
		"token_endpoint":                   baseURL + "/oauth/token",
		"registration_endpoint":            baseURL + "/oauth/register",
		"scopes_supported":                 []string{ScopeMemoriesRead, ScopeMemoriesWrite, ScopeKnowledgeRead, ScopeKnowledgeWrite, ScopeSkillsRead, ScopeSkillsWrite, ScopeAdmin},
		"response_types_supported":         []string{"code"},
		"code_challenge_methods_supported": []string{"S256"},
	}
}

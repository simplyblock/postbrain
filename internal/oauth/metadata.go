package oauth

import (
	"sort"
	"strings"

	"github.com/simplyblock/postbrain/internal/authz"
)

// ServerMetadata builds RFC 8414 OAuth authorization server metadata.
func ServerMetadata(baseURL string) map[string]any {
	baseURL = strings.TrimSuffix(baseURL, "/")

	perms := authz.AllPermissions()
	scopes := make([]string, len(perms))
	for i, p := range perms {
		scopes[i] = string(p)
	}
	sort.Strings(scopes)

	return map[string]any{
		"issuer":                           baseURL,
		"authorization_endpoint":           baseURL + "/oauth/authorize",
		"token_endpoint":                   baseURL + "/oauth/token",
		"registration_endpoint":            baseURL + "/oauth/register",
		"scopes_supported":                 scopes,
		"response_types_supported":         []string{"code"},
		"code_challenge_methods_supported": []string{"S256"},
	}
}

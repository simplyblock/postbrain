package rest

import "net/http"

// scopeAuthzContextMiddleware is retained for middleware chain compatibility.
// Scope access is now enforced per-request via AuthorizeContextScope using the
// authz.TokenResolver injected by the auth middleware.
func (ro *Router) scopeAuthzContextMiddleware(next http.Handler) http.Handler {
	return next
}

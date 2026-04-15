package oauth

import (
	"net/http"
	"strings"
)

// HandleRevoke implements RFC 7009 token revocation with self-token-only
// semantics: the caller must authenticate via "Authorization: Bearer <token>"
// and may only revoke that same token.  This prevents unauthenticated actors
// from revoking arbitrary tokens (DoS / forced-logout attack).
//
// Per RFC 7009 §2.2, the endpoint always returns 200 for tokens it will not
// revoke (wrong owner, already revoked, unknown) to avoid leaking token
// existence information.  Only missing/invalid bearer authentication returns
// 401 so callers know they must re-authenticate.
func (s *Server) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusOK, "")
		return
	}

	// ── Step 1: require bearer authentication ───────────────────────────────
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	bearerRaw := strings.TrimPrefix(authHeader, "Bearer ")
	bearerHash := hashSHA256Hex(bearerRaw)

	bearerToken, err := s.tokens.Lookup(r.Context(), bearerHash)
	if err != nil || bearerToken == nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token")
		return
	}

	// ── Step 2: self-revocation only ────────────────────────────────────────
	// The body token must be the same as the authenticated bearer token.
	// Requests targeting a different token return 200 with no action (RFC 7009).
	rawToken := r.Form.Get("token")
	if rawToken != "" && hashSHA256Hex(rawToken) == bearerHash {
		_ = s.tokens.Revoke(r.Context(), bearerToken.ID)
	}

	w.WriteHeader(http.StatusOK)
}

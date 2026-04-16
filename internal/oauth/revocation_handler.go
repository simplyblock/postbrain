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
	// ── Step 1: require bearer authentication ───────────────────────────────
	// Check auth before parsing the body to reject unauthenticated requests
	// cheaply and avoid unnecessary work (including large-body DoS).
	// RFC 9110 §11.1 treats scheme names as case-insensitive; trim whitespace
	// around the token value per RFC 9110 §5.6.2.
	authHeader := r.Header.Get("Authorization")
	scheme, rest, hasSep := strings.Cut(authHeader, " ")
	if !hasSep || !strings.EqualFold(scheme, "Bearer") {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	bearerRaw := strings.TrimSpace(rest)
	if bearerRaw == "" {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	bearerHash := hashSHA256Hex(bearerRaw)

	bearerToken, err := s.tokens.Lookup(r.Context(), bearerHash)
	if err != nil || bearerToken == nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token")
		return
	}

	// Parse the form body only after authentication succeeds.
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusOK, "")
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

package oauth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// HandleRevoke revokes token when it exists. Unknown tokens still return 200.
func (s *Server) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusOK, "")
		return
	}
	rawToken := r.Form.Get("token")
	if rawToken != "" {
		sum := sha256.Sum256([]byte(rawToken))
		hash := hex.EncodeToString(sum[:])
		token, err := s.tokens.Lookup(r.Context(), hash)
		if err == nil && token != nil {
			_ = s.tokens.Revoke(r.Context(), token.ID)
		}
	}
	w.WriteHeader(http.StatusOK)
}

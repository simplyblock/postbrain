package oauth

import "net/http"

// HandleMetadata serves RFC 8414 metadata.
func (s *Server) HandleMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ServerMetadata(s.cfg.BaseURL))
}

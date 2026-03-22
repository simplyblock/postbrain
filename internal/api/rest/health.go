package rest

import (
	"net/http"
)

// handleHealth returns the server health status.
func (ro *Router) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"schema_version":   0, // TODO: read from db
		"expected_version": 0,
		"schema_dirty":     false,
	})
}

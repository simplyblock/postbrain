package rest

import (
	"net/http"

	"github.com/simplyblock/postbrain/internal/db"
)

// handleHealth returns the server health status, including the current schema
// version from the database when a pool is available.
func (ro *Router) handleHealth(w http.ResponseWriter, r *http.Request) {
	if ro.pool == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":           "ok",
			"schema_version":   0,
			"expected_version": db.ExpectedVersion,
			"schema_dirty":     false,
		})
		return
	}

	version, dirty, err := db.SchemaVersion(r.Context(), ro.pool)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read schema version")
		return
	}

	code := http.StatusOK
	status := "ok"
	if dirty || (db.ExpectedVersion > 0 && int(version) != db.ExpectedVersion) {
		code = http.StatusServiceUnavailable
		status = "degraded"
	}

	writeJSON(w, code, map[string]any{
		"status":           status,
		"schema_version":   version,
		"expected_version": db.ExpectedVersion,
		"schema_dirty":     dirty,
	})
}

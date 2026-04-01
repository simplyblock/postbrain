package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSearchArtifacts_MissingQ_Not400 verifies that GET /v1/knowledge/search
// does NOT require the q parameter — empty / absent query is valid for
// browse/recall. The handler is called directly with a nil store, so it will
// panic when it reaches the DB call; that panic is recovered silently.
// The only assertion is that the handler did NOT return 400 (i.e., it did not
// add a required-parameter guard for q).
func TestSearchArtifacts_MissingQ_Not400(t *testing.T) {
	ro := &Router{} // nil knwStore — DB call will panic, caught below

	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/search", nil)
	w := httptest.NewRecorder()

	// Capture the status written before any panic.  A panic at the DB layer
	// means the handler reached the Recall call (i.e., no early 400 return).
	var panicked bool
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		ro.searchArtifacts(w, req)
	}()

	// Either a panic (reached DB without 400) or a non-400 status is acceptable.
	if !panicked && w.Code == http.StatusBadRequest {
		t.Errorf("GET /v1/knowledge/search without q must not return 400; "+
			"missing q is not a validation error (body: %s)", w.Body.String())
	}
}

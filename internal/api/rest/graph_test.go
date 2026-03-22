package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListEntities_NilPool_Returns500 verifies that listEntities with a nil pool returns 500.
// The request goes through the full auth middleware (nil pool → 401), so we test the handler
// directly via a helper that bypasses auth.
func TestListEntities_NilPool_Returns500(t *testing.T) {
	ro := &Router{} // nil pool
	req := httptest.NewRequest(http.MethodGet, "/v1/entities?scope_id=00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	ro.listEntities(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in 500 response")
	}
}

// TestQueryCypher_Returns501 verifies that queryCypher always returns 501.
func TestQueryCypher_Returns501(t *testing.T) {
	ro := &Router{}
	body := `{"cypher":"MATCH (n) RETURN n","scope_id":"00000000-0000-0000-0000-000000000001"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/graph/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ro.queryCypher(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if resp["error"] != "AGE unavailable" {
		t.Errorf("expected error=AGE unavailable, got %v", resp["error"])
	}
}

// TestGetGraph_NilPool_Returns500 verifies that getGraph with a nil pool returns 500.
func TestGetGraph_NilPool_Returns500(t *testing.T) {
	ro := &Router{}
	req := httptest.NewRequest(http.MethodGet, "/v1/graph?scope_id=00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	ro.getGraph(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// TestListEntities_MissingScopeID_Returns400 verifies that missing scope_id returns 400.
func TestListEntities_MissingScopeID_Returns400(t *testing.T) {
	ro := &Router{}
	req := httptest.NewRequest(http.MethodGet, "/v1/entities", nil)
	w := httptest.NewRecorder()
	ro.listEntities(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

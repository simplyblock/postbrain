package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// newTestHandler creates a Handler with nil pool (no DB) for unit testing.
// Templates are still parsed and rendered; DB-dependent data is simply absent.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := NewHandler(nil, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// TestHandleKnowledge_NoScope_Renders200 verifies that GET /ui/knowledge without
// a scope parameter renders successfully (zero UUID = all scopes).
func TestHandleKnowledge_NoScope_Renders200(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge", nil)
	w := httptest.NewRecorder()

	h.handleKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected HTML content-type, got %q", ct)
	}
}

// TestHandleKnowledge_WithScope_Renders200 verifies that GET /ui/knowledge?scope=<uuid>
// renders successfully and does not panic with a nil pool.
func TestHandleKnowledge_WithScope_Renders200(t *testing.T) {
	h := newTestHandler(t)
	scopeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge?scope="+scopeID.String(), nil)
	w := httptest.NewRecorder()

	h.handleKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestHandleKnowledge_QueryAndStatus_Renders200 verifies that GET /ui/knowledge
// with q and status params renders successfully without touching the DB.
func TestHandleKnowledge_QueryAndStatus_Renders200(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/knowledge?q=foo&status=published", nil)
	w := httptest.NewRecorder()

	h.handleKnowledge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

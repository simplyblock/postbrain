package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestHandleSetScopeRepo_MissingRepoURL_RendersError verifies that
// POST /ui/scopes/{id}/repo without repo_url renders the scopes page with
// a form error rather than returning a 500 or panicking.
func TestHandleSetScopeRepo_MissingRepoURL_RendersError(t *testing.T) {
	h := newTestHandler(t)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/ui/scopes/"+id.String()+"/repo",
		strings.NewReader("")) // no repo_url field
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleSetScopeRepo(w, req)

	// Must render the scopes page (200 with HTML), not a 5xx error.
	if w.Code >= http.StatusInternalServerError {
		t.Errorf("expected non-5xx response, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected HTML content-type, got %q", ct)
	}
	// Error message must be present in the rendered body.
	if !strings.Contains(w.Body.String(), "repo_url is required") {
		t.Errorf("expected 'repo_url is required' in response body")
	}
}

// TestHandleSyncScopeRepo_NilPool_RendersGracefully verifies that
// POST /ui/scopes/{id}/repo/sync with a nil pool renders an error page
// rather than panicking or redirecting.
func TestHandleSyncScopeRepo_NilPool_RendersGracefully(t *testing.T) {
	h := newTestHandler(t) // nil pool
	id := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/ui/scopes/"+id.String()+"/repo/sync", nil)
	w := httptest.NewRecorder()

	// Must not panic.
	h.handleSyncScopeRepo(w, req)

	// With nil pool the handler calls renderScopes with "service unavailable".
	// That renders the scopes template — expect a non-5xx HTML response.
	if w.Code >= http.StatusInternalServerError {
		t.Errorf("expected non-5xx response, got %d", w.Code)
	}
}

// TestHandleSyncStatus_UnknownScope_ReturnsJSON verifies that
// GET /ui/scopes/{id}/repo/sync/status for a scope that has never been
// indexed returns a valid JSON response (not a 500 or panic).
func TestHandleSyncStatus_UnknownScope_ReturnsJSON(t *testing.T) {
	h := newTestHandler(t)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/ui/scopes/"+id.String()+"/repo/sync/status", nil)
	w := httptest.NewRecorder()

	h.handleSyncStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty JSON body")
	}
}

// TestHandleSetScopeOwner_MissingPrincipalID_RendersError verifies that
// POST /ui/scopes/{id}/owner without principal_id renders the scopes page
// with a form error instead of panicking.
func TestHandleSetScopeOwner_MissingPrincipalID_RendersError(t *testing.T) {
	h := newTestHandler(t)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/ui/scopes/"+id.String()+"/owner",
		strings.NewReader("")) // no principal_id field
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleSetScopeOwner(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Errorf("expected non-5xx response, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected HTML content-type, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "principal_id is required") {
		t.Errorf("expected 'principal_id is required' in response body")
	}
}

// TestHandleScopes_RendersImprovedTableLayout verifies that
// GET /ui/scopes includes the responsive table wrapper and grouped
// action controls used by the updated scopes design.
func TestHandleScopes_RendersImprovedTableLayout(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/scopes", nil)
	w := httptest.NewRecorder()

	h.handleScopes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "scopes-table-wrap") {
		t.Errorf("expected scopes-table-wrap class in response body")
	}
	if !strings.Contains(body, "scope-actions") {
		t.Errorf("expected scope-actions class in response body")
	}
	if !strings.Contains(body, "scope-main") {
		t.Errorf("expected scope-main class in response body")
	}
	if !strings.Contains(body, "scope-meta") {
		t.Errorf("expected scope-meta class in response body")
	}
}

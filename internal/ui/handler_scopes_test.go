package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/db"
)

// TestScopesHierarchy_XSSPayload_NotInjectedViaInnerHTML is a regression test
// for stored XSS in the scope hierarchy tree.  Before the fix, scope name and
// externalId values were concatenated into an HTML string and assigned to
// innerHTML, allowing stored script injection.
//
// After the fix the renderNode function must use the DOM API
// (createElement / textContent) so user-supplied values are never treated as
// HTML markup.  We verify this by:
//  1. Rendering the scopes template with XSS payloads in name and externalId.
//  2. Asserting the rendered output does not contain an innerHTML assignment
//     that could carry user-controlled HTML (the old s.externalId / s.name
//     string-concatenation pattern).
//  3. Asserting the rendered output contains textContent usage (the safe pattern).
func TestScopesHierarchy_XSSPayload_NotInjectedViaInnerHTML(t *testing.T) {
	h := newTestHandler(t)

	xssName := `<img src=x onerror=alert('xss-name')>`
	xssExtID := `<script>alert('xss-extid')</script>`

	scopeID := uuid.New()
	data := struct {
		Principals     []*db.Principal
		Scopes         []*db.Scope
		ScopeFormError string
		SyncStatus     map[string]codegraph.SyncStatus
		ChildCount     map[string]int64
		CanManage      map[string]bool
		CanDelete      map[string]bool
		OwnerNames     map[string]string
	}{
		Scopes: []*db.Scope{
			{
				ID:         scopeID,
				Kind:       "project",
				ExternalID: xssExtID,
				Name:       xssName,
			},
		},
		SyncStatus: make(map[string]codegraph.SyncStatus),
		ChildCount: make(map[string]int64),
		CanManage:  map[string]bool{scopeID.String(): false},
		CanDelete:  map[string]bool{scopeID.String(): false},
		OwnerNames: make(map[string]string),
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/scopes", nil)
	w := httptest.NewRecorder()
	h.render(w, req, "scopes", "Scopes", data)

	if w.Code != http.StatusOK {
		t.Fatalf("render status = %d, want 200", w.Code)
	}

	body := w.Body.String()

	// The rendered JS must NOT concatenate user values into innerHTML.
	// The old vulnerable pattern was: ... + s.externalId + ... assigned to innerHTML.
	// After the fix, renderNode builds DOM nodes and uses textContent.
	if strings.Contains(body, `s.externalId + "</strong>"`) ||
		strings.Contains(body, `"<strong>" + s.externalId`) ||
		strings.Contains(body, `s.name + ")</span>"`) {
		t.Errorf("rendered page contains vulnerable innerHTML string-concatenation pattern with user fields")
	}

	// The hierarchy script must use textContent for user-controlled values.
	if !strings.Contains(body, "textContent") {
		t.Errorf("rendered page must use textContent for safe DOM insertion; innerHTML string-concat must be gone")
	}
}

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

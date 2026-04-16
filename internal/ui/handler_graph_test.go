package ui

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGraphTooltip_XSSPayload_NotInjectedViaInnerHTML is a regression test for
// stored XSS in the graph tooltip.  Before the fix, entity name and type were
// interpolated into a template-literal HTML string assigned to innerHTML,
// allowing any user who could POST /v1/memories with crafted entity names to
// execute script in browsers of users viewing /ui/graph.
//
// After the fix the tooltip must use the DOM API (textContent) so user-supplied
// values are never parsed as markup.
func TestGraphTooltip_XSSPayload_NotInjectedViaInnerHTML(t *testing.T) {
	h := newTestHandler(t)

	xssName := `<img src=x onerror=alert('xss-name')>`
	xssType := `<script>alert('xss-type')</script>`

	// Build graph JSON with XSS payloads in node name and type.
	graphJSON, err := json.Marshal(map[string]any{
		"nodes": []map[string]any{
			{"id": "1", "name": xssName, "type": xssType},
		},
		"links": []map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal graph JSON: %v", err)
	}

	data := struct {
		Scopes    any
		ScopeID   string
		NodeCount int
		EdgeCount int
		GraphJSON template.JS
	}{
		ScopeID:   "00000000-0000-0000-0000-000000000000",
		NodeCount: 1,
		EdgeCount: 0,
		GraphJSON: template.JS(graphJSON), //nolint:gosec // test-only: controlled payload
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/graph", nil)
	w := httptest.NewRecorder()
	h.render(w, req, "graph", "Entity Graph", data)

	if w.Code != http.StatusOK {
		t.Fatalf("render status = %d, want 200", w.Code)
	}

	body := w.Body.String()

	// The vulnerable pattern: template-literal HTML with n.name/n.type
	// assigned directly to innerHTML.
	if strings.Contains(body, "tooltip.innerHTML") {
		t.Errorf("rendered page still uses tooltip.innerHTML — must use DOM API (textContent) instead")
	}

	// Safe pattern must be present: textContent used for user values.
	if !strings.Contains(body, "textContent") {
		t.Errorf("rendered page must use textContent for tooltip content; innerHTML must be removed")
	}
}

func TestHandleGraph3D_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/graph3d", nil)
	w := httptest.NewRecorder()

	h.handleGraph3D(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Entity Graph 3D") {
		t.Fatalf("expected 3d graph title in response body")
	}
	if !strings.Contains(body, "3d-force-graph") {
		t.Fatalf("expected 3d-force-graph script in response body")
	}
}

func TestHandleGraph3D_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/graph3d", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/ui/login" {
		t.Fatalf("Location = %q, want %q", got, "/ui/login")
	}
}

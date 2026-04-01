package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleCollections ─────────────────────────────────────────────────────────

func TestHandleCollections_NilPool_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/collections", nil)
	w := httptest.NewRecorder()

	h.handleCollections(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandleCollections_WithScopeID_NilPool_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet,
		"/ui/collections?scope_id=00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.handleCollections(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleCollections_InvalidScopeID_NilPool_Renders200(t *testing.T) {
	t.Parallel()
	// Invalid UUID in scope_id is silently ignored when pool is nil.
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/collections?scope_id=not-a-uuid", nil)
	w := httptest.NewRecorder()

	h.handleCollections(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleCollections_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/collections", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want %q", loc, "/ui/login")
	}
}

// ── handleCollectionDetail ────────────────────────────────────────────────────

func TestHandleCollectionDetail_InvalidUUID_Returns404(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/collections/not-a-uuid", nil)
	w := httptest.NewRecorder()

	h.handleCollectionDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCollectionDetail_NilPool_Returns404(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet,
		"/ui/collections/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.handleCollectionDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (nil pool: bug was 500)", w.Code, http.StatusNotFound)
	}
}

func TestHandleCollectionDetail_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet,
		"/ui/collections/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want %q", loc, "/ui/login")
	}
}

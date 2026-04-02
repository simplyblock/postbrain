package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleMemories ────────────────────────────────────────────────────────────

func TestHandleMemories_NilPool_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories", nil)
	w := httptest.NewRecorder()

	h.handleMemories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandleMemories_WithQuery_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories?q=deployment", nil)
	w := httptest.NewRecorder()

	h.handleMemories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleMemories_HTMXRequest_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	h.handleMemories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleMemories_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want %q", loc, "/ui/login")
	}
}

// ── handleMemoryDetail ────────────────────────────────────────────────────────

func TestHandleMemoryDetail_InvalidUUID_Returns404(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories/not-a-uuid", nil)
	w := httptest.NewRecorder()

	h.handleMemoryDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleMemoryDetail_NilPool_Returns404(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.handleMemoryDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleMemoryDetail_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/memories/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want %q", loc, "/ui/login")
	}
}

// ── handleMemoryForget ────────────────────────────────────────────────────────

func TestHandleMemoryForget_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/memories/not-a-uuid/forget", nil)
	w := httptest.NewRecorder()

	h.handleMemoryForget(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleMemoryForget_NilPool_Returns503(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/memories/00000000-0000-0000-0000-000000000001/forget", nil)
	w := httptest.NewRecorder()

	h.handleMemoryForget(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleMemoryForget_UnauthenticatedViaRouter_RedirectsToLogin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost,
		"/ui/memories/00000000-0000-0000-0000-000000000001/forget", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

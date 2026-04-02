package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

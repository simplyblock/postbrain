package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewHandler_NilPool_Succeeds verifies that NewHandler with a nil pool parses templates without error.
func TestNewHandler_NilPool_Succeeds(t *testing.T) {
	h, err := NewHandler(nil, nil)
	if err != nil {
		t.Fatalf("NewHandler(nil, nil) returned error: %v", err)
	}
	if h == nil {
		t.Fatal("NewHandler(nil, nil) returned nil handler")
	}
}

// TestLoginGET_Returns200 verifies that GET /ui/login returns 200 with the login form.
func TestLoginGET_Returns200(t *testing.T) {
	h, err := NewHandler(nil, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Sign in to Postbrain") {
		t.Error("expected login form in response")
	}
}

// TestUIRoot_NoCookie_RedirectsToLogin verifies that GET /ui without a session cookie redirects to /ui/login.
func TestUIRoot_NoCookie_RedirectsToLogin(t *testing.T) {
	h, err := NewHandler(nil, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("expected redirect to /ui/login, got %q", loc)
	}
}

// TestUIMetrics_NoCookie_RedirectsToLogin verifies that GET /ui/metrics without a session cookie redirects.
func TestUIMetrics_NoCookie_RedirectsToLogin(t *testing.T) {
	h, err := NewHandler(nil, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/login" {
		t.Errorf("expected redirect to /ui/login, got %q", loc)
	}
}

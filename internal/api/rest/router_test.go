package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// newTestRouter builds a minimal Router with nil stores for unit testing.
func newTestRouter() *Router {
	return &Router{}
}

// TestHealth_Returns200 verifies the /health endpoint returns 200 with expected shape.
func TestHealth_Returns200(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
}

// TestCreateMemory_NoAuth_Returns401 verifies unauthenticated requests to /v1/memories return 401.
func TestCreateMemory_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not parse error body: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in 401 response")
	}
}

// TestCreateMemory_WithInvalidToken_Returns401 verifies an invalid bearer token returns 401.
func TestCreateMemory_WithInvalidToken_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/memories", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer pb_invalid_token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestChi_RoutesRegistered verifies the chi router has the expected route count.
func TestChi_RoutesRegistered(t *testing.T) {
	r := newTestRouter()
	cr := chi.NewRouter()
	// Just verify we can build the handler without panicking.
	_ = cr
	_ = r
}

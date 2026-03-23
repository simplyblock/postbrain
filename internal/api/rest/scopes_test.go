package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListScopes_NoAuth_Returns401 verifies unauthenticated requests to GET /v1/scopes return 401.
func TestListScopes_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/scopes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not parse body: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in 401 response")
	}
}

// TestCreateScope_NoAuth_Returns401 verifies unauthenticated POST /v1/scopes returns 401.
func TestCreateScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/scopes", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestGetScope_NoAuth_Returns401 verifies unauthenticated GET /v1/scopes/{id} returns 401.
func TestGetScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v1/scopes/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestUpdateScope_NoAuth_Returns401 verifies unauthenticated PUT /v1/scopes/{id} returns 401.
func TestUpdateScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v1/scopes/00000000-0000-0000-0000-000000000001", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestDeleteScope_NoAuth_Returns401 verifies unauthenticated DELETE /v1/scopes/{id} returns 401.
func TestDeleteScope_NoAuth_Returns401(t *testing.T) {
	r := newTestRouter()
	handler := r.Handler()

	req := httptest.NewRequest(http.MethodDelete, "/v1/scopes/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

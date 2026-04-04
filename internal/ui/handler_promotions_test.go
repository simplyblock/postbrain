package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePromotions_NilPool_Renders200(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/promotions", nil)
	w := httptest.NewRecorder()

	h.handlePromotions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandlePromotions_InvalidScopeID_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/promotions?scope_id=not-a-uuid", nil)
	w := httptest.NewRecorder()

	h.handlePromotions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid scope id") {
		t.Errorf("response body = %q, want invalid scope id", w.Body.String())
	}
}

func TestHandlePromotions_InvalidStatus_Returns400(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/promotions?status=not-a-status", nil)
	w := httptest.NewRecorder()

	h.handlePromotions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid status") {
		t.Errorf("response body = %q, want invalid status", w.Body.String())
	}
}

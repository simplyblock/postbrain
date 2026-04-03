package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHandleUpdatePrincipal_InvalidID_RendersError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/principals/not-a-uuid",
		strings.NewReader("slug=alice&display_name=Alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("expected non-5xx status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid principal id") {
		t.Fatalf("expected invalid principal id error in response body")
	}
}

func TestHandleUpdatePrincipal_MissingFields_RendersError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/ui/principals/"+id.String(),
		strings.NewReader("slug=alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("expected non-5xx status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "slug and display_name are required") {
		t.Fatalf("expected missing fields error in response body")
	}
}

func TestHandleUpdatePrincipal_NilPool_RendersError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/ui/principals/"+id.String(),
		strings.NewReader("slug=alice&display_name=Alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("expected non-5xx status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "service unavailable") {
		t.Fatalf("expected service unavailable in response body")
	}
}

func TestHandlePrincipals_RendersEditDialog(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/principals", nil)
	w := httptest.NewRecorder()

	h.handlePrincipals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "id=\"dlg-principal-edit\"") {
		t.Fatalf("expected edit dialog in principals page")
	}
}
